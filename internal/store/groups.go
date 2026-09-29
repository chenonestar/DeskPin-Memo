package store

import (
	"database/sql"
	"errors"
	"strings"

	"deskpinmemo/internal/hlc"
)

var groupFields = []string{"name", "color", "sort_order", "deleted"}

// CreateGroup 新建分组。
func (s *Store) CreateGroup(name, color string) (Group, error) {
	return s.CreateGroupOpts(name, color, false)
}

// CreateGroupOpts 新建分组，isDefault 仅用于「收件箱」。
func (s *Store) CreateGroupOpts(name, color string, isDefault bool) (Group, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Group{}, errors.New("分组名不能为空")
	}
	g := Group{ID: newID(), Name: name, Color: color, IsDefault: isDefault}
	err := s.Tx(func(tx *sql.Tx) error {
		var max sql.NullFloat64
		_ = tx.QueryRow(`SELECT MAX(sort_order) FROM groups WHERE deleted=0`).Scan(&max)
		g.SortOrder = max.Float64 + 1
		en, err := s.enc(name)
		if err != nil {
			return err
		}
		h := s.clock.Next()
		_, err = tx.Exec(`INSERT INTO groups(id,name,color,sort_order,is_default,hlc,device_id,field_hlc,deleted)
			VALUES(?,?,?,?,?,?,?,?,0)`, g.ID, en, color, g.SortOrder, b2i(isDefault), h, s.deviceID,
			hlc.NewFieldHLC(h, groupFields...).JSON())
		return err
	})
	return g, err
}

// ListGroups 返回未删除分组（按 sort_order）。
func (s *Store) ListGroups() ([]Group, error) {
	rows, err := s.db.Query(`SELECT id,name,color,sort_order,is_default FROM groups WHERE deleted=0 ORDER BY is_default DESC, sort_order, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Group
	for rows.Next() {
		var g Group
		var def int
		if err := rows.Scan(&g.ID, &g.Name, &g.Color, &g.SortOrder, &def); err != nil {
			return nil, err
		}
		g.IsDefault = def == 1
		g.Name, g.Locked = s.dec(g.Name)
		out = append(out, g)
	}
	return out, rows.Err()
}

// InboxID 返回默认分组「收件箱」id。
func (s *Store) InboxID() (string, error) {
	var id string
	err := s.db.QueryRow(`SELECT id FROM groups WHERE is_default=1 AND deleted=0 LIMIT 1`).Scan(&id)
	return id, err
}

// UpdateGroup 重命名 / 改色 / 调整排序。
func (s *Store) UpdateGroup(id string, name, color *string, sortOrder *float64) error {
	return s.Tx(func(tx *sql.Tx) error {
		h := s.clock.Next()
		var fields []string
		if name != nil {
			n := strings.TrimSpace(*name)
			if n == "" {
				return errors.New("分组名不能为空")
			}
			en, err := s.enc(n)
			if err != nil {
				return err
			}
			if _, err := tx.Exec(`UPDATE groups SET name=? WHERE id=?`, en, id); err != nil {
				return err
			}
			fields = append(fields, "name")
		}
		if color != nil {
			if _, err := tx.Exec(`UPDATE groups SET color=? WHERE id=?`, *color, id); err != nil {
				return err
			}
			fields = append(fields, "color")
		}
		if sortOrder != nil {
			if _, err := tx.Exec(`UPDATE groups SET sort_order=? WHERE id=?`, *sortOrder, id); err != nil {
				return err
			}
			fields = append(fields, "sort_order")
		}
		if len(fields) == 0 {
			return nil
		}
		return s.touch(tx, "groups", id, h, fields...)
	})
}

// DeleteGroup 删除分组，其事项移回收件箱；收件箱不可删除。
func (s *Store) DeleteGroup(id string) error {
	return s.Tx(func(tx *sql.Tx) error {
		var def int
		if err := tx.QueryRow(`SELECT is_default FROM groups WHERE id=? AND deleted=0`, id).Scan(&def); err != nil {
			return err
		}
		if def == 1 {
			return errors.New("收件箱不能删除")
		}
		var inbox string
		if err := tx.QueryRow(`SELECT id FROM groups WHERE is_default=1 AND deleted=0`).Scan(&inbox); err != nil {
			return err
		}
		rows, err := tx.Query(`SELECT id FROM items WHERE group_id=?`, id)
		if err != nil {
			return err
		}
		var ids []string
		for rows.Next() {
			var iid string
			if err := rows.Scan(&iid); err != nil {
				rows.Close()
				return err
			}
			ids = append(ids, iid)
		}
		rows.Close()
		for _, iid := range ids {
			if _, err := tx.Exec(`UPDATE items SET group_id=?, updated_at=? WHERE id=?`, inbox, s.nowMs(), iid); err != nil {
				return err
			}
			if err := s.touch(tx, "items", iid, s.clock.Next(), "group_id"); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(`UPDATE groups SET deleted=1 WHERE id=?`, id); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM windows WHERE group_id=?`, id); err != nil {
			return err
		}
		return s.touch(tx, "groups", id, s.clock.Next(), "deleted")
	})
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}
