//go:build windows

package winsys

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	u16 "unicode/utf16"

	"deskpinmemo/internal/scheduler"
	"deskpinmemo/internal/toast"
)

// AppUserModelID / ProtocolScheme 见 internal/toast。
const (
	AppUserModelID = toast.AppUserModelID
	ProtocolScheme = toast.ProtocolScheme
)

const createNoWindow = 0x08000000

// PrepareIcon 把通知图标写到数据目录并返回路径（Toast 的 appLogoOverride 和通知身份的 IconUri 共用）。
// 只写程序自己的数据目录，不碰注册表。
func PrepareIcon(dataDir string) string {
	iconPath := filepath.Join(dataDir, "icons", "notify.png")
	if b, err := iconFS.ReadFile("icons/notify.png"); err == nil {
		_ = os.MkdirAll(filepath.Dir(iconPath), 0o755)
		_ = os.WriteFile(iconPath, b, 0o644)
	}
	return iconPath
}

// ---- Toast ----

// Toaster 通过 WinRT ToastNotificationManager 发送系统 Toast（FR-303/304）。
//
// Full 为真：使用已登记的通知身份 + deskpin:// 协议，通知带「完成 / 稍后」按钮；
// 为假（绿色版默认）：借用系统自带的通知身份，不依赖任何注册表项，通知不带按钮。
type Toaster struct {
	IconPath string
	full     atomic.Bool
}

// SetFull 切换完整 / 精简模式（用户在设置里启用或关闭「系统集成」时）。
func (t *Toaster) SetFull(on bool) { t.full.Store(on) }

// Notify 实现 scheduler.Notifier。完整模式下按钮用协议激活，点击后由系统启动 DeskPinMemo.exe deskpin://…，
// 单实例机制会把参数转交给已运行的实例。
func (t *Toaster) Notify(n scheduler.Notification) error {
	return runPowerShell(toastScript(n, t.IconPath, t.full.Load()))
}

// RemoveToast 撤回已弹出的通知（其他设备已处理该提醒，AC-S04）。
func (t *Toaster) RemoveToast(tag string) error {
	return runPowerShell(fmt.Sprintf(`[Windows.UI.Notifications.ToastNotificationManager]::History.Remove('%s','deskpin','%s')`,
		psQuote(tag), psQuote(toast.ID(t.full.Load()))))
}

func psQuote(s string) string { return strings.ReplaceAll(s, "'", "''") }

// ClearToastHistory 清掉通知中心里已弹出的本程序通知（两种身份都清，尽力而为）。
func ClearToastHistory() {
	for _, id := range []string{toast.AppUserModelID, toast.BorrowedAppUserModelID} {
		_ = runPowerShell(fmt.Sprintf(`[Windows.UI.Notifications.ToastNotificationManager]::History.Clear('%s')`, psQuote(id)))
	}
}

func toastScript(n scheduler.Notification, icon string, full bool) string {
	tag := n.Tag
	if tag == "" {
		tag = "reminder"
	}
	return fmt.Sprintf(`
[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] | Out-Null
[Windows.Data.Xml.Dom.XmlDocument, Windows.Data.Xml.Dom.XmlDocument, ContentType = WindowsRuntime] | Out-Null
$x = New-Object Windows.Data.Xml.Dom.XmlDocument
$x.LoadXml('%s')
$t = New-Object Windows.UI.Notifications.ToastNotification $x
$t.Tag = '%s'
$t.Group = 'deskpin'
[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier('%s').Show($t)
`, psQuote(toast.XML(n, icon, full)), psQuote(tag), psQuote(toast.ID(full)))
}

func runPowerShell(script string) error {
	u := u16.Encode([]rune(script))
	buf := make([]byte, 0, len(u)*2)
	for _, c := range u {
		buf = append(buf, byte(c), byte(c>>8))
	}
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass",
		"-WindowStyle", "Hidden", "-EncodedCommand", base64.StdEncoding.EncodeToString(buf))
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("发送通知失败: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
