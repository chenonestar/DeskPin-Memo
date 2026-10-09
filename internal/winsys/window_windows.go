//go:build windows

package winsys

import (
	"errors"
	"fmt"
	"log"
	"os"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"deskpinmemo/internal/app"

	"golang.org/x/sys/windows"
)

// Shell 是 app.Shell 的 Windows 实现，管理便签所在的（唯一）WebView 窗口。
type Shell struct {
	mu       sync.Mutex
	hwnd     windows.HWND
	mode     string
	dragging atomic.Bool
	quit     func()
	loop     *Loop

	// 窗口外观与鼠标穿透（FR-208）。
	// ctMu 只保护下面几个简单字段，持锁期间绝不调用 Win32：SetWindowLongPtr 等会向窗口所属线程
	// 同步发消息，而 UI 线程自己也可能正等这把锁，持锁调用会死锁。
	ctMu          sync.Mutex
	ct            bool          // 鼠标穿透功能是否开启（不含按住 Ctrl 的临时恢复）
	alpha         byte          // 窗口级透明度 0–255
	radius        int           // 圆角（DIP）
	rgnW, rgnH    int           // 最近一次设置圆角区域时的窗口尺寸
	brush         uintptr       // 我们创建的底色画刷
	stopPoll      chan struct{} // 穿透开启期间的兜底检查
	ctrlCh        chan struct{}
	applying      atomic.Bool             // applyStyles 单飞：同一时刻只有一个 goroutine 在改样式
	dirty         atomic.Bool             // 单飞期间又有新的调用：当前执行者结束后需要再应用一遍
	lastOn        atomic.Bool             // 最近一次应用的「当前是否处于穿透」
	ctState       map[windows.HWND]exBits // 仅在 applyStylesOnce 内访问（单飞，无需加锁）：我们加上的样式位
	interactive   bool                    // 同上：穿透开启但按住 Ctrl 临时恢复交互
	OnInteractive func(bool)              // 按住 Ctrl 临时恢复交互 / 松开时回调（前端据此显示状态）
}

// ErrPinFallback 表示「钉在桌面」不可用，已降级为「置底窗口」（10.1 风险应对）。
var ErrPinFallback = errors.New("当前系统不支持嵌入桌面层，已降级为置底窗口")

var _ app.Shell = (*Shell)(nil)

// NewShell 创建外壳；quit 用于退出程序。窗口句柄稍后通过 Attach 绑定。
func NewShell(quit func()) *Shell {
	s := &Shell{quit: quit, mode: "desktop", ctState: map[windows.HWND]exBits{}, alpha: 255, ctrlCh: make(chan struct{}, 1)}
	go func() {
		for range s.ctrlCh { // 串行处理 Ctrl 状态变化；以实际按键状态为准，不依赖事件顺序
			s.applyStyles()
		}
	}()
	return s
}

// SetLoop 关联消息循环（托盘徽标等）。
func (s *Shell) SetLoop(l *Loop) { s.loop = l }

