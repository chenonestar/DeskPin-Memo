// Package scheduler 实现提醒调度（FR-301/306/307，NFR-03/05）：
// 用单个定时器睡到最近一次触发时间，而不是逐秒轮询；睡眠唤醒/时间变更后重新计算并补发错过的提醒。
package scheduler

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"deskpinmemo/internal/store"
)

// MissedThreshold：晚于触发时间超过该值才算「错过」，汇总补发（NFR-05 触发误差 ≤ 5 s）。
const MissedThreshold = 30 * time.Second

// MaxSleep 是单次睡眠上限。Go 的单调时钟在系统休眠/改时间后不可靠，
// 上限用来兜底，同时 Windows 的电源/时间变更消息会调用 Kick 立即重算。
const MaxSleep = 30 * time.Second

// Action 是通知按钮（FR-304）。
type Action struct {
	ID    string `json:"id"` // done | snooze10 | snooze60 | tomorrow
	Label string `json:"label"`
}

// Notification 是发给通知实现的内容。
type Notification struct {
	Tag     string   `json:"tag"` // 用于替换/撤回（V3 同步撤回已弹出通知）
	Title   string   `json:"title"`
	Body    string   `json:"body"`
	ItemID  string   `json:"itemId"`
	Actions []Action `json:"actions"`
	Strong  bool     `json:"strong"` // 强提醒：置顶窗口 + 提示音
	Summary bool     `json:"summary"`
	Missed  bool     `json:"missed"`
}

// Notifier 抽象系统通知，Windows 下为 Toast。
type Notifier interface {
	Notify(n Notification) error
}

// DefaultActions 是 FR-304 规定的四个操作。
var DefaultActions = []Action{
	{"done", "完成"}, {"snooze10", "10 分钟后"}, {"snooze60", "1 小时后"}, {"tomorrow", "明天"},
}

// DND 是免打扰时段（FR-307）。
type DND struct {
	Enabled bool   `json:"enabled"`
	Start   string `json:"start"` // HH:MM
	End     string `json:"end"`   // HH:MM
}

func parseHM(s string) (int, bool) {
	var h, m int
	if _, err := fmt.Sscanf(s, "%d:%d", &h, &m); err != nil || h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, false
	}
	return h*60 + m, true
}

// Adjust 若 t 落在免打扰时段内，返回时段结束时间；否则原样返回。支持跨午夜时段。
func (d DND) Adjust(t time.Time, loc *time.Location) time.Time {
	if !d.Enabled {
		return t
	}
	s, ok1 := parseHM(d.Start)
	e, ok2 := parseHM(d.End)
	if !ok1 || !ok2 || s == e {
		return t
	}
	lt := t.In(loc)
	cur := lt.Hour()*60 + lt.Minute()
	at := func(day time.Time, mins int) time.Time {
		return time.Date(day.Year(), day.Month(), day.Day(), mins/60, mins%60, 0, 0, loc)
	}
	if s < e { // 同日时段，如 13:00–14:00
		if cur >= s && cur < e {
			return at(lt, e)
		}
		return t
	}
	// 跨午夜，如 22:00–08:00
	switch {
	case cur >= s:
		return at(lt.AddDate(0, 0, 1), e)
	case cur < e:
		return at(lt, e)
	}
	return t
}

// Source 是调度器需要的存储能力。
type Source interface {
	PendingReminders() ([]store.PendingReminder, error)
	MarkFired(ids []string, at int64) error
}

// Scheduler 调度提醒。
type Scheduler struct {
	src      Source
	notify   Notifier
	now      func() time.Time
	loc      func() *time.Location
	dnd      func() DND
	strong   func() bool
	onFired  func()
	kick     chan struct{}
	mu       sync.Mutex
	nextWake time.Time
}

// Config 是创建调度器所需依赖。
type Config struct {
	Source   Source
	Notifier Notifier
	Now      func() time.Time
	Loc      func() *time.Location
	DND      func() DND
	// StrongEnabled 返回是否启用「高优先级强提醒」。
	StrongEnabled func() bool
	// OnFired 在有提醒触发后调用（刷新界面/托盘状态）。
	OnFired func()
}

// New 创建调度器。
func New(c Config) *Scheduler {
	s := &Scheduler{src: c.Source, notify: c.Notifier, now: c.Now, loc: c.Loc, dnd: c.DND, strong: c.StrongEnabled,
		onFired: c.OnFired, kick: make(chan struct{}, 1)}
	if s.now == nil {
		s.now = time.Now
	}
	if s.loc == nil {
		s.loc = func() *time.Location { return time.Local }
	}
	if s.dnd == nil {
		s.dnd = func() DND { return DND{} }
	}
	if s.strong == nil {
		s.strong = func() bool { return false }
	}
	return s
}

