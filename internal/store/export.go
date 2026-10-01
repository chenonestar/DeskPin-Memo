package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"deskpinmemo/internal/hlc"
)

// ExportFormatVersion 是导出 JSON 的格式版本。
const ExportFormatVersion = 1

// syncTables 是导出/导入涉及的表（windows 为设备相关，不导出）。
var exportTables = []string{"groups", "items", "reminders", "subtasks", "tags", "item_tags"}

// Dump 是导出 JSON 的顶层结构：format_version、exported_at 与全部表数据。
type Dump struct {
	FormatVersion int                         `json:"format_version"`
	ExportedAt    string                      `json:"exported_at"`
	DeviceID      string                      `json:"device_id"`
	Tables        map[string][]map[string]any `json:"tables"`
	Settings      map[string]json.RawMessage  `json:"settings,omitempty"`
}

func (s *Store) dumpTable(name string) ([]map[string]any, error) {
	rows, err := s.db.Query(`SELECT * FROM ` + name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols, _ := rows.Columns()
	var out []map[string]any
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		m := make(map[string]any, len(cols))
		for i, c := range cols {
			if b, ok := vals[i].([]byte); ok {
				vals[i] = string(b)
			}
			m[c] = vals[i]
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// decryptedFields 是各表需要加解密的列。
var sensitive = map[string][]string{
	"items":  {"title", "note"},
	"groups": {"name"},
	"tags":   {"name"},
	// 子任务标题同样是用户内容：加密开启时随其他字段一起加密
	"subtasks": {"title"},
}

// Export 生成完整数据快照（敏感字段为明文；加密导出由上层用导出密码封装）。
func (s *Store) Export() (*Dump, error) {
	if s.Locked() {
		return nil, errors.New("数据已加密且尚未解锁")
	}
	d := &Dump{FormatVersion: ExportFormatVersion, ExportedAt: s.now().UTC().Format(time.RFC3339),
		DeviceID: s.deviceID, Tables: map[string][]map[string]any{}}
	for _, t := range exportTables {
		rows, err := s.dumpTable(t)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			for _, col := range sensitive[t] {
				if v, ok := r[col].(string); ok {
					r[col], _ = s.dec(v)
				}
			}
		}
		if rows == nil {
			rows = []map[string]any{}
		}
		d.Tables[t] = rows
	}
	set, err := s.AllSettings()
	if err != nil {
		return nil, err
	}
	d.Settings = set
	return d, nil
}

// ImportMode 为导入方式。
type ImportMode string

const (
	ImportMerge     ImportMode = "merge"
	ImportOverwrite ImportMode = "overwrite"
)

// ImportStats 是导入结果统计。
type ImportStats struct {
	Items     int `json:"items"`
	Groups    int `json:"groups"`
	Reminders int `json:"reminders"`
	Tags      int `json:"tags"`
	Subtasks  int `json:"subtasks"`
	Skipped   int `json:"skipped"`
}

func tableColumns(tx *sql.Tx, table string) (map[string]bool, error) {
	rows, err := tx.Query(`SELECT name FROM pragma_table_info(?)`, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := map[string]bool{}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		m[n] = true
	}
	return m, rows.Err()
}

// Import 从导出数据恢复；merge 按 HLC 取新者，overwrite 先清空再写入（调用方须二次确认）。
func (s *Store) Import(d *Dump, mode ImportMode) (ImportStats, error) {
	var st ImportStats
	if d == nil || d.FormatVersion != ExportFormatVersion {
		return st, fmt.Errorf("不支持的导出格式版本")
	}
	if s.Locked() {
		return st, errors.New("数据已加密且尚未解锁")
	}
	err := s.Tx(func(tx *sql.Tx) error {
		if mode == ImportOverwrite {
			for _, q := range []string{`DELETE FROM item_tags`, `DELETE FROM subtasks`, `DELETE FROM reminders`, `DELETE FROM items`,
				`DELETE FROM tags`, `DELETE FROM windows`, `DELETE FROM groups`, `DELETE FROM items_fts`} {
				if _, err := tx.Exec(q); err != nil {
					return err
				}
			}
		}
		var localInbox string
		_ = tx.QueryRow(`SELECT id FROM groups WHERE is_default=1 AND deleted=0`).Scan(&localInbox)
		remap := map[string]string{} // 导入的收件箱 → 本地收件箱（合并时）

		for _, t := range exportTables {
			cols, err := tableColumns(tx, t)
			if err != nil {
				return err
			}
			rows := d.Tables[t]
			if t == "groups" { // 默认分组排前面，便于 remap
				sort.SliceStable(rows, func(i, j int) bool { return num(rows[i]["is_default"]) > num(rows[j]["is_default"]) })
			}
			for _, r := range rows {
				if t == "groups" && localInbox != "" && num(r["is_default"]) == 1 && str(r["id"]) != localInbox {
					remap[str(r["id"])] = localInbox
					st.Skipped++
					continue
				}
				if t == "items" {
					if m, ok := remap[str(r["group_id"])]; ok {
						r["group_id"] = m
					}
				}
				if t == "subtasks" { // 父事项不存在（例如导出文件不完整）则跳过，避免外键错误
					var n int
					_ = tx.QueryRow(`SELECT COUNT(*) FROM items WHERE id=?`, str(r["item_id"])).Scan(&n)
					if n == 0 {
						st.Skipped++
						continue
					}
				}
				if mode == ImportMerge {
					newer, err := incomingNewer(tx, t, r)
					if err != nil {
						return err
					}
					if !newer {
						st.Skipped++
						continue
					}
				}
				for _, col := range sensitive[t] {
					if v, ok := r[col].(string); ok {
						e, err := s.enc(v)
						if err != nil {
							return err
						}
						r[col] = e
					}
				}
				if err := insertRow(tx, t, cols, r); err != nil {
					return fmt.Errorf("导入 %s 失败: %w", t, err)
				}
				switch t {
				case "items":
					st.Items++
				case "groups":
					st.Groups++
				case "reminders":
					st.Reminders++
				case "tags":
					st.Tags++
				case "subtasks":
					st.Subtasks++
				}
				if h := str(r["hlc"]); h != "" {
					s.clock.Observe(h)
				}
			}
		}
		if mode == ImportOverwrite {
			for k, v := range d.Settings {
				if _, err := tx.Exec(`INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, k, string(v)); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return st, err
	}
	if err := s.ensureInbox(); err != nil {
		return st, err
	}
	return st, s.RebuildFTS()
}

func incomingNewer(tx *sql.Tx, table string, r map[string]any) (bool, error) {
	var q string
	var args []any
	if table == "item_tags" {
		q, args = `SELECT hlc FROM item_tags WHERE item_id=? AND tag_id=?`, []any{str(r["item_id"]), str(r["tag_id"])}
	} else {
		q, args = `SELECT hlc FROM `+table+` WHERE id=?`, []any{str(r["id"])}
	}
	var cur string
	err := tx.QueryRow(q, args...).Scan(&cur)
	if errors.Is(err, sql.ErrNoRows) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return hlc.Compare(str(r["hlc"]), cur) > 0, nil
}

func insertRow(tx *sql.Tx, table string, valid map[string]bool, r map[string]any) error {
	var cols []string
	for k := range r {
		if valid[k] { // 只接受表内已知列，忽略其余键
			cols = append(cols, k)
		}
	}
	sort.Strings(cols)
	sorted := make([]any, len(cols))
	for i, c := range cols {
		sorted[i] = r[c]
	}
	ph := strings.TrimSuffix(strings.Repeat("?,", len(cols)), ",")
	_, err := tx.Exec(`INSERT OR REPLACE INTO `+table+`(`+strings.Join(cols, ",")+`) VALUES(`+ph+`)`, sorted...)
	return err
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

func num(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int64:
		return float64(n)
	case int:
		return float64(n)
	case json.Number:
		f, _ := n.Float64()
		return f
	}
	return 0
}

// Markdown 导出可读清单（FR-603）。
func (s *Store) Markdown(loc *time.Location) (string, error) {
	if s.Locked() {
		return "", errors.New("数据已加密且尚未解锁")
	}
	groups, err := s.ListGroups()
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("# 桌面备忘钉 事项清单\n\n导出时间：" + s.now().In(loc).Format("2006-01-02 15:04") + "\n")
	for _, g := range groups {
		items, err := s.ListItems(Filter{GroupID: g.ID})
		if err != nil {
			return "", err
		}
		SortItems(items, s.now(), loc)
		b.WriteString("\n## " + g.Name + "\n\n")
		if len(items) == 0 {
			b.WriteString("_（空）_\n")
		}
		for _, it := range items {
			box := "[ ]"
			if it.Status == StatusDone {
				box = "[x]"
			}
			line := "- " + box + " " + it.Title
			if it.DueAt != nil {
				line += "（截止 " + time.UnixMilli(*it.DueAt).In(loc).Format("2006-01-02 15:04") + "）"
			}
			for _, t := range it.Tags {
				line += " #" + t
			}
			b.WriteString(line + "\n")
			for _, sub := range it.Subtasks {
				sbox := "[ ]"
				if sub.Done {
					sbox = "[x]"
				}
				b.WriteString("  - " + sbox + " " + sub.Title + "\n")
			}
			if strings.TrimSpace(it.Note) != "" {
				for _, l := range strings.Split(strings.TrimSpace(it.Note), "\n") {
					b.WriteString("  > " + l + "\n")
				}
			}
		}
	}
	return b.String(), nil
}
