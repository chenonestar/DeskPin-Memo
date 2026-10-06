// Package app 是绑定给前端的 API 层（Wails 绑定 / 开发服务器 RPC 共用）。
package app

import (
	"errors"
	"strings"

	"deskpinmemo/internal/datadir"
	"deskpinmemo/internal/regtrace"
	"deskpinmemo/internal/scheduler"
	"deskpinmemo/internal/service"
	"deskpinmemo/internal/store"
)

// 便签窗口的尺寸约束（4.1）与标题栏高度。
const (
	DefaultW, DefaultH = 300, 420
	MinW, MinH         = 220, 120
	MaxW, MaxH         = 600, 1000
	TitleBarH          = 28
	QuickW, QuickH     = 560, 132
)

// App 是前端可调用的方法集合；所有导出方法都会暴露给前端。
type App struct {
	svc   *service.Service
	shell Shell

	// 当前便签显示的分组，以及进入快速输入前的窗口状态
	groupID string
	quick   *quickSaved // 快速输入框 / 设置窗口打开期间保存的便签窗口状态
	overlay string      // "quick" | "settings" | ""
	alerts  int         // 尚未处理的强提醒数量（期间需要鼠标交互，暂停鼠标穿透）
	Version string
	Emit    func(event string, data any)
}

type quickSaved struct {
	bounds  Rect
	mode    string
	visible bool
}

// New 创建 App。
func New(svc *service.Service, shell Shell, version string) *App {
	a := &App{svc: svc, shell: shell, Version: version, Emit: func(string, any) {}}
	a.groupID, _ = svc.Store().InboxID()
	return a
}

// Service 返回业务层（外壳使用）。
func (a *App) Service() *service.Service { return a.svc }

// SetShell 在窗口创建后注入外壳实现。
func (a *App) SetShell(s Shell) { a.shell = s }

// ---------------------------------------------------------------- 数据

func (a *App) Bootstrap() (service.Bootstrap, error)             { return a.svc.Bootstrap() }
func (a *App) GroupContent(id string) (service.GroupView, error) { return a.svc.GroupContent(id) }
func (a *App) SmartView(name string) ([]service.ItemView, error) { return a.svc.SmartView(name) }
func (a *App) ByTag(tag string) ([]service.ItemView, error)      { return a.svc.ByTag(tag) }
func (a *App) Search(q string) ([]service.ItemView, error)       { return a.svc.Search(q) }
func (a *App) Trash() ([]service.ItemView, error)                { return a.svc.Trash() }
func (a *App) EmptyTrash() (int, error)                          { return a.svc.EmptyTrash() }
func (a *App) Undo() (string, error)                             { return a.svc.Undo() }
func (a *App) Delete(id string) error                            { return a.svc.Delete(id) }
func (a *App) Restore(id string) (service.ItemView, error)       { return a.svc.Restore(id) }
func (a *App) Reorder(id, beforeID string) error                 { return a.svc.Reorder(id, beforeID) }

func (a *App) CreateItem(in store.NewItem) (service.ItemView, error) { return a.svc.CreateItem(in) }
func (a *App) UpdateItem(id string, p store.ItemPatch) (service.ItemView, error) {
	return a.svc.UpdateItem(id, p)
}
func (a *App) Toggle(id string, done bool) (service.ItemView, error) { return a.svc.Toggle(id, done) }
func (a *App) SetReminders(id string, ins []store.ReminderInput) (service.ItemView, error) {
	return a.svc.SetReminders(id, ins)
}

// ---- 子任务（FR-108）----

func (a *App) AddSubtask(itemID, title string) (store.Subtask, error) {
	return a.svc.AddSubtask(itemID, title)
}
func (a *App) ToggleSubtask(id string, done bool) (store.Subtask, error) {
	return a.svc.ToggleSubtask(id, done)
}
func (a *App) RenameSubtask(id, title string) (store.Subtask, error) {
	return a.svc.RenameSubtask(id, title)
}
func (a *App) DeleteSubtask(id string) error { return a.svc.DeleteSubtask(id) }
func (a *App) ReorderSubtasks(itemID string, orderedIDs []string) error {
	return a.svc.ReorderSubtasks(itemID, orderedIDs)
}

