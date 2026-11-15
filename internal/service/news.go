package service

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"deskpinmemo/internal/feed"
	"deskpinmemo/internal/store"
)

// RSS 新闻订阅（摸鱼功能 FR-701~707）。
//
// 网络原则：设置里没有开启时，程序不发出任何网络请求（后台循环只看设置，不碰网络）。
// 唯一例外是用户在设置页亲手点的「测试连接」。

// 保留策略：每个源最多 100 条，7 天前的自动清理。
const (
	newsKeepPerFeed = 100
	newsKeepDays    = 7
	newsParallel    = 4
)

type newsRuntime struct {
	wake      chan struct{}
	transport http.RoundTripper // 测试注入；为空时按代理设置新建
	sysProxy  feed.ProxyFunc    // 「跟随系统代理」时使用（Windows 由外壳提供，其他平台读环境变量）
	userAgent string
	roundMu   sync.Mutex // 同一时刻只跑一轮拉取
}

func (s *Service) initNews(o Options) {
	s.news.wake = make(chan struct{}, 1)
	s.news.transport = o.HTTPTransport
	s.news.sysProxy = o.SystemProxy
	if s.news.sysProxy == nil {
		s.news.sysProxy = http.ProxyFromEnvironment
	}
	s.news.userAgent = "DeskPinMemo/" + o.Version
	if o.Version == "" {
		s.news.userAgent = "DeskPinMemo"
	}
}

// SetSystemProxy 注入「跟随系统」时使用的代理（Windows 外壳在启动时调用）。
func (s *Service) SetSystemProxy(p feed.ProxyFunc) {
	if p != nil {
		s.news.sysProxy = p
	}
}

func (s *Service) newsClient() (*http.Client, error) {
	st := s.GetSettings()
	var p feed.ProxyFunc
	switch st.NewsProxy {
	case "none":
		p = feed.NoProxy
	case "manual":
		var err error
		if p, err = feed.ManualProxy(st.NewsProxyURL); err != nil {
			return nil, err
		}
	default:
		p = s.news.sysProxy
	}
	return feed.NewClient(p, s.news.transport), nil
}

// NewsKick 唤醒后台拉取循环（设置变更、添加订阅、电脑唤醒）。
func (s *Service) NewsKick() {
	select {
	case s.news.wake <- struct{}{}:
	default:
	}
}

// NewsResume 电脑从睡眠中唤醒：开启了新闻时立即补拉一次。
func (s *Service) NewsResume() {
	if s.GetSettings().NewsEnabled {
		_ = s.st.ResetFeedSchedule()
		s.NewsKick()
	}
}

// RunNews 是后台拉取循环，直到 ctx 结束。没开启新闻时只是定时看一眼设置，不碰网络。
func (s *Service) RunNews(ctx context.Context) {
	for {
		wait := time.Minute
		if s.GetSettings().NewsEnabled {
			s.fetchDue(ctx)
			if next, ok := s.st.NextFeedDue(); ok {
				if d := time.UnixMilli(next).Sub(s.now()); d < wait {
					wait = max(d, time.Second)
				}
			}
		}
		t := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-s.news.wake:
			t.Stop()
		case <-t.C:
		}
	}
}

// fetchDue 拉取所有到期的订阅源（最多 4 个并行），返回新增条数。
func (s *Service) fetchDue(ctx context.Context) int {
	s.news.roundMu.Lock()
	defer s.news.roundMu.Unlock()
	if !s.GetSettings().NewsEnabled {
		return 0
	}
	due, err := s.st.DueFeeds(s.now().UnixMilli())
	if err != nil || len(due) == 0 {
		return 0
	}
	c, err := s.newsClient()
	if err != nil {
		for _, f := range due {
			_ = s.st.RecordFetch(f.ID, store.FetchRecord{Error: err.Error(), NextFetchAt: s.nextFetch(f.FailCount + 1)})
		}
		s.Emit("feeds:changed", nil)
		return 0
	}
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, newsParallel)
	added := 0
	for _, f := range due {
		wg.Add(1)
		sem <- struct{}{}
		go func(f store.Feed) {
			defer wg.Done()
			defer func() { <-sem }()
			n, _ := s.fetchOne(ctx, c, f)
			mu.Lock()
			added += n
			mu.Unlock()
		}(f)
	}
	wg.Wait()
	if added > 0 {
		_ = s.st.PruneFeedItems(newsKeepPerFeed, s.now().AddDate(0, 0, -newsKeepDays).UnixMilli())
		s.Emit("news:changed", nil)
	}
	s.Emit("feeds:changed", nil)
	return added
}

