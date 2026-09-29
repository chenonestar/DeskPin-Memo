package store

import (
	"database/sql"
	"strings"
	"unicode/utf8"
)

// ftsUpsert 维护全文索引；加密模式下不写索引（索引会存明文）。
func (s *Store) ftsUpsert(tx *sql.Tx, id, title, note string, isNew bool) error {
	if s.encrypted.Load() {
		return nil
	}
	if !isNew { // item_id 未建索引，删除是全表扫描；新建时无需删除
		if _, err := tx.Exec(`DELETE FROM items_fts WHERE item_id=?`, id); err != nil {
			return err
		}
	}
	_, err := tx.Exec(`INSERT INTO items_fts(item_id,title,note) VALUES(?,?,?)`, id, title, note)
	return err
}

// RebuildFTS 重建全文索引（关闭加密后、导入后调用）。
func (s *Store) RebuildFTS() error {
	return s.Tx(func(tx *sql.Tx) error {
		if _, err := tx.Exec(`DELETE FROM items_fts`); err != nil {
			return err
		}
		if s.encrypted.Load() {
			return nil
		}
		_, err := tx.Exec(`INSERT INTO items_fts(item_id,title,note) SELECT id,title,note FROM items WHERE status!='deleted'`)
		return err
	})
}

// Search 全局搜索标题和备注（含已完成，FR-404）。
func (s *Store) Search(q string, limit int) ([]Item, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return []Item{}, nil
	}
	if limit <= 0 {
		limit = 200
	}
	if s.encrypted.Load() {
		return s.searchInMemory(q, limit)
	}
	var rows *sql.Rows
	var err error
	if utf8.RuneCountInString(q) >= 3 { // trigram 至少 3 个字符
		rows, err = s.db.Query(`SELECT `+prefixed("i.", itemCols)+` FROM items_fts f JOIN items i ON i.id=f.item_id
			WHERE items_fts MATCH ? AND i.status!='deleted' LIMIT ?`, `"`+strings.ReplaceAll(q, `"`, `""`)+`"`, limit)
	} else {
		like := "%" + escapeLike(q) + "%"
		rows, err = s.db.Query(`SELECT `+prefixed("i.", itemCols)+` FROM items i
			WHERE i.status!='deleted' AND (i.title LIKE ? ESCAPE '\' OR i.note LIKE ? ESCAPE '\') LIMIT ?`, like, like, limit)
	}
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
	return items, nil
}

// searchInMemory 加密模式：解锁后在内存中解密检索（NFR/10.1：≤500 ms）。
func (s *Store) searchInMemory(q string, limit int) ([]Item, error) {
	if s.Locked() {
		return []Item{}, nil
	}
	all, err := s.ListItems(Filter{})
	if err != nil {
		return nil, err
	}
	lq := strings.ToLower(q)
	var out []Item
	for _, it := range all {
		if strings.Contains(strings.ToLower(it.Title), lq) || strings.Contains(strings.ToLower(it.Note), lq) {
			out = append(out, it)
			if len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}

func prefixed(p, cols string) string {
	parts := strings.Split(cols, ",")
	for i := range parts {
		parts[i] = p + parts[i]
	}
	return strings.Join(parts, ",")
}

func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}
