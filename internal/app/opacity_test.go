package app

import "testing"

func TestEffectiveOpacityRules(t *testing.T) {
	base := OpacityInput{Base: 1, FadeEnabled: true, FadedLevel: 0.4}
	with := func(f func(*OpacityInput)) OpacityInput { in := base; f(&in); return in }
	for _, c := range []struct {
		name string
		in   OpacityInput
		want float64
		why  OpacityReason
	}{
		{"没开自动变淡：就是基础透明度，鼠标在不在都一样", OpacityInput{Base: 0.8, MouseAway: true, FadedLevel: 0.4}, 0.8, ReasonBase},
		{"自动变淡，鼠标在便签上：基础透明度", with(func(i *OpacityInput) { i.MouseAway = false }), 1, ReasonBase},
		{"自动变淡，鼠标离开：变淡后的透明度", with(func(i *OpacityInput) { i.MouseAway = true }), 0.4, ReasonFaded},
		{"变淡后的透明度不会比基础透明度更不透明", with(func(i *OpacityInput) { i.MouseAway = true; i.Base = 0.3 }), 0.3, ReasonFaded},
		{"变淡后的透明度超出范围时被限制（下限）", with(func(i *OpacityInput) { i.MouseAway = true; i.FadedLevel = 0.01 }), FadedMin, ReasonFaded},
		{"变淡后的透明度超出范围时被限制（上限）", with(func(i *OpacityInput) { i.MouseAway = true; i.FadedLevel = 0.99 }), FadedMax, ReasonFaded},
		// 核心规则：穿透 + 自动变淡
		{"开了鼠标穿透：自动变淡不适用，按基础透明度（不会永远停在变淡状态）", with(func(i *OpacityInput) { i.ClickThrough = true; i.MouseAway = true }), 1, ReasonFadeSkips},
		{"穿透开启、基础 70%：就是 70%", with(func(i *OpacityInput) { i.ClickThrough = true; i.MouseAway = true; i.Base = 0.7 }), 0.7, ReasonFadeSkips},
		{"穿透但没开自动变淡：没有什么需要解释的", OpacityInput{Base: 0.7, ClickThrough: true, MouseAway: true}, 0.7, ReasonBase},
		// 叠加界面和强提醒优先于一切
		{"叠加界面打开：不透明（即使变淡 + 鼠标不在）", with(func(i *OpacityInput) { i.Overlay = true; i.MouseAway = true; i.Base = 0.5 }), 1, ReasonOverlay},
		{"强提醒：不透明（即使穿透 + 很淡）", with(func(i *OpacityInput) { i.Alert = true; i.ClickThrough = true; i.Base = 0.3 }), 1, ReasonAlert},
		{"叠加界面优先于强提醒的原因说明", with(func(i *OpacityInput) { i.Overlay = true; i.Alert = true }), 1, ReasonOverlay},
	} {
		got := EffectiveOpacity(c.in)
		if got.Value != c.want || got.Reason != c.why {
			t.Errorf("%s: got %+v want {%v %s}", c.name, got, c.want, c.why)
		}
	}
}