func (a *App) BuildRepeat(kind string, arg int) (string, error) { return a.svc.BuildRepeat(kind, arg) }
func (a *App) QuickParse(text, groupID string) (service.QuickPreview, error) {
	return a.svc.QuickParse(text, groupID)
}

// QuickCreate 由快速输入文本创建事项。groupID 为空时使用当前便签分组。
func (a *App) QuickCreate(text, groupID string) (service.ItemView, error) {
	if groupID == "" {
		groupID = a.groupID
	}
	return a.svc.QuickCreate(text, groupID)
}

// SnoozeMinutes 稍后提醒（分钟）。
func (a *App) SnoozeMinutes(itemID string, minutes int) error {
	return a.svc.HandleAction(itemID, map[int]string{10: "snooze10", 60: "snooze60"}[minutes])
}

// HandleAction 处理通知按钮：done | snooze10 | snooze60 | tomorrow。
func (a *App) HandleAction(itemID, action string) error { return a.svc.HandleAction(itemID, action) }

func (a *App) Groups() ([]store.Group, error) { return a.svc.Groups() }
func (a *App) CreateGroup(name, color string) (store.Group, error) {
	return a.svc.CreateGroup(name, color)
}
func (a *App) UpdateGroup(id string, name, color *string) error {
	return a.svc.UpdateGroup(id, name, color)
}
func (a *App) DeleteGroup(id string) error {
	if err := a.svc.DeleteGroup(id); err != nil {
		return err
	}
	if id == a.groupID {
		a.groupID, _ = a.svc.Store().InboxID()
	}
	return nil
}

// ---------------------------------------------------------------- 设置

func (a *App) GetSettings() service.Settings { return a.svc.GetSettings() }
func (a *App) SaveSettings(s service.Settings) (service.Settings, error) {
	return a.svc.SaveSettings(s)
}

// ---------------------------------------------------------------- 便签窗口

// WindowState 是前端需要的窗口状态。
type WindowState struct {
	store.Window
	Visible bool `json:"visible"`
}

// CurrentGroup 返回便签当前显示的分组。
func (a *App) CurrentGroup() string { return a.groupID }

// ShowGroup 切换便签显示的分组，并套用该分组保存的窗口状态。
func (a *App) ShowGroup(groupID string) (WindowState, error) {
	if a.shell != nil && a.quick == nil {
		a.saveWindow() // 先保存旧分组的位置
	}
	a.groupID = groupID
	return a.ApplyWindow()
}

// ApplyWindow 读取当前分组的窗口状态并应用到窗口：位置、尺寸、模式（FR-203 / AC-04）。
func (a *App) ApplyWindow() (WindowState, error) {
	w, err := a.svc.GetWindow(a.groupID)
	if err != nil {
		return WindowState{}, err
	}
	if w.Width == 0 {
		w.Width, w.Height = DefaultW, DefaultH
	}
	sc := a.shell.Scale()
	r := Rect{X: w.X, Y: w.Y, W: int(float64(w.Width) * sc), H: a.effectiveHeight(w, sc)}
	if w.X == 0 && w.Y == 0 { // 首次启动：放在主屏右上角
		area := a.shell.CursorMonitorArea()
		r.X, r.Y = area.X+area.W-r.W-40, area.Y+60
	}
	a.shell.SetBounds(r) // 外壳负责把不存在屏幕上的位置拉回主屏（AC-12）
	_ = a.shell.SetMode(w.Mode)
	a.applyClickThrough()
	return WindowState{Window: w, Visible: a.shell.Visible()}, nil
}

// applyClickThrough 把「是否鼠标穿透」应用到窗口。快速输入框、设置窗口、每日概览和强提醒
// 打开期间必须能用鼠标操作，所以暂时关闭穿透，结束后按保存的状态恢复。
func (a *App) applyClickThrough() {
	w, err := a.svc.GetWindow(a.groupID)
	if err != nil {
		return
	}
	a.shell.SetClickThrough(w.ClickThrough && a.quick == nil && a.alerts == 0)
}

