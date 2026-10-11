package regtrace

// State 是影响注册表内容的两个设置。
type State struct {
	Integration bool // 允许写注册表（带按钮的通知需要通知身份 + 协议）；绿色版默认 false
	Autostart   bool // 已经包含「Integration 为假时必为假」的约束
}

// Apply 让注册表与设置一致。prev 为 nil 表示程序刚启动。
//
//   - Integration 关（绿色版默认）：**不写任何东西**。启动时只做一件事——如果旧版本留下了指向本 exe（或已失效路径）
//     的自启项，就把它删掉（SyncAutostart(false)，不动别的副本的有效条目）；
//   - 启动时 Integration 开：登记通知身份和协议，并同步自启项；
//   - 运行中 Integration 关→开：登记通知身份和协议（自启另行按 Autostart 变化写入）；
//   - 运行中 Integration 开→关：清除本程序写入的全部项（含 Windows 自动生成的记录）。
//
// cleared 为真表示执行了清除，调用方可顺带清理通知中心里已弹出的历史通知。
func Apply(reg Registry, exePath, iconPath string, prev *State, want State, exists func(string) bool) (msgs []string, cleared bool, err error) {
	say := func(s string) {
		if s != "" {
			msgs = append(msgs, s)
		}
	}
	if prev == nil {
		if want.Integration {
			if changed, err := Register(reg, exePath, iconPath); err != nil {
				return msgs, false, err
			} else if changed {
				say("已登记通知身份和 deskpin:// 协议")
			}
		}
		m, err := SyncAutostart(reg, exePath, want.Integration && want.Autostart, exists)
		say(m)
		return msgs, false, err
	}
	switch {
	case !prev.Integration && want.Integration:
		if _, err := Register(reg, exePath, iconPath); err != nil {
			return msgs, false, err
		}
		say("已启用系统集成：登记通知身份和 deskpin:// 协议")
	case prev.Integration && !want.Integration:
		removed, err := Clear(reg, exePath)
		say("已关闭系统集成，清除注册表项 " + itoa(len(removed)) + " 个")
		return msgs, true, err
	}
	if want.Integration && want.Autostart != prev.Autostart {
		// 用户明确切换了开机自启：强制写入 / 删除
		if err := SetAutostart(reg, exePath, want.Autostart); err != nil {
			return msgs, false, err
		}
		if want.Autostart {
			say("已写入开机自启项")
		} else {
			say("已移除开机自启项")
		}
	}
	return msgs, false, nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}
