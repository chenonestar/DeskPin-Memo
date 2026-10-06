// Package regtrace 管理本程序写入当前用户注册表（HKCU）的全部项：
// Toast 通知身份（AppUserModelId）、deskpin:// 协议、开机自启（Run）。
//
// 逻辑与真正的注册表访问分离（Registry 接口），因此可以在任意平台用内存实现测试。
// 所有项都在 HKCU 下，不需要管理员权限。
package regtrace

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
)

// 注册表路径（相对 HKCU）。
const (
	RunKey      = `Software\Microsoft\Windows\CurrentVersion\Run`
	RunValue    = "DeskPinMemo"
	AUMIDKey    = `Software\Classes\AppUserModelId\DeskPinMemo.App`
	ProtocolKey = `Software\Classes\deskpin`
	// 以下两项不是我们主动写的，而是 Windows 在我们弹出通知 / 显示托盘图标后自动生成的记录；
	// 同样位于 HKCU，同样可以安全删除（再次使用时系统会重新生成）。
	NotifSettingsKey = `Software\Microsoft\Windows\CurrentVersion\Notifications\Settings\DeskPinMemo.App`
	TrayIconsKey     = `Control Panel\NotifyIconSettings` // Windows 11：每个托盘程序一个子项，ExecutablePath 指向 exe
)

// Registry 是对 HKCU 的最小抽象。
type Registry interface {
	GetString(path, name string) (string, bool)
	SetString(path, name, value string) error
	SetDWord(path, name string, v uint32) error
	DeleteValue(path, name string) error // 不存在视为成功
	DeleteTree(path string) error        // 连同子项一起删除；不存在视为成功
	KeyExists(path string) bool
	SubKeys(path string) []string // 直接子项名；不存在返回空
}

// Trace 描述一项已写入的注册表内容，用于在设置页如实展示。
type Trace struct {
	Key    string `json:"key"`    // 完整路径，如 HKCU\Software\...
	Desc   string `json:"desc"`   // 用途说明
	Detail string `json:"detail"` // 当前值（如指向的 exe 路径）
}

// Register 写入通知身份和协议（Toast 必需）。只在内容变化时写入；返回是否有改动。
func Register(reg Registry, exePath, iconPath string) (changed bool, err error) {
	set := func(path, name, want string) error {
		if cur, ok := reg.GetString(path, name); ok && cur == want {
			return nil
		}
		changed = true
		return reg.SetString(path, name, want)
	}
	for _, e := range []struct{ path, name, val string }{
		{AUMIDKey, "DisplayName", "桌面备忘钉"},
		{AUMIDKey, "IconUri", iconPath},
		{ProtocolKey, "", "URL:DeskPin Memo"},
		{ProtocolKey, "URL Protocol", ""},
		{ProtocolKey + `\shell\open\command`, "", fmt.Sprintf(`"%s" "%%1"`, exePath)}, // 路径随 exe 位置自动更新
	} {
		if err := set(e.path, e.name, e.val); err != nil {
			return changed, err
		}
	}
	if err := reg.SetDWord(AUMIDKey, "ShowInSettings", 1); err != nil {
		return changed, err
	}
	return changed, nil
}

// RunCommand 生成自启命令行；--autostart 表示静默启动。
func RunCommand(exePath string) string { return fmt.Sprintf(`"%s" --autostart`, exePath) }

// ParseRunCommand 从自启命令行取出 exe 路径（支持带引号和不带引号两种写法）。
func ParseRunCommand(cmd string) string {
	cmd = strings.TrimSpace(cmd)
	if strings.HasPrefix(cmd, `"`) {
		if end := strings.Index(cmd[1:], `"`); end >= 0 {
			return cmd[1 : 1+end]
		}
		return strings.Trim(cmd, `"`)
	}
	if i := strings.Index(cmd, " --"); i >= 0 {
		return cmd[:i]
	}
	return cmd
}

