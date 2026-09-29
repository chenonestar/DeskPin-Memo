// Package nlp 解析快速输入框中的中文自然语言（FR-102）：
// 「明天下午3点 交报告」「周五 买菜」「每月5日 09:00 交报销单」「#工作 @生活 !高」。
package nlp

import (
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"deskpinmemo/internal/recur"
)

// DefaultHour 是只有日期没有具体时刻时使用的默认小时（09:00）。
const DefaultHour = 9

// Result 是解析结果，回车前展示给用户预览。
type Result struct {
	Title      string     `json:"title"`
	DueAt      *time.Time `json:"-"`
	DueMs      *int64     `json:"dueAt"`
	HasTime    bool       `json:"hasTime"` // 输入中是否指定了具体时刻
	DueText    string     `json:"dueText"` // 命中的时间片段原文
	Tags       []string   `json:"tags"`
	Group      string     `json:"group"`
	Priority   *int       `json:"priority"`
	RepeatRule string     `json:"repeatRule"`
	RepeatText string     `json:"repeatText"`
}

const numRe = `[0-9零〇一二两三四五六七八九十]+`

var (
	reTag      = regexp.MustCompile(`#([^\s#@!]+)`)
	reGroup    = regexp.MustCompile(`@([^\s#@!]+)`)
	rePrio     = regexp.MustCompile(`!(高|中|低)|!!`)
	reEveryWD  = regexp.MustCompile(`每(?:个)?工作日`)
	reEveryDay = regexp.MustCompile(`每天|每日`)
	reEveryWk  = regexp.MustCompile(`每(?:个)?(?:周|星期|礼拜)([一二三四五六日天1-7])`)
	reEveryMo  = regexp.MustCompile(`每(?:个)?月(` + numRe + `)[日号]`)
	reEveryYr  = regexp.MustCompile(`每年`)
	reEveryN   = regexp.MustCompile(`每(` + numRe + `)天`)

	reRelDur   = regexp.MustCompile(`(` + numRe + `)(?:个)?(小时|分钟|天|周|星期)(?:以)?后`)
	reISO      = regexp.MustCompile(`(\d{4})[-/年](\d{1,2})[-/月](\d{1,2})[日号]?`)
	reMD       = regexp.MustCompile(`(\d{1,2})[/月](\d{1,2})[日号]?`)
	reNextMo   = regexp.MustCompile(`下(?:个)?月(` + numRe + `)[日号]`)
	reDayOnly  = regexp.MustCompile(`(` + numRe + `)[日号]`)
	reMonthEnd = regexp.MustCompile(`月底|月末`)
	reWeekday  = regexp.MustCompile(`(下下|下|本|这)?(?:个)?(?:周|星期|礼拜)([一二三四五六日天])`)
	reWord     = regexp.MustCompile(`大后天|后天|明天|明日|今天|今日|今晚|明早|明晚|昨天`)

	reDaypart  = `(凌晨|清晨|早上|早晨|上午|中午|下午|傍晚|晚上|夜里|半夜)?`
	reClock    = regexp.MustCompile(reDaypart + `(` + numRe + `)\s*(?:[点时]|[:：])\s*(半|一刻|三刻|` + numRe + `)?\s*(?:分)?`)
	reColon    = regexp.MustCompile(reDaypart + `(\d{1,2})[:：](\d{2})`)
	reDaypartO = regexp.MustCompile(`凌晨|清晨|早上|早晨|上午|中午|下午|傍晚|晚上|夜里|半夜`)
	spaces     = regexp.MustCompile(`\s+`)
)

var cnDigit = map[rune]int{'零': 0, '〇': 0, '一': 1, '二': 2, '两': 2, '三': 3, '四': 4, '五': 5, '六': 6, '七': 7, '八': 8, '九': 9}

// cnNum 将「12」「十二」「二十三」「两」转成整数。
func cnNum(s string) (int, bool) {
	if n, err := strconv.Atoi(s); err == nil {
		return n, true
	}
	if s == "" {
		return 0, false
	}
	r := []rune(s)
	total, cur := 0, 0
	seenTen := false
	for _, c := range r {
		switch {
		case c == '十':
			if cur == 0 {
				cur = 1
			}
			total += cur * 10
			cur = 0
			seenTen = true
		default:
			d, ok := cnDigit[c]
			if !ok {
				return 0, false
			}
			if seenTen || total == 0 {
				cur = cur*10 + d
			} else {
				return 0, false
			}
		}
	}
	return total + cur, true
}

func weekdayOf(s string) time.Weekday {
	switch s {
	case "一", "1":
		return time.Monday
	case "二", "2":
		return time.Tuesday
	case "三", "3":
		return time.Wednesday
	case "四", "4":
		return time.Thursday
	case "五", "5":
		return time.Friday
	case "六", "6":
		return time.Saturday
	}
	return time.Sunday // 日 / 天 / 7
}

