package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"deskpinmemo/internal/feed"
	"deskpinmemo/internal/store"
)

// fakeNet 是假的网络：按地址返回预设内容，并统计请求次数（用于断言「关闭时零请求」）。
type fakeNet struct {
	mu    sync.Mutex
	feeds map[string]string
	calls atomic.Int32
}

func (f *fakeNet) RoundTrip(r *http.Request) (*http.Response, error) {
	f.calls.Add(1)
	f.mu.Lock()
	body, ok := f.feeds[r.URL.String()]
	f.mu.Unlock()
	if !ok {
		return &http.Response{StatusCode: 404, Body: io.NopCloser(strings.NewReader("")), Header: http.Header{}, Request: r}, nil
	}
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}, Request: r}, nil
}

func (f *fakeNet) set(url, body string) { f.mu.Lock(); f.feeds[url] = body; f.mu.Unlock() }

func rssOf(title string, items ...string) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<rss><channel><title>%s</title>`, title)
	for i, it := range items {
		fmt.Fprintf(&b, `<item><title>%s</title><link>https://example.com/%d</link><guid>%s</guid><pubDate>Sat, 14 Nov 2026 0%d:00:00 +0800</pubDate></item>`, it, i, it, i)
	}
	b.WriteString(`</channel></rss>`)
	return b.String()
}

