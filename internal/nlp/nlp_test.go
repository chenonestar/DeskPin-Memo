package nlp

import (
	"testing"
	"time"
)

var loc = time.FixedZone("CST", 8*3600)

// 2026-09-29 是周二，上午 10:30
var now = time.Date(2026, 9, 29, 10, 30, 0, 0, loc)

func at(y int, m time.Month, d, h, mi int) time.Time { return time.Date(y, m, d, h, mi, 0, 0, loc) }

func TestParse(t *testing.T) {
	cases := []struct {
		in    string
		title string
		due   *time.Time
		rule  string
	}{
		{"明天下午3点 交报告", "交报告", ptr(at(2026, 9, 30, 15, 0)), ""}, // AC-01
		{"周五 买菜", "买菜", ptr(at(2026, 10, 2, 9, 0)), ""},
		{"下周三 开周会", "开周会", ptr(at(2026, 10, 7, 9, 0)), ""},
		{"本周日 大扫除", "大扫除", ptr(at(2026, 10, 4, 9, 0)), ""},
		{"后天上午十点半 面试", "面试", ptr(at(2026, 10, 1, 10, 30)), ""},
		{"今晚 看球", "看球", ptr(at(2026, 9, 29, 20, 0)), ""},
		{"下午3点 回邮件", "回邮件", ptr(at(2026, 9, 29, 15, 0)), ""},
		{"上午9点 已经过了", "已经过了", ptr(at(2026, 9, 30, 9, 0)), ""},
		{"2小时后 取快递", "取快递", ptr(at(2026, 9, 29, 12, 30)), ""},
		{"30分钟后开会", "开会", ptr(at(2026, 9, 29, 11, 0)), ""},
		{"3天后 复查", "复查", ptr(at(2026, 10, 2, 9, 0)), ""},
		{"10月5日 15:30 订机票", "订机票", ptr(at(2026, 10, 5, 15, 30)), ""},
		{"2027-01-02 交税", "交税", ptr(at(2027, 1, 2, 9, 0)), ""},
		{"5号 交房租", "交房租", ptr(at(2026, 10, 5, 9, 0)), ""},
		{"下个月10号 续费", "续费", ptr(at(2026, 10, 10, 9, 0)), ""},
		{"月底 结算", "结算", ptr(at(2026, 9, 30, 9, 0)), ""},
		{"每个工作日 09:00 站会", "站会", ptr(at(2026, 9, 30, 9, 0)), "FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR"},
		{"每月5日 09:00 交报销单", "交报销单", ptr(at(2026, 10, 5, 9, 0)), "FREQ=MONTHLY;BYMONTHDAY=5"},
		{"每周五 下午5点 写周报", "写周报", ptr(at(2026, 10, 2, 17, 0)), "FREQ=WEEKLY;BYDAY=FR"},
		{"每天 22:00 吃药", "吃药", ptr(at(2026, 9, 29, 22, 0)), "FREQ=DAILY"},
		{"每3天 浇花", "浇花", ptr(at(2026, 10, 2, 9, 0)), "FREQ=DAILY;INTERVAL=3"},
		{"买牛奶", "买牛奶", nil, ""},
		{"明天", "明天", nil, ""}, // 只有时间没有标题：整段作为标题
	}
	for _, c := range cases {
		r := Parse(c.in, now, loc)
		if r.Title != c.title {
			t.Errorf("%q: title=%q want %q", c.in, r.Title, c.title)
		}
		if (r.DueAt == nil) != (c.due == nil) || (c.due != nil && !r.DueAt.Equal(*c.due)) {
			t.Errorf("%q: due=%v want %v", c.in, r.DueAt, c.due)
		}
		if r.RepeatRule != c.rule {
			t.Errorf("%q: rule=%q want %q", c.in, r.RepeatRule, c.rule)
		}
	}
}

func TestMonthEnd(t *testing.T) {
	r := Parse("月底 结算", now, loc)
	if r.DueAt == nil || r.DueAt.Day() != 30 || r.DueAt.Month() != time.September {
		t.Fatalf("本月底应为 9/30: %v", r.DueAt)
	}
}

func TestTagsGroupPriority(t *testing.T) {
	r := Parse("明天 交报告 #工作 #紧急 @项目 !高", now, loc)
	if r.Title != "交报告" || len(r.Tags) != 2 || r.Tags[0] != "工作" || r.Group != "项目" || r.Priority == nil || *r.Priority != 2 {
		t.Fatalf("%+v", r)
	}
	r = Parse("买菜 !低", now, loc)
	if r.Priority == nil || *r.Priority != 0 || r.Title != "买菜" {
		t.Fatalf("%+v", r)
	}
}

func TestCnNum(t *testing.T) {
	for s, want := range map[string]int{"3": 3, "十": 10, "十二": 12, "二十": 20, "二十三": 23, "两": 2, "九": 9} {
		if got, ok := cnNum(s); !ok || got != want {
			t.Errorf("%s: %d %v", s, got, ok)
		}
	}
}

func ptr[T any](v T) *T { return &v }
