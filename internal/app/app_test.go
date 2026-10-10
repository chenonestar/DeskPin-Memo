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

// ---- 外观：默认颜色 / 透明度 / 鼠标离开变淡 ----

func TestDefaultColorFollowsSettings(t *testing.T) { // 设置 → 外观 → 默认便签颜色
	a, sh := newApp(t)
	if _, err := a.ApplyWindow(); err != nil { // 首次显示会保存窗口位置（过去会把默认色写死）
		t.Fatal(err)
	}
	a.saveWindow()
	if sh.Background != "#FFF3B0" {
		t.Fatalf("初始应使用默认色: %q", sh.Background)
	}
	s := a.GetSettings()
	s.StickyColor = "#D9F2C9"
	if _, err := a.SaveSettings(s); err != nil {
		t.Fatal(err)
	}
	if sh.Background != "#D9F2C9" {
		t.Fatalf("修改默认颜色后，没单独设置过颜色的便签应立刻跟随: %q", sh.Background)
	}
	if w, _ := a.svc.GetWindow(a.groupID); w.Color != "" {
		t.Fatalf("窗口记录里不应写死默认色: %q", w.Color)
	}
	// 单独设置的颜色优先于默认值
	a.SetWindowColor("#FFD9E4")
	if sh.Background != "#FFD9E4" {
		t.Fatal(sh.Background)
	}
	s.StickyColor = "#CFE8FF"
	a.SaveSettings(s)
	if sh.Background != "#FFD9E4" {
		t.Fatal("单独设置过颜色的便签不应被默认色覆盖")
	}
	// 恢复默认外观后重新跟随
	ws, err := a.ResetWindowAppearance()
	if err != nil || ws.Color != "" || ws.Opacity != 0 || sh.Background != "#CFE8FF" {
		t.Fatalf("%+v %v %q", ws, err, sh.Background)
	}
}

func TestDefaultOpacityFollowsSettings(t *testing.T) {
	a, sh := newApp(t)
	a.ApplyWindow()
	a.saveWindow()
	if sh.Opacity != 1 {
		t.Fatal(sh.Opacity)
	}
	s := a.GetSettings()
	s.Opacity = 0.6
	a.SaveSettings(s)
	if sh.Opacity != 0.6 {
		t.Fatalf("默认透明度变化应立刻生效: %v", sh.Opacity)
	}
	a.SetWindowOpacity(0.8) // 单独设置
	if sh.Opacity != 0.8 {
		t.Fatal(sh.Opacity)
	}
	s.Opacity = 0.4
	a.SaveSettings(s)
	if sh.Opacity != 0.8 {
		t.Fatal("单独设置过的透明度不应被默认值覆盖")
	}
}

func TestFadeOnLeaveUsesWindowOpacityNotBlackBackground(t *testing.T) {
	a, sh := newApp(t)
	sh.Native = true
	a.ApplyWindow()
	s := a.GetSettings()
	s.FadeOnLeave = true
	a.SaveSettings(s)
	if ws, _ := a.ApplyWindow(); !ws.NativeOpacity {
		t.Fatal("前端需要知道外壳处理窗口级透明度，以免再叠加 CSS opacity")
	}
	// 启动时假定鼠标不在便签上 → 已变淡
	if sh.Opacity >= 1 || sh.Opacity < 0.25 {
		t.Fatalf("启动时应为变淡状态: %v", sh.Opacity)
	}
	a.SetFaded(false) // 鼠标进入
	if sh.Opacity != 1 {
		t.Fatalf("鼠标在便签上应完全显示: %v", sh.Opacity)
	}
	a.SetFaded(true) // 鼠标离开
	faded := sh.Opacity
	if faded >= 1 || faded < 0.25 {
		t.Fatal(faded)
	}
	// 关闭设置项后不再变淡
	s.FadeOnLeave = false
	a.SaveSettings(s)
	if sh.Opacity != 1 {
		t.Fatalf("关闭「鼠标离开后自动变淡」后应保持不透明: %v", sh.Opacity)
	}
}

