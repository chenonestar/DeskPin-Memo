//go:build windows

package winsys

import (
	"embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

//go:embed icons/*
var iconFS embed.FS

// 托盘菜单命令（FR-209）。
const (
	CmdNew          = 1
	CmdToggle       = 2
	CmdSettings     = 3
	CmdQuit         = 4
	CmdClickThrough = 5
)

const (
	hotkeyQuick     = 1
	hotkeyToggle    = 2
	hotkeyClickThru = 3
	wmRunOnLoop     = wmUser + 2
	wmSettingChange = 0x001A
	wmInput         = 0x00FF
	ridInput        = 0x10000003
	ridevRemove     = 0x00000001
	ridevInputSink  = 0x00000100
)

type rawInputDevice struct {
	UsagePage uint16
	Usage     uint16
	Flags     uint32
	Target    windows.HWND
}

// Callbacks 是消息循环触发的回调，均在独立 goroutine 中调用，不会阻塞消息泵。
type Callbacks struct {
	OnQuick        func()
	OnToggle       func()
	OnClickThrough func()
	OnResume       func() // 睡眠唤醒（FR-306）
	OnTimeChange   func() // 系统时间 / 时区改变（NFR-05）
	OnTaskbar      func() // 资源管理器重启（AC-03）
	OnDisplay      func() // 显示器配置变化
	OnMenu         func(cmd int)
	OnEndSession   func()
	OnCtrl         func()      // Ctrl 键按下 / 抬起（仅在鼠标穿透开启期间才会收到）
	ClickThroughOn func() bool // 托盘菜单勾选状态
}

// Loop 是隐藏窗口 + 消息循环：热键、托盘、电源、任务栏重建、时间变更都经它接收。
type Loop struct {
	cb          Callbacks
	hwnd        windows.HWND
	dir         string
	ready       chan error
	run         chan func()
	mu          sync.Mutex
	alert       bool
	dark        bool
	icons       map[string]windows.Handle
	taskbar     uint32
	closed      bool
	toggleLabel string
}

// StartLoop 启动消息循环线程。dataDir 用于释放托盘图标文件。
func StartLoop(cb Callbacks, dataDir string) (*Loop, error) {
	l := &Loop{cb: cb, dir: filepath.Join(dataDir, "icons"), ready: make(chan error, 1),
		run: make(chan func(), 16), icons: map[string]windows.Handle{}}
	go l.thread()
	if err := <-l.ready; err != nil {
		return nil, err
	}
	return l, nil
}

var loopSelf *Loop // WndProc 是全局回调，用包级变量找回实例

func (l *Loop) thread() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	loopSelf = l
	if err := l.extractIcons(); err != nil {
		l.ready <- err
		return
	}
	inst, _, _ := pGetModuleHandleW.Call(0)
	cls := utf16("DeskPinMemoMessageWindow")
	wc := wndClassEx{WndProc: syscall.NewCallback(wndProc), Instance: windows.Handle(inst), ClassName: cls}
	wc.Size = uint32(unsafe.Sizeof(wc))
	if r, _, err := pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
		l.ready <- fmt.Errorf("注册窗口类失败: %v", err)
		return
	}
	// 必须是普通顶层（隐藏）窗口，message-only 窗口收不到广播消息（TaskbarCreated / 电源 / 时间）
	h, _, err := pCreateWindowExW.Call(wsExToolWindow, uintptr(unsafe.Pointer(cls)), uintptr(unsafe.Pointer(utf16("DeskPinMemoMsg"))),
		0, 0, 0, 0, 0, 0, 0, inst, 0)
	if h == 0 {
		l.ready <- fmt.Errorf("创建消息窗口失败: %v", err)
		return
	}
	l.hwnd = windows.HWND(h)
	tb, _, _ := pRegisterWindowMessageW.Call(uintptr(unsafe.Pointer(utf16("TaskbarCreated"))))
	l.taskbar = uint32(tb)
	l.dark = taskbarIsDark()
	l.addTray()
	l.ready <- nil

	var m msg
	for {
		r, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		pDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
	l.deleteTray()
}

