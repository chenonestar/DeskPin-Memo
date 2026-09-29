package store

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// BackupKeep 是自动备份保留份数（FR-602）。
const BackupKeep = 14

// BackupTo 使用 SQLite 在线备份（VACUUM INTO，读取一致快照，不拷贝写入中的文件）。
// 文件名：<tag>-YYYYMMDD-HHmmss.db，tag 为空时使用 data。
func (s *Store) BackupTo(dir, tag string) (string, error) {
	if tag == "" {
		tag = "data"
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	name := fmt.Sprintf("%s-%s.db", tag, s.now().Format("20060102-150405"))
	dst := filepath.Join(dir, name)
	if _, err := os.Stat(dst); err == nil {
		return dst, nil // 同一秒内重复调用
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.db.Exec(`VACUUM INTO ?`, dst); err != nil {
		os.Remove(dst)
		return "", err
	}
	return dst, nil
}

// DailyBackup 每天首次启动备份一次，保留最近 BackupKeep 份。
func (s *Store) DailyBackup(dir string) (created string, err error) {
	today := s.now().Format("20060102")
	matches, _ := filepath.Glob(filepath.Join(dir, "data-"+today+"-*.db"))
	if len(matches) == 0 {
		if created, err = s.BackupTo(dir, "data"); err != nil {
			return "", err
		}
	}
	return created, pruneBackups(dir, BackupKeep)
}

func pruneBackups(dir string, keep int) error {
	all, err := filepath.Glob(filepath.Join(dir, "data-*.db"))
	if err != nil {
		return err
	}
	var files []string
	for _, f := range all {
		if !strings.HasSuffix(f, "-wal.db") {
			files = append(files, f)
		}
	}
	sort.Strings(files) // 文件名含时间戳，字典序即时间序
	for len(files) > keep {
		if err := os.Remove(files[0]); err != nil {
			return err
		}
		files = files[1:]
	}
	return nil
}

// ListBackups 返回备份文件（新→旧）。
func ListBackups(dir string) []string {
	files, _ := filepath.Glob(filepath.Join(dir, "*.db"))
	if files == nil {
		files = []string{} // JSON 序列化为 [] 而不是 null
	}
	sort.Sort(sort.Reverse(sort.StringSlice(files)))
	return files
}

var _ = time.Second