func TestFadedOpacityIsAbsoluteSetting(t *testing.T) {
	a, sh := newApp(t)
	sh.Native = true
	s := a.GetSettings()
	s.FadeOnLeave, s.FadedOpacity = true, 0.3
	a.SaveSettings(s)
	a.ApplyWindow()
	if sh.Opacity != 0.3 {
		t.Fatalf("变淡后应为设置的绝对值: %v", sh.Opacity)
	}
	s.FadedOpacity = 0.6
	a.SaveSettings(s)
	if sh.Opacity != 0.6 {
		t.Fatalf("修改变淡透明度应立刻生效: %v", sh.Opacity)
	}
	// 便签本身比变淡值更透明时，不会反而变得更不透明
	a.SetWindowOpacity(0.5)
	if sh.Opacity != 0.5 {
		t.Fatalf("变淡值不应超过便签自身透明度: %v", sh.Opacity)
	}
	// 越界值被限制在 20%–80%
	s.FadedOpacity = 0.05
	a.SaveSettings(s)
	if got := a.GetSettings().FadedOpacity; got != 0.2 {
		t.Fatalf("clamp: %v", got)
	}
}

func TestClickThroughIgnoresFade(t *testing.T) {
	a, sh := newApp(t)
	sh.Native = true
	s := a.GetSettings()
	s.FadeOnLeave = true
	a.SaveSettings(s)
	ws, _ := a.ApplyWindow()
	if ws.FadeIgnored || sh.Opacity >= 1 {
		t.Fatalf("未穿透时应变淡: %+v %v", ws, sh.Opacity)
	}
	if _, err := a.ToggleClickThrough(); err != nil {
		t.Fatal(err)
	}
	if sh.Opacity != 1 {
		t.Fatalf("穿透开启时自动变淡不生效: %v", sh.Opacity)
	}
	if ws, _ := a.WindowInfo(); !ws.FadeIgnored {
		t.Fatalf("应提示自动变淡未生效: %+v", ws)
	}
	if !a.GetSettings().FadeOnLeave {
		t.Fatal("不应改写已保存的设置")
	}
	if _, err := a.ToggleClickThrough(); err != nil {
		t.Fatal(err)
	}
	if ws, _ := a.WindowInfo(); sh.Opacity >= 1 || ws.FadeIgnored {
		t.Fatalf("关闭穿透后恢复自动变淡: %v", sh.Opacity)
	}
}

func TestOverlaysAndAlertsForceOpaque(t *testing.T) {
	a, sh := newApp(t)
	sh.Native = true
	s := a.GetSettings()
	s.FadeOnLeave, s.Opacity = true, 0.5
	a.SaveSettings(s)
	a.ApplyWindow()
	if sh.Opacity >= 0.5 {
		t.Fatalf("应处于变淡状态: %v", sh.Opacity)
	}
	a.OpenSettings()
	if sh.Opacity != 1 {
		t.Fatalf("设置窗口打开期间必须不透明: %v", sh.Opacity)
	}
	if sh.Radius != 10 {
		t.Fatalf("设置窗口圆角: %d", sh.Radius)
	}
	a.CloseSettings()
	if sh.Opacity != 0.5 || sh.Radius != 8 { // 刚关闭：鼠标多半还在窗口上，不立刻变淡
		t.Fatalf("关闭后恢复便签的透明度 / 圆角: %v %d", sh.Opacity, sh.Radius)
	}
	a.OpenQuick()
	if sh.Opacity != 1 || sh.Radius != 12 {
		t.Fatalf("快速输入框: %v %d", sh.Opacity, sh.Radius)
	}
	a.CloseQuick()
	a.SetFaded(true)
	n := scheduler.Notification{Title: "x", Strong: true}
	a.ShowStrongAlert(n)
	if sh.Opacity != 1 {
		t.Fatalf("强提醒期间必须不透明: %v", sh.Opacity)
	}
	a.StrongAlertDone()
	if sh.Opacity >= 0.5 {
		t.Fatalf("强提醒处理完后恢复变淡: %v", sh.Opacity)
	}
}
