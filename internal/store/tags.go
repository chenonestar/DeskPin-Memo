package store

import (
	"database/sql"
	"strings"

	"deskpinmemo/internal/hlc"
)

func normTag(t string) string {
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(t), "#"))
}

// setTags 将事项的标签集合替换为 names（在事务内）。
func (s *Store) setTags(tx *sql.Tx, itemID string, names []string) error {
	existing, err := s.tagNames(tx) // id → 明文名
	if err != nil {
		return err
	}
	byName := map[string]string{}
	for id, n := range existing {
		byName[strings.ToLower(n)] = id
	}
	want := map[string]bool{}
	for _, raw := range names {
		n := normTag(raw)
		if n == "" {
			continue
		}
		id, ok := byName[strings.ToLower(n)]
		if !ok {
			id = newID()
			en, err := s.enc(n)
			if err != nil {
				return err
			}
			h := s.clock.Next()
			if _, err := tx.Exec(`INSERT INTO tags(id,name,hlc,device_id,field_hlc,deleted) VALUES(?,?,?,?,?,0)`,
				id, en, h, s.deviceID, hlc.NewFieldHLC(h, "name", "deleted").JSON()); err != nil {
				return err
			}
			byName[strings.ToLower(n)] = id
		}
		want[id] = true
	}
	// 当前关联
	rows, err := tx.Query(`SELECT tag_id, deleted, field_hlc FROM item_tags WHERE item_id=?`, itemID)
	if err != nil {
		return err
	}
	type link struct {
		deleted bool
		fh      string
	}
	cur := map[string]link{}
	for rows.Next() {
		var tid, fh string
		var d int
		if err := rows.Scan(&tid, &d, &fh); err != nil {
			rows.Close()
			return err
		}
		cur[tid] = link{d == 1, fh}
	}
	rows.Close()
	for tid := range want {
		l, ok := cur[tid]
		h := s.clock.Next()
		switch {
		case !ok:
			if _, err := tx.Exec(`INSERT INTO item_tags(item_id,tag_id,hlc,device_id,field_hlc,deleted) VALUES(?,?,?,?,?,0)`,
				itemID, tid, h, s.deviceID, hlc.NewFieldHLC(h, "deleted").JSON()); err != nil {
				return err
			}
		case l.deleted:
			if _, err := tx.Exec(`UPDATE item_tags SET deleted=0, hlc=?, device_id=?, field_hlc=? WHERE item_id=? AND tag_id=?`,
				h, s.deviceID, hlc.ParseFieldHLC(l.fh).Touch(h, "deleted").JSON(), itemID, tid); err != nil {
				return err
			}
		}
	}
	for tid, l := range cur {
		if !want[tid] && !l.deleted {
			h := s.clock.Next()
			if _, err := tx.Exec(`UPDATE item_tags SET deleted=1, hlc=?, device_id=?, field_hlc=? WHERE item_id=? AND tag_id=?`,
				h, s.deviceID, hlc.ParseFieldHLC(l.fh).Touch(h, "deleted").JSON(), itemID, tid); err != nil {
				return err
			}
		}
	}
	return nil
}

// ListTags 返回所有在用标签名。
func (s *Store) ListTags() ([]string, error) {
	m, err := s.tagNames(s.db)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT DISTINCT tag_id FROM item_tags it JOIN items i ON i.id=it.item_id
		WHERE it.deleted=0 AND i.status!='deleted'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		if n, ok := m[id]; ok {
			out = append(out, n)
		}
	}
	return out, rows.Err()
}
