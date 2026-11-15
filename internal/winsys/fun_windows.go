//go:build windows

package winsys

import (
	"net/http"
	"net/url"
	"unsafe"

	"deskpinmemo/internal/feed"

	"golang.org/x/sys/windows"
)

var (
	winhttp                                = windows.NewLazySystemDLL("winhttp.dll")
	pWinHttpGetIEProxyConfigForCurrentUser = winhttp.NewProc("WinHttpGetIEProxyConfigForCurrentUser")
	pGetLastInputInfo                      = user32.NewProc("GetLastInputInfo")
	pGetTickCount                          = kernel32.NewProc("GetTickCount")
	pGlobalFree                            = kernel32.NewProc("GlobalFree")
)

// IdleSeconds 返回整个系统多久没有鼠标 / 键盘输入（狗狗据此打盹）。
func (s *Shell) IdleSeconds() int {
	var lii struct {
		cbSize uint32
		dwTime uint32
	}
	lii.cbSize = uint32(unsafe.Sizeof(lii))
	if r, _, _ := pGetLastInputInfo.Call(uintptr(unsafe.Pointer(&lii))); r == 0 {
		return 0
	}
	now, _, _ := pGetTickCount.Call()
	return int((uint32(now) - lii.dwTime) / 1000) // 无符号相减，49.7 天回绕时也正确
}

// SystemProxy 返回「跟随系统代理」时使用的代理函数：每次请求时读取 Windows 当前用户的代理设置
// （设置 → 网络和 Internet → 代理里的「手动设置代理」），用户改了设置无需重启程序。
// 只配置了「自动检测」或「使用设置脚本（PAC）」时退回读环境变量（PAC 暂不支持）。
func SystemProxy() feed.ProxyFunc {
	return func(req *http.Request) (*url.URL, error) {
		proxy, bypass, ok := ieProxyConfig()
		if !ok || proxy == "" {
			return http.ProxyFromEnvironment(req)
		}
		return feed.WinProxy(proxy, bypass)(req)
	}
}

func ieProxyConfig() (proxy, bypass string, ok bool) {
	var cfg struct {
		AutoDetect    int32
		AutoConfigURL *uint16
		Proxy         *uint16
		ProxyBypass   *uint16
	}
	if r, _, _ := pWinHttpGetIEProxyConfigForCurrentUser.Call(uintptr(unsafe.Pointer(&cfg))); r == 0 {
		return "", "", false
	}
	take := func(p *uint16) string {
		if p == nil {
			return ""
		}
		s := windows.UTF16PtrToString(p)
		pGlobalFree.Call(uintptr(unsafe.Pointer(p))) // 这些字符串由调用方负责释放
		return s
	}
	_ = take(cfg.AutoConfigURL)
	return take(cfg.Proxy), take(cfg.ProxyBypass), true
}
