package toast

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"deskpinmemo/internal/scheduler"
)

// Kind 是一条通知的种类（只用于日志，不含标题 / 正文等用户内容）。
func Kind(n scheduler.Notification) string {
	switch {
	case n.Tag == "pet":
		return "狗狗提醒"
	case n.Tag == "test":
		return "测试通知"
	case n.Summary:
		return "提醒汇总"
	case n.Missed:
		return "错过的提醒"
	case n.Strong:
		return "强提醒"
	}
	return "提醒"
}

var quoted = regexp.MustCompile(`'[^']*'`)

// 发送方式（写进日志）。
const (
	MethodNative     = "直接调用"       // 程序直接调用 Windows 通知接口（不启动其他进程）
	MethodPowerShell = "PowerShell" // 启动 PowerShell 发送（旧方式 / 兜底）
)

// Mask 把错误变成可以写进明文日志的一行：只取第一行，单引号里的内容（脚本参数，含通知 XML）打码，最长 160 字。
func Mask(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.TrimSpace(err.Error())
	if i := strings.IndexByte(msg, '\n'); i >= 0 {
		msg = msg[:i]
	}
	msg = quoted.ReplaceAllString(msg, "'…'")
	if r := []rune(msg); len(r) > 160 {
		msg = string(r[:160]) + "…"
	}
	return msg
}

// LogLine 生成一行发送记录：种类、身份、发送方式、结果、耗时。不写标题和正文——日志是明文，开了加密也不能泄露事项内容。
func LogLine(kind string, full bool, method string, d time.Duration, err error) string {
	id := "借用 PowerShell 身份"
	if full {
		id = "程序自己的身份"
	}
	res := "成功"
	if err != nil {
		res = "失败（" + Mask(err) + "）"
	}
	return fmt.Sprintf("系统通知：%s，%s，%s，%s，耗时 %d ms", kind, id, method, res, d.Milliseconds())
}
