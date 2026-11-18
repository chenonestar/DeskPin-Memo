package toast

import (
	"errors"
	"strings"
	"testing"
	"time"

	"deskpinmemo/internal/scheduler"
)

func sample() scheduler.Notification {
	return scheduler.Notification{Title: "交报告", Body: "<b>", ItemID: "i1", Strong: true,
		Actions: []scheduler.Action{{ID: "done", Label: "完成"}, {ID: "snooze10", Label: "10 分钟后"}}}
}

func TestFullModeHasProtocolAndButtons(t *testing.T) {
	x := XML(sample(), `C:\d\notify.png`, true)
	for _, want := range []string{`activationType="protocol"`, `deskpin://action?item=i1&amp;a=done`, `完成`, `scenario="reminder"`, `&lt;b&gt;`} {
		if !strings.Contains(x, want) {
			t.Errorf("缺少 %q:\n%s", want, x)
		}
	}
	if ID(true) != AppUserModelID {
		t.Fatal(ID(true))
	}
}

func TestLeanModeNeedsNoRegistration(t *testing.T) {
	x := XML(sample(), `C:\d\notify.png`, false)
	for _, bad := range []string{"protocol", "deskpin://", "<actions>", "<action "} {
		if strings.Contains(x, bad) {
			t.Errorf("精简模式不应包含 %q（没有注册协议，按钮点了也回不来）:\n%s", bad, x)
		}
	}
	if !strings.Contains(x, "交报告") || !strings.Contains(x, `scenario="reminder"`) {
		t.Errorf("标题 / 场景应保留:\n%s", x)
	}
	if ID(false) != BorrowedAppUserModelID || !strings.Contains(BorrowedAppUserModelID, "powershell.exe") {
		t.Fatal(ID(false))
	}
}

func TestLogLineHasNoUserContent(t *testing.T) {
	n := scheduler.Notification{Title: "机密：给王总的报价", Body: "底价 88 万", Tag: "item-1", Strong: true}
	ok := LogLine(Kind(n), false, 820*time.Millisecond, nil)
	if ok != "系统通知：强提醒，借用 PowerShell 身份，成功，耗时 820 ms" {
		t.Fatal(ok)
	}
	err := errors.New("exit status 1: 使用“1”个参数调用“LoadXml”时发生异常: $x.LoadXml('<toast><text>机密：给王总的报价</text></toast>')\n第二行 底价 88 万")
	bad := LogLine(Kind(scheduler.Notification{Tag: "pet"}), true, 3*time.Second, err)
	if strings.Contains(bad, "机密") || strings.Contains(bad, "88 万") || !strings.Contains(bad, "失败") || !strings.Contains(bad, "狗狗提醒") || !strings.Contains(bad, "程序自己的身份") || !strings.Contains(bad, "3000 ms") {
		t.Fatal(bad)
	}
	if Kind(scheduler.Notification{Summary: true}) != "提醒汇总" || Kind(scheduler.Notification{Missed: true}) != "错过的提醒" || Kind(scheduler.Notification{}) != "提醒" {
		t.Fatal("kind")
	}
}
