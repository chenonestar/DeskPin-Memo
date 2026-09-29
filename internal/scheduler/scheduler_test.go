package scheduler

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"deskpinmemo/internal/store"
)

type fakeNotifier struct {
	mu   sync.Mutex
	got  []Notification
	fail bool
}

func (f *fakeNotifier) Notify(n Notification) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail {
		return context.DeadlineExceeded
	}
	f.got = append(f.got, n)
	return nil
}

func setup(t *testing.T, start time.Time) (*store.Store, *Scheduler, *fakeNotifier, *time.Time) {
	now := start
	st, err := store.Open(filepath.Join(t.TempDir(), "d.db"), store.Options{Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	fn := &fakeNotifier{}
	s := New(Config{Source: st, Notifier: fn, Now: func() time.Time { return now }, Loc: func() *time.Location { return time.UTC },
		StrongEnabled: func() bool { return true }})
	return st, s, fn, &now
}

func remindAt(t time.Time) []store.ReminderInput {
	ms := t.UnixMilli()
	return []store.ReminderInput{{RemindAt: &ms}}
}

func TestFiresOnTimeOnce(t *testing.T) { // AC-05 的调度部分
	t0 := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	st, s, fn, now := setup(t, t0)
	it, _ := st.CreateItem(store.NewItem{Title: "开会", Reminders: remindAt(t0.Add(2 * time.Minute))})
	next := s.Fire()
	if len(fn.got) != 0 || !next.Equal(t0.Add(2*time.Minute)) {
		t.Fatalf("未到点不应触发, next=%v", next)
	}
	*now = t0.Add(2*time.Minute + time.Second)
	s.Fire()
	if len(fn.got) != 1 || fn.got[0].Title != "开会" || fn.got[0].ItemID != it.ID || len(fn.got[0].Actions) != 4 {
		t.Fatalf("%+v", fn.got)
	}
	s.Fire()
	if len(fn.got) != 1 {
		t.Fatal("不应重复触发")
	}
	// 稍后 10 分钟
	st.SnoozeItem(it.ID, now.Add(10*time.Minute).UnixMilli())
	*now = now.Add(10*time.Minute + time.Second)
	s.Fire()
	if len(fn.got) != 2 {
		t.Fatal("稍后提醒应再次弹出")
	}
}

func TestMissedSummary(t *testing.T) { // FR-306 / AC-06
	t0 := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	st, s, fn, now := setup(t, t0)
	for i, title := range []string{"甲", "乙", "丙"} {
		st.CreateItem(store.NewItem{Title: title, Reminders: remindAt(t0.Add(time.Duration(i+1) * time.Minute))})
	}
	*now = t0.Add(3 * time.Hour) // 睡眠 3 小时后唤醒
	s.Fire()
	if len(fn.got) != 1 || !fn.got[0].Summary || fn.got[0].Title != "错过了 3 条提醒" {
		t.Fatalf("应汇总补发一条: %+v", fn.got)
	}
	if p, _ := st.PendingReminders(); len(p) != 0 {
		t.Fatal("补发后应标记已触发")
	}
}

func TestSingleMissedAndLocked(t *testing.T) {
	t0 := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	st, s, fn, now := setup(t, t0)
	st.CreateItem(store.NewItem{Title: "单条", Reminders: remindAt(t0.Add(time.Minute))})
	*now = t0.Add(time.Hour)
	s.Fire()
	if len(fn.got) != 1 || !fn.got[0].Missed || fn.got[0].Summary {
		t.Fatalf("%+v", fn.got)
	}
}

func TestStrongForHighPriority(t *testing.T) {
	t0 := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	st, s, fn, now := setup(t, t0)
	hi := 2
	st.CreateItem(store.NewItem{Title: "重要", Priority: &hi, Reminders: remindAt(t0.Add(time.Second))})
	st.CreateItem(store.NewItem{Title: "普通", Reminders: remindAt(t0.Add(time.Second))})
	*now = t0.Add(2 * time.Second)
	s.Fire()
	strong := map[string]bool{}
	for _, n := range fn.got {
		strong[n.Title] = n.Strong
	}
	if !strong["重要"] || strong["普通"] {
		t.Fatalf("%v", strong)
	}
}

func TestLockedNotificationHidesContent(t *testing.T) { // AC-15
	t0 := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	st, s, fn, now := setup(t, t0)
	st.CreateItem(store.NewItem{Title: "机密事项", Reminders: remindAt(t0.Add(time.Second))})
	st.SetEncrypted(true) // 已加密、未解锁
	*now = t0.Add(2 * time.Second)
	s.Fire()
	if len(fn.got) != 1 || fn.got[0].Title != "有 1 条事项提醒，解锁后查看" || fn.got[0].Body != "" || len(fn.got[0].Actions) != 0 {
		t.Fatalf("%+v", fn.got)
	}
}

func TestNotifyFailureRetries(t *testing.T) {
	t0 := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	st, s, fn, now := setup(t, t0)
	st.CreateItem(store.NewItem{Title: "x", Reminders: remindAt(t0.Add(time.Second))})
	fn.fail = true
	*now = t0.Add(2 * time.Second)
	if next := s.Fire(); next.IsZero() {
		t.Fatal("失败后应安排重试")
	}
	fn.fail = false
	*now = now.Add(6 * time.Second)
	s.Fire()
	if len(fn.got) != 1 {
		t.Fatal("重试应成功")
	}
}

func TestDND(t *testing.T) {
	loc := time.UTC
	d := DND{Enabled: true, Start: "22:00", End: "08:00"}
	at := func(h, m int) time.Time { return time.Date(2026, 6, 1, h, m, 0, 0, loc) }
	for _, c := range []struct{ in, want time.Time }{
		{at(23, 0), time.Date(2026, 6, 2, 8, 0, 0, 0, loc)},
		{at(3, 0), at(8, 0)},
		{at(12, 0), at(12, 0)},
		{at(8, 0), at(8, 0)},
	} {
		if got := d.Adjust(c.in, loc); !got.Equal(c.want) {
			t.Errorf("%v -> %v want %v", c.in, got, c.want)
		}
	}
	if got := (DND{}).Adjust(at(23, 0), loc); !got.Equal(at(23, 0)) {
		t.Fatal("未启用")
	}
	// 调度中：免打扰内的提醒延后到时段结束
	t0 := time.Date(2026, 6, 1, 21, 59, 0, 0, time.UTC)
	st, s, fn, now := setup(t, t0)
	s.dnd = func() DND { return d }
	st.CreateItem(store.NewItem{Title: "夜里", Reminders: remindAt(at(23, 0))})
	*now = at(23, 0).Add(time.Second)
	if next := s.Fire(); len(fn.got) != 0 || !next.Equal(time.Date(2026, 6, 2, 8, 0, 0, 0, loc)) {
		t.Fatalf("免打扰应延后: %v %v", fn.got, next)
	}
	*now = time.Date(2026, 6, 2, 8, 0, 1, 0, loc)
	s.Fire()
	if len(fn.got) != 1 {
		t.Fatal("时段结束后应弹出")
	}
}

func TestRunLoopWakesOnKick(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "d.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	fn := &fakeNotifier{}
	s := New(Config{Source: st, Notifier: fn})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.Run(ctx); close(done) }()
	time.Sleep(50 * time.Millisecond)
	at := time.Now().Add(300 * time.Millisecond)
	st.CreateItem(store.NewItem{Title: "定时", Reminders: remindAt(at)})
	s.Kick()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		fn.mu.Lock()
		n := len(fn.got)
		fn.mu.Unlock()
		if n == 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	fn.mu.Lock()
	n := len(fn.got)
	fn.mu.Unlock()
	if n != 1 {
		t.Fatalf("Run 循环应准时触发, got %d", n)
	}
	cancel()
	<-done
}
