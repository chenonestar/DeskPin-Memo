package store

import (
	"database/sql"
	"errors"

	"deskpinmemo/internal/hlc"
)

var reminderFields = []string{"remind_at", "offset_minutes", "repeat_rule", "snooze_until", "last_fired_at", "deleted"}

// effectiveRemindAt 计算提醒的绝对触发时间。
func effectiveRemindAt(due *int64, in ReminderInput) *int64 {
	switch {
	case in.OffsetMinutes > 0 && due != nil:
		t := *due - int64(in.OffsetMinutes)*60_000
		return &t
	case in.RemindAt != nil:
		return in.RemindAt
	case due != nil:
		return due
	}
	return nil
}

func (s *Store) setReminders(tx *sql.Tx, itemID string, due *int64, ins []ReminderInput) error {
	rows, err := tx.Query(`SELECT id FROM reminders WHERE item_id=? AND deleted=0`, itemID)
	if err != nil {
		return err
	}
	var old []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		old = append(old, id)
	}
	rows.Close()
	for _, id := range old {
		if _, err := tx.Exec(`UPDATE reminders SET deleted=1 WHERE id=?`, id); err != nil {
			return err
		}
		if err := s.touch(tx, "reminders", id, s.clock.Next(), "deleted"); err != nil {
			return err
		}
	}
	for _, in := range ins {
		if in.OffsetMinutes > 0 && due == nil {
			return errors.New("「截止前提醒」需要先设置截止时间")
		}
		at := effectiveRemindAt(due, in)
		if at == nil && in.RepeatRule == "" {
			continue
		}
		h := s.clock.Next()
		if _, err := tx.Exec(`INSERT INTO reminders(id,item_id,remind_at,offset_minutes,repeat_rule,hlc,device_id,field_hlc,deleted)
			VALUES(?,?,?,?,?,?,?,?,0)`, newID(), itemID, nullable(at), in.OffsetMinutes, in.RepeatRule, h, s.deviceID,
			hlc.NewFieldHLC(h, reminderFields...).JSON()); err != nil {
			return err
		}
	}
	return nil
}

// SetReminders 用 ins 替换事项的全部提醒。
func (s *Store) SetReminders(itemID string, ins []ReminderInput) (Item, error) {
	var out Item
	err := s.Tx(func(tx *sql.Tx) error {
		var due sql.NullInt64
		if err := tx.QueryRow(`SELECT due_at FROM items WHERE id=?`, itemID).Scan(&due); err != nil {
			return err
		}
		if err := s.setReminders(tx, itemID, ni(due), ins); err != nil {
			return err
		}
		if err := s.touch(tx, "items", itemID, s.clock.Next(), "title"); err != nil { // 使事项 hlc 前进，便于同步感知
			return err
		}
		var err error
		out, err = getItem(s, tx, itemID)
		return err
	})
	return out, err
}

// recomputeOffsetReminders 截止时间变化后重算「截止前 N 分钟」提醒，并清除已触发标记。
func (s *Store) recomputeOffsetReminders(tx *sql.Tx, itemID string, due *int64) error {
	rows, err := tx.Query(`SELECT id, offset_minutes FROM reminders WHERE item_id=? AND deleted=0 AND offset_minutes>0`, itemID)
	if err != nil {
		return err
	}
	type r struct {
		id  string
		off int
	}
	var rs []r
	for rows.Next() {
		var x r
		if err := rows.Scan(&x.id, &x.off); err != nil {
			rows.Close()
			return err
		}
		rs = append(rs, x)
	}
	rows.Close()
	for _, x := range rs {
		at := effectiveRemindAt(due, ReminderInput{OffsetMinutes: x.off})
		if _, err := tx.Exec(`UPDATE reminders SET remind_at=?, snooze_until=NULL, last_fired_at=NULL WHERE id=?`, nullable(at), x.id); err != nil {
			return err
		}
		if err := s.touch(tx, "reminders", x.id, s.clock.Next(), "remind_at", "snooze_until", "last_fired_at"); err != nil {
			return err
		}
	}
	return nil
}

// PendingReminder 是尚未触发的提醒（含事项信息，供调度器使用）。
type PendingReminder struct {
	Reminder
	FireAt   int64  `json:"fireAt"`
	Title    string `json:"title"`
	Note     string `json:"note"`
	Priority int    `json:"priority"`
	GroupID  string `json:"groupId"`
	Locked   bool   `json:"locked"`
}

// PendingReminders 返回所有待触发提醒（仅未完成、未删除事项），按触发时间升序。
func (s *Store) PendingReminders() ([]PendingReminder, error) {
	rows, err := s.db.Query(`
SELECT r.id, r.item_id, r.remind_at, r.offset_minutes, r.repeat_rule, r.snooze_until, r.last_fired_at,
       COALESCE(r.snooze_until, r.remind_at) AS fire_at, i.title, i.note, i.priority, i.group_id
FROM reminders r JOIN items i ON i.id=r.item_id
WHERE r.deleted=0 AND i.status='todo'
  AND COALESCE(r.snooze_until, r.remind_at) IS NOT NULL
  AND (r.last_fired_at IS NULL OR r.last_fired_at < COALESCE(r.snooze_until, r.remind_at))
ORDER BY fire_at, r.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PendingReminder
	for rows.Next() {
		var p PendingReminder
		var at, sn, lf sql.NullInt64
		if err := rows.Scan(&p.ID, &p.ItemID, &at, &p.OffsetMinutes, &p.RepeatRule, &sn, &lf, &p.FireAt, &p.Title, &p.Note, &p.Priority, &p.GroupID); err != nil {
			return nil, err
		}
		p.RemindAt, p.SnoozeUntil, p.LastFiredAt = ni(at), ni(sn), ni(lf)
		var l1, l2 bool
		p.Title, l1 = s.dec(p.Title)
		p.Note, l2 = s.dec(p.Note)
		p.Locked = l1 || l2
		out = append(out, p)
	}
	return out, rows.Err()
}

// MarkFired 标记提醒已触发。
func (s *Store) MarkFired(reminderIDs []string, at int64) error {
	return s.Tx(func(tx *sql.Tx) error {
		for _, id := range reminderIDs {
			if _, err := tx.Exec(`UPDATE reminders SET last_fired_at=? WHERE id=?`, at, id); err != nil {
				return err
			}
			if err := s.touch(tx, "reminders", id, s.clock.Next(), "last_fired_at"); err != nil {
				return err
			}
		}
		return nil
	})
}

// SnoozeItem 对事项所有提醒执行「稍后提醒」：若无提醒则新建一条。
func (s *Store) SnoozeItem(itemID string, until int64) error {
	return s.Tx(func(tx *sql.Tx) error {
		rows, err := tx.Query(`SELECT id FROM reminders WHERE item_id=? AND deleted=0`, itemID)
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
		if len(ids) == 0 {
			h := s.clock.Next()
			_, err := tx.Exec(`INSERT INTO reminders(id,item_id,remind_at,snooze_until,hlc,device_id,field_hlc,deleted)
				VALUES(?,?,?,?,?,?,?,0)`, newID(), itemID, until, until, h, s.deviceID, hlc.NewFieldHLC(h, reminderFields...).JSON())
			return err
		}
		for _, id := range ids {
			if _, err := tx.Exec(`UPDATE reminders SET snooze_until=? WHERE id=?`, until, id); err != nil {
				return err
			}
			if err := s.touch(tx, "reminders", id, s.clock.Next(), "snooze_until"); err != nil {
				return err
			}
		}
		return nil
	})
}