// SetClickThrough 开启 / 关闭鼠标穿透（FR-208）并保存。
func (a *App) SetClickThrough(on bool) (WindowState, error) {
	w, _ := a.svc.GetWindow(a.groupID)
	w.ClickThrough = on
	if _, err := a.svc.SaveWindow(w); err != nil {
		return WindowState{}, err
	}
	a.applyClickThrough()
	ws, err := a.windowState()
	if err == nil {
		a.Emit("window:state", ws)
	}
	return ws, err
}

// ToggleClickThrough 切换鼠标穿透（托盘菜单：开启后窗口不再响应鼠标，需要一个不依赖窗口的入口）。
func (a *App) ToggleClickThrough() (WindowState, error) {
	w, _ := a.svc.GetWindow(a.groupID)
	return a.SetClickThrough(!w.ClickThrough)
}

// ClickThroughEnabled 返回当前分组便签是否开启了鼠标穿透（托盘菜单勾选状态）。
func (a *App) ClickThroughEnabled() bool {
	w, err := a.svc.GetWindow(a.groupID)
	return err == nil && w.ClickThrough
}

// StrongAlertDone 前端处理完一条强提醒后调用；全部处理完后恢复鼠标穿透。
func (a *App) StrongAlertDone() {
	if a.alerts > 0 {
		a.alerts--
	}
	a.applyClickThrough()
}

func (a *App) effectiveHeight(w store.Window, sc float64) int {
	if w.Collapsed {
		return int(TitleBarH * sc)
	}
	return int(float64(w.Height) * sc)
}

// saveWindow 把窗口当前位置/尺寸写回数据库。
func (a *App) saveWindow() {
	if a.quick != nil { // 快速输入框期间不保存
		return
	}
	w, err := a.svc.GetWindow(a.groupID)
	if err != nil {
		return
	}
	r, sc := a.shell.Bounds(), a.shell.Scale()
	w.X, w.Y = r.X, r.Y
	if !w.Collapsed && r.W > 0 {
		w.Width, w.Height = int(float64(r.W)/sc+0.5), int(float64(r.H)/sc+0.5)
	} else if r.W > 0 {
		w.Width = int(float64(r.W)/sc + 0.5)
	}
	_, _ = a.svc.SaveWindow(w)
}

// SetWindowMode 切换 钉在桌面 / 置顶 / 普通 三种模式（FR-202）。
func (a *App) SetWindowMode(mode string) (WindowState, error) {
	if mode != "desktop" && mode != "top" && mode != "normal" {
		return WindowState{}, errors.New("无效的窗口模式")
	}
	if err := a.shell.SetMode(mode); err != nil {
		return WindowState{}, err
	}
	w, _ := a.svc.GetWindow(a.groupID)
	w.Mode = mode
	if _, err := a.svc.SaveWindow(w); err != nil {
		return WindowState{}, err
	}
	a.saveWindow()
	return a.windowState()
}

func (a *App) windowState() (WindowState, error) {
	w, err := a.svc.GetWindow(a.groupID)
	return WindowState{Window: w, Visible: a.shell.Visible()}, err
}

// SetWindowLocked 锁定 / 解锁位置（FR-204）。
func (a *App) SetWindowLocked(locked bool) (WindowState, error) {
	w, _ := a.svc.GetWindow(a.groupID)
	w.Locked = locked
	if _, err := a.svc.SaveWindow(w); err != nil {
		return WindowState{}, err
	}
	return a.windowState()
}

// SetWindowOpacity 设置透明度 30%–100%（FR-205）。
func (a *App) SetWindowOpacity(opacity float64) (WindowState, error) {
	w, _ := a.svc.GetWindow(a.groupID)
	w.Opacity = opacity
	if _, err := a.svc.SaveWindow(w); err != nil {
		return WindowState{}, err
	}
	return a.windowState()
}

