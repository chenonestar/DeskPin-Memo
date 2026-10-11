// Package toast 生成 Windows Toast 通知的 XML（与平台无关，便于测试）。
//
// 两种模式：
//   - 完整模式（已注册通知身份和 deskpin:// 协议）：点击通知和「完成 / 稍后」按钮通过协议回到程序；
//   - 精简模式（绿色版默认，不写注册表）：借用系统已有的通知身份显示，没有协议回调，所以不带按钮、
//     点击通知也不触发任何动作；需要处理的提醒由程序窗口内的强提醒承担。
package toast

import (
	"fmt"
	"html"
	"net/url"
	"path/filepath"
	"strings"

	"deskpinmemo/internal/scheduler"
)

// AppUserModelID 是完整模式使用的通知身份（需要写入 HKCU，见 regtrace）。
const AppUserModelID = "DeskPinMemo.App"

// BorrowedAppUserModelID 是 Windows 自带 PowerShell 的通知身份。精简模式借用它发 Toast，
// 不需要在注册表里注册任何东西；代价是通知来源显示为「Windows PowerShell」。
const BorrowedAppUserModelID = `{1AC14E77-02E7-4E5D-B744-2EB1AE5198B7}\WindowsPowerShell\v1.0\powershell.exe`

// ProtocolScheme 是通知按钮回调用的自定义协议：deskpin://action?item=ID&a=done
const ProtocolScheme = "deskpin"

// ID 返回对应模式下使用的通知身份。
func ID(full bool) string {
	if full {
		return AppUserModelID
	}
	return BorrowedAppUserModelID
}

// XML 生成 Toast 的 XML。full 为 false 时不含协议激活和按钮。
func XML(n scheduler.Notification, icon string, full bool) string {
	var b strings.Builder
	scenario := ""
	if n.Strong {
		scenario = ` scenario="reminder"`
	}
	if full {
		launch := ProtocolScheme + "://open"
		if n.ItemID != "" {
			launch += "?item=" + url.QueryEscape(n.ItemID)
		}
		fmt.Fprintf(&b, `<toast launch="%s" activationType="protocol"%s><visual><binding template="ToastGeneric">`, html.EscapeString(launch), scenario)
	} else {
		fmt.Fprintf(&b, `<toast%s><visual><binding template="ToastGeneric">`, scenario)
	}
	if icon != "" {
		fmt.Fprintf(&b, `<image placement="appLogoOverride" src="%s"/>`, html.EscapeString("file:///"+filepath.ToSlash(icon)))
	}
	fmt.Fprintf(&b, `<text>%s</text>`, html.EscapeString(n.Title))
	if n.Body != "" {
		fmt.Fprintf(&b, `<text>%s</text>`, html.EscapeString(n.Body))
	}
	b.WriteString(`</binding></visual>`)
	if full && len(n.Actions) > 0 && n.ItemID != "" {
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
