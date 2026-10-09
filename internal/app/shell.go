package app

import "deskpinmemo/internal/regtrace"

// Rect 是屏幕坐标下的矩形（物理像素）。
type Rect struct {
	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w"`
	H int `json:"h"`
}

// Shell 抽象与操作系统相关的能力（窗口、对话框）。Windows 下由 winsys 实现，
// 开发服务器 / 测试使用 NopShell。前端和业务层不直接接触任何 Win32 API（7.4）。
type Shell interface {
	// 窗口模式：desktop（钉在桌面）| top（置顶）| normal（普通）
	SetMode(mode string) error
	Bounds() Rect
	SetBounds(r Rect)
	Scale() float64 // 当前窗口所在屏幕的缩放（1.0 = 100%）
	// BeginDrag / BeginResize 由前端标题栏 / 缩放柄触发，直到鼠标释放；onEnd 用于保存位置。
	BeginDrag(onEnd func())
	BeginResize(minW, minH, maxW, maxH int, onEnd func())
	// 窗口外观。Wails 窗口本身是不透明的（背景画刷 + 透明的网页），所以透明度必须用窗口级 alpha，
	// 不能用 CSS opacity（那样便签颜色会叠在黑色窗口背景上，看起来变黑）。
	// NativeOpacity 为 true 表示外壳自己处理透明度，前端不要再用 CSS。
	NativeOpacity() bool
	SetOpacity(o float64)     // 0.3–1
	SetBackground(hex string) // 窗口底色（#RRGGBB），避免缩放 / 刷新时露出黑色
	SetCornerRadius(dip int)  // 窗口圆角（用窗口区域裁剪，不依赖透明）；0 = 直角
	// SetClickThrough 让窗口对鼠标透明（FR-208）；on 为 true 时按住 Ctrl 会临时恢复交互。
	SetClickThrough(on bool)
	SetVisible(v bool)
	Visible() bool
	Focus()
	// CursorMonitorArea 返回鼠标所在显示器的工作区（快速输入框居中用）。
	CursorMonitorArea() Rect
	// PickFile 弹出保存 / 打开对话框，返回路径（取消返回空串）。
	PickFile(save bool, title, defaultName, pattern string) (string, error)
	// PickFolder 弹出选择文件夹对话框，返回路径（取消返回空串）。
	PickFolder(title string) (string, error)
	// RegistryTraces 列出本程序写入当前用户注册表（HKCU）的全部项；ClearRegistry 删除它们。
	RegistryTraces() []regtrace.Trace
	ClearRegistry() ([]string, error)
	// Restart 退出并重新启动程序（切换数据目录后需要）。
	Restart() error
	OpenPath(path string) error
	SetOverdueBadge(n int)
	Beep()
	Quit()
}

// NopShell 是无操作实现（开发服务器、单元测试）。
type NopShell struct {
	R            Rect
	Mode         string
	Hidden       bool
	Overdue      int
	Restarted    bool
	Native       bool    // 模拟「外壳处理窗口级透明度」
	Opacity      float64 // 最近一次 SetOpacity
	Background   string
	Radius       int
	reg          *regtrace.MemRegistry // 模拟的注册表（开发服务器 / 测试）
	ClickThrough bool
}

func (n *NopShell) SetMode(m string) error                                { n.Mode = m; return nil }
func (n *NopShell) Bounds() Rect                                          { return n.R }
func (n *NopShell) SetBounds(r Rect)                                      { n.R = r }
func (n *NopShell) Scale() float64                                        { return 1 }
func (n *NopShell) BeginDrag(func())                                      {}
func (n *NopShell) BeginResize(_, _, _, _ int, _ func())                  {}
func (n *NopShell) SetVisible(v bool)                                     { n.Hidden = !v }
func (n *NopShell) Visible() bool                                         { return !n.Hidden }
func (n *NopShell) Focus()                                                {}
func (n *NopShell) CursorMonitorArea() Rect                               { return Rect{0, 0, 1920, 1040} }
func (n *NopShell) PickFile(bool, string, string, string) (string, error) { return "", nil }
func (n *NopShell) NativeOpacity() bool                                   { return n.Native }
func (n *NopShell) SetOpacity(o float64)                                  { n.Opacity = o }
func (n *NopShell) SetBackground(h string)                                { n.Background = h }
func (n *NopShell) SetCornerRadius(d int)                                 { n.Radius = d }
func (n *NopShell) SetClickThrough(on bool)                               { n.ClickThrough = on }
func (n *NopShell) PickFolder(string) (string, error)                     { return "", nil }
func (n *NopShell) Restart() error                                        { n.Restarted = true; return nil }
func (n *NopShell) OpenPath(string) error                                 { return nil }
func (n *NopShell) SetOverdueBadge(c int)                                 { n.Overdue = c }
func (n *NopShell) Beep()                                                 {}
func (n *NopShell) Quit()                                                 {}

func (n *NopShell) memReg() *regtrace.MemRegistry {
	if n.reg == nil {
		n.reg = regtrace.NewMemRegistry()
		_, _ = regtrace.Register(n.reg, fakeExe, `C:\Users\me\AppData\Roaming\DeskPinMemo\icons\notify.png`)
		_ = regtrace.SetAutostart(n.reg, fakeExe, true)
	}
	return n.reg
}

func (n *NopShell) RegistryTraces() []regtrace.Trace { return regtrace.Traces(n.memReg(), fakeExe) }
func (n *NopShell) ClearRegistry() ([]string, error) { return regtrace.Clear(n.memReg(), fakeExe) }

// fakeExe 是 NopShell 模拟的 exe 路径。
const fakeExe = `D:\Portable\DeskPinMemo\DeskPinMemo.exe`