// FindWindow 在本进程内按标题查找主窗口（Wails v2 不直接暴露 HWND）。
func FindWindow(title string, timeout time.Duration) (windows.HWND, error) {
	pid := uint32(os.Getpid())
	deadline := time.Now().Add(timeout)
	for {
		var found windows.HWND
		cb := syscall.NewCallback(func(h uintptr, _ uintptr) uintptr {
			var wpid uint32
			pGetWindowThreadProcessId.Call(h, uintptr(unsafe.Pointer(&wpid)))
			if wpid == pid && windowText(windows.HWND(h)) == title {
				found = windows.HWND(h)
				return 0
			}
			return 1
		})
		pEnumWindows.Call(cb, 0)
		if found != 0 {
			return found, nil
		}
		if time.Now().After(deadline) {
			return 0, fmt.Errorf("未找到窗口 %q", title)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func getParentRaw(h windows.HWND) windows.HWND {
	r, _, _ := pGetParent.Call(uintptr(h))
	return windows.HWND(r)
}

// Attach 绑定窗口句柄，并设置不出现在任务栏 / Alt+Tab 的扩展样式（WS_EX_TOOLWINDOW）。
func (s *Shell) Attach(h windows.HWND) {
	s.mu.Lock()
	s.hwnd = h
	s.mu.Unlock()
	s.applyToolWindow()
}

// HWND 返回窗口句柄。
func (s *Shell) HWND() windows.HWND {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.hwnd
}

func (s *Shell) applyToolWindow() {
	h := s.HWND()
	if h == 0 {
		return
	}
	ex := getWindowLong(h, gwlExStyle)
	ex = (ex | wsExToolWindow) &^ wsExAppWindow
	setWindowLong(h, gwlExStyle, ex)
	pSetWindowPos.Call(uintptr(h), 0, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoZOrder|swpNoActivate|swpFrameChanged)
}

// ---- 钉在桌面 / 置顶 / 普通 ----

// findDesktopHost 找到承载桌面图标层 SHELLDLL_DefView 的顶层窗口（Progman 或 WorkerW）。
// 不发送 0x052C：那会把 DefView 搬进 WorkerW；两种结构都能通过枚举找到宿主。
func findDesktopHost() windows.HWND {
	var host windows.HWND
	cb := syscall.NewCallback(func(h uintptr, _ uintptr) uintptr {
		cls := className(windows.HWND(h))
		if cls != "Progman" && cls != "WorkerW" {
			return 1
		}
		dv, _, _ := pFindWindowExW.Call(h, 0, uintptr(unsafe.Pointer(utf16("SHELLDLL_DefView"))), 0)
		if dv != 0 {
			host = windows.HWND(h)
			return 0
		}
		return 1
	})
	pEnumWindows.Call(cb, 0)
	if host == 0 {
		r, _, _ := pFindWindowW.Call(uintptr(unsafe.Pointer(utf16("Progman"))), 0)
		host = windows.HWND(r)
	}
	return host
}

// SetMode 切换窗口模式。desktop 失败时降级为置底窗口并返回 ErrPinFallback。
func (s *Shell) SetMode(mode string) error {
	h := s.HWND()
	if h == 0 {
		return errors.New("窗口尚未就绪")
	}
	s.mu.Lock()
	s.mode = mode
	s.mu.Unlock()
	switch mode {
	case "desktop":
		if err := s.attachDesktop(h); err != nil {
			s.detach(h)
			pSetWindowPos.Call(uintptr(h), hwndBottom, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoActivate)
			go s.Repin() // 开机自启时桌面宿主可能尚未就绪：后台重试挂载
			return ErrPinFallback
		}
	case "top":
		s.detach(h)
		pSetWindowPos.Call(uintptr(h), hwndTopmost, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoActivate|swpShowWindow)
	default:
		s.detach(h)
		pSetWindowPos.Call(uintptr(h), hwndNoTopmost, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoActivate|swpShowWindow)
	}
	s.applyToolWindow()
	return nil
}

func (s *Shell) attachDesktop(h windows.HWND) error {
	host := findDesktopHost()
	if host == 0 {
		return errors.New("找不到桌面窗口")
	}
	if getParentRaw(h) == host {
		return nil
	}
	r := getWindowRect(h)
	style := getWindowLong(h, gwlStyle)
	style = (style &^ wsPopup) | wsChild | wsVisible
	setWindowLong(h, gwlStyle, style)
	if ret, _, err := pSetParent.Call(uintptr(h), uintptr(host)); ret == 0 {
		return fmt.Errorf("SetParent 失败: %v", err)
	}
	hr := getWindowRect(host)
	pMoveWindow.Call(uintptr(h), uintptr(r.Left-hr.Left), uintptr(r.Top-hr.Top), uintptr(r.Right-r.Left), uintptr(r.Bottom-r.Top), 1)
	// 放到宿主内最上层（高于图标层 DefView，但整体仍在所有普通窗口之下）
	pSetWindowPos.Call(uintptr(h), hwndTopZ, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoActivate|swpShowWindow|swpFrameChanged)
	return nil
}

func (s *Shell) detach(h windows.HWND) {
	if getParentRaw(h) == 0 {
		return
	}
	r := getWindowRect(h)
	style := getWindowLong(h, gwlStyle)
	style = (style &^ wsChild) | wsPopup | wsVisible
	setWindowLong(h, gwlStyle, style)
	pSetParent.Call(uintptr(h), 0)
	pMoveWindow.Call(uintptr(h), uintptr(r.Left), uintptr(r.Top), uintptr(r.Right-r.Left), uintptr(r.Bottom-r.Top), 1)
}

// Repin 在资源管理器重启（TaskbarCreated）或显示器变化后重新挂载，最多重试 ~15 秒（AC-03）。
func (s *Shell) Repin() {
	s.mu.Lock()
	mode := s.mode
	s.mu.Unlock()
	if mode != "desktop" {
		return
	}
	go func() {
		for i := 0; i < 30; i++ {
			h := s.HWND()
			if h == 0 {
				return
			}
			// 宿主重建后旧父窗口失效：先 detach 再 attach
			if p := getParentRaw(h); p != 0 {
				if ok, _, _ := pIsWindow.Call(uintptr(p)); ok == 0 || p != findDesktopHost() {
					s.detach(h)
				}
			}
			if s.attachDesktop(h) == nil {
				return
			}
			time.Sleep(500 * time.Millisecond)
		}
	}()
}

// ---- 位置与尺寸 ----

func (s *Shell) Bounds() app.Rect {
	r := getWindowRect(s.HWND())
	return app.Rect{X: int(r.Left), Y: int(r.Top), W: int(r.Right - r.Left), H: int(r.Bottom - r.Top)}
}

func (s *Shell) SetBounds(r app.Rect) {
	h := s.HWND()
	if h == 0 {
		return
	}
	r = clampToMonitors(r)
	x, y := r.X, r.Y
	if p := getParentRaw(h); p != 0 {
		pr := getWindowRect(p)
		x, y = x-int(pr.Left), y-int(pr.Top)
	}
	pMoveWindow.Call(uintptr(h), uintptr(x), uintptr(y), uintptr(r.W), uintptr(r.H), 1)
	s.applyRegion()
}

func (s *Shell) moveResize(r app.Rect) {
	h := s.HWND()
	x, y := r.X, r.Y
	if p := getParentRaw(h); p != 0 {
		pr := getWindowRect(p)
		x, y = x-int(pr.Left), y-int(pr.Top)
	}
	pMoveWindow.Call(uintptr(h), uintptr(x), uintptr(y), uintptr(r.W), uintptr(r.H), 1)
	s.applyRegion()
}

func (s *Shell) Scale() float64 {
	h := s.HWND()
	if h == 0 {
		return 1
	}
	dpi, _, _ := pGetDpiForWindow.Call(uintptr(h))
	if dpi == 0 {
		return 1
	}
	return float64(dpi) / 96
}

// BeginDrag 由前端标题栏 mousedown 触发：轮询鼠标位置直到左键释放。
// 不使用 HTCAPTION，因为窗口被 SetParent 到桌面层后系统拖动行为不可靠。
func (s *Shell) BeginDrag(onEnd func()) {
	if s.dragging.Swap(true) {
		return
	}
	go func() {
		defer s.dragging.Store(false)
		start, r0 := cursorPos(), s.Bounds()
		for keyDown(vkLButton) {
			p := cursorPos()
			s.moveResize(app.Rect{X: r0.X + int(p.X-start.X), Y: r0.Y + int(p.Y-start.Y), W: r0.W, H: r0.H})
			time.Sleep(8 * time.Millisecond)
		}
		s.SetBounds(s.Bounds()) // 松开后确保仍在某个显示器内
		if onEnd != nil {
			onEnd()
		}
	}()
}

func (s *Shell) BeginResize(minW, minH, maxW, maxH int, onEnd func()) {
	if s.dragging.Swap(true) {
		return
	}
	go func() {
		defer s.dragging.Store(false)
		start, r0 := cursorPos(), s.Bounds()
		clamp := func(v, lo, hi int) int {
			if v < lo {
				return lo
			}
			if v > hi {
				return hi
			}
			return v
		}
		for keyDown(vkLButton) {
			p := cursorPos()
			s.moveResize(app.Rect{X: r0.X, Y: r0.Y,
				W: clamp(r0.W+int(p.X-start.X), minW, maxW), H: clamp(r0.H+int(p.Y-start.Y), minH, maxH)})
			time.Sleep(8 * time.Millisecond)
		}
		if onEnd != nil {
			onEnd()
		}
	}()
}

// ---- 显示 / 焦点 ----

func (s *Shell) SetVisible(v bool) {
	h := s.HWND()
	if v {
		pShowWindow.Call(uintptr(h), swShowNoActivate)
	} else {
		pShowWindow.Call(uintptr(h), swHide)
	}
}

func (s *Shell) Visible() bool {
	r, _, _ := pIsWindowVisible.Call(uintptr(s.HWND()))
	return r != 0
}

func (s *Shell) Focus() {
	h := s.HWND()
	pShowWindow.Call(uintptr(h), swShow)
	pSetForegroundWindow.Call(uintptr(h))
}

func (s *Shell) Beep() { pMessageBeep.Call(0x40) }

func (s *Shell) Quit() {
	if s.loop != nil {
		s.loop.Close()
	}
	if s.quit != nil {
		s.quit()
	}
}

func (s *Shell) SetOverdueBadge(n int) {
	if s.loop != nil {
		s.loop.SetAlert(n > 0)
	}
}

func (s *Shell) OpenPath(path string) error {
	r, _, err := pShellExecuteW.Call(0, uintptr(unsafe.Pointer(utf16("open"))), uintptr(unsafe.Pointer(utf16(path))), 0, 0, 1)
	if r <= 32 {
		return fmt.Errorf("打开失败: %v", err)
	}
	return nil
}

// ---- 显示器 ----

func monitorRects() (all []rect, primary rect) {
	cb := syscall.NewCallback(func(hm, _, _, _ uintptr) uintptr {
		var mi monitorInfo
		mi.Size = uint32(unsafe.Sizeof(mi))
		pGetMonitorInfoW.Call(hm, uintptr(unsafe.Pointer(&mi)))
		all = append(all, mi.Work)
		if mi.Flags&1 != 0 {
			primary = mi.Work
		}
		return 1
	})
	pEnumDisplayMonitors.Call(0, 0, cb, 0)
	if primary == (rect{}) && len(all) > 0 {
		primary = all[0]
	}
	return
}

// clampToMonitors 保证窗口至少有一部分在某个显示器内；显示器不存在时回到主屏（FR-203 / AC-12）。
func clampToMonitors(r app.Rect) app.Rect {
	mons, primary := monitorRects()
	if len(mons) == 0 {
		return r
	}
	cx, cy := int32(r.X+r.W/2), int32(r.Y+r.H/2)
	target := primary
	for _, m := range mons {
		if cx >= m.Left && cx < m.Right && cy >= m.Top && cy < m.Bottom {
			target = m
			break
		}
	}
	// 不在任何显示器内 → 主屏；在内 → 仅把越界部分拉回工作区
	inside := false
	for _, m := range mons {
		if cx >= m.Left && cx < m.Right && cy >= m.Top && cy < m.Bottom {
			inside = true
		}
	}
	if !inside {
		r.X, r.Y = int(target.Left)+40, int(target.Top)+60
	}
	if r.W > int(target.Right-target.Left) {
		r.W = int(target.Right - target.Left)
	}
	if r.H > int(target.Bottom-target.Top) {
		r.H = int(target.Bottom - target.Top)
	}
	if r.X < int(target.Left) {
		r.X = int(target.Left)
	}
	if r.Y < int(target.Top) {
		r.Y = int(target.Top)
	}
	if r.X+r.W > int(target.Right) {
		r.X = int(target.Right) - r.W
	}
	if r.Y+r.H > int(target.Bottom) {
		r.Y = int(target.Bottom) - r.H
	}
	return r
}

func (s *Shell) CursorMonitorArea() app.Rect {
	p := cursorPos()
	hm, _, _ := pMonitorFromPoint.Call(uintptr(*(*int64)(unsafe.Pointer(&p))), monitorDefaultToPrimary)
	var mi monitorInfo
	mi.Size = uint32(unsafe.Sizeof(mi))
	pGetMonitorInfoW.Call(hm, uintptr(unsafe.Pointer(&mi)))
	w := mi.Work
	return app.Rect{X: int(w.Left), Y: int(w.Top), W: int(w.Right - w.Left), H: int(w.Bottom - w.Top)}
}

// ---- 文件对话框 ----

func (s *Shell) PickFile(save bool, title, defaultName, pattern string) (string, error) {
	buf := make([]uint16, 1024)
	if defaultName != "" {
		copy(buf, windows.StringToUTF16(defaultName))
	}
	// 过滤器：以 NUL 分隔、双 NUL 结尾
	filter := windows.StringToUTF16("文件 (" + pattern + ")")
	filter = append(filter, windows.StringToUTF16(pattern)...)
	filter = append(filter, windows.StringToUTF16("所有文件 (*.*)")...)
	filter = append(filter, windows.StringToUTF16("*.*")...)
	filter = append(filter, 0)
	ofn := openFileName{
		Owner:  s.HWND(),
		Filter: &filter[0],
		File:   &buf[0], MaxFile: uint32(len(buf)),
		Title: utf16(title),
		Flags: 0x00080000 | 0x00000800 | 0x00001000, // EXPLORER | PATHMUSTEXIST | FILEMUSTEXIST(open)
	}
	ofn.StructSize = uint32(unsafe.Sizeof(ofn))
	var proc *windows.LazyProc
	if save {
		ofn.Flags = 0x00080000 | 0x00000002 | 0x00000800 // EXPLORER | OVERWRITEPROMPT | PATHMUSTEXIST
		if len(pattern) > 2 {
			ofn.DefExt = utf16(pattern[2:])
		}
		proc = pGetSaveFileNameW
	} else {
		proc = pGetOpenFileNameW
	}
	r, _, _ := proc.Call(uintptr(unsafe.Pointer(&ofn)))
	if r == 0 {
		return "", nil // 用户取消
	}
	return windows.UTF16ToString(buf), nil
}

// ---- 窗口外观与鼠标穿透（FR-208）----

// exBits 记录哪些扩展样式位是我们加上的。关闭时只撤销这些位——
// 不再「还原到最初的整个样式值」，那样会把 Wails / 系统之后加上的位一并抹掉。
type exBits struct{ layered, transparent bool }

// NativeOpacity 表示透明度由窗口级 alpha 处理：前端不应再叠加 CSS opacity。
// （Wails 窗口本身不透明，背景是画刷；CSS opacity 只会让便签颜色叠在黑色背景上变暗。）
func (s *Shell) NativeOpacity() bool { return true }

// SetOpacity 设置窗口级透明度（0.3–1）。WS_EX_LAYERED + LWA_ALPHA 对整个窗口含 WebView2 子窗口生效。
func (s *Shell) SetOpacity(o float64) {
	if o < 0.05 {
		o = 0.05
	}
	if o > 1 {
		o = 1
	}
	a := byte(o*255 + 0.5)
	s.ctMu.Lock()
	changed := s.alpha != a
	s.alpha = a
	s.ctMu.Unlock()
	if changed {
		s.applyStyles()
	}
}

// SetBackground 设置窗口底色（#RRGGBB）：缩放、刷新时露出的是这个颜色而不是黑色。
func (s *Shell) SetBackground(hex string) {
	h := s.HWND()
	if h == 0 || len(hex) != 7 || hex[0] != '#' {
		return
	}
	var r, g, b uint8
	if _, err := fmt.Sscanf(hex[1:], "%02x%02x%02x", &r, &g, &b); err != nil {
		return
	}
	br, _, _ := pCreateSolidBrush.Call(uintptr(r) | uintptr(g)<<8 | uintptr(b)<<16)
	if br == 0 {
		return
	}
	old, _, _ := pSetClassLongPtrW.Call(uintptr(h), ^uintptr(9), br) // GCLP_HBRBACKGROUND = -10
	s.ctMu.Lock()
	prev := s.brush
	s.brush = br
	s.ctMu.Unlock()
	if prev != 0 && prev == old {
		pDeleteObject.Call(prev) // 只释放我们自己创建的旧画刷
	}
	pInvalidateRect.Call(uintptr(h), 0, 1)
}

// SetCornerRadius 用窗口区域把四个角裁成圆角（DIP）。窗口本身不透明，无法靠 CSS 做出透明的圆角。
func (s *Shell) SetCornerRadius(dip int) {
	s.ctMu.Lock()
	s.radius = dip
	s.rgnW, s.rgnH = 0, 0 // 强制重建
	s.ctMu.Unlock()
	s.applyRegion()
}

func (s *Shell) applyRegion() {
	h := s.HWND()
	if h == 0 {
		return
	}
	r := getWindowRect(h)
	w, hh := int(r.Right-r.Left), int(r.Bottom-r.Top)
	s.ctMu.Lock()
	rad := s.radius
	if w == s.rgnW && hh == s.rgnH {
		s.ctMu.Unlock()
		return
	}
	s.rgnW, s.rgnH = w, hh
	s.ctMu.Unlock()
	if rad <= 0 || w <= 0 || hh <= 0 {
		pSetWindowRgn.Call(uintptr(h), 0, 1)
		return
	}
	d := int(float64(rad)*s.Scale()) * 2
	rgn, _, _ := pCreateRoundRectRgn.Call(0, 0, uintptr(w+1), uintptr(hh+1), uintptr(d), uintptr(d))
	if rgn == 0 {
		return
	}
	if ok, _, _ := pSetWindowRgn.Call(uintptr(h), rgn, 1); ok == 0 {
		pDeleteObject.Call(rgn) // 设置成功后区域归系统所有；失败才需要自己释放
	}
}

// SetClickThrough 开启 / 关闭鼠标穿透。开启后窗口带上 WS_EX_LAYERED|WS_EX_TRANSPARENT，
// 鼠标点击落到下面的窗口；按住 Ctrl 时临时恢复交互。
//
// 检测 Ctrl 有两条互相独立的路径，任何一条都够用：
//  1. 键盘原始输入（RIDEV_INPUTSINK，WM_INPUT）：事件驱动，Ctrl 一按下 / 抬起立刻响应；
//  2. 开启期间每 200ms 比对一次「期望状态」与「已应用状态」的兜底：只在功能开启时才存在，
//     每次只读一个按键状态，几乎不占 CPU；原始输入即使因系统原因收不到事件，状态也会收敛。
func (s *Shell) SetClickThrough(on bool) {
	s.ctMu.Lock()
	s.ct = on
	running := s.stopPoll != nil
	s.ctMu.Unlock()
	if s.loop != nil {
		s.loop.WatchCtrl(on)
	}
	switch {
	case on && !running:
		s.startPoll()
	case !on && running:
		s.stopPolling()
	}
	s.applyStyles()
}

func (s *Shell) startPoll() {
	stop := make(chan struct{})
	s.ctMu.Lock()
	s.stopPoll = stop
	s.ctMu.Unlock()
	go func() {
		t := time.NewTicker(200 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				s.ctMu.Lock()
				ct := s.ct
				s.ctMu.Unlock()
				if want := ct && !keyDown(vkControl); want != s.lastOn.Load() {
					s.applyStyles()
				}
			}
		}
	}()
}

func (s *Shell) stopPolling() {
	s.ctMu.Lock()
	if s.stopPoll != nil {
		close(s.stopPoll)
		s.stopPoll = nil
	}
	s.ctMu.Unlock()
}

// OnCtrl 由消息循环在 Ctrl 键按下 / 抬起时调用。
func (s *Shell) OnCtrl() {
	select {
	case s.ctrlCh <- struct{}{}:
	default: // 已有待处理的刷新，合并
	}
}

// applyStyles 根据「穿透开关 / Ctrl 状态 / 透明度」重新计算并应用窗口扩展样式。幂等、单飞：
// 如果已有 goroutine 正在应用，这里只留下「需要再来一遍」的标记就返回，由那个 goroutine 负责补跑。
func (s *Shell) applyStyles() {
	s.dirty.Store(true)
	for s.dirty.Load() {
		if !s.applying.CompareAndSwap(false, true) {
			return
		}
		for s.dirty.Swap(false) {
			s.applyStylesOnce()
		}
		s.applying.Store(false)
	}
}

func (s *Shell) applyStylesOnce() {
	root := s.HWND()
	if root == 0 {
		return
	}
	targets := []windows.HWND{root}
	cb := syscall.NewCallback(func(h uintptr, _ uintptr) uintptr {
		targets = append(targets, windows.HWND(h))
		return 1
	})
	pEnumChildWindows.Call(uintptr(root), cb, 0)

	s.ctMu.Lock()
	ct, alpha := s.ct, s.alpha
	s.ctMu.Unlock()
	ctrl := keyDown(vkControl)
	on := ct && !ctrl
	interactive := ct && ctrl
	for h := range s.ctState { // 清理已销毁窗口的记录（句柄可能被复用）
		if ok, _, _ := pIsWindow.Call(uintptr(h)); ok == 0 {
			delete(s.ctState, h)
		}
	}
	changed, failed := 0, 0
	for _, h := range targets {
		isRoot := h == root
		wantTransparent := on
		wantLayered := on || (isRoot && alpha < 255)
		ex := getWindowLong(h, gwlExStyle)
		st := s.ctState[h]
		nx := ex
		if wantTransparent && nx&wsExTransparent == 0 {
			nx |= wsExTransparent
			st.transparent = true
		} else if !wantTransparent && st.transparent {
			nx &^= wsExTransparent
			st.transparent = false
		}
		if wantLayered && nx&wsExLayered == 0 {
			nx |= wsExLayered
			st.layered = true
		} else if !wantLayered && st.layered {
			nx &^= wsExLayered
			st.layered = false
		}
		s.ctState[h] = st
		if nx != ex {
			setWindowLong(h, gwlExStyle, nx)
			if getWindowLong(h, gwlExStyle) != nx {
				failed++ // 其他进程拥有的窗口（WebView2 的部分子窗口）不允许修改样式；根窗口生效即可
			} else {
				changed++
			}
			pSetWindowPos.Call(uintptr(h), 0, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoZOrder|swpNoActivate|swpFrameChanged)
		}
		if isRoot && getWindowLong(h, gwlExStyle)&wsExLayered != 0 {
			if ok, _, err := pSetLayeredWindowAttributes.Call(uintptr(h), 0, uintptr(alpha), lwaAlpha); ok == 0 {
				log.Printf("SetLayeredWindowAttributes 失败: %v", err)
			}
		}
	}
	if on != s.lastOn.Load() || interactive != s.interactive {
		log.Printf("窗口样式：鼠标穿透=%v（功能开启=%v，Ctrl=%v），窗口 %d 个，样式已修改 %d、被系统拒绝 %d，根窗口扩展样式=%#x，透明度=%d",
			on, ct, ctrl, len(targets), changed, failed, getWindowLong(root, gwlExStyle), alpha)
	}
	s.lastOn.Store(on)
	if interactive != s.interactive {
		s.interactive = interactive
		if cb := s.OnInteractive; cb != nil {
			go cb(interactive)
		}
	}
}