func dayStart(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

// cut 删除 s 中 [loc[0], loc[1])，并返回命中的原文。
func cut(s string, loc []int) (rest, hit string) {
	return s[:loc[0]] + " " + s[loc[1]:], s[loc[0]:loc[1]]
}

// Parse 解析输入。now 与 loc 由调用方提供，便于测试。
func Parse(input string, now time.Time, loc *time.Location) Result {
	if loc == nil {
		loc = time.Local
	}
	now = now.In(loc)
	res := Result{Tags: []string{}}
	s := input

	// —— 标签 / 分组 / 优先级 ——
	for _, m := range reTag.FindAllStringSubmatch(s, -1) {
		res.Tags = append(res.Tags, m[1])
	}
	s = reTag.ReplaceAllString(s, " ")
	if m := reGroup.FindStringSubmatch(s); m != nil {
		res.Group = m[1]
	}
	s = reGroup.ReplaceAllString(s, " ")
	if m := rePrio.FindStringSubmatch(s); m != nil {
		p := recurPrio(m[0])
		res.Priority = &p
	}
	s = rePrio.ReplaceAllString(s, " ")

	var date *time.Time // 只含日期（当日 0 点）
	var hour, min = -1, 0
	var dateTexts []string

	// —— 周期规则（先于日期，避免「每周五」被当作「周五」）——
	var repeatAnchor *time.Time
	setRepeat := func(rule, text string, anchor *time.Time) {
		res.RepeatRule, res.RepeatText = rule, text
		repeatAnchor = anchor
	}
	today := dayStart(now)
	switch {
	case reEveryWD.MatchString(s):
		loc := reEveryWD.FindStringIndex(s)
		var hit string
		s, hit = cut(s, loc)
		r, _ := recur.Build(recur.Workday, 0)
		a := nextWorkday(today)
		setRepeat(r, hit, &a)
	case reEveryWk.MatchString(s):
		m := reEveryWk.FindStringSubmatch(s)
		var hit string
		s, hit = cut(s, reEveryWk.FindStringIndex(s))
		wd := weekdayOf(m[1])
		r, _ := recur.Build(recur.Weekly, int(wd))
		a := nextWeekday(today, wd, true)
		setRepeat(r, hit, &a)
	case reEveryMo.MatchString(s):
		m := reEveryMo.FindStringSubmatch(s)
		if d, ok := cnNum(m[1]); ok && d >= 1 && d <= 31 {
			var hit string
			s, hit = cut(s, reEveryMo.FindStringIndex(s))
			r, _ := recur.Build(recur.Monthly, d)
			a := nextMonthDay(today, d)
			setRepeat(r, hit, &a)
		}
	case reEveryN.MatchString(s):
		m := reEveryN.FindStringSubmatch(s)
		if n, ok := cnNum(m[1]); ok && n >= 1 {
			var hit string
			s, hit = cut(s, reEveryN.FindStringIndex(s))
			r, _ := recur.Build(recur.EveryN, n)
			a := today
			setRepeat(r, hit, &a)
		}
	case reEveryDay.MatchString(s):
		var hit string
		s, hit = cut(s, reEveryDay.FindStringIndex(s))
		r, _ := recur.Build(recur.Daily, 0)
		a := today
		setRepeat(r, hit, &a)
	case reEveryYr.MatchString(s):
		var hit string
		s, hit = cut(s, reEveryYr.FindStringIndex(s))
		r, _ := recur.Build(recur.Yearly, 0)
		a := today
		setRepeat(r, hit, &a)
	}

	// —— 相对时长：N 小时/分钟/天/周后 ——
	var relDue *time.Time
	if m := reRelDur.FindStringSubmatch(s); m != nil {
		if n, ok := cnNum(m[1]); ok {
			var hit string
			s, hit = cut(s, reRelDur.FindStringIndex(s))
			dateTexts = append(dateTexts, hit)
			var t time.Time
			switch m[2] {
			case "分钟":
				t = now.Add(time.Duration(n) * time.Minute)
			case "小时":
				t = now.Add(time.Duration(n) * time.Hour)
			case "天":
				t = now.AddDate(0, 0, n)
				h, mi := DefaultHour, 0
				if !strings.Contains(hit, "点") {
					t = time.Date(t.Year(), t.Month(), t.Day(), h, mi, 0, 0, loc)
				}
			default: // 周 / 星期
				t = now.AddDate(0, 0, 7*n)
				t = time.Date(t.Year(), t.Month(), t.Day(), DefaultHour, 0, 0, 0, loc)
			}
			if m[2] == "分钟" || m[2] == "小时" {
				t = t.Truncate(time.Minute)
				res.HasTime = true
			}
			relDue = &t
		}
	}

	// —— 日期 ——
	if relDue == nil {
		switch {
		case reISO.MatchString(s):
			m := reISO.FindStringSubmatch(s)
			y, _ := strconv.Atoi(m[1])
			mo, _ := strconv.Atoi(m[2])
			d, _ := strconv.Atoi(m[3])
			if validDate(y, mo, d) {
				var hit string
				s, hit = cut(s, reISO.FindStringIndex(s))
				dateTexts = append(dateTexts, hit)
				t := time.Date(y, time.Month(mo), d, 0, 0, 0, 0, loc)
				date = &t
			}
		case reNextMo.MatchString(s):
			m := reNextMo.FindStringSubmatch(s)
			if d, ok := cnNum(m[1]); ok && d >= 1 && d <= 31 {
				var hit string
				s, hit = cut(s, reNextMo.FindStringIndex(s))
				dateTexts = append(dateTexts, hit)
				first := time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, loc)
				t := time.Date(first.Year(), first.Month(), d, 0, 0, 0, 0, loc)
				if t.Month() != first.Month() { // 31 号落到下下月，退到当月最后一天
					t = first.AddDate(0, 1, -1)
				}
				date = &t
			}
		case reMonthEnd.MatchString(s):
			var hit string
			s, hit = cut(s, reMonthEnd.FindStringIndex(s))
			dateTexts = append(dateTexts, hit)
			t := time.Date(now.Year(), now.Month()+1, 0, 0, 0, 0, 0, loc)
			date = &t
		case reWord.MatchString(s):
			w := reWord.FindString(s)
			var hit string
			s, hit = cut(s, reWord.FindStringIndex(s))
			dateTexts = append(dateTexts, hit)
			t := today
			switch w {
			case "明天", "明日", "明早", "明晚":
				t = today.AddDate(0, 0, 1)
			case "后天":
				t = today.AddDate(0, 0, 2)
			case "大后天":
				t = today.AddDate(0, 0, 3)
			case "昨天":
				t = today.AddDate(0, 0, -1)
			}
			switch w {
			case "今晚", "明晚":
				hour = 20
			case "明早":
				hour = 8
			}
			date = &t
		case reWeekday.MatchString(s):
			m := reWeekday.FindStringSubmatch(s)
			var hit string
			s, hit = cut(s, reWeekday.FindStringIndex(s))
			dateTexts = append(dateTexts, hit)
			wd := weekdayOf(m[2])
			var t time.Time
			switch m[1] {
			case "下":
				t = weekOffset(today, wd, 1)
			case "下下":
				t = weekOffset(today, wd, 2)
			case "本", "这":
				t = weekOffset(today, wd, 0)
			default: // 无前缀：今天起最近的那个星期 X（今天也算）
				t = nextWeekday(today, wd, false)
			}
			date = &t
		case reMD.MatchString(s):
			m := reMD.FindStringSubmatch(s)
			mo, _ := strconv.Atoi(m[1])
			d, _ := strconv.Atoi(m[2])
			if validDate(now.Year(), mo, d) {
				var hit string
				s, hit = cut(s, reMD.FindStringIndex(s))
				dateTexts = append(dateTexts, hit)
				t := time.Date(now.Year(), time.Month(mo), d, 0, 0, 0, 0, loc)
				if t.Before(today) {
					t = t.AddDate(1, 0, 0)
				}
				date = &t
			}
		case reDayOnly.MatchString(s):
			m := reDayOnly.FindStringSubmatch(s)
			if d, ok := cnNum(m[1]); ok && d >= 1 && d <= 31 {
				var hit string
				s, hit = cut(s, reDayOnly.FindStringIndex(s))
				dateTexts = append(dateTexts, hit)
				t := nextMonthDay(today, d)
				date = &t
			}
		}
	}

	// —— 时刻 ——
	if relDue == nil {
		if m := reColon.FindStringSubmatch(s); m != nil {
			h, _ := strconv.Atoi(m[2])
			mi, _ := strconv.Atoi(m[3])
			if h < 24 && mi < 60 {
				var hit string
				s, hit = cut(s, reColon.FindStringIndex(s))
				dateTexts = append(dateTexts, hit)
				hour, min = adjustHour(h, m[1]), mi
				res.HasTime = true
			}
		} else if m := reClock.FindStringSubmatch(s); m != nil {
			if h, ok := cnNum(m[2]); ok && h <= 24 {
				mi := 0
				switch m[3] {
				case "半":
					mi = 30
				case "一刻":
					mi = 15
				case "三刻":
					mi = 45
				case "":
				default:
					if v, ok := cnNum(m[3]); ok && v < 60 {
						mi = v
					}
				}
				var hit string
				s, hit = cut(s, reClock.FindStringIndex(s))
				dateTexts = append(dateTexts, hit)
				hour, min = adjustHour(h, m[1]), mi
				res.HasTime = true
			}
		} else if w := reDaypartO.FindString(s); w != "" && hour < 0 {
			// 只有时段没有钟点：上午 9、中午 12、下午 15、晚上 20 ...
			var hit string
			s, hit = cut(s, reDaypartO.FindStringIndex(s))
			dateTexts = append(dateTexts, hit)
			hour = map[string]int{"凌晨": 6, "清晨": 7, "早上": 8, "早晨": 8, "上午": 9, "中午": 12, "下午": 15,
				"傍晚": 18, "晚上": 20, "夜里": 22, "半夜": 23}[w]
			res.HasTime = true
		} else if hour >= 0 {
			res.HasTime = true // 今晚 / 明早 等词自带时刻
		}
	}
	if hour >= 0 {
		res.HasTime = true
	}

	// —— 合成截止时间 ——
	switch {
	case relDue != nil:
		res.DueAt = relDue
	default:
		base := date
		if base == nil && repeatAnchor != nil {
			base = repeatAnchor
		}
		if base == nil && hour >= 0 { // 只有时刻：今天还没过就今天，否则明天
			t := today
			if time.Date(t.Year(), t.Month(), t.Day(), hour, min, 0, 0, loc).Before(now) {
				t = t.AddDate(0, 0, 1)
			}
			base = &t
		}
		if base != nil {
			h, mi := hour, min
			if h < 0 {
				h, mi = DefaultHour, 0
			}
			t := time.Date(base.Year(), base.Month(), base.Day(), h, mi, 0, 0, loc)
			if res.RepeatRule != "" && repeatAnchor != nil && date == nil && t.Before(now) {
				// 「每天 09:00」现在已过 9 点：从明天开始
				if next, ok, _ := recur.Next(res.RepeatRule, t, now, loc); ok {
					t = next
				}
			}
			res.DueAt = &t
		}
	}
	if res.DueAt != nil {
		ms := res.DueAt.UnixMilli()
		res.DueMs = &ms
	}
	res.DueText = strings.TrimSpace(strings.Join(dateTexts, " "))

	title := strings.TrimSpace(spaces.ReplaceAllString(s, " "))
	title = strings.Trim(title, "，,。.;；、 ")
	if title == "" || utf8.RuneCountInString(title) == 0 {
		title = strings.TrimSpace(input)
		res.DueAt, res.DueMs, res.RepeatRule, res.RepeatText, res.DueText, res.HasTime = nil, nil, "", "", "", false
	}
	res.Title = title
	return res
}