func (l *Loop) extractIcons() error {
	if err := os.MkdirAll(l.dir, 0o755); err != nil {
		return err
	}
	ents, err := iconFS.ReadDir("icons")
	if err != nil {
		return err
	}
	for _, e := range ents {
		if !strings.HasSuffix(e.Name(), ".ico") {
			continue
		}
		b, _ := iconFS.ReadFile("icons/" + e.Name())
		if err := os.WriteFile(filepath.Join(l.dir, e.Name()), b, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func (l *Loop) icon(name string) windows.Handle {
	if h, ok := l.icons[name]; ok {
		return h
	}
	r, _, _ := pLoadImageW.Call(0, uintptr(unsafe.Pointer(utf16(filepath.Join(l.dir, name)))), imageIcon, 16, 16, lrLoadFromFile)
	l.icons[name] = windows.Handle(r)
	return windows.Handle(r)
}

func (l *Loop) trayIconName() string {
	n := "tray-light"
	if l.dark {
		n = "tray-dark"
	}
	if l.alert {
		n += "-alert"
	}
	return n + ".ico"
}

func (l *Loop) nid() notifyIconData {
	var d notifyIconData
	d.Size = uint32(unsafe.Sizeof(d))
	d.Wnd = l.hwnd
	d.ID = 1
	d.Flags = nifMessage | nifIcon | nifTip
	d.CallbackMessage = wmTray
	d.Icon = l.icon(l.trayIconName())
	copy(d.Tip[:], windows.StringToUTF16("桌面备忘钉 DeskPin Memo"))
	return d
}

func (l *Loop) addTray() {
	d := l.nid()
	pShellNotifyIconW.Call(nimAdd, uintptr(unsafe.Pointer(&d)))
}

func (l *Loop) modifyTray() {
	d := l.nid()
	pShellNotifyIconW.Call(nimModify, uintptr(unsafe.Pointer(&d)))
}

func (l *Loop) deleteTray() {
	d := l.nid()
	pShellNotifyIconW.Call(nimDelete, uintptr(unsafe.Pointer(&d)))
}

// SetAlert 有逾期事项时托盘图标右下角显示红点。
func (l *Loop) SetAlert(on bool) {
	l.do(func() {
		if l.alert != on {
			l.alert = on
			l.modifyTray()
		}
	})
}

// do 把函数投递到消息循环线程执行（RegisterHotKey 等必须在窗口所属线程调用）。
func (l *Loop) do(f func()) {
	l.mu.Lock()
	closed := l.closed
	l.mu.Unlock()
	if closed || l.hwnd == 0 {
		return
	}
	l.run <- f
	pPostMessageW.Call(uintptr(l.hwnd), wmRunOnLoop, 0, 0)
}

// Close 结束消息循环并移除托盘图标。
func (l *Loop) Close() {
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return
	}
	l.closed = true
	l.mu.Unlock()
	pPostMessageW.Call(uintptr(l.hwnd), wmDestroy, 0, 0)
}

// WatchCtrl 开启 / 关闭键盘原始输入监听（RIDEV_INPUTSINK：窗口不在前台也能收到 WM_INPUT）。
// 关闭鼠标穿透时取消注册，不再有任何额外开销。
func (l *Loop) WatchCtrl(on bool) {
	l.do(func() {
		d := rawInputDevice{UsagePage: 1, Usage: 6} // 通用桌面 / 键盘
		if on {
			d.Flags, d.Target = ridevInputSink, l.hwnd
		} else {
			d.Flags = ridevRemove
		}
		pRegisterRawInputDevices.Call(uintptr(unsafe.Pointer(&d)), 1, unsafe.Sizeof(d))
	})
}

// isCtrlRawInput 判断 WM_INPUT 是否为 Ctrl 键事件（x64 布局：RAWINPUTHEADER 24 字节，RAWKEYBOARD.VKey 在偏移 30）。
func isCtrlRawInput(hRawInput uintptr) bool {
	var buf [48]byte
	size := uint32(len(buf))
	n, _, _ := pGetRawInputData.Call(hRawInput, ridInput, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)), 24)
	if int32(n) <= 0 || *(*uint32)(unsafe.Pointer(&buf[0])) != 1 { // RIM_TYPEKEYBOARD
		return false
	}
	vk := *(*uint16)(unsafe.Pointer(&buf[30]))
	return vk == vkControl || vk == 0xA2 || vk == 0xA3 // VK_CONTROL / VK_LCONTROL / VK_RCONTROL
}