// SetWindowColor 设置便签颜色。
func (a *App) SetWindowColor(color string) (WindowState, error) {
	w, _ := a.svc.GetWindow(a.groupID)
	w.Color = color
	if _, err := a.svc.SaveWindow(w); err != nil {
		return WindowState{}, err
	}
	return a.windowState()
}

// ToggleCollapse 双击标题栏折叠为一行（FR-207）。
func (a *App) ToggleCollapse() (WindowState, error) {
	w, _ := a.svc.GetWindow(a.groupID)
	if !w.Collapsed {
		a.saveWindow() // 记住展开时的高度
		w, _ = a.svc.GetWindow(a.groupID)
	}
	w.Collapsed = !w.Collapsed
	if _, err := a.svc.SaveWindow(w); err != nil {
		return WindowState{}, err
	}
	r, sc := a.shell.Bounds(), a.shell.Scale()
	r.H = a.effectiveHeight(w, sc)
	a.shell.SetBounds(r)
	return a.windowState()
}

// BeginDrag 前端在标题栏 mousedown 时调用；锁定或折叠以外的窗口才可拖动。
func (a *App) BeginDrag() {
	if w, _ := a.svc.GetWindow(a.groupID); w.Locked || a.quick != nil {
		return
	}
	a.shell.BeginDrag(a.saveWindow)
}

// BeginResize 前端在右下角缩放柄 mousedown 时调用。
func (a *App) BeginResize() {
	w, _ := a.svc.GetWindow(a.groupID)
	if w.Locked || w.Collapsed || a.quick != nil {
		return
	}
	sc := a.shell.Scale()
	f := func(v int) int { return int(float64(v) * sc) }
	a.shell.BeginResize(f(MinW), f(MinH), f(MaxW), f(MaxH), a.saveWindow)
}

// ToggleVisible 显示 / 隐藏全部便签（Ctrl+Alt+M、托盘菜单）。
func (a *App) ToggleVisible() bool {
	v := !a.shell.Visible()
	a.shell.SetVisible(v)
	return v
}

// ---------------------------------------------------------------- 快速输入框

// Wails v2 只支持单窗口：快速输入框与设置窗口以「临时改变便签窗口形态」的方式实现——
// 记住当前位置/模式，变成置顶的目标尺寸，关闭后原样恢复。
const (
	SettingsW, SettingsH = 720, 540
	OverviewW, OverviewH = 380, 480
)

func (a *App) openOverlay(kind string, w, h int, topFraction bool) {
	if a.quick == nil {
		a.quick = &quickSaved{bounds: a.shell.Bounds(), visible: a.shell.Visible()}
		if win, err := a.svc.GetWindow(a.groupID); err == nil {
			a.quick.mode = win.Mode
		}
	}
	a.overlay = kind
	area, sc := a.shell.CursorMonitorArea(), a.shell.Scale()
	pw, ph := int(float64(w)*sc), int(float64(h)*sc)
	y := area.Y + (area.H-ph)/2
	if topFraction { // 快速输入框：屏幕居中偏上
		y = area.Y + area.H/5
	}
	a.shell.SetBounds(Rect{X: area.X + (area.W-pw)/2, Y: y, W: pw, H: ph})
	_ = a.shell.SetMode("top")
	a.applyClickThrough() // a.quick 已设置 → 暂时关闭穿透
	a.shell.SetVisible(true)
	a.shell.Focus()
}

func (a *App) closeOverlay() {
	q := a.quick
	if q == nil {
		return
	}
	a.quick, a.overlay = nil, ""
	a.shell.SetBounds(q.bounds)
	_ = a.shell.SetMode(q.mode)
	a.shell.SetVisible(q.visible)
	a.applyClickThrough()
}

// OpenQuick 唤出快速输入框：窗口临时变为置顶、居中偏上的 560 px 宽输入条（4.2）。
func (a *App) OpenQuick() {
	a.openOverlay("quick", QuickW, QuickH, true)
	a.Emit("overlay:open", "quick")
}

