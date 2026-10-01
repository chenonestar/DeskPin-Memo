// Package store 封装 SQLite 存储：事务写入、HLC/设备字段维护、FTS5 搜索、备份。
package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"deskpinmemo/internal/hlc"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

// Codec 是字段级加解密接口（title/note/tag 名/分组名）。
type Codec interface {
	Enc(string) (string, error)
	Dec(string) (string, error)
}

const lockedText = "🔒 已加密"

// Store 是数据库访问层。
type Store struct {
	db       *sql.DB
	path     string
	deviceID string
	clock    *hlc.Clock
	now      func() time.Time

	mu        sync.Mutex // 序列化写事务（SQLite 单写者）
	codec     atomic.Pointer[codecBox]
	encrypted atomic.Bool // 数据库处于加密模式（无论是否已解锁）
}

type codecBox struct{ c Codec }

// Options 为 Open 的可选项。
type Options struct {
	Now func() time.Time // 测试用
	// JournalMode 为 SQLite 日志模式，默认 WAL。数据库放在网盘同步目录时用 DELETE
	// （单文件，没有 -wal/-shm 伴生文件，不易被同步工具拷到不一致的状态）。
	JournalMode string
}

// Open 打开（必要时创建并迁移）数据库。
func Open(path string, opts ...Options) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	jm := "WAL"
	if len(opts) > 0 && (opts[0].JournalMode == "DELETE" || opts[0].JournalMode == "WAL") {
		jm = opts[0].JournalMode
	}
	dsn := "file:" + filepath.ToSlash(path) +
		"?_pragma=journal_mode(" + jm + ")&_pragma=synchronous(FULL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=secure_delete(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // 单连接：写串行，读也简单
	s := &Store{db: db, path: path, now: time.Now}
	if len(opts) > 0 && opts[0].Now != nil {
		s.now = opts[0].Now
	}
	if err := migrate(db, s.backupBeforeMigrate); err != nil {
		db.Close()
		return nil, err
	}
	if err := s.initMeta(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) backupBeforeMigrate() error {
	_, err := s.BackupTo(filepath.Join(filepath.Dir(s.path), "backups"), "premigrate")
	return err
}

// Close 关闭数据库。
func (s *Store) Close() error { return s.db.Close() }

// Path 返回数据库文件路径。
func (s *Store) Path() string { return s.path }

// DeviceID 返回本机设备 id（UUIDv7，首次启动生成且永不改变）。
func (s *Store) DeviceID() string { return s.deviceID }

// Clock 返回 HLC 时钟。
func (s *Store) Clock() *hlc.Clock { return s.clock }

func (s *Store) nowMs() int64 { return s.now().UnixMilli() }

func (s *Store) initMeta() error {
	var id string
	err := s.db.QueryRow(`SELECT value FROM meta WHERE key='device_id'`).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		u, uerr := uuid.NewV7()
		if uerr != nil {
			return uerr
		}
		id = u.String()
		if _, err = s.db.Exec(`INSERT INTO meta(key,value) VALUES('device_id',?),('schema_version',?)`,
			id, fmt.Sprint(CurrentSchemaVersion)); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else {
		_, _ = s.db.Exec(`INSERT INTO meta(key,value) VALUES('schema_version',?)
			ON CONFLICT(key) DO UPDATE SET value=excluded.value`, fmt.Sprint(CurrentSchemaVersion))
	}
	s.deviceID = id
	if s.now != nil && s.clock == nil {
		s.clock = hlc.NewWithNow(id, s.now)
	}
	// 用库内最大 HLC 播种，防止系统时间回拨后倒退
	var maxH sql.NullString
	_ = s.db.QueryRow(`SELECT MAX(h) FROM (
		SELECT MAX(hlc) h FROM items UNION ALL SELECT MAX(hlc) FROM reminders
		UNION ALL SELECT MAX(hlc) FROM groups UNION ALL SELECT MAX(hlc) FROM tags)`).Scan(&maxH)
	if maxH.Valid {
		s.clock.Seed(maxH.String)
	}
	var enc string
	if err := s.db.QueryRow(`SELECT value FROM meta WHERE key='encrypted'`).Scan(&enc); err == nil && enc == "1" {
		s.encrypted.Store(true)
	}
	return s.ensureInbox()
}

