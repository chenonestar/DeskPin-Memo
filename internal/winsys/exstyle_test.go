package winsys

import "testing"

func TestWantExBits(t *testing.T) {
	cases := []struct {
		name               string
		isRoot, ct         bool
		alpha              byte
		wantTrans, wantLay bool
	}{
		{"根窗口 默认", true, false, 255, false, false},
		{"根窗口 仅变淡", true, false, 100, false, true},
		{"根窗口 穿透", true, true, 255, true, true},
		{"根窗口 穿透+透明度", true, true, 100, true, true},
		// 过去的 bug：穿透时给 WebView2 子窗口也加了 WS_EX_LAYERED（从未设置 alpha）→ 整个界面黑块
		{"子窗口 默认", false, false, 255, false, false},
		{"子窗口 穿透时也不动", false, true, 255, false, false},
		{"子窗口 变淡时也不动", false, false, 100, false, false},
		{"子窗口 穿透+变淡", false, true, 100, false, false},
	}
	for _, c := range cases {
		tr, ly := wantExBits(c.isRoot, c.ct, c.alpha)
		if tr != c.wantTrans || ly != c.wantLay {
			t.Errorf("%s: got transparent=%v layered=%v", c.name, tr, ly)
		}
	}
}

func TestFramelessStyleRemovesCaptionButKeepsOthers(t *testing.T) {
	const overlapped = 0x00CF0000 // WS_OVERLAPPEDWINDOW
	const popup, visible, clip = 0x80000000, 0x10000000, 0x02000000
	in := uintptr(overlapped | popup | visible | clip)
	out := framelessStyle(in)
	if out&(styleCaption|styleSysMenu|styleThickFrame|styleMinimizeBox|styleMaximizeBox) != 0 {
		t.Fatalf("仍带有标题栏 / 边框样式位: %#x", out)
	}
	if out&(popup|visible|clip) != popup|visible|clip {
		t.Fatalf("不相关的样式位不应被改动: %#x", out)
	}
	if framelessStyle(out) != out {
		t.Fatal("应当幂等")
	}
}
