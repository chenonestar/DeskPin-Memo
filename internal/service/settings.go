package service

import "deskpinmemo/internal/scheduler"

// Settings 是全局设置（4.3 设置窗口的各页签）。
type Settings struct {
	// 常规
	// SystemIntegration：允许写入当前用户注册表以启用「带按钮的系统通知」和「开机自启」。
	// 仅对绿色版有意义（默认关闭 = 零注册表写入）；安装版始终启用。
	SystemIntegration bool   `json:"systemIntegration"`
	Autostart         bool   `json:"autostart"`
	Language          string `json:"language"`
	// 外观
	Theme        string  `json:"theme"`        // system | light | dark
	FontSize     int     `json:"fontSize"`     // 12 | 14 | 16
	Opacity      float64 `json:"opacity"`      // 0.3–1
	FadeOnLeave  bool    `json:"fadeOnLeave"`  // 鼠标离开后自动变淡（鼠标穿透开启的便签不适用）
	FadedOpacity float64 `json:"fadedOpacity"` // 变淡后的透明度（20%–80%），仅在开启自动变淡时有意义
	StickyColor  string  `json:"stickyColor"`
	// 提醒
	DefaultRemindTime string        `json:"defaultRemindTime"` // 只有日期时的默认时刻 HH:MM
	Sound             bool          `json:"sound"`
	StrongReminder    bool          `json:"strongReminder"`
	DND               scheduler.DND `json:"dnd"`
	// 快捷键
	HotkeyQuick  string `json:"hotkeyQuick"`
	HotkeyToggle string `json:"hotkeyToggle"`
	// 切换鼠标穿透：不依赖「按住 Ctrl」的检测，任何时候都能一键开关，是穿透的保底出口
	HotkeyClickThrough string `json:"hotkeyClickThrough"`
	// 数据
	DailyOverview bool `json:"dailyOverview"`

	// 摸鱼：RSS 新闻订阅（默认关闭；关闭时程序不发出任何网络请求）
	NewsEnabled   bool   `json:"newsEnabled"`
	NewsSetupDone bool   `json:"newsSetupDone"` // 首次开启时已经选过预置订阅源
	NewsInterval  int    `json:"newsInterval"`  // 拉取间隔（分钟）：15 | 30 | 60
	NewsProxy     string `json:"newsProxy"`     // system | none | manual
	NewsProxyURL  string `json:"newsProxyUrl"`  // 手动代理地址，如 127.0.0.1:7890
	// 摸鱼：桌面宠物「戴眼镜的狗狗」
	PetEnabled bool   `json:"petEnabled"`
	PetCorner  string `json:"petCorner"`  // right | left
	PetBubbles bool   `json:"petBubbles"` // 偶尔说话
	PetWorry   bool   `json:"petWorry"`   // 有逾期事项时露出担心的表情
	// 摸鱼：下班倒计时 / 今天已赚
	ShowCountdown bool    `json:"showCountdown"`
	WorkStart     string  `json:"workStart"` // HH:MM
	WorkEnd       string  `json:"workEnd"`   // HH:MM（狗狗到点伸懒腰也用它）
	MonthlySalary float64 `json:"monthlySalary"`
	// 摸鱼：正经模式（老板键）——一键收起新闻条、宠物、倒计时，可同时切到指定分组
	HotkeyBoss  string `json:"hotkeyBoss"`
	BossGroupID string `json:"bossGroupId"`
}

// DefaultSettings 返回默认设置。
func DefaultSettings() Settings {
	return Settings{
		SystemIntegration: true, Autostart: true, Language: "zh-CN", Theme: "system", FontSize: 14, Opacity: 1,
		StickyColor: "#FFF3B0", DefaultRemindTime: "09:00", Sound: true,
		DND:                scheduler.DND{Enabled: false, Start: "22:00", End: "08:00"},
		HotkeyQuick:        "Ctrl+Alt+N",
		HotkeyToggle:       "Ctrl+Alt+M",
		HotkeyClickThrough: "Ctrl+Alt+P",
		FadedOpacity:       0.4,
		DailyOverview:      true, // FR-308：每天首次开机弹出今日事项汇总（可关闭）
		NewsInterval:       30,
		NewsProxy:          "system",
		PetCorner:          "right",
		PetBubbles:         true,
		PetWorry:           true,
		WorkStart:          "09:00",
		WorkEnd:            "18:00",
		HotkeyBoss:         "Ctrl+Alt+H",
	}
}

func (s Settings) normalized() Settings {
	d := DefaultSettings()
	if s.Theme != "light" && s.Theme != "dark" {
		s.Theme = "system"
	}
	if s.FontSize != 12 && s.FontSize != 14 && s.FontSize != 16 {
		s.FontSize = d.FontSize
	}
	if s.Opacity < 0.3 || s.Opacity > 1 {
		s.Opacity = d.Opacity
	}
	if s.Language == "" {
		s.Language = d.Language
	}
	if s.StickyColor == "" {
		s.StickyColor = d.StickyColor
	}
	if s.DefaultRemindTime == "" {
		s.DefaultRemindTime = d.DefaultRemindTime
	}
	if s.DND.Start == "" {
		s.DND.Start = d.DND.Start
	}
	if s.DND.End == "" {
		s.DND.End = d.DND.End
	}
	if s.HotkeyQuick == "" {
		s.HotkeyQuick = d.HotkeyQuick
	}
	if s.HotkeyClickThrough == "" {
		s.HotkeyClickThrough = d.HotkeyClickThrough
	}
	if s.FadedOpacity == 0 {
		s.FadedOpacity = d.FadedOpacity
	}
	if s.FadedOpacity < 0.2 {
		s.FadedOpacity = 0.2
	}
	if s.FadedOpacity > 0.8 {
		s.FadedOpacity = 0.8
	}
	if s.HotkeyToggle == "" {
		s.HotkeyToggle = d.HotkeyToggle
	}
	if s.NewsInterval != 15 && s.NewsInterval != 30 && s.NewsInterval != 60 {
		s.NewsInterval = d.NewsInterval
	}
	if s.NewsProxy != "none" && s.NewsProxy != "manual" {
		s.NewsProxy = "system"
	}
	if s.PetCorner != "left" {
		s.PetCorner = "right"
	}
	if !validHHMM(s.WorkStart) {
		s.WorkStart = d.WorkStart
	}
	if !validHHMM(s.WorkEnd) {
		s.WorkEnd = d.WorkEnd
	}
	if s.MonthlySalary < 0 || s.MonthlySalary > 1e8 {
		s.MonthlySalary = 0
	}
	if s.HotkeyBoss == "" {
		s.HotkeyBoss = d.HotkeyBoss
	}
	return s
}

func validHHMM(v string) bool {
	if len(v) != 5 || v[2] != ':' {
		return false
	}
	h, m := (int(v[0]-'0'))*10+int(v[1]-'0'), (int(v[3]-'0'))*10+int(v[4]-'0')
	return v[0] >= '0' && v[0] <= '2' && v[1] >= '0' && v[1] <= '9' && v[3] >= '0' && v[3] <= '5' && v[4] >= '0' && v[4] <= '9' && h < 24 && m < 60
}
