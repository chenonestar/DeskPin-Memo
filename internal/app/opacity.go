package app

// 窗口「实际透明度」的唯一裁决点。把所有会影响透明度的规则集中成一个纯函数，
// 便于对每种组合写单元测试，以后新增规则也不会悄悄改出冲突。

// FadedMin / FadedMax 是「变淡后透明度」设置的取值范围。
const (
	FadedMin = 0.2
	FadedMax = 0.8
)

// OpacityInput 是决定实际透明度的全部状态。
type OpacityInput struct {
	Base         float64 // 便签的基础透明度（已解析：自己的设置，否则默认值），0.3–1
	FadeEnabled  bool    // 设置：鼠标离开后自动变淡
	FadedLevel   float64 // 设置：变淡后的透明度（绝对值）
	ClickThrough bool    // 该便签开启了鼠标穿透
	MouseAway    bool    // 鼠标当前不在便签上
	Overlay      bool    // 快速输入框 / 设置 / 概览打开中
	Alert        bool    // 有未处理的强提醒
}

// OpacityReason 说明实际透明度为什么是这个值，界面据此给出解释。
type OpacityReason string

const (
	ReasonBase      OpacityReason = "base"      // 就是基础透明度
	ReasonFaded     OpacityReason = "faded"     // 鼠标不在，已变淡
	ReasonOverlay   OpacityReason = "overlay"   // 叠加界面打开：必须看得清、点得到
	ReasonAlert     OpacityReason = "alert"     // 强提醒：必须手动处理
	ReasonFadeSkips OpacityReason = "fadeSkips" // 开了自动变淡，但鼠标穿透使它不适用，按基础透明度显示
)

// OpacityResult 是裁决结果。
type OpacityResult struct {
	Value  float64
	Reason OpacityReason
}

// EffectiveOpacity 的规则按优先级：
//  1. 叠加界面 / 强提醒：一律不透明；
//  2. 鼠标穿透开启：「自动变淡」不适用——它依赖鼠标进入 / 离开事件，而穿透正是把这些事件拿掉了，
//     否则便签会永远停在变淡状态。这里只是在计算时忽略，不改动用户保存的设置，关闭穿透后自动恢复；
//  3. 自动变淡且鼠标不在：变淡后的透明度，但不会比基础透明度更不透明；
//  4. 其余：基础透明度。
func EffectiveOpacity(in OpacityInput) OpacityResult {
	switch {
	case in.Overlay:
		return OpacityResult{1, ReasonOverlay}
	case in.Alert:
		return OpacityResult{1, ReasonAlert}
	case in.FadeEnabled && in.ClickThrough:
		return OpacityResult{in.Base, ReasonFadeSkips}
	case in.FadeEnabled && in.MouseAway:
		v := ClampFaded(in.FadedLevel)
		if v > in.Base {
			v = in.Base
		}
		return OpacityResult{v, ReasonFaded}
	}
	return OpacityResult{in.Base, ReasonBase}
}

// ClampFaded 把变淡后的透明度限制在 20%–80%。
func ClampFaded(v float64) float64 {
	if v < FadedMin {
		return FadedMin
	}
	if v > FadedMax {
		return FadedMax
	}
	return v
}