func recurPrio(s string) int {
	switch s {
	case "!高", "!!":
		return 2
	case "!低":
		return 0
	}
	return 1
}

func adjustHour(h int, part string) int {
	switch part {
	case "下午", "傍晚", "晚上", "夜里":
		if h < 12 {
			h += 12
		}
	case "中午":
		if h < 11 {
			h += 12
		}
	case "凌晨", "半夜":
		if h == 12 {
			h = 0
		}
	}
	if h == 24 {
		h = 0
	}
	return h
}

func validDate(y, m, d int) bool {
	if m < 1 || m > 12 || d < 1 || d > 31 {
		return false
	}
	t := time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC)
	return t.Day() == d
}

// nextWeekday 返回从 from 起（含/不含当天）最近的星期 wd。
func nextWeekday(from time.Time, wd time.Weekday, excludeToday bool) time.Time {
	diff := (int(wd) - int(from.Weekday()) + 7) % 7
	if diff == 0 && excludeToday {
		diff = 7
	}
	return from.AddDate(0, 0, diff)
}

func nextWorkday(from time.Time) time.Time {
	for from.Weekday() == time.Saturday || from.Weekday() == time.Sunday {
		from = from.AddDate(0, 0, 1)
	}
	return from
}

// weekOffset 返回「本周（周一为一周开始）+ n 周」的星期 wd。
func weekOffset(today time.Time, wd time.Weekday, n int) time.Time {
	off := (int(today.Weekday()) + 6) % 7 // 距本周一天数
	monday := today.AddDate(0, 0, -off)
	d := (int(wd) + 6) % 7
	return monday.AddDate(0, 0, 7*n+d)
}

// nextMonthDay 返回从 from 起（含当天）最近一个「每月 d 日」。
func nextMonthDay(from time.Time, d int) time.Time {
	for i := 0; i < 14; i++ {
		m := time.Date(from.Year(), from.Month()+time.Month(i), 1, 0, 0, 0, 0, from.Location())
		t := time.Date(m.Year(), m.Month(), d, 0, 0, 0, 0, from.Location())
		if t.Month() == m.Month() && !t.Before(from) {
			return t
		}
	}
	return from
}
