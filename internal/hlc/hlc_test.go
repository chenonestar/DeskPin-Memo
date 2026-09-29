package hlc

import (
	"testing"
	"time"
)

func TestMonotonicEvenWhenClockGoesBack(t *testing.T) {
	now := time.UnixMilli(1_700_000_000_000)
	c := NewWithNow("0190a1b2-c3d4-7000-8000-000000000000", func() time.Time { return now })
	prev := c.Next()
	if len(prev) != Len {
		t.Fatalf("len=%d", len(prev))
	}
	for i := 0; i < 50; i++ {
		if i == 10 {
			now = now.Add(-time.Hour) // 系统时间回调 1 小时（AC-16）
		}
		if i%7 == 0 {
			now = now.Add(time.Millisecond)
		}
		n := c.Next()
		if Compare(n, prev) <= 0 {
			t.Fatalf("not increasing: %s <= %s", n, prev)
		}
		prev = n
	}
}

func TestParseAndObserve(t *testing.T) {
	now := time.UnixMilli(1000)
	c := NewWithNow("abcdef12-0000", func() time.Time { return now })
	remote := NewWithNow("ffffffff", func() time.Time { return time.UnixMilli(5000) }).Next()
	c.Observe(remote)
	n := c.Next()
	if Compare(n, remote) <= 0 {
		t.Fatalf("observe failed %s %s", n, remote)
	}
	ms, _, node, err := Parse(n)
	if err != nil || ms < 5000 || node != "abcdef12" {
		t.Fatalf("parse: %d %s %v", ms, node, err)
	}
}

func TestFieldHLC(t *testing.T) {
	f := NewFieldHLC("A", "title", "due_at")
	f = f.Touch("B", "title")
	if f["title"] != "B" || f["due_at"] != "A" {
		t.Fatal(f)
	}
	if got := ParseFieldHLC(f.JSON()); got["title"] != "B" {
		t.Fatal(got)
	}
	w := Merge(FieldHLC{"title": "B", "due_at": "A"}, FieldHLC{"title": "A", "due_at": "C"})
	if w["title"] || !w["due_at"] {
		t.Fatal(w)
	}
}
