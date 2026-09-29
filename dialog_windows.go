//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

// fatalDialog 在无法启动时弹出系统消息框（Wails 尚未创建窗口）。
func fatalDialog(msg string) {
	mb := syscall.NewLazyDLL("user32.dll").NewProc("MessageBoxW")
	t, _ := syscall.UTF16PtrFromString("桌面备忘钉")
	m, _ := syscall.UTF16PtrFromString(msg)
	mb.Call(0, uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(t)), 0x10)
}
