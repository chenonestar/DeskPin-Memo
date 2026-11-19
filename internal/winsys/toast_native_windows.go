//go:build windows

package winsys

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"sync"
	"time"

	"git.sr.ht/~jackmordaunt/go-toast/v2/wintoast"
)

// 直接调用 Windows 通知接口（WinRT ToastNotificationManager）发送通知，不启动 PowerShell 等其他进程。
// 有的公司电脑用管理员策略禁止运行 PowerShell（日志里是「Access to %1 has been restricted by your Administrator
// by policy rule」），旧的发送方式在那里几乎每次都失败。
//
// 只调用 wintoast.Push：它在进程内初始化 COM 并发送，不调用 SetAppData，因此不写注册表（绿色版零写入不受影响）。
// 所有发送都在同一个锁定的系统线程上进行，避免 COM 初始化落在不同线程上。

type nativeJob struct {
	aumid, xml string
	done       chan error
}

var (
	nativeOnce sync.Once
	nativeJobs = make(chan nativeJob, 8)
)

// nativeDisabled：环境变量 DESKPIN_TOAST=powershell 时只用旧方式（排查问题用）。
var nativeDisabled = os.Getenv("DESKPIN_TOAST") == "powershell"

func pushNative(aumid, xml string) error {
	if nativeDisabled {
		return errors.New("已通过 DESKPIN_TOAST=powershell 关闭直接调用")
	}
	nativeOnce.Do(func() {
		go func() {
			runtime.LockOSThread() // COM 套间与线程绑定：始终在这一个线程上初始化和发送
			for j := range nativeJobs {
				j.done <- safeNativePush(j.aumid, j.xml)
			}
		}()
	})
	done := make(chan error, 1)
	select {
	case nativeJobs <- nativeJob{aumid, xml, done}:
	case <-time.After(5 * time.Second):
		return errors.New("直接调用排队超时")
	}
	select {
	case err := <-done:
		return err
	case <-time.After(10 * time.Second):
		return errors.New("直接调用超时")
	}
}

func safeNativePush(aumid, xml string) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("直接调用出错: %v", r)
		}
	}()
	return wintoast.Push(aumid, xml) // 不带任何选项：不会退回 PowerShell（由调用方决定是否兜底）
}