func setupNews(t *testing.T) (*env, *fakeNet) {
	t.Helper()
	dir := t.TempDir()
	now := time.Date(2026, 11, 15, 10, 0, 0, 0, cst)
	st, err := store.Open(filepath.Join(dir, "data.db"), store.Options{Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	net := &fakeNet{feeds: map[string]string{}}
	svc, err := New(st, dir, Options{Now: func() time.Time { return now }, Loc: func() *time.Location { return cst }, HTTPTransport: net, Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	return &env{svc, st, &now, dir}, net
}

func TestNewsDisabledMeansZeroNetwork(t *testing.T) {
	e, net := setupNews(t)
	if e.svc.GetSettings().NewsEnabled {
		t.Fatal("新闻默认关闭")
	}
	// 即使数据库里有订阅源（例如关闭前订阅过），关闭时后台循环也绝不联网
	_, _ = e.st.AddFeed("https://a.com/feed", "A", false)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { e.svc.RunNews(ctx); close(done) }()
	for i := 0; i < 5; i++ {
		e.svc.NewsKick()
		e.svc.NewsResume()
		time.Sleep(10 * time.Millisecond)
	}
	if n, _ := e.svc.RefreshNews(); n != 0 {
		t.Fatal("关闭时刷新什么也不做")
	}
	if _, err := e.svc.AddFeed("https://b.com/feed"); err != nil {
		t.Fatal(err)
	}
	cancel()
	<-done
	if c := net.calls.Load(); c != 0 {
		t.Fatalf("新闻关闭时不应有任何网络请求，实际 %d 次", c)
	}
}

func TestNewsSetupFetchAndRead(t *testing.T) {
	e, net := setupNews(t)
	p := feed.Presets[0]
	net.set(p.URL, rssOf("少数派", "第一条", "第二条"))
	presets, _ := e.svc.FeedPresets()
	if len(presets) != len(feed.Presets) || presets[0].Added {
		t.Fatal(presets)
	}
	if _, err := e.svc.SetupNews([]string{"https://not-a-preset/x"}); err == nil {
		t.Fatal("只接受预置源")
	}
	s, err := e.svc.SetupNews([]string{p.URL})
	if err != nil || !s.NewsEnabled || !s.NewsSetupDone {
		t.Fatal(s, err)
	}
	if n, _ := e.svc.RefreshNews(); n != 2 {
		t.Fatalf("应拉到 2 条: %d", n)
	}
	items, _ := e.svc.NewsItems("", 10)
	if len(items) != 2 || items[0].FeedTitle != "少数派" || items[0].Title != "第二条" {
		t.Fatalf("%+v", items)
	}
	// 再拉一次：新增 1 条
	net.set(p.URL, rssOf("少数派", "第一条", "第二条", "第三条"))
	if n, _ := e.svc.RefreshNews(); n != 1 {
		t.Fatalf("增量: %d", n)
	}
	if e.svc.NewsUnread() != 3 {
		t.Fatal(e.svc.NewsUnread())
	}
	link, err := e.svc.OpenNewsItem(items[0].ID)
	if err != nil || !strings.HasPrefix(link, "https://example.com/") || e.svc.NewsUnread() != 2 {
		t.Fatal(link, err, e.svc.NewsUnread())
	}
	_ = e.svc.MarkAllNewsRead("")
	if e.svc.NewsUnread() != 0 {
		t.Fatal("全部已读")
	}
	// 失败的源：记录可读原因，失败计数增加，其他源不受影响
	bad, _ := e.svc.AddFeed("https://missing.example.com/rss")
	if bad.LastError == "" || bad.FailCount != 1 {
		t.Fatalf("添加时立即试拉，失败原因写进状态: %+v", bad)
	}
	if _, err := e.svc.AddFeed(p.URL); err == nil {
		t.Fatal("重复订阅")
	}
	feeds, _ := e.svc.ListFeeds()
	if len(feeds) != 2 || feeds[0].LastError != "" || feeds[0].LastOKAt == nil {
		t.Fatalf("%+v", feeds)
	}
	// 测试连接不保存
	if r := e.svc.TestFeed(p.URL); !r.OK || r.Count != 3 || r.Title != "少数派" {
		t.Fatalf("%+v", r)
	}
	if r := e.svc.TestFeed("https://missing.example.com/rss"); r.OK || !strings.Contains(r.Error, "404") {
		t.Fatalf("%+v", r)
	}
	// 停用的源不再拉取、不再显示
	off := false
	_, _ = e.svc.UpdateFeed(feeds[0].ID, nil, &off)
	before := net.calls.Load()
	_, _ = e.svc.RefreshNews()
	if items, _ := e.svc.NewsItems("", 10); len(items) != 0 {
		t.Fatal("停用的源不显示")
	}
	if net.calls.Load() != before+1 { // 只剩那个失败的源
		t.Fatalf("停用的源不拉取: %d → %d", before, net.calls.Load())
	}
}

func TestNewsManualProxyValidation(t *testing.T) {
	e, _ := setupNews(t)
	s := e.svc.GetSettings()
	s.NewsProxy, s.NewsProxyURL = "manual", "::bad::"
	e.svc.SaveSettings(s)
	if r := e.svc.TestFeed("https://a.com/feed"); r.OK || !strings.Contains(r.Error, "代理地址") {
		t.Fatalf("%+v", r)
	}
}

func TestPetLevelsAndUnlocks(t *testing.T) {
	cases := []struct{ exp, level, floor, next int }{{0, 1, 0, 50}, {49, 1, 0, 50}, {50, 2, 50, 150}, {149, 2, 50, 150}, {150, 3, 150, 300}, {2250, 10, 2250, 0}, {99999, 10, 2250, 0}}
	for _, c := range cases {
		l, f, n := PetLevel(c.exp)
		if l != c.level || f != c.floor || n != c.next {
			t.Errorf("exp=%d: %d %d %d", c.exp, l, f, n)
		}
	}
	acc, gl := PetUnlocks(1)
	if len(acc) != 1 || acc[0] != "" || len(gl) != 1 || gl[0] != "round" {
		t.Fatal(acc, gl)
	}
	acc, gl = PetUnlocks(10)
	if len(acc) != 5 || len(gl) != 4 {
		t.Fatal(acc, gl)
	}
}

func TestPetGainsExpFromCompletedItems(t *testing.T) {
	e := setup(t, time.Date(2026, 11, 15, 10, 0, 0, 0, cst))
	var events []PetEvent
	e.svc.Emit = func(ev string, d any) {
		if ev == "pet:event" {
			events = append(events, d.(PetEvent))
		}
	}
	it, _ := e.svc.CreateItem(store.NewItem{Title: "写周报"})
	// 没开宠物：完成事项不涉及宠物
	_, _ = e.svc.Toggle(it.ID, true)
	if _, ok, _ := e.st.GetPet(); ok || len(events) != 0 {
		t.Fatal("没开启宠物时不创建、不加经验")
	}
	_, _ = e.svc.Toggle(it.ID, false)
	s := e.svc.GetSettings()
	s.PetEnabled = true
	e.svc.SaveSettings(s)
	p, _ := e.svc.GetPet()
	if p.Name != PetDefaultName || p.Level != 1 || p.Days != 1 {
		t.Fatalf("%+v", p)
	}
	for i := 0; i < 5; i++ {
		x, _ := e.svc.CreateItem(store.NewItem{Title: fmt.Sprint("事项", i)})
		_, _ = e.svc.Toggle(x.ID, true)
	}
	p, _ = e.svc.GetPet()
	if p.Exp != 50 || p.Level != 2 || p.Completed != 5 || len(events) != 5 || !events[4].LevelUp || events[3].LevelUp {
		t.Fatalf("%+v %+v", p, events)
	}
	// 撤销完成：经验扣回
	if _, err := e.svc.Undo(); err != nil {
		t.Fatal(err)
	}
	p, _ = e.svc.GetPet()
	if p.Exp != 40 || p.Completed != 4 {
		t.Fatalf("撤销后扣回: %+v", p)
	}
	// 换装只能选已解锁的
	if _, err := e.svc.SetPetLook("crown", "round"); err == nil {
		t.Fatal("皇冠 9 级才解锁")
	}
	if _, err := e.svc.RenamePet("  "); err == nil {
		t.Fatal("名字不能为空")
	}
	if v, err := e.svc.RenamePet("旺财"); err != nil || v.Name != "旺财" {
		t.Fatal(v, err)
	}
	// 陪伴天数
	*e.now = e.now.Add(49 * time.Hour)
	if p, _ = e.svc.GetPet(); p.Days != 3 {
		t.Fatal(p.Days)
	}
}

func TestPetStatusCountsToday(t *testing.T) {
	e := setup(t, time.Date(2026, 11, 15, 10, 0, 0, 0, cst))
	at := func(h int) *int64 { v := time.Date(2026, 11, 15, h, 0, 0, 0, cst).UnixMilli(); return &v }
	a, _ := e.svc.CreateItem(store.NewItem{Title: "上午", DueAt: at(9)}) // 已逾期
	_, _ = e.svc.CreateItem(store.NewItem{Title: "下午", DueAt: at(15)})
	b, _ := e.svc.CreateItem(store.NewItem{Title: "晚上", DueAt: at(20)})
	_, _ = e.svc.Toggle(b.ID, true)
	st, err := e.svc.PetStatus()
	if err != nil || st.Overdue != 1 || st.TodayTotal != 3 || st.TodayLeft != 2 {
		t.Fatalf("%+v %v", st, err)
	}
	_, _ = e.svc.Toggle(a.ID, true)
	if st, _ = e.svc.PetStatus(); st.Overdue != 0 || st.TodayLeft != 1 {
		t.Fatalf("%+v", st)
	}
}

func TestFunSettingsNormalize(t *testing.T) {
	e := setup(t, time.Now())
	s := e.svc.GetSettings()
	if s.NewsInterval != 30 || s.NewsProxy != "system" || s.PetCorner != "right" || !s.PetBubbles || !s.PetWorry || s.WorkEnd != "18:00" || s.HotkeyBoss != "Ctrl+Alt+H" || s.NewsEnabled || s.PetEnabled || s.ShowCountdown {
		t.Fatalf("默认值: %+v", s)
	}
	s.NewsInterval, s.NewsProxy, s.PetCorner, s.WorkEnd, s.WorkStart, s.MonthlySalary = 7, "weird", "top", "25:00", "9:00", -1
	got, _ := e.svc.SaveSettings(s)
	if got.NewsInterval != 30 || got.NewsProxy != "system" || got.PetCorner != "right" || got.WorkEnd != "18:00" || got.WorkStart != "09:00" || got.MonthlySalary != 0 {
		t.Fatalf("%+v", got)
	}
}

func TestPetNamingPlaysAndWater(t *testing.T) {
	e := setup(t, time.Date(2026, 11, 16, 10, 0, 0, 0, cst))
	p, _ := e.svc.GetPet()
	if p.Name != "博士" || p.Named {
		t.Fatalf("默认名「博士」，尚未确认: %+v", p)
	}
	if p, _ = e.svc.RenamePet("摸摸"); !p.Named || p.Name != "摸摸" {
		t.Fatal(p)
	}
	if p, _ = e.svc.PlayBall(); p.Plays != 1 {
		t.Fatal(p)
	}
	_, _ = e.svc.DrinkWater()
	if p, _ = e.svc.DrinkWater(); p.WaterToday != 2 {
		t.Fatal(p.WaterToday)
	}
	*e.now = e.now.Add(24 * time.Hour)
	if p, _ = e.svc.GetPet(); p.WaterToday != 0 {
		t.Fatal("第二天还没喝时为 0", p.WaterToday)
	}
	if p, _ = e.svc.DrinkWater(); p.WaterToday != 1 {
		t.Fatal(p.WaterToday)
	}
	if p, _ = e.svc.ResetPet(); p.Named || p.Name != "博士" || p.Plays != 0 {
		t.Fatalf("重置后重新起名: %+v", p)
	}
	s := e.svc.GetSettings()
	if !s.PetHoliday || s.PetWater || s.PetWaterInterval != 45 || s.PetWaterNotify || s.CountdownMoney != "hover" || s.PetActivity != "normal" {
		t.Fatalf("默认值: %+v", s)
	}
	s.PetWaterInterval, s.PetActivity = 7, "crazy"
	if got, _ := e.svc.SaveSettings(s); got.PetWaterInterval != 45 || got.PetActivity != "normal" {
		t.Fatal(got.PetWaterInterval, got.PetActivity)
	}
}
