package app

import (
	"path/filepath"
	"testing"

	"deskpinmemo/internal/scheduler"
	"deskpinmemo/internal/service"
	"deskpinmemo/internal/store"
)

func newApp(t *testing.T) (*App, *NopShell) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	svc, err := service.New(st, dir)
	if err != nil {
		t.Fatal(err)
	}
	sh := &NopShell{R: Rect{X: 100, Y: 100, W: 300, H: 420}}
	a := New(svc, sh, "test")
	return a, sh
}

func TestClickThroughPersistsAndApplies(t *testing.T) { // FR-208
	a, sh := newApp(t)
	if sh.ClickThrough {
		t.Fatal("默认关闭")
	}
	ws, err := a.SetClickThrough(true)
	if err != nil || !ws.ClickThrough || !sh.ClickThrough {
		t.Fatalf("开启后应写入状态并应用到窗口: %+v %v shell=%v", ws, err, sh.ClickThrough)
	}
	// 重启：重新应用窗口状态时恢复穿透
	sh.ClickThrough = false
	if _, err := a.ApplyWindow(); err != nil {
		t.Fatal(err)
	}
	if !sh.ClickThrough || !a.ClickThroughEnabled() {
		t.Fatal("重启后应恢复鼠标穿透")
	}
	ws, _ = a.ToggleClickThrough() // 托盘菜单
	if ws.ClickThrough || sh.ClickThrough || a.ClickThroughEnabled() {
		t.Fatal("切换后应关闭")
	}
}

func TestClickThroughSuspendedDuringOverlays(t *testing.T) {
	a, sh := newApp(t)
	a.SetClickThrough(true)
	for name, open := range map[string]func(){"快速输入": a.OpenQuick, "设置": a.OpenSettings, "概览": a.OpenOverview} {
		open()
		if sh.ClickThrough {
			t.Fatalf("%s 打开期间必须能用鼠标操作，穿透应暂停", name)
		}
		a.closeOverlay()
		if !sh.ClickThrough {
			t.Fatalf("%s 关闭后应恢复穿透", name)
		}
	}
	// 关闭后 App 持有的「想要穿透」不丢
	if !a.ClickThroughEnabled() {
		t.Fatal("保存的状态不应被叠加界面改掉")
	}
}

func TestClickThroughSuspendedDuringStrongAlerts(t *testing.T) {
	a, sh := newApp(t)
	a.SetClickThrough(true)
	n := scheduler.Notification{Title: "x", Strong: true}
	a.ShowStrongAlert(n)
	a.ShowStrongAlert(n)
	if sh.ClickThrough {
		t.Fatal("强提醒必须手动处理，期间应暂停穿透")
	}
	a.StrongAlertDone()
	if sh.ClickThrough {
		t.Fatal("还有未处理的强提醒时不应恢复")
	}
	a.StrongAlertDone()
	if !sh.ClickThrough {
		t.Fatal("全部处理完后恢复穿透")
	}
	a.StrongAlertDone() // 多余的调用不应让计数变负
	a.ShowStrongAlert(n)
	if sh.ClickThrough {
		t.Fatal()
	}
	a.StrongAlertDone()
	if !sh.ClickThrough {
		t.Fatal()
	}
}

func TestClickThroughIsPerWindowGroup(t *testing.T) {
	a, sh := newApp(t)
	a.SetClickThrough(true)
	g, _ := a.CreateGroup("另一个", "")
	if _, err := a.ShowGroup(g.ID); err != nil {
		t.Fatal(err)
	}
	if sh.ClickThrough || a.ClickThroughEnabled() {
		t.Fatal("每个分组的便签单独保存穿透状态")
	}
	inbox, _ := a.svc.Store().InboxID()
	a.ShowGroup(inbox)
	if !sh.ClickThrough {
		t.Fatal("切回原分组恢复")
	}
}

func TestClickThroughEmitsWindowState(t *testing.T) {
	a, _ := newApp(t)
	var got []any
	a.Emit = func(ev string, d any) {
		if ev == "window:state" {
			got = append(got, d)
		}
	}
	a.SetClickThrough(true)
	if len(got) != 1 {
		t.Fatal("托盘切换后需要通知前端刷新")
	}
	if ws := got[0].(WindowState); !ws.ClickThrough {
		t.Fatalf("%+v", ws)
	}
}

func TestClearRegistryTraces(t *testing.T) { // 绿色版：删除文件夹前清除注册表痕迹
	a, sh := newApp(t)
	if got := a.RegistryTraces(); len(got) != 3 {
		t.Fatalf("应列出自启、通知身份、协议 3 项: %+v", got)
	}
	s := a.GetSettings()
	s.Autostart = true
	a.SaveSettings(s)
	removed, err := a.ClearRegistryTraces()
	if err != nil || len(removed) != 3 {
		t.Fatal(removed, err)
	}
	if got := a.RegistryTraces(); len(got) != 0 {
		t.Fatalf("清除后应为空: %+v", got)
	}
	if a.GetSettings().Autostart {
		t.Fatal("清除时必须同时关闭「开机自启」设置，否则下次启动会重新写入")
	}
	// 再次清除不报错，返回空列表而不是 nil（前端按数组处理）
	removed, err = a.ClearRegistryTraces()
	if err != nil || removed == nil || len(removed) != 0 {
		t.Fatal(removed, err)
	}
	_ = sh
}
