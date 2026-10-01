package store

import (
	"database/sql"
	"errors"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"deskpinmemo/internal/hlc"
)

const MaxTitleRunes = 200

var itemFields = []string{"title", "note", "group_id", "priority", "due_at", "status",
	"completed_at", "deleted_at", "sort_order", "series_id"}

const itemCols = `id,title,note,group_id,priority,due_at,status,completed_at,deleted_at,sort_order,series_id,created_at,updated_at,hlc,device_id,field_hlc`

func scanItem(sc interface{ Scan(...any) error }) (Item, error) {
	var it Item
	var due, comp, del sql.NullInt64
	if err := sc.Scan(&it.ID, &it.Title, &it.Note, &it.GroupID, &it.Priority, &due, &it.Status, &comp, &del,
		&it.SortOrder, &it.SeriesID, &it.CreatedAt, &it.UpdatedAt, &it.HLC, &it.DeviceID, &it.FieldHLC); err != nil {
		return it, err
	}
	it.DueAt, it.CompletedAt, it.DeletedAt = ni(due), ni(comp), ni(del)
	it.Tags = []string{}
	it.Reminders = []Reminder{}
	it.Subtasks = []Subtask{}
	return it, nil
}

func ni(v sql.NullInt64) *int64 {
	if v.Valid {
		return &v.Int64
	}
	return nil
}

func nullable(p *int64) any {
	if p == nil {
		return nil
	}
	return *p
}

// decorate 解密并附加标签、提醒。调用前必须已读完 rows（单连接）。
func (s *Store) decorate(q execer, items []Item) error {
	if len(items) == 0 {
		return nil
	}
	idx := make(map[string]int, len(items))
	for i := range items {
		var l1, l2 bool
		items[i].Title, l1 = s.dec(items[i].Title)
		items[i].Note, l2 = s.dec(items[i].Note)
		items[i].Locked = l1 || l2
		idx[items[i].ID] = i
	}
	// 标签
	tagNames, err := s.tagNames(q)
	if err != nil {
		return err
	}
	rows, err := q.Query(`SELECT item_id, tag_id FROM item_tags WHERE deleted=0`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var iid, tid string
		if err := rows.Scan(&iid, &tid); err != nil {
			rows.Close()
			return err
		}
		if i, ok := idx[iid]; ok {
			if n, ok := tagNames[tid]; ok {
				items[i].Tags = append(items[i].Tags, n)
			}
		}
	}
	rows.Close()
	// 提醒
	rrows, err := q.Query(`SELECT id,item_id,remind_at,offset_minutes,repeat_rule,snooze_until,last_fired_at FROM reminders WHERE deleted=0 ORDER BY id`)
	if err != nil {
		return err
	}
	for rrows.Next() {
		var r Reminder
		var at, sn, lf sql.NullInt64
		if err := rrows.Scan(&r.ID, &r.ItemID, &at, &r.OffsetMinutes, &r.RepeatRule, &sn, &lf); err != nil {
			rrows.Close()
			return err
		}
		r.RemindAt, r.SnoozeUntil, r.LastFiredAt = ni(at), ni(sn), ni(lf)
		if i, ok := idx[r.ItemID]; ok {
			items[i].Reminders = append(items[i].Reminders, r)
		}
	}
	if err := rrows.Err(); err != nil {
		return err
	}
	rrows.Close()
	return s.loadSubtasks(q, items, idx)
}

func (s *Store) tagNames(q execer) (map[string]string, error) {
	rows, err := q.Query(`SELECT id,name FROM tags WHERE deleted=0`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := map[string]string{}
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		n, _ := s.dec(name)
		m[id] = n
	}
	return m, rows.Err()
}

// GetItem 读取单个事项（含已删除）。
func (s *Store) GetItem(id string) (Item, error) {
	return getItem(s, s.db, id)
}

func getItem(s *Store, q execer, id string) (Item, error) {
	it, err := scanItem(q.QueryRow(`SELECT `+itemCols+` FROM items WHERE id=?`, id))
	if err != nil {
		return it, err
	}
	list := []Item{it}
	if err := s.decorate(q, list); err != nil {
		return it, err
	}
	return list[0], nil
}

