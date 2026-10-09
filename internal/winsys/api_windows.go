//go:build windows

// Package winsys 封装 Windows 专有能力：钉在桌面、置顶、托盘、全局热键、睡眠唤醒、Toast、开机自启。
// 所有 Win32 调用集中在此包（7.3、7.4）。
package winsys

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32   = windows.NewLazySystemDLL("user32.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")
	gdi32    = windows.NewLazySystemDLL("gdi32.dll")
	shell32  = windows.NewLazySystemDLL("shell32.dll")
	comdlg32 = windows.NewLazySystemDLL("comdlg32.dll")

	pFindWindowW                = user32.NewProc("FindWindowW")
	pFindWindowExW              = user32.NewProc("FindWindowExW")
	pEnumWindows                = user32.NewProc("EnumWindows")
	pSetWindowRgn               = user32.NewProc("SetWindowRgn")
	pSetClassLongPtrW           = user32.NewProc("SetClassLongPtrW")
	pInvalidateRect             = user32.NewProc("InvalidateRect")
	pCreateRoundRectRgn         = gdi32.NewProc("CreateRoundRectRgn")
	pCreateSolidBrush           = gdi32.NewProc("CreateSolidBrush")
	pDeleteObject               = gdi32.NewProc("DeleteObject")
	pEnumChildWindows           = user32.NewProc("EnumChildWindows")
	pSetLayeredWindowAttributes = user32.NewProc("SetLayeredWindowAttributes")
	pGetRawInputData            = user32.NewProc("GetRawInputData")
	pRegisterRawInputDevices    = user32.NewProc("RegisterRawInputDevices")
	pGetWindowThreadProcessId   = user32.NewProc("GetWindowThreadProcessId")
	pGetWindowTextW             = user32.NewProc("GetWindowTextW")
	pGetClassNameW              = user32.NewProc("GetClassNameW")
	pSetParent                  = user32.NewProc("SetParent")
	pGetParent                  = user32.NewProc("GetParent")
	pSetWindowPos               = user32.NewProc("SetWindowPos")
	pGetWindowRect              = user32.NewProc("GetWindowRect")
	pMoveWindow                 = user32.NewProc("MoveWindow")
	pShowWindow                 = user32.NewProc("ShowWindow")
	pIsWindowVisible            = user32.NewProc("IsWindowVisible")
	pIsWindow                   = user32.NewProc("IsWindow")
	pSetForegroundWindow        = user32.NewProc("SetForegroundWindow")
	pGetWindowLongPtrW          = user32.NewProc("GetWindowLongPtrW")
	pSetWindowLongPtrW          = user32.NewProc("SetWindowLongPtrW")
	pGetCursorPos               = user32.NewProc("GetCursorPos")
	pGetAsyncKeyState           = user32.NewProc("GetAsyncKeyState")
	pMonitorFromPoint           = user32.NewProc("MonitorFromPoint")
	pMonitorFromWindow          = user32.NewProc("MonitorFromWindow")
	pGetMonitorInfoW            = user32.NewProc("GetMonitorInfoW")
	pEnumDisplayMonitors        = user32.NewProc("EnumDisplayMonitors")
	pGetDpiForWindow            = user32.NewProc("GetDpiForWindow")
	pRegisterHotKey             = user32.NewProc("RegisterHotKey")
	pUnregisterHotKey           = user32.NewProc("UnregisterHotKey")
	pRegisterClassExW           = user32.NewProc("RegisterClassExW")
	pCreateWindowExW            = user32.NewProc("CreateWindowExW")
	pDefWindowProcW             = user32.NewProc("DefWindowProcW")
	pGetMessageW                = user32.NewProc("GetMessageW")
	pTranslateMessage           = user32.NewProc("TranslateMessage")
	pDispatchMessageW           = user32.NewProc("DispatchMessageW")
	pPostMessageW               = user32.NewProc("PostMessageW")
	pPostQuitMessage            = user32.NewProc("PostQuitMessage")
	pRegisterWindowMessageW     = user32.NewProc("RegisterWindowMessageW")
	pCreatePopupMenu            = user32.NewProc("CreatePopupMenu")
	pAppendMenuW                = user32.NewProc("AppendMenuW")
	pTrackPopupMenu             = user32.NewProc("TrackPopupMenu")
	pDestroyMenu                = user32.NewProc("DestroyMenu")
	pLoadImageW                 = user32.NewProc("LoadImageW")
	pDestroyIcon                = user32.NewProc("DestroyIcon")
	pMessageBeep                = user32.NewProc("MessageBeep")
	pSystemParametersInfoW      = user32.NewProc("SystemParametersInfoW")
	pSetProcessDPIAware         = user32.NewProc("SetProcessDpiAwarenessContext")
	pShellNotifyIconW           = shell32.NewProc("Shell_NotifyIconW")
	pShellExecuteW              = shell32.NewProc("ShellExecuteW")
	pGetSaveFileNameW           = comdlg32.NewProc("GetSaveFileNameW")
	pGetOpenFileNameW           = comdlg32.NewProc("GetOpenFileNameW")
	pGetModuleHandleW           = kernel32.NewProc("GetModuleHandleW")
)