// ---- 全局热键（FR-501）----

// ParseHotkey 解析 "Ctrl+Alt+N" 形式的快捷键。
func ParseHotkey(s string) (mods uint32, vk uint32, err error) {
	parts := strings.Split(strings.ReplaceAll(strings.ToUpper(s), " ", ""), "+")
	if len(parts) < 2 {
		return 0, 0, errors.New("快捷键至少需要一个修饰键和一个按键")
	}
	for _, p := range parts[:len(parts)-1] {
		switch p {
		case "CTRL", "CONTROL":
			mods |= modControl
		case "ALT":
			mods |= modAlt
		case "SHIFT":
			mods |= modShift
		case "WIN", "META":
			mods |= modWin
		default:
			return 0, 0, fmt.Errorf("未知修饰键 %q", p)
		}
	}
	if mods == 0 {
		return 0, 0, errors.New("缺少修饰键")
	}
	k := parts[len(parts)-1]
	switch {
	case len(k) == 1 && k[0] >= 'A' && k[0] <= 'Z', len(k) == 1 && k[0] >= '0' && k[0] <= '9':
		vk = uint32(k[0])
	case len(k) >= 2 && k[0] == 'F':
		var n int
		if _, e := fmt.Sscanf(k, "F%d", &n); e != nil || n < 1 || n > 24 {
			return 0, 0, fmt.Errorf("未知按键 %q", k)
		}
		vk = uint32(0x70 + n - 1)
	default:
		return 0, 0, fmt.Errorf("未知按键 %q", k)
	}
	return mods, vk, nil
}

// RegisterHotkeys 注册（或重新注册）全局热键。注册失败即判定冲突（7.3），返回冲突说明。
func (l *Loop) RegisterHotkeys(quick, toggle, clickThrough string) (conflicts []string) {
	done := make(chan []string, 1)
	l.do(func() {
		var out []string
		pUnregisterHotKey.Call(uintptr(l.hwnd), hotkeyQuick)
		pUnregisterHotKey.Call(uintptr(l.hwnd), hotkeyToggle)
		pUnregisterHotKey.Call(uintptr(l.hwnd), hotkeyClickThru)
		for id, hk := range map[uintptr]string{hotkeyQuick: quick, hotkeyToggle: toggle, hotkeyClickThru: clickThrough} {
			mods, vk, err := ParseHotkey(hk)
			if err != nil {
				out = append(out, fmt.Sprintf("%s：%v", hk, err))
				continue
			}
			if r, _, _ := pRegisterHotKey.Call(uintptr(l.hwnd), id, uintptr(mods|modNoRepeat), uintptr(vk)); r == 0 {
				out = append(out, hk+" 已被其他程序占用，请在设置中更换")
			}
		}
		done <- out
	})
	return <-done
}

// ---- 窗口过程 ----

