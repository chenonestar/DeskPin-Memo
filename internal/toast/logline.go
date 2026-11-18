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

// LogLine 生成一行发送记录：种类、身份、结果、耗时。不写标题和正文——日志是明文，开了加密也不能泄露事项内容。
// 失败时附上 PowerShell 输出的第一行，其中单引号里的内容（脚本参数，含通知 XML）一律打码。
func LogLine(kind string, full bool, d time.Duration, err error) string {
	id := "借用 PowerShell 身份"
	if full {
		id = "程序自己的身份"
	}
	res := "成功"
	if err != nil {
		msg := strings.TrimSpace(err.Error())
		if i := strings.IndexByte(msg, '\n'); i >= 0 {
			msg = msg[:i]
		}
		msg = quoted.ReplaceAllString(msg, "'…'")
		if r := []rune(msg); len(r) > 160 {
			msg = string(r[:160]) + "…"
		}
		res = "失败（" + msg + "）"
	}
	return fmt.Sprintf("系统通知：%s，%s，%s，耗时 %d ms", kind, id, res, d.Milliseconds())
}
