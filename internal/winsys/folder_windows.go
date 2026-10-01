//go:build windows

package winsys

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	ole32                = windows.NewLazySystemDLL("ole32.dll")
	pCoInitializeEx      = ole32.NewProc("CoInitializeEx")
	pCoUninitialize      = ole32.NewProc("CoUninitialize")
	pCoTaskMemFree       = ole32.NewProc("CoTaskMemFree")
	pSHBrowseForFolderW  = shell32.NewProc("SHBrowseForFolderW")
	pSHGetPathFromIDList = shell32.NewProc("SHGetPathFromIDListW")
)

type browseInfo struct {
	Owner       windows.HWND
	Root        uintptr
	DisplayName *uint16
	Title       *uint16
	Flags       uint32
	Callback    uintptr
	LParam      uintptr
	Image       int32
}

const (
	bifReturnOnlyFSDirs = 0x0001
	bifNewDialogStyle   = 0x0040
	coinitApartment     = 0x2
)

// PickFolder 弹出系统「选择文件夹」对话框（FR-605）。对话框需要 STA 的 COM 环境，
// 因此固定到一个 OS 线程并临时初始化 COM。
func (s *Shell) PickFolder(title string) (string, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	hr, _, _ := pCoInitializeEx.Call(0, coinitApartment)
	if int32(hr) >= 0 { // S_OK / S_FALSE 都需要配对 CoUninitialize
		defer pCoUninitialize.Call()
	}
	display := make([]uint16, windows.MAX_PATH)
	bi := browseInfo{Owner: s.HWND(), DisplayName: &display[0], Title: utf16(title), Flags: bifReturnOnlyFSDirs | bifNewDialogStyle}
	pidl, _, _ := pSHBrowseForFolderW.Call(uintptr(unsafe.Pointer(&bi)))
	if pidl == 0 {
		return "", nil // 用户取消
	}
	defer pCoTaskMemFree.Call(pidl)
	buf := make([]uint16, windows.MAX_PATH)
	if ok, _, _ := pSHGetPathFromIDList.Call(pidl, uintptr(unsafe.Pointer(&buf[0]))); ok == 0 {
		return "", fmt.Errorf("所选位置不是文件夹")
	}
	return windows.UTF16ToString(buf), nil
}

// Restart 退出并重新启动自己。单实例锁在本进程退出前不会释放，所以由一个隐藏的 cmd
// 先等待约 2 秒再启动新进程。
func (s *Shell) Restart() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command("cmd.exe")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CmdLine:       fmt.Sprintf(`cmd.exe /C ping -n 3 127.0.0.1 >nul & start "" "%s"`, exe),
		HideWindow:    true,
		CreationFlags: createNoWindow | 0x00000008, // CREATE_NO_WINDOW | DETACHED_PROCESS
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("无法重新启动: %w", err)
	}
	_ = cmd.Process.Release()
	s.Quit()
	return nil
}
