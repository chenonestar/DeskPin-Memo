//go:build windows

package winsys

import (
	"encoding/base64"
	"fmt"
	"html"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	u16 "unicode/utf16"

	"deskpinmemo/internal/regtrace"
	"deskpinmemo/internal/scheduler"
)

// AppUserModelID 用于 Toast 归属（1.5 通知图标与 AUMID）。
const AppUserModelID = "DeskPinMemo.App"

// ProtocolScheme 是通知按钮回调用的自定义协议：deskpin://action?item=ID&a=done
const ProtocolScheme = "deskpin"

const createNoWindow = 0x08000000

// RegisterIdentity 注册 AUMID（显示名与图标）和 deskpin:// 协议，均写入 HKCU，无需管理员权限。
// 没有 AUMID 注册时 Win10 会丢弃未打包应用的 Toast。内容未变化时不重复写入；
// 程序目录被移动后，下次启动会自动把协议命令更新到新位置。
func RegisterIdentity(exePath, dataDir string) error {
	iconPath := filepath.Join(dataDir, "icons", "notify.png")
	if b, err := iconFS.ReadFile("icons/notify.png"); err == nil {
		_ = os.MkdirAll(filepath.Dir(iconPath), 0o755)
		_ = os.WriteFile(iconPath, b, 0o644)
	}
	_, err := regtrace.Register(winReg{}, exePath, iconPath)
	return err
}

// ---- Toast ----

// Toaster 通过 WinRT ToastNotificationManager 发送系统 Toast（FR-303/304）。
type Toaster struct {
	IconPath string
}

// Notify 实现 scheduler.Notifier。按钮用协议激活，点击后由系统启动 DeskPinMemo.exe deskpin://…，
// 单实例机制会把参数转交给已运行的实例。
func (t *Toaster) Notify(n scheduler.Notification) error {
	return runPowerShell(toastScript(n, t.IconPath))
}

// RemoveToast 撤回已弹出的通知（其他设备已处理该提醒，AC-S04）。
func (t *Toaster) RemoveToast(tag string) error {
	return runPowerShell(fmt.Sprintf(`[Windows.UI.Notifications.ToastNotificationManager]::History.Remove('%s','deskpin','%s')`,
		psQuote(tag), AppUserModelID))
}

func psQuote(s string) string { return strings.ReplaceAll(s, "'", "''") }

func toastXML(n scheduler.Notification, icon string) string {
	var b strings.Builder
	launch := ProtocolScheme + "://open"
	if n.ItemID != "" {
		launch += "?item=" + url.QueryEscape(n.ItemID)
	}
	scenario := ""
	if n.Strong {
		scenario = ` scenario="reminder"`
	}
	fmt.Fprintf(&b, `<toast launch="%s" activationType="protocol"%s><visual><binding template="ToastGeneric">`, html.EscapeString(launch), scenario)
	if icon != "" {
		fmt.Fprintf(&b, `<image placement="appLogoOverride" src="%s"/>`, html.EscapeString("file:///"+filepath.ToSlash(icon)))
	}
	fmt.Fprintf(&b, `<text>%s</text>`, html.EscapeString(n.Title))
	if n.Body != "" {
		fmt.Fprintf(&b, `<text>%s</text>`, html.EscapeString(n.Body))
	}
	b.WriteString(`</binding></visual>`)
	if len(n.Actions) > 0 && n.ItemID != "" {
		b.WriteString(`<actions>`)
		for _, a := range n.Actions {
			args := fmt.Sprintf("%s://action?item=%s&a=%s", ProtocolScheme, url.QueryEscape(n.ItemID), url.QueryEscape(a.ID))
			fmt.Fprintf(&b, `<action content="%s" arguments="%s" activationType="protocol"/>`, html.EscapeString(a.Label), html.EscapeString(args))
		}
		b.WriteString(`</actions>`)
	}
	b.WriteString(`<audio src="ms-winsoundevent:Notification.Reminder"/></toast>`)
	return b.String()
}

func toastScript(n scheduler.Notification, icon string) string {
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
`, psQuote(toastXML(n, icon)), psQuote(tag), AppUserModelID)
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

// ---- 开机自启（FR-503）----

// SetAutostart 按用户的明确操作写入 / 删除 HKCU\...\Run，无需管理员权限。启动参数 --autostart 表示静默启动。
func SetAutostart(enable bool, exePath string) error {
	return regtrace.SetAutostart(winReg{}, exePath, enable)
}

// AutostartEnabled 返回当前是否已配置自启。
func AutostartEnabled() bool {
	_, ok := winReg{}.GetString(regtrace.RunKey, regtrace.RunValue)
	return ok
}
