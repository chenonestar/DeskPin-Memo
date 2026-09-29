// Package recur 封装 iCalendar RRULE 周期规则（FR-302）。
package recur

import (
	"fmt"
	"strings"
	"time"

	"github.com/teambition/rrule-go"
)

// Kind 是 UI 提供的周期类型。
type Kind string

const (
	Daily   Kind = "daily"   // 每天
	Workday Kind = "workday" // 每个工作日
	Weekly  Kind = "weekly"  // 每周 X
	Monthly Kind = "monthly" // 每月 X 日
	Yearly  Kind = "yearly"  // 每年
	EveryN  Kind = "everyn"  // 每 N 天
	Custom  Kind = "custom"  // 直接给 RRULE
)

var weekdayCodes = []string{"SU", "MO", "TU", "WE", "TH", "FR", "SA"}

// Build 由 UI 选项生成 RRULE 字符串。
//   - Weekly: arg 为 time.Weekday（0=周日）
//   - Monthly: arg 为 1..31
//   - EveryN: arg 为 N（≥1）
func Build(kind Kind, arg int) (string, error) {
	switch kind {
	case Daily:
		return "FREQ=DAILY", nil
	case Workday:
		return "FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR", nil
	case Weekly:
		if arg < 0 || arg > 6 {
			return "", fmt.Errorf("recur: 星期参数无效 %d", arg)
		}
		return "FREQ=WEEKLY;BYDAY=" + weekdayCodes[arg], nil
	case Monthly:
		if arg < 1 || arg > 31 {
			return "", fmt.Errorf("recur: 日期参数无效 %d", arg)
		}
		return fmt.Sprintf("FREQ=MONTHLY;BYMONTHDAY=%d", arg), nil
	case Yearly:
		return "FREQ=YEARLY", nil
	case EveryN:
		if arg < 1 {
			return "", fmt.Errorf("recur: 间隔无效 %d", arg)
		}
		return fmt.Sprintf("FREQ=DAILY;INTERVAL=%d", arg), nil
	}
	return "", fmt.Errorf("recur: 未知类型 %q", kind)
}

func option(rule string, dtstart time.Time, loc *time.Location) (*rrule.ROption, error) {
	rule = strings.TrimPrefix(strings.TrimSpace(rule), "RRULE:")
	opt, err := rrule.StrToROptionInLocation(rule, loc)
	if err != nil {
		return nil, err
	}
	// 以本地墙钟时间作为起点，保证夏令时/时区下「每天 09:00」仍是 09:00。
	opt.Dtstart = dtstart.In(loc)
	return opt, nil
}

// Validate 检查规则是否合法。
func Validate(rule string) error {
	_, err := option(rule, time.Now(), time.UTC)
	return err
}

// Next 返回严格晚于 after 的下一次发生时间；dtstart 为系列起点（用于对齐时分）。
func Next(rule string, dtstart, after time.Time, loc *time.Location) (time.Time, bool, error) {
	if loc == nil {
		loc = time.Local
	}
	opt, err := option(rule, dtstart, loc)
	if err != nil {
		return time.Time{}, false, err
	}
	r, err := rrule.NewRRule(*opt)
	if err != nil {
		return time.Time{}, false, err
	}
	t := r.After(after, false)
	if t.IsZero() {
		return time.Time{}, false, nil
	}
	return t, true, nil
}

// Describe 返回中文描述，用于界面展示。
func Describe(rule string) string {
	rule = strings.TrimPrefix(strings.TrimSpace(rule), "RRULE:")
	m := map[string]string{}
	for _, p := range strings.Split(rule, ";") {
		if kv := strings.SplitN(p, "=", 2); len(kv) == 2 {
			m[kv[0]] = kv[1]
		}
	}
	names := map[string]string{"MO": "一", "TU": "二", "WE": "三", "TH": "四", "FR": "五", "SA": "六", "SU": "日"}
	switch m["FREQ"] {
	case "DAILY":
		if n := m["INTERVAL"]; n != "" && n != "1" {
			return "每 " + n + " 天"
		}
		return "每天"
	case "WEEKLY":
		if m["BYDAY"] == "MO,TU,WE,TH,FR" {
			return "每个工作日"
		}
		var ds []string
		for _, d := range strings.Split(m["BYDAY"], ",") {
			if n, ok := names[d]; ok {
				ds = append(ds, "周"+n)
			}
		}
		if len(ds) > 0 {
			return "每" + strings.Join(ds, "、")
		}
		return "每周"
	case "MONTHLY":
		if d := m["BYMONTHDAY"]; d != "" {
			return "每月 " + d + " 日"
		}
		return "每月"
	case "YEARLY":
		return "每年"
	}
	return rule
}