// CloseQuick 关闭快速输入框并恢复便签窗口。
func (a *App) CloseQuick() {
	a.closeOverlay()
	a.Emit("overlay:close", nil)
}

// OpenSettings 打开设置窗口（4.3）。
func (a *App) OpenSettings() {
	a.openOverlay("settings", SettingsW, SettingsH, false)
	a.Emit("overlay:open", "settings")
}

// CloseSettings 关闭设置窗口。
func (a *App) CloseSettings() {
	a.closeOverlay()
	a.Emit("overlay:close", nil)
}

// OpenOverview 打开每日概览（FR-308）。
func (a *App) OpenOverview() {
	a.openOverlay("overview", OverviewW, OverviewH, false)
	a.Emit("overlay:open", "overview")
}

// CloseOverview 关闭每日概览。
func (a *App) CloseOverview() {
	a.closeOverlay()
	a.Emit("overlay:close", nil)
}

// Overview 返回概览内容（逾期 + 今天到期）。
func (a *App) Overview() (service.Overview, error) { return a.svc.Overview() }

// MaybeShowOverview 启动时调用：当天首次启动且有事项需要关注时弹出每日概览。
func (a *App) MaybeShowOverview() bool {
	ok, err := a.svc.ClaimDailyOverview()
	if err != nil || !ok {
		return false
	}
	a.OpenOverview()
	return true
}

// Overlay 返回当前叠加界面："quick" | "settings" | ""（前端启动时据此决定渲染）。
func (a *App) Overlay() string { return a.overlay }

// ---------------------------------------------------------------- 数据管理

func (a *App) DataDir() string            { return a.svc.DataDir() }
func (a *App) OpenDataDir() error         { return a.shell.OpenPath(a.svc.DataDir()) }
func (a *App) BackupNow() (string, error) { return a.svc.BackupNow() }
func (a *App) ListBackups() []string      { return a.svc.ListBackups() }
func (a *App) OpenBackupDir() error       { return a.shell.OpenPath(a.svc.BackupDir()) }
func (a *App) AppVersion() string         { return a.Version }

// ExportJSONDialog 弹出保存对话框并导出 JSON；password 非空为加密导出。返回保存路径（取消为空）。
func (a *App) ExportJSONDialog(password string) (string, error) {
	p, err := a.shell.PickFile(true, "导出数据", "DeskPinMemo-export.json", "*.json")
	if err != nil || p == "" {
		return "", err
	}
	return p, a.svc.ExportJSON(p, password)
}

// ExportMarkdownDialog 导出可读清单。
func (a *App) ExportMarkdownDialog() (string, error) {
	p, err := a.shell.PickFile(true, "导出清单", "DeskPinMemo-list.md", "*.md")
	if err != nil || p == "" {
		return "", err
	}
	return p, a.svc.ExportMarkdown(p)
}

// PickImportFile 选择要导入的文件。
func (a *App) PickImportFile() (string, error) {
	return a.shell.PickFile(false, "选择导出文件", "", "*.json")
}

func (a *App) InspectImport(path, password string) (service.ImportInfo, error) {
	return a.svc.InspectImport(path, password)
}

// ImportJSON 导入；overwrite=true 为覆盖（前端必须先二次确认）。
func (a *App) ImportJSON(path, password string, overwrite bool) (store.ImportStats, error) {
	return a.svc.ImportJSON(path, password, overwrite)
}

// ---------------------------------------------------------------- 加密

func (a *App) EncryptionStatus() service.EncStatus { return a.svc.EncryptionStatus() }
func (a *App) EnableEncryption(password, mode string) (string, error) {
	return a.svc.EnableEncryption(password, mode)
}
func (a *App) Unlock(password string) error { return a.svc.Unlock(password) }
func (a *App) ResetPasswordWithRecovery(recoveryKey, newPassword string) error {
	return a.svc.ResetPasswordWithRecovery(recoveryKey, newPassword)
}
func (a *App) ChangePassword(oldPw, newPw string) error { return a.svc.ChangePassword(oldPw, newPw) }
func (a *App) SetUnlockMode(mode, password string) error {
	return a.svc.SetUnlockMode(mode, password)
}
func (a *App) DisableEncryption(password string) error { return a.svc.DisableEncryption(password) }
func (a *App) DeleteOldBackups() (int, error)          { return a.svc.DeletePlaintextBackups() }

