package store

import (
	"database/sql"
	"errors"
)

const windowCols = `id,group_id,mode,x,y,width,height,monitor_id,opacity,locked,collapsed,color,click_through`

func scanWindow(sc interface{ Scan(...any) error }) (Window, error) {
	var w Window
	var lk, co, ct int
	err := sc.Scan(&w.ID, &w.GroupID, &w.Mode, &w.X, &w.Y, &w.Width, &w.Height, &w.MonitorID, &w.Opacity, &lk, &co, &w.Color, &ct)
	w.Locked, w.Collapsed, w.ClickThrough = lk == 1, co == 1, ct == 1
	return w, err
}

// ListWindows 返回所有便签窗口状态。
func (s *Store) ListWindows() ([]Window, error) {
	rows, err := s.db.Query(`SELECT ` + windowCols + ` FROM windows ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Window
	for rows.Next() {
		w, err := scanWindow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// GetWindowByGroup 返回分组对应的窗口；不存在返回 sql.ErrNoRows。
func (s *Store) GetWindowByGroup(groupID string) (Window, error) {
	return scanWindow(s.db.QueryRow(`SELECT `+windowCols+` FROM windows WHERE group_id=?`, groupID))
}

// SaveWindow 新建或更新窗口状态（拖动/缩放后自动保存，FR-203）。
func (s *Store) SaveWindow(w Window) (Window, error) {
	if w.GroupID == "" {
		return w, errors.New("窗口必须关联分组")
	}
	if w.Mode == "" {
		w.Mode = "desktop"
	}
	// 0 表示「跟随设置里的默认透明度」；其余限制在 30%–100%
	if w.Opacity != 0 {
		if w.Opacity < 0.3 {
			w.Opacity = 0.3
		}
		if w.Opacity > 1 {
			w.Opacity = 1
		}
	}
	err := s.Tx(func(tx *sql.Tx) error {
		if w.ID == "" {
			var id string
			if err := tx.QueryRow(`SELECT id FROM windows WHERE group_id=?`, w.GroupID).Scan(&id); err == nil {
				w.ID = id
			} else {
				w.ID = newID()
			}
		}
		_, err := tx.Exec(`INSERT INTO windows(`+windowCols+`) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)
			ON CONFLICT(id) DO UPDATE SET group_id=excluded.group_id, mode=excluded.mode, x=excluded.x, y=excluded.y,
			width=excluded.width, height=excluded.height, monitor_id=excluded.monitor_id, opacity=excluded.opacity,
			locked=excluded.locked, collapsed=excluded.collapsed, color=excluded.color, click_through=excluded.click_through`,
			w.ID, w.GroupID, w.Mode, w.X, w.Y, w.Width, w.Height, w.MonitorID, w.Opacity, b2i(w.Locked), b2i(w.Collapsed), w.Color, b2i(w.ClickThrough))
		return err
	})
	return w, err
}

// DeleteWindow 删除窗口记录（关闭便签窗口）。
func (s *Store) DeleteWindow(groupID string) error {
	_, err := s.db.Exec(`DELETE FROM windows WHERE group_id=?`, groupID)
	return err
}