func (s *Service) nextFetch(fails int) int64 {
	iv := time.Duration(s.GetSettings().NewsInterval) * time.Minute
	return feed.NextFetch(s.now(), iv, fails).UnixMilli()
}

func (s *Service) fetchOne(ctx context.Context, c *http.Client, f store.Feed) (int, error) {
	r, err := feed.Fetch(ctx, c, f.URL, s.news.userAgent, f.ETag, f.LastModified)
	if err != nil {
		_ = s.st.RecordFetch(f.ID, store.FetchRecord{Error: feed.Describe(err), NextFetchAt: s.nextFetch(f.FailCount + 1)})
		return 0, err
	}
	added := 0
	if !r.NotModified {
		items := make([]store.NewFeedItem, 0, len(r.Feed.Items))
		for _, it := range r.Feed.Items {
			if !feed.SafeLink(it.Link) {
				continue
			}
			var pub int64
			if !it.Published.IsZero() {
				pub = it.Published.UnixMilli()
			}
			items = append(items, store.NewFeedItem{GUID: it.GUID, Title: it.Title, Link: it.Link, Summary: it.Summary, PublishedAt: pub})
		}
		if added, err = s.st.SaveFeedItems(f.ID, items); err != nil {
			return 0, err
		}
	}
	return added, s.st.RecordFetch(f.ID, store.FetchRecord{OK: true, Title: r.Feed.Title, ETag: r.ETag, LastModified: r.LastModified, NextFetchAt: s.nextFetch(0)})
}

// ---- 前端 API ----

// PresetView 是预置订阅源及其是否已订阅。
type PresetView struct {
	feed.Preset
	Added bool `json:"added"`
}

// FeedPresets 返回预置订阅源列表。
func (s *Service) FeedPresets() ([]PresetView, error) {
	fs, err := s.st.ListFeeds()
	if err != nil {
		return nil, err
	}
	have := map[string]bool{}
	for _, f := range fs {
		have[f.URL] = true
	}
	out := make([]PresetView, 0, len(feed.Presets))
	for _, p := range feed.Presets {
		out = append(out, PresetView{Preset: p, Added: have[p.URL]})
	}
	return out, nil
}

// SetupNews 首次开启新闻：订阅勾选的预置源（可以一个都不选），然后开启新闻。
func (s *Service) SetupNews(urls []string) (Settings, error) {
	presets := map[string]feed.Preset{}
	for _, p := range feed.Presets {
		presets[p.URL] = p
	}
	for _, u := range urls {
		p, ok := presets[u]
		if !ok {
			return s.GetSettings(), errors.New("不是预置的订阅源：" + u)
		}
		if _, err := s.st.AddFeed(p.URL, p.Name, true); err != nil && !errors.Is(err, store.ErrFeedExists) {
			return s.GetSettings(), err
		}
	}
	st := s.GetSettings()
	st.NewsEnabled, st.NewsSetupDone = true, true
	return s.SaveSettings(st)
}

// ListFeeds 返回全部订阅源（含状态）。
func (s *Service) ListFeeds() ([]store.Feed, error) { return s.st.ListFeeds() }

// AddFeed 添加自定义订阅源。新闻已开启时立即拉取一次，返回带状态的记录（拉取失败不算添加失败，原因显示在状态里）。
func (s *Service) AddFeed(rawURL string) (store.Feed, error) {
	u, err := feed.NormalizeURL(rawURL)
	if err != nil {
		return store.Feed{}, err
	}
	f, err := s.st.AddFeed(u, "", false)
	if err != nil {
		return f, err
	}
	if s.GetSettings().NewsEnabled {
		if c, err := s.newsClient(); err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), feed.Timeout+2*time.Second)
			if n, _ := s.fetchOne(ctx, c, f); n > 0 {
				s.Emit("news:changed", nil)
			}
			cancel()
		}
		f, _ = s.st.GetFeed(f.ID)
	}
	s.Emit("feeds:changed", nil)
	return f, nil
}

