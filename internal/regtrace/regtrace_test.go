package regtrace

import (
	"strings"
	"testing"
)

const exeA = `D:\Tools\DeskPinMemo\DeskPinMemo.exe`
const exeB = `E:\Moved\DeskPinMemo.exe`

func alive(paths ...string) func(string) bool {
	return func(p string) bool {
		for _, a := range paths {
			if strings.EqualFold(a, p) {
				return true
			}
		}
		return false
	}
}

func TestRegisterWritesAndIsIdempotent(t *testing.T) {
	r := NewMemRegistry()
	changed, err := Register(r, exeA, `C:\cfg\icons\notify.png`)
	if err != nil || !changed {
		t.Fatal(changed, err)
	}
	if cmd, _ := r.GetString(ProtocolKey+`\shell\open\command`, ""); cmd != `"D:\Tools\DeskPinMemo\DeskPinMemo.exe" "%1"` {
		t.Fatal(cmd)
	}
	if n, _ := r.GetString(AUMIDKey, "DisplayName"); n != "桌面备忘钉" {
		t.Fatal(n)
	}
	// 再次启动：没有变化，不重复写
	if changed, _ := Register(r, exeA, `C:\cfg\icons\notify.png`); changed {
		t.Fatal("内容相同不应重复写入")
	}
	// 文件夹被移动：协议命令随之更新（通知按钮继续可用）
	changed, _ = Register(r, exeB, `C:\cfg\icons\notify.png`)
	if cmd, _ := r.GetString(ProtocolKey+`\shell\open\command`, ""); !changed || !strings.Contains(cmd, exeB) {
		t.Fatalf("移动后协议命令应更新: %v %s", changed, cmd)
	}
}

func TestParseRunCommand(t *testing.T) {
	for in, want := range map[string]string{
		`"D:\a b\x.exe" --autostart`: `D:\a b\x.exe`,
		`D:\a\x.exe --autostart`:     `D:\a\x.exe`,
		`D:\a\x.exe`:                 `D:\a\x.exe`,
		`  "C:\x.exe"  `:             `C:\x.exe`,
	} {
		if got := ParseRunCommand(in); got != want {
			t.Errorf("%q → %q want %q", in, got, want)
		}
	}
}

func TestSetAutostartForced(t *testing.T) {
	r := NewMemRegistry()
	SetAutostart(r, exeA, true)
	if v, _ := r.GetString(RunKey, RunValue); v != `"`+exeA+`" --autostart` {
		t.Fatal(v)
	}
	SetAutostart(r, exeA, false)
	if _, ok := r.GetString(RunKey, RunValue); ok {
		t.Fatal("关闭后应删除")
	}
	if err := SetAutostart(r, exeA, false); err != nil {
		t.Fatal("删除不存在的值不应报错")
	}
}

func TestSyncAutostartMovedFolder(t *testing.T) { // 移动文件夹后自愈
	r := NewMemRegistry()
	SetAutostart(r, exeA, true)
	// 在新位置启动：旧位置已不存在 → 更新为新位置
	msg, err := SyncAutostart(r, exeB, true, alive(exeB))
	if err != nil || !strings.Contains(msg, "已失效") {
		t.Fatal(msg, err)
	}
	if v, _ := r.GetString(RunKey, RunValue); !strings.Contains(v, exeB) {
		t.Fatal(v)
	}
	// 已一致：无改动
	if msg, _ := SyncAutostart(r, exeB, true, alive(exeB)); msg != "" {
		t.Fatalf("应无改动: %s", msg)
	}
	// 缺失 → 写入
	r2 := NewMemRegistry()
	if msg, _ := SyncAutostart(r2, exeA, true, alive(exeA)); !strings.Contains(msg, "写入") {
		t.Fatal(msg)
	}
	// 路径大小写不同视为同一个
	r3 := NewMemRegistry()
	SetAutostart(r3, strings.ToUpper(exeA), true)
	if msg, _ := SyncAutostart(r3, exeA, true, alive(exeA)); msg != "" && !strings.Contains(msg, "更新") {
		t.Fatal(msg)
	}
}