// ---------------------------------------------------------------- 数据目录（FR-605）

func (a *App) DataDirStatus() service.DataDirStatus    { return a.svc.DataDirStatus() }
func (a *App) InspectDataDir(path string) datadir.Info { return a.svc.InspectDataDir(path) }

// PickDataDir 弹出文件夹选择对话框。
func (a *App) PickDataDir() (string, error) { return a.shell.PickFolder("选择新的数据目录") }

// ChangeDataDir 切换数据目录（copy | use | replace），切换在重启后生效。
func (a *App) ChangeDataDir(target, mode string) (service.DataDirChange, error) {
	return a.svc.ChangeDataDir(target, mode)
}

// RestartApp 保存窗口状态后重启程序。
func (a *App) RestartApp() error {
	a.saveWindow()
	return a.shell.Restart()
}

// ---------------------------------------------------------------- 系统注册表项

// RegistryTraces 列出本程序写入当前用户注册表（HKCU）的项：开机自启、通知身份、deskpin:// 协议。
func (a *App) RegistryTraces() []regtrace.Trace {
	t := a.shell.RegistryTraces()
	if t == nil {
		t = []regtrace.Trace{}
	}
	return t
}

// ClearRegistryTraces 清除全部注册表项（删除绿色版文件夹前使用）。
// 同时把「开机自启」设置改为关闭，否则下次启动会按设置重新写入。
// 通知身份和协议是 Toast 必需的，程序再次启动时会重新写入。返回被删除的项。
func (a *App) ClearRegistryTraces() ([]string, error) {
	s := a.svc.GetSettings()
	if s.Autostart {
		s.Autostart = false
		if _, err := a.svc.SaveSettings(s); err != nil {
			return nil, err
		}
	}
	removed, err := a.shell.ClearRegistry()
	if removed == nil {
		removed = []string{}
	}
	return removed, err
}

// ---------------------------------------------------------------- 杂项

// Quit 退出程序。
func (a *App) Quit() {
	a.saveWindow()
	a.shell.Quit()
}

// StrongAlert 是强提醒事件的负载。
type StrongAlert struct {
	scheduler.Notification
	Sound bool `json:"sound"`
}

// ShowStrongAlert 强提醒（FR-305）：窗口置顶显示并播放提示音，前端弹出必须手动处理的窗口。
func (a *App) ShowStrongAlert(n scheduler.Notification) {
	if a.quick == nil {
		a.shell.SetVisible(true)
		_ = a.shell.SetMode("top")
	}
	a.alerts++
	a.applyClickThrough() // 强提醒必须手动处理，期间暂停穿透
	sound := a.svc.GetSettings().Sound
	if sound {
		a.shell.Beep()
	}
	a.Emit("reminder:strong", StrongAlert{Notification: n, Sound: sound})
}

// HandleProtocolURL 处理 deskpin://action?item=ID&a=done 形式的通知按钮回调。
func (a *App) HandleProtocolURL(raw string) error {
	raw = strings.TrimSpace(strings.Trim(raw, `"`))
	if !strings.HasPrefix(raw, "deskpin://") {
		return errors.New("不是 deskpin 协议链接")
	}
	rest := strings.TrimPrefix(raw, "deskpin://")
	path, query, _ := strings.Cut(rest, "?")
	kv := map[string]string{}
	for _, p := range strings.Split(query, "&") {
		if k, v, ok := strings.Cut(p, "="); ok {
			kv[k] = v
		}
	}
	switch strings.Trim(path, "/") {
	case "action":
		return a.svc.HandleAction(kv["item"], kv["a"])
	case "open":
		a.shell.SetVisible(true)
		a.shell.Focus()
	}
	return nil
}
