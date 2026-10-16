package winsys

// 窗口样式的纯决策函数（不依赖 Win32，可在任意平台测试）。

const (
	styleCaption     = 0x00C00000 // WS_CAPTION = WS_BORDER | WS_DLGFRAME
	styleSysMenu     = 0x00080000
	styleThickFrame  = 0x00040000
	styleMinimizeBox = 0x00020000
	styleMaximizeBox = 0x00010000
)

// framelessStyle 去掉所有会画出系统标题栏 / 边框的样式位。
// Wails 创建窗口时用的是 WS_OVERLAPPEDWINDOW，「无边框」完全依赖 WM_NCCALCSIZE 把非客户区压成 0；
// 一旦某次样式改动（切换钉桌面 / 置顶、分层窗口等）之后这一步没有重新生效，系统就会画出一条灰色标题栏
// （标题文字 + 关闭按钮）。直接去掉这些位，让窗口根本没有标题栏可画。本程序自己负责拖动和缩放。
func framelessStyle(style uintptr) uintptr {
	return style &^ (styleCaption | styleSysMenu | styleThickFrame | styleMinimizeBox | styleMaximizeBox)
}

// wantExBits 决定某个窗口应带哪些扩展样式。
//
// 鼠标穿透（WS_EX_TRANSPARENT|WS_EX_LAYERED）和窗口级透明度（WS_EX_LAYERED + alpha）都只作用于**根窗口**：
//   - 顶层窗口带 WS_EX_TRANSPARENT|WS_EX_LAYERED 时，系统做命中测试会整个跳过它（含全部子窗口），
//     子窗口不需要任何样式；
//   - 给子窗口（WebView2 的宿主窗口）加 WS_EX_LAYERED 而又不调用 SetLayeredWindowAttributes，
//     分层子窗口不会被绘制，整个界面显示为一块黑色。
func wantExBits(isRoot, clickThrough bool, alpha byte) (transparent, layered bool) {
	if !isRoot {
		return false, false
	}
	return clickThrough, clickThrough || alpha < 255
}