func TestSyncDoesNotStealOtherLiveCopy(t *testing.T) {
	// 已安装版（exeA，仍存在）开了自启；这时运行绿色版（exeB）
	r := NewMemRegistry()
	SetAutostart(r, exeA, true)
	both := alive(exeA, exeB)
	if msg, _ := SyncAutostart(r, exeB, true, both); msg != "" {
		t.Fatalf("绿色版启动时不应抢走另一个有效副本的自启项: %s", msg)
	}
	if v, _ := r.GetString(RunKey, RunValue); !strings.Contains(v, exeA) {
		t.Fatal(v)
	}
	// 绿色版自启设置为关（默认）：同样不能删掉别人的有效条目
	if msg, _ := SyncAutostart(r, exeB, false, both); msg != "" {
		t.Fatalf("不应删除别的副本的条目: %s", msg)
	}
	if _, ok := r.GetString(RunKey, RunValue); !ok {
		t.Fatal("已安装版的自启项被误删")
	}
	// 但指向本 exe 的条目在设置为关时应删除；失效条目也清掉
	SetAutostart(r, exeB, true)
	if msg, _ := SyncAutostart(r, exeB, false, both); !strings.Contains(msg, "移除") {
		t.Fatal(msg)
	}
	SetAutostart(r, `Z:\gone\x.exe`, true)
	if msg, _ := SyncAutostart(r, exeB, false, both); !strings.Contains(msg, "移除") {
		t.Fatalf("失效条目应清除: %s", msg)
	}
}

func TestTracesAndClear(t *testing.T) { // B：清除注册表痕迹
	r := NewMemRegistry()
	if len(Traces(r, exeA)) != 0 {
		t.Fatal("初始应为空")
	}
	Register(r, exeA, `C:\i.png`)
	SetAutostart(r, exeA, true)
	tr := Traces(r, exeA)
	if len(tr) != 3 {
		t.Fatalf("应列出 3 项: %+v", tr)
	}
	joined := ""
	for _, x := range tr {
		joined += x.Key + "|" + x.Detail + "\n"
		if !strings.HasPrefix(x.Key, `HKCU\`) || x.Desc == "" {
			t.Fatalf("%+v", x)
		}
	}
	for _, want := range []string{`Run\DeskPinMemo`, `AppUserModelId\DeskPinMemo.App`, `Classes\deskpin`, exeA} {
		if !strings.Contains(joined, want) {
			t.Errorf("缺少 %q:\n%s", want, joined)
		}
	}
	removed, err := Clear(r, exeA)
	if err != nil || len(removed) != 3 {
		t.Fatal(removed, err)
	}
	if len(Traces(r, exeA)) != 0 || r.KeyExists(ProtocolKey+`\shell\open\command`) || r.KeyExists(AUMIDKey) {
		t.Fatal("应全部清除，包括协议的子项")
	}
	// 再次清除不报错
	if _, err := Clear(r, exeA); err != nil {
		t.Fatal(err)
	}
	// 不影响注册表里的其他内容
	r.SetString(`Software\Other`, "x", "1")
	Register(r, exeA, "i")
	Clear(r, exeA)
	if v, ok := r.GetString(`Software\Other`, "x"); !ok || v != "1" {
		t.Fatal("不应动到无关项")
	}
}

func TestTracesIncludeWindowsGeneratedRecords(t *testing.T) {
	r := NewMemRegistry()
	Register(r, exeA, `C:\i.png`)
	SetAutostart(r, exeA, true)
	// Windows 弹出通知后自动生成；Windows 11 为托盘图标生成 NotifyIconSettings 子项
	r.SetDWord(NotifSettingsKey, "Enabled", 1)
	r.SetString(TrayIconsKey+`\111`, "ExecutablePath", exeA)
	r.SetString(TrayIconsKey+`\222`, "ExecutablePath", `C:\Windows\explorer.exe`) // 别的程序
	r.SetString(TrayIconsKey+`\333`, "ExecutablePath", strings.ToUpper(exeA))     // 同一个 exe，大小写不同

	tr := Traces(r, exeA)
	if len(tr) != 6 {
		t.Fatalf("自启 / 通知身份 / 协议 / 通知设置 / 2 条托盘记录 = 6: %+v", tr)
	}
	removed, err := Clear(r, exeA)
	if err != nil || len(removed) != 6 {
		t.Fatal(removed, err)
	}
	if r.KeyExists(NotifSettingsKey) || r.KeyExists(TrayIconsKey+`\111`) || r.KeyExists(TrayIconsKey+`\333`) {
		t.Fatal("应清除通知设置记录和本 exe 的托盘记录")
	}
	if !r.KeyExists(TrayIconsKey + `\222`) {
		t.Fatal("绝不能删除别的程序的托盘记录")
	}
	if v, _ := r.GetString(TrayIconsKey+`\222`, "ExecutablePath"); v == "" {
		t.Fatal("别的程序的记录内容应保持")
	}
	// exe 路径为空时不匹配任何托盘记录（避免误删）
	r.SetString(TrayIconsKey+`\444`, "ExecutablePath", "")
	Traces(r, "")
	Clear(r, "")
	if !r.KeyExists(TrayIconsKey + `\444`) {
		t.Fatal("未知 exe 路径时不应删除任何托盘记录")
	}
}
