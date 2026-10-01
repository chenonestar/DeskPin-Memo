// Package datadir 实现 FR-605：数据目录可改为自定义目录（如网盘同步目录）。
//
// 目录分两类：
//   - 配置目录（%APPDATA%\DeskPinMemo，固定不变）：日志、WebView 缓存、图标、location.json 指针。
//     这些不应被网盘同步，也不随数据迁移。
//   - 数据目录（默认等于配置目录，可自定义）：data.db 与 backups\。
//
// 启动时先读配置目录下的 location.json 得到数据目录；自定义目录不可用时回退到默认并给出提示。
package datadir

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const pointerFile = "location.json"

type pointer struct {
	DataDir string `json:"dataDir"`
}

// Resolution 是启动时解析数据目录的结果。
type Resolution struct {
	Dir      string // 实际使用的数据目录
	Custom   bool   // 是否为自定义目录
	Fallback string // 非空表示自定义目录不可用，已回退到默认目录（原因）
}

// Resolve 读取指针文件得到数据目录。configDir 同时是默认数据目录。
func Resolve(configDir string) Resolution {
	r := Resolution{Dir: configDir}
	b, err := os.ReadFile(filepath.Join(configDir, pointerFile))
	if err != nil {
		return r
	}
	var p pointer
	if json.Unmarshal(b, &p) != nil || strings.TrimSpace(p.DataDir) == "" {
		return r
	}
	dir := filepath.Clean(p.DataDir)
	if same(dir, configDir) {
		return r
	}
	if err := checkWritable(dir); err != nil {
		r.Fallback = fmt.Sprintf("自定义数据目录 %s 不可用（%v），已临时使用默认目录", dir, err)
		return r
	}
	return Resolution{Dir: dir, Custom: true}
}

// Set 写入指针；target 等于默认目录时删除指针（恢复默认）。原子写入。
func Set(configDir, target string) error {
	path := filepath.Join(configDir, pointerFile)
	if same(target, configDir) {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	b, _ := json.MarshalIndent(pointer{DataDir: filepath.Clean(target)}, "", "  ")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Info 描述一个候选数据目录（改目录前给界面展示）。
type Info struct {
	Path      string `json:"path"`
	Valid     bool   `json:"valid"`
	Error     string `json:"error"`
	HasData   bool   `json:"hasData"` // 目录里已有 data.db
	IsDefault bool   `json:"isDefault"`
	Warning   string `json:"warning"`
}

// Check 校验候选目录：必须是绝对路径、可创建、可写，且不能就是当前目录。
func Check(configDir, current, target string) Info {
	target = strings.TrimSpace(target)
	info := Info{Path: target}
	if target == "" {
		info.Error = "请输入目录路径"
		return info
	}
	if !filepath.IsAbs(target) {
		info.Error = "请使用完整路径（如 D:\\Sync\\DeskPinMemo）"
		return info
	}
	target = filepath.Clean(target)
	info.Path = target
	info.IsDefault = same(target, configDir)
	if same(target, current) {
		info.Error = "这已经是当前使用的数据目录"
		return info
	}
	if st, err := os.Stat(target); err == nil && !st.IsDir() {
		info.Error = "该路径是一个文件，不是目录"
		return info
	}
	if err := checkWritable(target); err != nil {
		info.Error = "无法在该目录写入：" + err.Error()
		return info
	}
	if _, err := os.Stat(filepath.Join(target, "data.db")); err == nil {
		info.HasData = true
	}
	info.Warning = warn(target)
	info.Valid = true
	return info
}

var syncMarkers = []string{"onedrive", "dropbox", "google drive", "googledrive", "icloud", "iclouddrive", "baidunetdisk", "百度网盘", "坚果云", "nutstore", "阿里云盘", "aliyundrive", "box sync"}

func warn(dir string) string {
	if strings.HasPrefix(dir, `\\`) {
		return "这是网络位置：网络中断或延迟可能导致写入失败，建议只在稳定的局域网环境使用。"
	}
	l := strings.ToLower(dir)
	for _, m := range syncMarkers {
		if strings.Contains(l, m) {
			return "看起来是网盘同步目录：请确保同一时间只有一台电脑在运行本程序，且同步工具不要在写入过程中占用 data.db。为降低风险，自定义目录下数据库使用单文件（回滚日志）模式。"
		}
	}
	return ""
}

func checkWritable(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".deskpin-write-test-*")
	if err != nil {
		return err
	}
	name := f.Name()
	f.Close()
	return os.Remove(name)
}

func same(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// CopyFile 复制文件（先写临时文件再改名，避免留下半个文件）。
func CopyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := out.ReadFrom(in); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Sync(); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dst)
}

// ErrNotEmpty 表示目标目录已有数据，需要用户选择如何处理。
var ErrNotEmpty = errors.New("目标目录已有数据")
