package store

import (
	"database/sql"
	"errors"
	"strings"

	"deskpinmemo/internal/hlc"
)

var subtaskFields = []string{"title", "done", "sort_order", "completed_at", "deleted"}

func validateSubtaskTitle(t string) (string, error) {
	t = strings.TrimSpace(t)
	if t == "" {
		return "", errors.New("子任务标题不能为空")
	}
	if len([]rune(t)) > MaxTitleRunes {
		return "", errors.New("子任务标题不能超过 200 字")
	}
	return t, nil
}

// loadSubtasks 为 items 附加未删除的子任务（按 sort_order）。调用前 rows 必须已关闭（单连接）。
func (s *Store) loadSubtasks(q execer, items []Item, idx map[string]int) error {
	rows, err := q.Query(`SELECT id,item_id,title,done,sort_order,completed_at,created_at FROM subtasks WHERE deleted=0 ORDER BY sort_order,id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var sub Subtask
		var done int
		var comp sql.NullInt64
		if err := rows.Scan(&sub.ID, &sub.ItemID, &sub.Title, &done, &sub.SortOrder, &comp, &sub.CreatedAt); err != nil {
			return err
		}
		sub.Done, sub.CompletedAt = done == 1, ni(comp)
		sub.Title, sub.Locked = s.dec(sub.Title)
		if i, ok := idx[sub.ItemID]; ok {
			items[i].Subtasks = append(items[i].Subtasks, sub)
		}
	}
	return rows.Err()
}

func (s *Store) addSubtask(tx *sql.Tx, itemID string, ns NewSubtask, order float64) (Subtask, error) {
	title, err := validateSubtaskTitle(ns.Title)
	if err != nil {
		return Subtask{}, err
	}
	et, err := s.enc(title)
	if err != nil {
		return Subtask{}, err
	}
	id := ns.ID
	if id == "" {
		id = newID()
	}
	now := s.nowMs()
	var comp any
	if ns.Done {
		comp = now
	}
	h := s.clock.Next()
	if _, err := tx.Exec(`INSERT INTO subtasks(id,item_id,title,done,sort_order,completed_at,created_at,hlc,device_id,field_hlc,deleted)
		VALUES(?,?,?,?,?,?,?,?,?,?,0)`, id, itemID, et, b2i(ns.Done), order, comp, now, h, s.deviceID,
		hlc.NewFieldHLC(h, subtaskFields...).JSON()); err != nil {
		return Subtask{}, err
	}
	sub := Subtask{ID: id, ItemID: itemID, Title: title, Done: ns.Done, SortOrder: order, CreatedAt: now}
	if ns.Done {
		sub.CompletedAt = &now
	}
	return sub, nil
}

// AddSubtask 在事项末尾追加一条子任务（FR-108）。
func (s *Store) AddSubtask(itemID, title string) (Subtask, error) {
	var out Subtask
	err := s.Tx(func(tx *sql.Tx) error {
		var status string
		if err := tx.QueryRow(`SELECT status FROM items WHERE id=?`, itemID).Scan(&status); err != nil {
			return err
		}
		if status == StatusDeleted {
			return errors.New("事项已删除")
		}
		var max sql.NullFloat64
		_ = tx.QueryRow(`SELECT MAX(sort_order) FROM subtasks WHERE item_id=? AND deleted=0`, itemID).Scan(&max)
		var err error
		out, err = s.addSubtask(tx, itemID, NewSubtask{Title: title}, max.Float64+1)
		return err
	})
	return out, err
}

func (s *Store) getSubtask(q execer, id string) (Subtask, error) {
	var sub Subtask
	var done int
	var comp sql.NullInt64
	err := q.QueryRow(`SELECT id,item_id,title,done,sort_order,completed_at,created_at FROM subtasks WHERE id=? AND deleted=0`, id).
		Scan(&sub.ID, &sub.ItemID, &sub.Title, &done, &sub.SortOrder, &comp, &sub.CreatedAt)
	if err != nil {
		return sub, err
	}
	sub.Done, sub.CompletedAt = done == 1, ni(comp)
	sub.Title, sub.Locked = s.dec(sub.Title)
	return sub, nil
}

// GetSubtask 读取单个子任务。
func (s *Store) GetSubtask(id string) (Subtask, error) { return s.getSubtask(s.db, id) }

// UpdateSubtask 修改标题和 / 或完成状态；只更新被改字段的 field_hlc。
func (s *Store) UpdateSubtask(id string, title *string, done *bool) (Subtask, error) {
	var out Subtask
	err := s.Tx(func(tx *sql.Tx) error {
		cur, err := s.getSubtask(tx, id)
		if err != nil {
			return err
		}
		if cur.Locked {
			return errLocked
		}
		var fields []string
		if title != nil {
			t, err := validateSubtaskTitle(*title)
			if err != nil {
				return err
			}
			if t != cur.Title {
				e, err := s.enc(t)
				if err != nil {
					return err
				}
				if _, err := tx.Exec(`UPDATE subtasks SET title=? WHERE id=?`, e, id); err != nil {
					return err
				}
				fields = append(fields, "title")
			}
		}
		if done != nil && *done != cur.Done {
			var comp any
			if *done {
				comp = s.nowMs()
			}
			if _, err := tx.Exec(`UPDATE subtasks SET done=?, completed_at=? WHERE id=?`, b2i(*done), comp, id); err != nil {
				return err
			}
			fields = append(fields, "done", "completed_at")
		}
		if len(fields) > 0 {
			if err := s.touch(tx, "subtasks", id, s.clock.Next(), fields...); err != nil {
				return err
			}
		}
		out, err = s.getSubtask(tx, id)
		return err
	})
	return out, err
}

// DeleteSubtask 软删除（墓碑，便于日后同步）。
func (s *Store) DeleteSubtask(id string) error {
	return s.Tx(func(tx *sql.Tx) error {
		if _, err := tx.Exec(`UPDATE subtasks SET deleted=1 WHERE id=?`, id); err != nil {
			return err
		}
		return s.touch(tx, "subtasks", id, s.clock.Next(), "deleted")
	})
}

// RestoreSubtask 撤销删除。
func (s *Store) RestoreSubtask(id string) error {
	return s.Tx(func(tx *sql.Tx) error {
		if _, err := tx.Exec(`UPDATE subtasks SET deleted=0 WHERE id=?`, id); err != nil {
			return err
		}
		return s.touch(tx, "subtasks", id, s.clock.Next(), "deleted")
	})
}

// PurgeSubtask 彻底删除（仅用于撤销「新建子任务」）。
func (s *Store) PurgeSubtask(id string) error {
	_, err := s.db.Exec(`DELETE FROM subtasks WHERE id=?`, id)
	return err
}

// ReorderSubtasks 按给定 id 顺序重排事项下的子任务；未列出的保持相对顺序排在后面。
func (s *Store) ReorderSubtasks(itemID string, ordered []string) error {
	return s.Tx(func(tx *sql.Tx) error {
		pos := 0.0
		set := map[string]bool{}
		for _, id := range ordered {
			set[id] = true
		}
		rows, err := tx.Query(`SELECT id FROM subtasks WHERE item_id=? AND deleted=0 ORDER BY sort_order,id`, itemID)
		if err != nil {
			return err
		}
		var rest []string
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			if !set[id] {
				rest = append(rest, id)
			}
		}
		rows.Close()
		for _, id := range append(append([]string{}, ordered...), rest...) {
			pos++
			var cur float64
			if err := tx.QueryRow(`SELECT sort_order FROM subtasks WHERE id=? AND item_id=?`, id, itemID).Scan(&cur); err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					continue // 不属于该事项的 id 忽略
				}
				return err
			}
			if cur == pos {
				continue
			}
			if _, err := tx.Exec(`UPDATE subtasks SET sort_order=? WHERE id=?`, pos, id); err != nil {
				return err
			}
			if err := s.touch(tx, "subtasks", id, s.clock.Next(), "sort_order"); err != nil {
				return err
			}
		}
		return nil
	})
}
