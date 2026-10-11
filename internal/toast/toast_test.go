package toast

import (
	"strings"
	"testing"

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