// UpdateFeed 改名 / 启用 / 停用（nil = 不改）。
func (s *Service) UpdateFeed(id string, title *string, enabled *bool) (store.Feed, error) {
	if err := s.st.UpdateFeed(id, title, enabled); err != nil {
		return store.Feed{}, err
	}
	if enabled != nil {
		s.Emit("news:changed", nil)
		s.NewsKick()
	}
	s.Emit("feeds:changed", nil)
	return s.st.GetFeed(id)
}

// DeleteFeed 删除订阅源及其条目。
func (s *Service) DeleteFeed(id string) error {
	if err := s.st.DeleteFeed(id); err != nil {
		return err
	}
	s.Emit("news:changed", nil)
	s.Emit("feeds:changed", nil)
	return nil
}

// RefreshNews 立即拉取全部已启用的源（「立即刷新」按钮），返回新增条数。没开启新闻时什么也不做。
func (s *Service) RefreshNews() (int, error) {
	if !s.GetSettings().NewsEnabled {
		return 0, nil
	}
	if err := s.st.ResetFeedSchedule(); err != nil {
		return 0, err
	}
	return s.fetchDue(context.Background()), nil
}

// FeedTest 是「测试连接」的结果。
type FeedTest struct {
	OK     bool   `json:"ok"`
	Title  string `json:"title"`
	Count  int    `json:"count"`
	Latest string `json:"latest"`
	Error  string `json:"error"`
}

// TestFeed 试拉一次（不保存）。这是用户亲手发起的操作，新闻关闭时也允许。
func (s *Service) TestFeed(rawURL string) FeedTest {
	c, err := s.newsClient()
	if err != nil {
		return FeedTest{Error: err.Error()}
	}
	r, err := feed.Fetch(context.Background(), c, rawURL, s.news.userAgent, "", "")
	if err != nil {
		return FeedTest{Error: feed.Describe(err)}
	}
	t := FeedTest{OK: true, Title: r.Feed.Title, Count: len(r.Feed.Items)}
	if len(r.Feed.Items) > 0 {
		t.Latest = r.Feed.Items[0].Title
	}
	return t
}

// NewsItems 返回最新条目；feedID 非空时只看该源。
func (s *Service) NewsItems(feedID string, limit int) ([]store.FeedItem, error) {
	if limit <= 0 || limit > 300 {
		limit = 50
	}
	return s.st.ListFeedItems(feedID, limit)
}

// NewsUnread 返回未读条数。
func (s *Service) NewsUnread() int { return s.st.UnreadFeedItems() }

// OpenNewsItem 标为已读并返回可以安全交给浏览器打开的链接。
func (s *Service) OpenNewsItem(id string) (string, error) {
	it, err := s.st.GetFeedItem(id)
	if err != nil {
		return "", err
	}
	if !feed.SafeLink(it.Link) {
		return "", errors.New("链接不安全，已阻止打开")
	}
	_ = s.st.MarkFeedItemRead(id)
	s.Emit("news:changed", nil)
	return it.Link, nil
}

// MarkAllNewsRead 全部标为已读（feedID 非空时只标该源）。
func (s *Service) MarkAllNewsRead(feedID string) error {
	if err := s.st.MarkAllFeedItemsRead(feedID); err != nil {
		return err
	}
	s.Emit("news:changed", nil)
	return nil
}

// onNewsSettings 在设置保存后调用：开启、间隔、代理变化时让全部源立即到期并唤醒循环。
func (s *Service) onNewsSettings(old, cur Settings) {
	if !cur.NewsEnabled {
		return
	}
	if !old.NewsEnabled || old.NewsInterval != cur.NewsInterval || old.NewsProxy != cur.NewsProxy || old.NewsProxyURL != cur.NewsProxyURL {
		_ = s.st.ResetFeedSchedule()
		s.NewsKick()
	}
}