const (
	wsPopup   = 0x80000000
	wsChild   = 0x40000000
	wsVisible = 0x10000000

	wsExToolWindow  = 0x00000080
	wsExAppWindow   = 0x00040000
	wsExNoActivate  = 0x08000000
	wsExLayered     = 0x00080000
	wsExTransparent = 0x00000020
	lwaAlpha        = 0x00000002
	vkControl       = 0x11

	gwlStyle   = -16
	gwlExStyle = -20

	swpNoSize       = 0x0001
	swpNoMove       = 0x0002
	swpNoZOrder     = 0x0004
	swpNoActivate   = 0x0010
	swpShowWindow   = 0x0040
	swpFrameChanged = 0x0020

	swHide           = 0
	swShowNoActivate = 4
	swShow           = 5

	vkLButton = 0x01

	wmHotkey         = 0x0312
	wmCommand        = 0x0111
	wmDestroy        = 0x0002
	wmTimeChange     = 0x001E
	wmPowerBroadcast = 0x0218
	wmDisplayChange  = 0x007E
	wmEndSession     = 0x0016
	wmApp            = 0x8000
	wmTray           = wmApp + 1
	wmUser           = 0x0400
	wmLButtonUp      = 0x0202
	wmRButtonUp      = 0x0205
	wmLButtonDblClk  = 0x0203
	wmContextMenu    = 0x007B

	pbtApmResumeAutomatic = 0x0012
	pbtApmResumeSuspend   = 0x0007

	modAlt      = 0x0001
	modControl  = 0x0002
	modShift    = 0x0004
	modWin      = 0x0008
	modNoRepeat = 0x4000

	nimAdd     = 0
	nimModify  = 1
	nimDelete  = 2
	nifMessage = 0x1
	nifIcon    = 0x2
	nifTip     = 0x4

	mfString    = 0x0
	mfSeparator = 0x800
	mfChecked   = 0x8

	tpmRightButton = 0x0002
	tpmReturnCmd   = 0x0100

	imageIcon      = 1
	lrLoadFromFile = 0x10

	monitorDefaultToNearest = 2
	monitorDefaultToPrimary = 1
)

// 特殊 HWND 值：HWND_TOP=0、HWND_BOTTOM=1、HWND_TOPMOST=-1、HWND_NOTOPMOST=-2。
var (
	hwndTopZ      = uintptr(0)
	hwndBottom    = uintptr(1)
	hwndTopmost   = ^uintptr(0)
	hwndNoTopmost = ^uintptr(1)
)

type point struct{ X, Y int32 }
type rect struct{ Left, Top, Right, Bottom int32 }

type monitorInfo struct {
	Size    uint32
	Monitor rect
	Work    rect
	Flags   uint32
}

type wndClassEx struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   windows.Handle
	Icon       windows.Handle
	Cursor     windows.Handle
	Background windows.Handle
	MenuName   *uint16
	ClassName  *uint16
	IconSm     windows.Handle
}

type msg struct {
	Hwnd    windows.HWND
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      point
}

type notifyIconData struct {
	Size            uint32
	Wnd             windows.HWND
	ID              uint32
	Flags           uint32
	CallbackMessage uint32
	Icon            windows.Handle
	Tip             [128]uint16
	State           uint32
	StateMask       uint32
	Info            [256]uint16
	Version         uint32
	InfoTitle       [64]uint16
	InfoFlags       uint32
	GuidItem        windows.GUID
	BalloonIcon     windows.Handle
}

type openFileName struct {
	StructSize    uint32
	Owner         windows.HWND
	Instance      windows.Handle
	Filter        *uint16
	CustomFilter  *uint16
	MaxCustFilter uint32
	FilterIndex   uint32
	File          *uint16
	MaxFile       uint32
	FileTitle     *uint16
	MaxFileTitle  uint32
	InitialDir    *uint16
	Title         *uint16
	Flags         uint32
	FileOffset    uint16
	FileExtension uint16
	DefExt        *uint16
	CustData      uintptr
	FnHook        uintptr
	TemplateName  *uint16
	PvReserved    uintptr
	DwReserved    uint32
	FlagsEx       uint32
}

func utf16(s string) *uint16 {
	p, _ := windows.UTF16PtrFromString(s)
	return p
}

func getWindowLong(h windows.HWND, idx int32) uintptr {
	r, _, _ := pGetWindowLongPtrW.Call(uintptr(h), uintptr(idx))
	return r
}

func setWindowLong(h windows.HWND, idx int32, v uintptr) {
	pSetWindowLongPtrW.Call(uintptr(h), uintptr(idx), v)
}

func getWindowRect(h windows.HWND) rect {
	var r rect
	pGetWindowRect.Call(uintptr(h), uintptr(unsafe.Pointer(&r)))
	return r
}

func cursorPos() point {
	var p point
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&p)))
	return p
}

func keyDown(vk int) bool {
	r, _, _ := pGetAsyncKeyState.Call(uintptr(vk))
	return r&0x8000 != 0
}

func windowText(h windows.HWND) string {
	buf := make([]uint16, 256)
	n, _, _ := pGetWindowTextW.Call(uintptr(h), uintptr(unsafe.Pointer(&buf[0])), 256)
	return windows.UTF16ToString(buf[:n])
}

func className(h windows.HWND) string {
	buf := make([]uint16, 256)
	n, _, _ := pGetClassNameW.Call(uintptr(h), uintptr(unsafe.Pointer(&buf[0])), 256)
	return windows.UTF16ToString(buf[:n])
}