func validateTitle(t string) (string, error) {
	t = strings.TrimSpace(t)
	if t == "" {
		return "", errors.New("标题不能为空")
	}
	if utf8.RuneCountInString(t) > MaxTitleRunes {
		return "", errors.New("标题不能超过 200 字")
	}
	return t, nil
}

// CreateItem 新建事项（FR-101）。
func (s *Store) CreateItem(in NewItem) (Item, error) {
	title, err := validateTitle(in.Title)
	if err != nil {
		return Item{}, err
	}
	var out Item
	err = s.Tx(func(tx *sql.Tx) error {
		gid := in.GroupID
		if gid == "" {
			if err := tx.QueryRow(`SELECT id FROM groups WHERE is_default=1 AND deleted=0`).Scan(&gid); err != nil {
				return err
			}
		}
		id := in.ID
		if id == "" {
			id = newID()
		}
		prio := PriorityMid
		if in.Priority != nil {
			prio = clampPrio(*in.Priority)
		}
		var maxSort sql.NullFloat64
		_ = tx.QueryRow(`SELECT MAX(sort_order) FROM items WHERE group_id=?`, gid).Scan(&maxSort)
		now := s.nowMs()
		et, err := s.enc(title)
		if err != nil {
			return err
		}
		en, err := s.enc(in.Note)
		if err != nil {
			return err
		}
		h := s.clock.Next()
		if _, err := tx.Exec(`INSERT INTO items(`+itemCols+`) VALUES(?,?,?,?,?,?,'todo',NULL,NULL,?,?,?,?,?,?,?)`,
			id, et, en, gid, prio, nullable(in.DueAt), maxSort.Float64+1, in.SeriesID, now, now, h, s.deviceID,
			hlc.NewFieldHLC(h, itemFields...).JSON()); err != nil {
			return err
		}
		if err := s.ftsUpsert(tx, id, title, in.Note, true); err != nil {
			return err
		}
		if err := s.setTags(tx, id, in.Tags); err != nil {
			return err
		}
		if err := s.setReminders(tx, id, in.DueAt, in.Reminders); err != nil {
			return err
		}
		for i, ns := range in.Subtasks {
			if _, err := s.addSubtask(tx, id, ns, float64(i+1)); err != nil {
				return err
			}
		}
		out, err = getItem(s, tx, id)
		return err
	})
	return out, err
}

func clampPrio(p int) int {
	if p < PriorityLow {
		return PriorityLow
	}
	if p > PriorityHigh {
		return PriorityHigh
	}
	return p
}

// UpdateItem 编辑事项；只更新被修改字段的 field_hlc（AC-16）。
func (s *Store) UpdateItem(id string, p ItemPatch) (Item, error) {
	var out Item
	err := s.Tx(func(tx *sql.Tx) error {
		cur, err := getItem(s, tx, id)
		if err != nil {
			return err
		}
		if cur.Locked {
			return errors.New("数据已加密且尚未解锁")
		}
		h := s.clock.Next()
		var fields []string
		set := func(col string, v any) error {
			_, err := tx.Exec(`UPDATE items SET `+col+`=? WHERE id=?`, v, id)
			fields = append(fields, col)
			return err
		}
		title, note := cur.Title, cur.Note
		if p.Title != nil {
			t, err := validateTitle(*p.Title)
			if err != nil {
				return err
			}
			if t != cur.Title {
				title = t
				e, err := s.enc(t)
				if err != nil {
					return err
				}
				if err := set("title", e); err != nil {
					return err
				}
			}
		}
		if p.Note != nil && *p.Note != cur.Note {
			note = *p.Note
			e, err := s.enc(note)
			if err != nil {
				return err
			}
			if err := set("note", e); err != nil {
				return err
			}
		}
		if p.GroupID != nil && *p.GroupID != cur.GroupID {
			if err := set("group_id", *p.GroupID); err != nil {
				return err
			}
		}
		if p.Priority != nil && clampPrio(*p.Priority) != cur.Priority {
			if err := set("priority", clampPrio(*p.Priority)); err != nil {
				return err
			}
		}
		dueChanged := false
		if p.ClearDue && cur.DueAt != nil {
			if err := set("due_at", nil); err != nil {
				return err
			}
			dueChanged = true
		} else if p.DueAt != nil && (cur.DueAt == nil || *cur.DueAt != *p.DueAt) {
			if err := set("due_at", *p.DueAt); err != nil {
				return err
			}
			dueChanged = true
		}
		if p.Sort != nil && *p.Sort != cur.SortOrder {
			if err := set("sort_order", *p.Sort); err != nil {
				return err
			}
		}
		if len(fields) > 0 {
			if _, err := tx.Exec(`UPDATE items SET updated_at=? WHERE id=?`, s.nowMs(), id); err != nil {
				return err
			}
			if err := s.touch(tx, "items", id, h, fields...); err != nil {
				return err
			}
		}
		if p.Title != nil || p.Note != nil {
			if err := s.ftsUpsert(tx, id, title, note, false); err != nil {
				return err
			}
		}
		if p.Tags != nil {
			if err := s.setTags(tx, id, *p.Tags); err != nil {
				return err
			}
		}
		if dueChanged {
			var due *int64
			if !p.ClearDue {
				due = p.DueAt
			}
			if err := s.recomputeOffsetReminders(tx, id, due); err != nil {
				return err
			}
		}
		out, err = getItem(s, tx, id)
		return err
	})
	return out, err
}