func wndProc(hwnd, message, wParam, lParam uintptr) uintptr {
	l := loopSelf
	if l == nil {
		r, _, _ := pDefWindowProcW.Call(hwnd, message, wParam, lParam)
		return r
	}
	safe := func(f func()) {
		if f != nil {
			go f()
		}
	}
	switch uint32(message) {
	case wmRunOnLoop:
		for {
			select {
			case f := <-l.run:
				f()
			default:
				return 0
			}
		}
	case wmHotkey:
		switch wParam {
		case hotkeyQuick:
			safe(l.cb.OnQuick)
		case hotkeyToggle:
			safe(l.cb.OnToggle)
		case hotkeyClickThru:
			safe(l.cb.OnClickThrough)
		}
		return 0
	case wmPowerBroadcast:
		if wParam == pbtApmResumeAutomatic || wParam == pbtApmResumeSuspend {
			safe(l.cb.OnResume)
		}
		return 1
	case wmTimeChange:
		safe(l.cb.OnTimeChange)
		return 0
	case wmSettingChange:
		nd := taskbarIsDark()
		if nd != l.dark {
			l.dark = nd
			l.modifyTray()
		}
		return 0
	case wmInput:
		if l.cb.OnCtrl != nil && isCtrlRawInput(lParam) {
			l.cb.OnCtrl()
		}
		r, _, _ := pDefWindowProcW.Call(hwnd, message, wParam, lParam) // RIM_INPUTSINK 要求交给 DefWindowProc 清理
		return r
	case wmDisplayChange:
		safe(l.cb.OnDisplay)
		return 0
	case wmEndSession:
		if wParam != 0 {
			safe(l.cb.OnEndSession)
		}
		return 0
	case wmTray:
		switch uint32(lParam) {
		case wmLButtonUp, wmLButtonDblClk:
			safe(l.cb.OnToggle)
		case wmRButtonUp, wmContextMenu:
			l.showMenu()
		}
		return 0
	case wmCommand:
		if cmd := int(wParam & 0xFFFF); cmd >= CmdNew && cmd <= CmdClickThrough && l.cb.OnMenu != nil {
			c := cmd
			go l.cb.OnMenu(c)
		}
		return 0
	case wmDestroy:
		l.deleteTray()
		pPostQuitMessage.Call(0)
		return 0
	}
	if l.taskbar != 0 && uint32(message) == l.taskbar { // 资源管理器重启：重建托盘并重新钉桌面
		l.addTray()
		safe(l.cb.OnTaskbar)
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, message, wParam, lParam)
	return r
}

func (l *Loop) showMenu() {
	m, _, _ := pCreatePopupMenu.Call()
	defer pDestroyMenu.Call(m)
	add := func(id uintptr, text string) {
		pAppendMenuW.Call(m, mfString, id, uintptr(unsafe.Pointer(utf16(text))))
	}
	add(CmdNew, "新建事项\tCtrl+Alt+N")
	add(CmdToggle, "显示/隐藏全部便签\tCtrl+Alt+M")
	ctFlag := uintptr(mfString)
	if l.cb.ClickThroughOn != nil && l.cb.ClickThroughOn() {
		ctFlag |= mfChecked
	}
	pAppendMenuW.Call(m, ctFlag, CmdClickThrough, uintptr(unsafe.Pointer(utf16("鼠标穿透（按住 Ctrl 临时操作）"))))
	add(CmdSettings, "设置")
	pAppendMenuW.Call(m, mfSeparator, 0, 0)
	add(CmdQuit, "退出")
	var p point
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&p)))
	pSetForegroundWindow.Call(uintptr(l.hwnd)) // 否则点击菜单外部菜单不会自动消失
	r, _, _ := pTrackPopupMenu.Call(m, tpmRightButton|tpmReturnCmd, uintptr(p.X), uintptr(p.Y), 0, uintptr(l.hwnd), 0)
	if r != 0 && l.cb.OnMenu != nil {
		go l.cb.OnMenu(int(r))
	}
}

func taskbarIsDark() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	v, _, err := k.GetIntegerValue("SystemUsesLightTheme")
	return err == nil && v == 0
}
