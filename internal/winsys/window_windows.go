//go:build windows

package winsys

import (
	"errors"
	"fmt"
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

	// 鼠标穿透（FR-208）
	ctMu   sync.Mutex
	ct     bool                     // 功能是否开启（不含按住 Ctrl 的临时恢复）
	ctOrig map[windows.HWND]uintptr // 修改前的扩展样式，关闭穿透时原样恢复
	ctrlCh chan struct{}
}

// ErrPinFallback 表示「钉在桌面」不可用，已降级为「置底窗口」（10.1 风险应对）。
var ErrPinFallback = errors.New("当前系统不支持嵌入桌面层，已降级为置底窗口")

var _ app.Shell = (*Shell)(nil)

// NewShell 创建外壳；quit 用于退出程序。窗口句柄稍后通过 Attach 绑定。
func NewShell(quit func()) *Shell {
	s := &Shell{quit: quit, mode: "desktop", ctOrig: map[windows.HWND]uintptr{}, ctrlCh: make(chan struct{}, 1)}
	go func() {
		for range s.ctrlCh { // 串行处理 Ctrl 状态变化；以实际按键状态为准，不依赖事件顺序
			s.refreshClickThrough()
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
}

func (s *Shell) moveResize(r app.Rect) {
	h := s.HWND()
	x, y := r.X, r.Y
	if p := getParentRaw(h); p != 0 {
		pr := getWindowRect(p)
		x, y = x-int(pr.Left), y-int(pr.Top)
	}
	pMoveWindow.Call(uintptr(h), uintptr(x), uintptr(y), uintptr(r.W), uintptr(r.H), 1)
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

// ---- 鼠标穿透（FR-208）----

// SetClickThrough 开启 / 关闭鼠标穿透。开启后窗口（含 WebView2 的全部子窗口）带上
// WS_EX_LAYERED|WS_EX_TRANSPARENT，鼠标点击落到下面的窗口；按住 Ctrl 时临时恢复交互。
// Ctrl 状态通过后台原始输入（RIDEV_INPUTSINK）事件驱动检测，没有轮询，空闲时不占 CPU（NFR-03）。
func (s *Shell) SetClickThrough(on bool) {
	s.ctMu.Lock()
	s.ct = on
	s.ctMu.Unlock()
	if s.loop != nil {
		s.loop.WatchCtrl(on)
	}
	s.refreshClickThrough()
}

// OnCtrl 由消息循环在 Ctrl 键按下 / 抬起时调用。
func (s *Shell) OnCtrl() {
	select {
	case s.ctrlCh <- struct{}{}:
	default: // 已有待处理的刷新，合并
	}
}

func (s *Shell) refreshClickThrough() {
	s.ctMu.Lock()
	want := s.ct
	s.ctMu.Unlock()
	s.applyTransparent(want && !keyDown(vkControl))
}

func (s *Shell) applyTransparent(on bool) {
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
	defer s.ctMu.Unlock()
	for h := range s.ctOrig { // 清理已销毁窗口的记录（句柄可能被复用）
		if ok, _, _ := pIsWindow.Call(uintptr(h)); ok == 0 {
			delete(s.ctOrig, h)
		}
	}
	for _, h := range targets {
		ex := getWindowLong(h, gwlExStyle)
		if _, seen := s.ctOrig[h]; !seen {
			s.ctOrig[h] = ex
		}
		if on {
			if ex&wsExTransparent != 0 && ex&wsExLayered != 0 {
				continue
			}
			setWindowLong(h, gwlExStyle, ex|wsExLayered|wsExTransparent)
			pSetLayeredWindowAttributes.Call(uintptr(h), 0, 255, lwaAlpha) // 完全不透明；没有这一句分层窗口不会显示
		} else {
			orig := s.ctOrig[h]
			if ex == orig {
				continue
			}
			setWindowLong(h, gwlExStyle, orig)
		}
		pSetWindowPos.Call(uintptr(h), 0, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoZOrder|swpNoActivate|swpFrameChanged)
	}
}
