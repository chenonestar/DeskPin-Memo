package store

import (
	"database/sql"
)

// transformSensitive 在单个事务内批量转换所有敏感字段（6.3「开启与关闭」）。
func (s *Store) transformSensitive(tx *sql.Tx, f func(string) (string, error)) error {
	for table, cols := range sensitive {
		for _, col := range cols {
			rows, err := tx.Query(`SELECT id, ` + col + ` FROM ` + table)
			if err != nil {
				return err
			}
			type kv struct{ id, v string }
			var list []kv
			for rows.Next() {
				var x kv
				if err := rows.Scan(&x.id, &x.v); err != nil {
					rows.Close()
					return err
				}
				list = append(list, x)
			}
			rows.Close()
			for _, x := range list {
				nv, err := f(x.v)
				if err != nil {
					return err
				}
				if _, err := tx.Exec(`UPDATE `+table+` SET `+col+`=? WHERE id=?`, nv, x.id); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// EnableEncryption 用 c 加密全部现有数据并停用 FTS（调用前应先备份）。
func (s *Store) EnableEncryption(c Codec) error {
	err := s.Tx(func(tx *sql.Tx) error {
		if err := s.transformSensitive(tx, c.Enc); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM items_fts`); err != nil {
			return err
		}
		_, err := tx.Exec(`INSERT INTO meta(key,value) VALUES('encrypted','1') ON CONFLICT(key) DO UPDATE SET value='1'`)
		return err
	})
	if err != nil {
		return err
	}
	s.SetCodec(c)
	s.encrypted.Store(true)
	return s.scrub()
}

// scrub 重写数据库文件并截断 WAL，清除空闲页里残留的明文。
func (s *Store) scrub() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.db.Exec(`VACUUM`); err != nil {
		return err
	}
	_, err := s.db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`)
	return err
}

// DisableEncryption 解密全部数据并重建 FTS；需要已解锁。
func (s *Store) DisableEncryption() error {
	c := s.curCodec()
	if c == nil {
		return errLocked
	}
	err := s.Tx(func(tx *sql.Tx) error {
		if err := s.transformSensitive(tx, c.Dec); err != nil {
			return err
		}
		_, err := tx.Exec(`INSERT INTO meta(key,value) VALUES('encrypted','0') ON CONFLICT(key) DO UPDATE SET value='0'`)
		return err
	})
	if err != nil {
		return err
	}
	s.encrypted.Store(false)
	s.SetCodec(nil)
	if err := s.RebuildFTS(); err != nil {
		return err
	}
	return s.scrub()
}