// SetDone 标记完成 / 取消完成（FR-104）。
func (s *Store) SetDone(id string, done bool) (Item, error) {
	var out Item
	err := s.Tx(func(tx *sql.Tx) error {
		st, comp := StatusTodo, any(nil)
		if done {
			st, comp = StatusDone, s.nowMs()
		}
		if _, err := tx.Exec(`UPDATE items SET status=?, completed_at=?, updated_at=? WHERE id=? AND status!='deleted'`,
			st, comp, s.nowMs(), id); err != nil {
			return err
		}
		if err := s.touch(tx, "items", id, s.clock.Next(), "status", "completed_at"); err != nil {
			return err
		}
		var err error
		out, err = getItem(s, tx, id)
		return err
	})
	return out, err
}

// SoftDelete 删除进入回收站（FR-105）。
func (s *Store) SoftDelete(id string) error {
	return s.Tx(func(tx *sql.Tx) error {
		if _, err := tx.Exec(`UPDATE items SET status='deleted', deleted_at=?, updated_at=? WHERE id=?`, s.nowMs(), s.nowMs(), id); err != nil {
			return err
		}
		return s.touch(tx, "items", id, s.clock.Next(), "status", "deleted_at")
	})
}

// Restore 从回收站恢复，所有字段和提醒保持不变。
func (s *Store) Restore(id string) (Item, error) {
	var out Item
	err := s.Tx(func(tx *sql.Tx) error {
		if _, err := tx.Exec(`UPDATE items SET status=CASE WHEN completed_at IS NULL THEN 'todo' ELSE 'done' END,
			deleted_at=NULL, updated_at=? WHERE id=? AND status='deleted'`, s.nowMs(), id); err != nil {
			return err
		}
		if err := s.touch(tx, "items", id, s.clock.Next(), "status", "deleted_at"); err != nil {
			return err
		}
		var err error
		out, err = getItem(s, tx, id)
		return err
	})
	return out, err
}

// TrashRetention 是回收站保留期（30 天）。
const TrashRetention = 30 * 24 * time.Hour

// PurgeExpired 清除删除超过 30 天的事项；返回清除数量。
func (s *Store) PurgeExpired() (int, error) {
	cutoff := s.now().Add(-TrashRetention).UnixMilli()
	return s.purge(`status='deleted' AND deleted_at<=?`, cutoff)
}

// EmptyTrash 清空回收站（不可撤销，调用方须二次确认）。
func (s *Store) EmptyTrash() (int, error) { return s.purge(`status='deleted' AND ?=?`, 1, 1) }

