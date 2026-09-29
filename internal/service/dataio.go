package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"deskpinmemo/internal/crypto"
	"deskpinmemo/internal/store"
)

// envelope 是「加密导出」的外层结构（单独设导出密码，6.3）。
type envelope struct {
	FormatVersion int            `json:"format_version"`
	Encrypted     bool           `json:"encrypted"`
	Data          crypto.Wrapped `json:"data"`
}

// ExportJSON 导出完整数据（可恢复）。password 非空则加密导出；空则明文（调用前 UI 需提示风险）。
func (s *Service) ExportJSON(path, password string) error {
	d, err := s.st.Export()
	if err != nil {
		return err
	}
	raw, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}
	if password != "" {
		w, err := crypto.Seal(raw, password)
		if err != nil {
			return err
		}
		if raw, err = json.MarshalIndent(envelope{FormatVersion: store.ExportFormatVersion, Encrypted: true, Data: w}, "", "  "); err != nil {
			return err
		}
	}
	return writeAtomic(path, raw)
}

// ExportMarkdown 导出可读清单（FR-603）。
func (s *Service) ExportMarkdown(path string) error {
	md, err := s.st.Markdown(s.loc())
	if err != nil {
		return err
	}
	return writeAtomic(path, []byte(md))
}

// ImportInfo 描述待导入文件（导入前让 UI 决定是否需要密码 / 二次确认）。
type ImportInfo struct {
	Encrypted bool `json:"encrypted"`
	Items     int  `json:"items"`
	Groups    int  `json:"groups"`
}

func (s *Service) readDump(path, password string) (*store.Dump, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var env envelope
	if json.Unmarshal(raw, &env) == nil && env.Encrypted {
		if password == "" {
			return nil, errors.New("该导出文件已加密，请输入导出密码")
		}
		if raw, err = crypto.Open(env.Data, password); err != nil {
			return nil, err
		}
	}
	var d store.Dump
	if err := json.Unmarshal(raw, &d); err != nil {
		return nil, fmt.Errorf("不是有效的导出文件: %w", err)
	}
	if d.FormatVersion != store.ExportFormatVersion || d.Tables == nil {
		return nil, errors.New("不支持的导出格式")
	}
	return &d, nil
}

// InspectImport 读取导出文件概要。
func (s *Service) InspectImport(path, password string) (ImportInfo, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ImportInfo{}, err
	}
	var env envelope
	if json.Unmarshal(raw, &env) == nil && env.Encrypted && password == "" {
		return ImportInfo{Encrypted: true}, nil
	}
	d, err := s.readDump(path, password)
	if err != nil {
		return ImportInfo{}, err
	}
	return ImportInfo{Encrypted: env.Encrypted, Items: len(d.Tables["items"]), Groups: len(d.Tables["groups"])}, nil
}

// ImportJSON 从导出文件恢复；overwrite 为覆盖（不可撤销，UI 必须二次确认），否则合并。
// 覆盖前自动备份当前数据库。
func (s *Service) ImportJSON(path, password string, overwrite bool) (store.ImportStats, error) {
	d, err := s.readDump(path, password)
	if err != nil {
		return store.ImportStats{}, err
	}
	mode := store.ImportMerge
	if overwrite {
		mode = store.ImportOverwrite
		if _, err := s.st.BackupTo(s.BackupDir(), "pre-import"); err != nil {
			return store.ImportStats{}, err
		}
	}
	st, err := s.st.Import(d, mode)
	if err == nil {
		s.changed()
	}
	return st, err
}

// BackupNow 立即备份并返回文件路径。
func (s *Service) BackupNow() (string, error) { return s.st.BackupTo(s.BackupDir(), "data") }

// ListBackups 返回备份文件列表。
func (s *Service) ListBackups() []string { return store.ListBackups(s.BackupDir()) }

func writeAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

var _ = time.Second
