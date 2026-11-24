package logx

import (
	"sync"
	"time"
)

// Every 限制同一类日志的频率：每个间隔内只放行第一条，其余的计数，下次放行时一并返回被省略的条数。
type Every struct {
	mu      sync.Mutex
	d       time.Duration
	last    time.Time
	skipped int
}

// NewEvery 返回间隔为 d 的限频器。
func NewEvery(d time.Duration) *Every { return &Every{d: d} }

// Allow 判断现在能不能记一条；ok 为 true 时 skipped 是上次记录之后被省略的条数。
func (e *Every) Allow(now time.Time) (ok bool, skipped int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.last.IsZero() && now.Sub(e.last) < e.d && now.After(e.last) {
		e.skipped++
		return false, 0
	}
	e.last = now
	skipped, e.skipped = e.skipped, 0
	return true, skipped
}