func (s *Store) ensureInbox() error {
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM groups WHERE is_default=1`).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	_, err := s.CreateGroupOpts(InboxName, "#F5C542", true)
	return err
}

// ---- meta / settings ----

// GetMeta 读取 meta 键。
func (s *Store) GetMeta(key string) (string, bool) {
	var v string
	if err := s.db.QueryRow(`SELECT value FROM meta WHERE key=?`, key).Scan(&v); err != nil {
		return "", false
	}
	return v, true
}

// SetMeta 写入 meta 键。
func (s *Store) SetMeta(key, value string) error {
	_, err := s.db.Exec(`INSERT INTO meta(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}

// GetSetting 读取设置（JSON）到 out；不存在返回 false。
func (s *Store) GetSetting(key string, out any) (bool, error) {
	var v string
	err := s.db.QueryRow(`SELECT value FROM settings WHERE key=?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, json.Unmarshal([]byte(v), out)
}

// SetSetting 写入设置。
func (s *Store) SetSetting(key string, val any) error {
	b, err := json.Marshal(val)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, string(b))
	return err
}

// AllSettings 返回全部设置的原始 JSON。
func (s *Store) AllSettings() (map[string]json.RawMessage, error) {
	rows, err := s.db.Query(`SELECT key,value FROM settings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := map[string]json.RawMessage{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		m[k] = json.RawMessage(v)
	}
	return m, rows.Err()
}

// ---- 加密编解码 ----

// SetCodec 设置字段编解码器（nil = 明文/未解锁）。
func (s *Store) SetCodec(c Codec) { s.codec.Store(&codecBox{c: c}) }

// SetEncrypted 标记数据库处于加密模式。
func (s *Store) SetEncrypted(on bool) {
	s.encrypted.Store(on)
	v := "0"
	if on {
		v = "1"
	}
	_ = s.SetMeta("encrypted", v)
}

// Encrypted 返回是否处于加密模式。
func (s *Store) Encrypted() bool { return s.encrypted.Load() }

// Locked 返回加密模式下尚未解锁。
func (s *Store) Locked() bool { return s.encrypted.Load() && s.curCodec() == nil }

func (s *Store) curCodec() Codec {
	if b := s.codec.Load(); b != nil {
		return b.c
	}
	return nil
}

func (s *Store) enc(v string) (string, error) {
	if !s.encrypted.Load() {
		return v, nil
	}
	c := s.curCodec()
	if c == nil {
		return "", errors.New("数据已加密且尚未解锁")
	}
	return c.Enc(v)
}

func (s *Store) dec(v string) (string, bool) {
	c := s.curCodec()
	if c == nil {
		if s.encrypted.Load() {
			return lockedText, true
		}
		return v, false
	}
	out, err := c.Dec(v)
	if err != nil {
		return lockedText, true
	}
	return out, false
}

// ---- 事务与 HLC 辅助 ----

type execer interface {
	Exec(query string, args ...any) (sql.Result, error)
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}

// Tx 在串行化的事务中执行 fn；出错回滚。
func (s *Store) Tx(fn func(tx *sql.Tx) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

// touch 更新记录的 hlc / device_id / field_hlc：只更新被修改字段的 HLC。
func (s *Store) touch(tx execer, table, id string, h string, fields ...string) error {
	var cur string
	if err := tx.QueryRow(`SELECT field_hlc FROM `+table+` WHERE id=?`, id).Scan(&cur); err != nil {
		return err
	}
	f := hlc.ParseFieldHLC(cur).Touch(h, fields...)
	_, err := tx.Exec(`UPDATE `+table+` SET hlc=?, device_id=?, field_hlc=? WHERE id=?`, h, s.deviceID, f.JSON(), id)
	return err
}

func newID() string {
	u, err := uuid.NewV7()
	if err != nil {
		return uuid.NewString()
	}
	return u.String()
}

var errLocked = errors.New("数据已加密且尚未解锁")