func (s *Store) purge(where string, args ...any) (int, error) {
	n := 0
	err := s.Tx(func(tx *sql.Tx) error {
		rows, err := tx.Query(`SELECT id FROM items WHERE `+where, args...)
		if err != nil {
			return err
		}
		var ids []string
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			ids = append(ids, id)
		}
		rows.Close()
		for _, id := range ids {
			for _, q := range []string{
				`DELETE FROM subtasks WHERE item_id=?`, `DELETE FROM reminders WHERE item_id=?`, `DELETE FROM item_tags WHERE item_id=?`,
				`DELETE FROM items_fts WHERE item_id=?`, `DELETE FROM items WHERE id=?`} {
				if _, err := tx.Exec(q, id); err != nil {
					return err
				}
			}
		}
		n = len(ids)
		return nil
	})
	return n, err
}

// ListItems 按条件列出事项（未排序，见 SortItems）。
func (s *Store) ListItems(f Filter) ([]Item, error) {
	q := `SELECT ` + itemCols + ` FROM items WHERE `
	var args []any
	switch f.Status {
	case "":
		q += `status IN ('todo','done')`
	default:
		q += `status=?`
		args = append(args, f.Status)
	}
	if f.GroupID != "" {
		q += ` AND group_id=?`
		args = append(args, f.GroupID)
	}
	if f.NoDue {
		q += ` AND due_at IS NULL`
	}
	if f.DueFrom != nil {
		q += ` AND due_at>=?`
		args = append(args, *f.DueFrom)
	}
	if f.DueTo != nil {
		q += ` AND due_at<?`
		args = append(args, *f.DueTo)
	}
	if f.Limit > 0 {
		q += ` LIMIT ?`
		args = append(args, f.Limit)
	}
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	var items []Item
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		items = append(items, it)
	}
	rows.Close()
	if err := s.decorate(s.db, items); err != nil {
		return nil, err
	}
	if f.Tag != "" {
		kept := items[:0]
		for _, it := range items {
			for _, t := range it.Tags {
				if strings.EqualFold(t, f.Tag) {
					kept = append(kept, it)
					break
				}
			}
		}
		items = kept
	}
	return items, nil
}

// Rank 是 FR-106 的分级：逾期 0 < 今天 1 < 有截止时间 2 < 无截止时间 3。
func Rank(it Item, now time.Time, loc *time.Location) int {
	if it.DueAt == nil {
		return 3
	}
	due := time.UnixMilli(*it.DueAt)
	if it.Status == StatusTodo && due.Before(now) {
		return 0
	}
	y1, m1, d1 := due.In(loc).Date()
	y2, m2, d2 := now.In(loc).Date()
	if y1 == y2 && m1 == m2 && d1 == d2 {
		return 1
	}
	return 2
}

// SortItems 按 FR-106 排序：分级 > 优先级（高在前）> 手动顺序 > 截止时间 > 创建时间。
func SortItems(items []Item, now time.Time, loc *time.Location) {
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		if ra, rb := Rank(a, now, loc), Rank(b, now, loc); ra != rb {
			return ra < rb
		}
		if a.Priority != b.Priority {
			return a.Priority > b.Priority
		}
		if a.SortOrder != b.SortOrder {
			return a.SortOrder < b.SortOrder
		}
		if a.DueAt != nil && b.DueAt != nil && *a.DueAt != *b.DueAt {
			return *a.DueAt < *b.DueAt
		}
		return a.CreatedAt < b.CreatedAt
	})
}

// SetSeries 记录周期事项所属系列 id。
func (s *Store) SetSeries(id, seriesID string) error {
	return s.Tx(func(tx *sql.Tx) error {
		if _, err := tx.Exec(`UPDATE items SET series_id=? WHERE id=?`, seriesID, id); err != nil {
			return err
		}
		return s.touch(tx, "items", id, s.clock.Next(), "series_id")
	})
}

// PurgeItem 彻底删除一条事项（仅用于撤销「新建」「自动生成的周期实例」）。
func (s *Store) PurgeItem(id string) error {
	_, err := s.purge(`id=?`, id)
	return err
}

// ItemExists 判断事项 id 是否已存在。
func (s *Store) ItemExists(id string) bool {
	var n int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM items WHERE id=?`, id).Scan(&n)
	return n > 0
}

// CountOverdue 返回逾期未完成事项数（托盘红点）。
func (s *Store) CountOverdue() int {
	var n int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM items WHERE status='todo' AND due_at IS NOT NULL AND due_at<?`, s.nowMs()).Scan(&n)
	return n
}
