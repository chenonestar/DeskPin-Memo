package store

import (
	"database/sql"
	"fmt"
)

// CurrentSchemaVersion 为数据库结构版本（NFR-11）。
const CurrentSchemaVersion = 3

// 时间一律以 UTC 毫秒时间戳（INTEGER）存储，显示时按本地时区转换。
// hlc / device_id / field_hlc / deleted 为 V3 同步预留字段，V1 建表即创建并维护。
var migrations = []string{
	// v1
	`
CREATE TABLE meta (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
CREATE TABLE settings (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL            -- JSON
);
CREATE TABLE groups (
  id         TEXT PRIMARY KEY,
  name       TEXT NOT NULL,
  color      TEXT NOT NULL DEFAULT '',
  sort_order REAL NOT NULL DEFAULT 0,
  is_default INTEGER NOT NULL DEFAULT 0,
  hlc        TEXT NOT NULL,
  device_id  TEXT NOT NULL,
  field_hlc  TEXT NOT NULL DEFAULT '{}',
  deleted    INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX idx_groups_hlc ON groups(hlc);
CREATE TABLE items (
  id           TEXT PRIMARY KEY,
  title        TEXT NOT NULL,
  note         TEXT NOT NULL DEFAULT '',
  group_id     TEXT NOT NULL REFERENCES groups(id),
  priority     INTEGER NOT NULL DEFAULT 1,      -- 0 低 1 中 2 高
  due_at       INTEGER,
  status       TEXT NOT NULL DEFAULT 'todo' CHECK (status IN ('todo','done','deleted')),
  completed_at INTEGER,
  deleted_at   INTEGER,
  sort_order   REAL NOT NULL DEFAULT 0,
  series_id    TEXT NOT NULL DEFAULT '',
  created_at   INTEGER NOT NULL,
  updated_at   INTEGER NOT NULL,
  hlc          TEXT NOT NULL,
  device_id    TEXT NOT NULL,
  field_hlc    TEXT NOT NULL DEFAULT '{}'
);
CREATE INDEX idx_items_group  ON items(group_id, status);
CREATE INDEX idx_items_due    ON items(due_at);
CREATE INDEX idx_items_status ON items(status);
CREATE INDEX idx_items_hlc    ON items(hlc);
CREATE TABLE reminders (
  id             TEXT PRIMARY KEY,
  item_id        TEXT NOT NULL REFERENCES items(id),
  remind_at      INTEGER,                       -- 已计算好的下次触发时间
  offset_minutes INTEGER NOT NULL DEFAULT 0,    -- 截止前 N 分钟；0 表示使用 remind_at 绝对时间
  repeat_rule    TEXT NOT NULL DEFAULT '',      -- iCalendar RRULE
  snooze_until   INTEGER,
  last_fired_at  INTEGER,
  hlc            TEXT NOT NULL,
  device_id      TEXT NOT NULL,
  field_hlc      TEXT NOT NULL DEFAULT '{}',
  deleted        INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX idx_reminders_item ON reminders(item_id);
CREATE INDEX idx_reminders_hlc  ON reminders(hlc);
CREATE TABLE tags (
  id        TEXT PRIMARY KEY,
  name      TEXT NOT NULL,
  hlc       TEXT NOT NULL,
  device_id TEXT NOT NULL,
  field_hlc TEXT NOT NULL DEFAULT '{}',
  deleted   INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX idx_tags_hlc ON tags(hlc);
CREATE TABLE item_tags (
  item_id   TEXT NOT NULL,
  tag_id    TEXT NOT NULL,
  hlc       TEXT NOT NULL,
  device_id TEXT NOT NULL,
  field_hlc TEXT NOT NULL DEFAULT '{}',
  deleted   INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (item_id, tag_id)
);
CREATE TABLE windows (
  id         TEXT PRIMARY KEY,
  group_id   TEXT NOT NULL,
  mode       TEXT NOT NULL DEFAULT 'desktop' CHECK (mode IN ('desktop','top','normal')),
  x          INTEGER NOT NULL DEFAULT 0,
  y          INTEGER NOT NULL DEFAULT 0,
  width      INTEGER NOT NULL DEFAULT 300,
  height     INTEGER NOT NULL DEFAULT 420,
  monitor_id TEXT NOT NULL DEFAULT '',
  opacity    REAL NOT NULL DEFAULT 1,
  locked     INTEGER NOT NULL DEFAULT 0,
  collapsed  INTEGER NOT NULL DEFAULT 0,
  color      TEXT NOT NULL DEFAULT ''
);
-- 全文索引：trigram 分词器支持中文子串检索；由 Go 代码维护（加密开启时停用，避免明文入索引）
CREATE VIRTUAL TABLE items_fts USING fts5(item_id UNINDEXED, title, note, tokenize='trigram');
`,
	// v2：子任务（FR-108）。同步预留字段与其他业务表一致。
	`
CREATE TABLE subtasks (
  id           TEXT PRIMARY KEY,
  item_id      TEXT NOT NULL REFERENCES items(id),
  title        TEXT NOT NULL,
  done         INTEGER NOT NULL DEFAULT 0,
  sort_order   REAL NOT NULL DEFAULT 0,
  completed_at INTEGER,
  created_at   INTEGER NOT NULL,
  hlc          TEXT NOT NULL,
  device_id    TEXT NOT NULL,
  field_hlc    TEXT NOT NULL DEFAULT '{}',
  deleted      INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX idx_subtasks_item ON subtasks(item_id);
CREATE INDEX idx_subtasks_hlc  ON subtasks(hlc);
`,
	// v3：便签窗口鼠标穿透（FR-208）。windows 表为设备相关状态，不含同步字段。
	`ALTER TABLE windows ADD COLUMN click_through INTEGER NOT NULL DEFAULT 0;`,
}

func migrate(db *sql.DB, backup func() error) error {
	var v int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&v); err != nil {
		return err
	}
	if v > CurrentSchemaVersion {
		return fmt.Errorf("数据库版本 %d 高于程序支持的版本 %d，请升级程序", v, CurrentSchemaVersion)
	}
	if v == CurrentSchemaVersion {
		return nil
	}
	if v > 0 && backup != nil { // 迁移前先备份
		if err := backup(); err != nil {
			return fmt.Errorf("迁移前备份失败: %w", err)
		}
	}
	for i := v; i < CurrentSchemaVersion; i++ {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(migrations[i]); err != nil {
			tx.Rollback()
			return fmt.Errorf("迁移到 v%d 失败: %w", i+1, err)
		}
		if _, err := tx.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, i+1)); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}