// Kick 通知调度器重新计算（数据变更、睡眠唤醒、系统时间/时区变化时调用）。
func (s *Scheduler) Kick() {
	select {
	case s.kick <- struct{}{}:
	default:
	}
}

// Run 阻塞运行直到 ctx 结束。启动时立即补发错过的提醒。
func (s *Scheduler) Run(ctx context.Context) {
	timer := time.NewTimer(time.Hour)
	defer timer.Stop()
	for {
		wait := s.tick()
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(wait)
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		case <-s.kick:
		}
	}
}

// tick 触发所有到期提醒，返回下次需要睡眠的时长。
func (s *Scheduler) tick() time.Duration {
	next := s.Fire()
	d := MaxSleep
	if !next.IsZero() {
		if until := next.Sub(s.now()); until < d {
			d = until
		}
	}
	if d < 50*time.Millisecond {
		d = 50 * time.Millisecond
	}
	return d
}

// Fire 触发所有已到期的提醒并返回之后最近的触发时间（无则零值）。
func (s *Scheduler) Fire() (next time.Time) {
	pending, err := s.src.PendingReminders()
	if err != nil {
		return time.Time{}
	}
	now := s.now()
	dnd, loc := s.dnd(), s.loc()
	var fresh, missed []store.PendingReminder
	for _, p := range pending {
		at := dnd.Adjust(time.UnixMilli(p.FireAt), loc)
		if at.After(now) {
			if next.IsZero() || at.Before(next) {
				next = at
			}
			continue
		}
		if now.Sub(at) > MissedThreshold {
			missed = append(missed, p)
		} else {
			fresh = append(fresh, p)
		}
	}
	if len(fresh)+len(missed) == 0 {
		return next
	}
	var fired []string
	send := func(n Notification, ids ...string) {
		if err := s.notify.Notify(n); err == nil {
			fired = append(fired, ids...)
		} else { // 通知失败：不标记已触发，下一轮重试；但避免忙等
			return
		}
	}
	for _, p := range fresh {
		send(single(p, false, s.strong()), p.ID)
	}
	switch {
	case len(missed) == 1:
		send(single(missed[0], true, false), missed[0].ID)
	case len(missed) > 1:
		var ids []string
		for _, p := range missed {
			ids = append(ids, p.ID)
		}
		send(summary(missed), ids...)
	}
	if len(fired) > 0 {
		_ = s.src.MarkFired(fired, now.UnixMilli())
		if s.onFired != nil {
			s.onFired()
		}
	}
	if len(fired) < len(fresh)+len(missed) {
		// 有通知失败：稍后重试
		retry := now.Add(5 * time.Second)
		if next.IsZero() || retry.Before(next) {
			next = retry
		}
	}
	return next
}

const lockedBody = "有 1 条事项提醒，解锁后查看"

func single(p store.PendingReminder, missed, strongOn bool) Notification {
	n := Notification{Tag: "reminder-" + p.ID, ItemID: p.ItemID, Actions: DefaultActions, Missed: missed}
	if p.Locked { // 未解锁：不显示内容（6.3）
		n.Title, n.Body, n.Actions = lockedBody, "", nil
		return n
	}
	n.Title = p.Title
	n.Body = summarize(p.Note, 80)
	if missed {
		n.Title = "错过的提醒：" + p.Title
	}
	n.Strong = strongOn && p.Priority == store.PriorityHigh
	return n
}

func summary(ps []store.PendingReminder) Notification {
	n := Notification{Tag: "reminder-summary", Summary: true, Missed: true}
	locked := 0
	var titles []string
	for _, p := range ps {
		if p.Locked {
			locked++
			continue
		}
		titles = append(titles, p.Title)
	}
	if len(titles) == 0 {
		n.Title = fmt.Sprintf("有 %d 条事项提醒，解锁后查看", len(ps))
		return n
	}
	n.Title = fmt.Sprintf("错过了 %d 条提醒", len(ps))
	shown := titles
	if len(shown) > 4 {
		shown = shown[:4]
	}
	n.Body = strings.Join(shown, "\n")
	if len(titles) > len(shown) || locked > 0 {
		n.Body += fmt.Sprintf("\n…等共 %d 条", len(ps))
	}
	return n
}

func summarize(s string, max int) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	r := []rune(s)
	if len(r) > max {
		return string(r[:max]) + "…"
	}
	return s
}
