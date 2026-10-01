package service

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"deskpinmemo/internal/datadir"
)

// DataDirStatus 是设置页「数据目录」区域需要的信息。
type DataDirStatus struct {
	Dir       string `json:"dir"`
	ConfigDir string `json:"configDir"`
	Custom    bool   `json:"custom"`
	Portable  bool   `json:"portable"`
	Fallback  string `json:"fallback"`
}

// DataDirStatus 返回当前数据目录状态。
func (s *Service) DataDirStatus() DataDirStatus {
	return DataDirStatus{Dir: s.dataDir, ConfigDir: s.configDir, Custom: filepath.Clean(s.dataDir) != filepath.Clean(s.configDir),
		Portable: s.portable, Fallback: s.fallback}
}

// InspectDataDir 校验候选目录，供界面在确认前展示（是否已有数据、网盘提示等）。
func (s *Service) InspectDataDir(target string) datadir.Info {
	if s.portable {
		return datadir.Info{Path: target, Error: "绿色版的数据固定保存在程序目录下的 data 文件夹"}
	}
	info := datadir.Check(s.configDir, s.dataDir, target)
	if info.Valid && within(info.Path, s.dataDir) {
		info.Valid, info.Error = false, "不能选择当前数据目录里面的子文件夹"
	}
	return info
}

// 切换数据目录的处理方式。
const (
	DataDirCopy    = "copy"    // 把当前数据复制到空目录
	DataDirUse     = "use"     // 直接使用目标目录里已有的数据（当前数据不会带过去）
	DataDirReplace = "replace" // 用当前数据替换目标目录里已有的数据（旧文件改名保留）
)

// DataDirChange 是切换结果。
type DataDirChange struct {
	NewDir          string `json:"newDir"`
	OldDir          string `json:"oldDir"`
	RestartRequired bool   `json:"restartRequired"`
	Kept            string `json:"kept"` // 被改名保留的旧文件（replace 模式）
}

// ChangeDataDir 切换数据目录。当前数据不会被删除（复制而非移动），切换在重启后生效：
// 数据库此刻仍被打开，因此只写入指针文件，由外壳负责重启程序。
func (s *Service) ChangeDataDir(target, mode string) (DataDirChange, error) {
	info := s.InspectDataDir(target)
	if !info.Valid {
		return DataDirChange{}, errors.New(info.Error)
	}
	res := DataDirChange{NewDir: info.Path, OldDir: s.dataDir, RestartRequired: true}
	dst := filepath.Join(info.Path, "data.db")
	switch mode {
	case DataDirUse:
		if !info.HasData {
			return res, errors.New("该目录里没有 data.db，无法直接使用")
		}
	case DataDirCopy:
		if info.HasData {
			return res, datadir.ErrNotEmpty
		}
	case DataDirReplace:
		if info.HasData {
			kept := fmt.Sprintf("%s.bak-%s", dst, s.now().Format("20060102-150405"))
			if err := os.Rename(dst, kept); err != nil {
				return res, fmt.Errorf("无法保留目标目录中的旧数据库: %w", err)
			}
			_ = os.Remove(dst + "-wal")
			_ = os.Remove(dst + "-shm")
			res.Kept = kept
		}
	default:
		return res, fmt.Errorf("未知的处理方式 %q", mode)
	}
	if mode != DataDirUse {
		if err := s.st.CopyTo(dst); err != nil {
			return res, fmt.Errorf("复制数据库失败: %w", err)
		}
		if err := s.copyBackups(filepath.Join(info.Path, "backups")); err != nil {
			return res, fmt.Errorf("复制备份失败: %w", err)
		}
	}
	if err := datadir.Set(s.configDir, info.Path); err != nil {
		return res, fmt.Errorf("保存数据目录设置失败: %w", err)
	}
	return res, nil
}

func (s *Service) copyBackups(dstDir string) error {
	files, _ := filepath.Glob(filepath.Join(s.BackupDir(), "*.db"))
	if len(files) == 0 {
		return nil
	}
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return err
	}
	for _, f := range files {
		dst := filepath.Join(dstDir, filepath.Base(f))
		if _, err := os.Stat(dst); err == nil {
			continue
		}
		if err := datadir.CopyFile(f, dst); err != nil {
			return err
		}
	}
	return nil
}

func within(child, parent string) bool {
	rel, err := filepath.Rel(filepath.Clean(parent), filepath.Clean(child))
	return err == nil && rel != "." && !strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel)
}

var _ = time.Second
