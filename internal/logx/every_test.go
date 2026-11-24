package logx

import (
	"testing"
	"time"
)

func TestEvery(t *testing.T) {
	e := NewEvery(time.Hour)
	t0 := time.Date(2026, 11, 24, 8, 0, 0, 0, time.Local)
	if ok, n := e.Allow(t0); !ok || n != 0 {
		t.Fatal("第一条要记", ok, n)
	}
	for i := 1; i <= 5; i++ {
		if ok, _ := e.Allow(t0.Add(time.Duration(i) * 10 * time.Minute)); ok {
			t.Fatal("一小时内只记一条", i)
		}
	}
	if ok, n := e.Allow(t0.Add(61 * time.Minute)); !ok || n != 5 {
		t.Fatal("过了一小时再记，并带上省略的条数", ok, n)
	}
	// 系统时间被往回调：不能因此一直不记
	if ok, _ := e.Allow(t0.Add(-2 * time.Hour)); !ok {
		t.Fatal("时间倒退时照常记")
	}
}