func samePath(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if runtime.GOOS == "windows" || strings.Contains(a, `\`) || strings.Contains(b, `\`) {
		return strings.EqualFold(strings.ReplaceAll(a, "/", `\`), strings.ReplaceAll(b, "/", `\`))
	}
	return a == b
}

// SetAutostart 按用户的明确操作写入 / 删除自启项（强制，不做任何「别人的条目」判断）。
func SetAutostart(reg Registry, exePath string, enable bool) error {
	if !enable {
		return reg.DeleteValue(RunKey, RunValue)
	}
	return reg.SetString(RunKey, RunValue, RunCommand(exePath))
}

// SyncAutostart 在程序启动 / 设置里与自启无关的项被修改时调用，让注册表与设置保持一致，
// 但比 SetAutostart 保守——不会动别的副本（例如已安装版）留下的有效条目：
//
//   - 设置为开：条目缺失、已指向本 exe、或指向的文件已不存在（目录被移动/删除）时更新为本 exe；
//     指向另一个仍然存在的 exe 时保持不动。
//   - 设置为关：只删除指向本 exe 或已失效的条目。
//
// 返回对所做改动的说明（无改动为空串），便于记日志。exists 用于判断文件是否还在。
func SyncAutostart(reg Registry, exePath string, enabled bool, exists func(path string) bool) (string, error) {
	cur, has := reg.GetString(RunKey, RunValue)
	target := ""
	if has {
		target = ParseRunCommand(cur)
	}
	ours := has && samePath(target, exePath)
	stale := has && !ours && !exists(target)
	switch {
	case enabled && ours && cur == RunCommand(exePath):
		return "", nil
	case enabled && (!has || ours || stale):
		if err := reg.SetString(RunKey, RunValue, RunCommand(exePath)); err != nil {
			return "", err
		}
		switch {
		case !has:
			return "已写入开机自启项", nil
		case stale:
			return fmt.Sprintf("开机自启原指向的位置已失效（%s），已更新为当前位置", target), nil
		default:
			return "已更新开机自启命令行", nil
		}
	case !enabled && has && (ours || stale):
		if err := reg.DeleteValue(RunKey, RunValue); err != nil {
			return "", err
		}
		return "已移除开机自启项", nil
	}
	return "", nil
}

// trayIconEntries 返回 Windows 11 为本 exe 生成的托盘图标记录（NotifyIconSettings\<编号>）。
func trayIconEntries(reg Registry, exePath string) []string {
	var out []string
	for _, sub := range reg.SubKeys(TrayIconsKey) {
		path := TrayIconsKey + `\` + sub
		if p, ok := reg.GetString(path, "ExecutablePath"); ok && exePath != "" && samePath(p, exePath) {
			out = append(out, path)
		}
	}
	return out
}

// Traces 列出目前实际存在的注册表项（含 Windows 自动生成的、与本程序相关的记录）。
func Traces(reg Registry, exePath string) []Trace {
	var out []Trace
	if v, ok := reg.GetString(RunKey, RunValue); ok {
		out = append(out, Trace{Key: `HKCU\` + RunKey + `\` + RunValue, Desc: "开机自启", Detail: v})
	}
	if reg.KeyExists(AUMIDKey) {
		icon, _ := reg.GetString(AUMIDKey, "IconUri")
		out = append(out, Trace{Key: `HKCU\` + AUMIDKey, Desc: "通知身份（没有它 Win10 不显示 Toast 通知）", Detail: icon})
	}
	if reg.KeyExists(ProtocolKey) {
		cmd, _ := reg.GetString(ProtocolKey+`\shell\open\command`, "")
		out = append(out, Trace{Key: `HKCU\` + ProtocolKey, Desc: "deskpin:// 协议（通知上的「完成 / 稍后」按钮回调）", Detail: cmd})
	}
	if reg.KeyExists(NotifSettingsKey) {
		out = append(out, Trace{Key: `HKCU\` + NotifSettingsKey, Desc: "通知设置记录（Windows 在首次弹出通知后自动生成）"})
	}
	for _, p := range trayIconEntries(reg, exePath) {
		out = append(out, Trace{Key: `HKCU\` + p, Desc: "托盘图标记录（Windows 11 自动生成）", Detail: exePath})
	}
	return out
}

// Clear 删除全部注册表项，返回被删除的项。程序下次启动时会重新写入通知身份和协议（Toast 需要）。
func Clear(reg Registry, exePath string) ([]string, error) {
	var removed []string
	for _, t := range Traces(reg, exePath) {
		removed = append(removed, t.Key)
	}
	// 先收集托盘记录再删：删除后无法再按 ExecutablePath 匹配
	tray := trayIconEntries(reg, exePath)
	if err := reg.DeleteValue(RunKey, RunValue); err != nil {
		return removed, err
	}
	for _, key := range append([]string{AUMIDKey, ProtocolKey, NotifSettingsKey}, tray...) {
		if err := reg.DeleteTree(key); err != nil {
			return removed, err
		}
	}
	return removed, nil
}
