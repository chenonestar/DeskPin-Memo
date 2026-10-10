package service

import "deskpinmemo/internal/scheduler"

// Settings 是全局设置（4.3 设置窗口的各页签）。
type Settings struct {
	// 常规
	Autostart bool   `json:"autostart"`
	Language  string `json:"language"`
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
}

// DefaultSettings 返回默认设置。
func DefaultSettings() Settings {
	return Settings{
		Autostart: true, Language: "zh-CN", Theme: "system", FontSize: 14, Opacity: 1,
		StickyColor: "#FFF3B0", DefaultRemindTime: "09:00", Sound: true,
		DND:                scheduler.DND{Enabled: false, Start: "22:00", End: "08:00"},
		HotkeyQuick:        "Ctrl+Alt+N",
		HotkeyToggle:       "Ctrl+Alt+M",
		HotkeyClickThrough: "Ctrl+Alt+P",
		FadedOpacity:       0.4,
		DailyOverview:      true, // FR-308：每天首次开机弹出今日事项汇总（可关闭）
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
	return s
}
