package store

import (
	"database/sql"
	"errors"
	"strings"
)

// Feed 是一个 RSS 订阅源及其拉取状态。
type Feed struct {
	ID           string `json:"id"`
	URL          string `json:"url"`
	Title        string `json:"title"`
	CustomTitle  bool   `json:"customTitle"`
	Enabled      bool   `json:"enabled"`
	Preset       bool   `json:"preset"`
	SortOrder    float64
	ETag         string `json:"-"`
	LastModified string `json:"-"`
	LastFetchAt  *int64 `json:"lastFetchAt"`
	LastOKAt     *int64 `json:"lastOkAt"`
	LastError    string `json:"lastError"`
	FailCount    int    `json:"failCount"`
	NextFetchAt  int64  `json:"nextFetchAt"`
	CreatedAt    int64  `json:"createdAt"`
	ItemCount    int    `json:"itemCount"`
	Unread       int    `json:"unread"`
}

// FeedItem 是一条新闻。
type FeedItem struct {
	ID          string `json:"id"`
	FeedID      string `json:"feedId"`
	FeedTitle   string `json:"feedTitle"`
	GUID        string `json:"-"`
	Title       string `json:"title"`
	Link        string `json:"link"`
	Summary     string `json:"summary"`
	PublishedAt int64  `json:"publishedAt"`
	FetchedAt   int64  `json:"fetchedAt"`
	Read        bool   `json:"read"`
}

// NewFeedItem 是写入时的条目。
type NewFeedItem struct {
	GUID, Title, Link, Summary string
	PublishedAt                int64 // 0 = 用拉取时间
}

// ErrFeedExists 表示该地址已经订阅过。
var ErrFeedExists = errors.New("已经订阅过这个地址")

const feedCols = `f.id,f.url,f.title,f.custom_title,f.enabled,f.preset,f.sort_order,f.etag,f.last_modified,f.last_fetch_at,f.last_ok_at,f.last_error,f.fail_count,f.next_fetch_at,f.created_at`

func scanFeed(sc interface{ Scan(...any) error }, extra ...any) (Feed, error) {
	var f Feed
	var ct, en, pr int
	var lf, lo sql.NullInt64
	dst := []any{&f.ID, &f.URL, &f.Title, &ct, &en, &pr, &f.SortOrder, &f.ETag, &f.LastModified, &lf, &lo, &f.LastError, &f.FailCount, &f.NextFetchAt, &f.CreatedAt}
	if err := sc.Scan(append(dst, extra...)...); err != nil {
		return f, err
	}
	f.CustomTitle, f.Enabled, f.Preset = ct == 1, en == 1, pr == 1
	if lf.Valid {
		f.LastFetchAt = &lf.Int64
	}
	if lo.Valid {
		f.LastOKAt = &lo.Int64
	}
	return f, nil
}

// ListFeeds 返回全部订阅源（含条目数与未读数），按添加顺序。
func (s *Store) ListFeeds() ([]Feed, error) {
	rows, err := s.db.Query(`SELECT ` + feedCols + `,
		(SELECT COUNT(*) FROM feed_items i WHERE i.feed_id=f.id),
		(SELECT COUNT(*) FROM feed_items i WHERE i.feed_id=f.id AND i.read=0)
		FROM feeds f ORDER BY f.sort_order, f.created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Feed{}
	for rows.Next() {
		var f Feed
		var n, u int
		if f, err = scanFeed(rows, &n, &u); err != nil {
			return nil, err
		}
		f.ItemCount, f.Unread = n, u
		out = append(out, f)
	}
	return out, rows.Err()
}

// GetFeed 按 id 读取订阅源。
func (s *Store) GetFeed(id string) (Feed, error) {
	return scanFeed(s.db.QueryRow(`SELECT `+feedCols+` FROM feeds f WHERE f.id=?`, id))
}

// AddFeed 添加订阅源；地址已存在返回 ErrFeedExists 与已有记录。
func (s *Store) AddFeed(url, title string, preset bool) (Feed, error) {
	var f Feed
	err := s.Tx(func(tx *sql.Tx) error {
		if ex, err := scanFeed(tx.QueryRow(`SELECT `+feedCols+` FROM feeds f WHERE f.url=?`, url)); err == nil {
			f = ex
			return ErrFeedExists
		}
		var maxOrder float64
		_ = tx.QueryRow(`SELECT COALESCE(MAX(sort_order),0) FROM feeds`).Scan(&maxOrder)
		f = Feed{ID: newID(), URL: url, Title: title, Enabled: true, Preset: preset, SortOrder: maxOrder + 1, CreatedAt: s.nowMs()}
		_, err := tx.Exec(`INSERT INTO feeds(id,url,title,enabled,preset,sort_order,created_at) VALUES(?,?,?,?,?,?,?)`,
			f.ID, f.URL, f.Title, 1, b2i(preset), f.SortOrder, f.CreatedAt)
		return err
	})
	return f, err
}

// UpdateFeed 修改订阅源名称 / 启用状态（nil = 不改）。名称改为空串表示恢复使用源里的标题。
func (s *Store) UpdateFeed(id string, title *string, enabled *bool) error {
	return s.Tx(func(tx *sql.Tx) error {
		if title != nil {
			t := strings.TrimSpace(*title)
			if _, err := tx.Exec(`UPDATE feeds SET title=CASE WHEN ?='' THEN title ELSE ? END, custom_title=? WHERE id=?`, t, t, b2i(t != ""), id); err != nil {
				return err
			}
		}
		if enabled != nil {
			if _, err := tx.Exec(`UPDATE feeds SET enabled=?, next_fetch_at=0 WHERE id=?`, b2i(*enabled), id); err != nil {
				return err
			}
		}
		return nil
	})
}

// DeleteFeed 删除订阅源及其全部条目。
func (s *Store) DeleteFeed(id string) error {
	return s.Tx(func(tx *sql.Tx) error {
		if _, err := tx.Exec(`DELETE FROM feed_items WHERE feed_id=?`, id); err != nil {
			return err
		}
		_, err := tx.Exec(`DELETE FROM feeds WHERE id=?`, id)
		return err
	})
}

// DueFeeds 返回已启用且到了拉取时间的订阅源。
func (s *Store) DueFeeds(nowMs int64) ([]Feed, error) {
	rows, err := s.db.Query(`SELECT `+feedCols+` FROM feeds f WHERE f.enabled=1 AND f.next_fetch_at<=? ORDER BY f.next_fetch_at`, nowMs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Feed
	for rows.Next() {
		f, err := scanFeed(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// NextFeedDue 返回最近一个已启用订阅源的下次拉取时间；没有则 ok 为 false。
func (s *Store) NextFeedDue() (int64, bool) {
	var v sql.NullInt64
	_ = s.db.QueryRow(`SELECT MIN(next_fetch_at) FROM feeds WHERE enabled=1`).Scan(&v)
	return v.Int64, v.Valid
}

// ResetFeedSchedule 让全部订阅源立即到期（唤醒、修改间隔、手动刷新）。
func (s *Store) ResetFeedSchedule() error {
	_, err := s.db.Exec(`UPDATE feeds SET next_fetch_at=0`)
	return err
}

// FetchRecord 是一次拉取的结果记录。
type FetchRecord struct {
	OK           bool
	Title        string // 源里的标题（成功时）
	ETag         string
	LastModified string
	Error        string
	NextFetchAt  int64
}

// RecordFetch 记录拉取结果。成功：清零失败计数、更新条件请求头与标题（用户没改过名时）；失败：记录原因、计数加一。
func (s *Store) RecordFetch(id string, r FetchRecord) error {
	now := s.nowMs()
	if r.OK {
		_, err := s.db.Exec(`UPDATE feeds SET last_fetch_at=?, last_ok_at=?, last_error='', fail_count=0, etag=?, last_modified=?, next_fetch_at=?,
			title=CASE WHEN custom_title=0 AND ?<>'' THEN ? ELSE title END WHERE id=?`,
			now, now, r.ETag, r.LastModified, r.NextFetchAt, r.Title, r.Title, id)
		return err
	}
	_, err := s.db.Exec(`UPDATE feeds SET last_fetch_at=?, last_error=?, fail_count=fail_count+1, next_fetch_at=? WHERE id=?`, now, r.Error, r.NextFetchAt, id)
	return err
}

// SaveFeedItems 写入条目（同一源内按 guid 去重，已有的保持不变），返回新增条数。
func (s *Store) SaveFeedItems(feedID string, items []NewFeedItem) (int, error) {
	added := 0
	now := s.nowMs()
	err := s.Tx(func(tx *sql.Tx) error {
		for _, it := range items {
			pub := it.PublishedAt
			if pub <= 0 || pub > now+3600_000 { // 没有日期，或日期在未来（时区写错）：按拉取时间
				pub = now
			}
			res, err := tx.Exec(`INSERT OR IGNORE INTO feed_items(id,feed_id,guid,title,link,summary,published_at,fetched_at) VALUES(?,?,?,?,?,?,?,?)`,
				newID(), feedID, it.GUID, it.Title, it.Link, it.Summary, pub, now)
			if err != nil {
				return err
			}
			if n, _ := res.RowsAffected(); n > 0 {
				added++
			}
		}
		return nil
	})
	return added, err
}

// PruneFeedItems 每个源只保留最新 keep 条，并删除发布时间早于 olderThanMs 的条目。
func (s *Store) PruneFeedItems(keep int, olderThanMs int64) error {
	return s.Tx(func(tx *sql.Tx) error {
		if _, err := tx.Exec(`DELETE FROM feed_items WHERE published_at<?`, olderThanMs); err != nil {
			return err
		}
		_, err := tx.Exec(`DELETE FROM feed_items WHERE id IN (
			SELECT id FROM (SELECT id, ROW_NUMBER() OVER (PARTITION BY feed_id ORDER BY published_at DESC, fetched_at DESC) AS rn FROM feed_items) WHERE rn>?)`, keep)
		return err
	})
}

const feedItemCols = `i.id,i.feed_id,f.title,i.guid,i.title,i.link,i.summary,i.published_at,i.fetched_at,i.read`

func scanFeedItem(sc interface{ Scan(...any) error }) (FeedItem, error) {
	var it FeedItem
	var rd int
	err := sc.Scan(&it.ID, &it.FeedID, &it.FeedTitle, &it.GUID, &it.Title, &it.Link, &it.Summary, &it.PublishedAt, &it.FetchedAt, &rd)
	it.Read = rd == 1
	return it, err
}

// ListFeedItems 返回最新的条目（只含已启用的源）；feedID 非空时只看该源。
func (s *Store) ListFeedItems(feedID string, limit int) ([]FeedItem, error) {
	q := `SELECT ` + feedItemCols + ` FROM feed_items i JOIN feeds f ON f.id=i.feed_id WHERE f.enabled=1`
	var args []any
	if feedID != "" {
		q += ` AND i.feed_id=?`
		args = append(args, feedID)
	}
	q += ` ORDER BY i.published_at DESC, i.fetched_at DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []FeedItem{}
	for rows.Next() {
		it, err := scanFeedItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// GetFeedItem 按 id 读取条目。
func (s *Store) GetFeedItem(id string) (FeedItem, error) {
	return scanFeedItem(s.db.QueryRow(`SELECT `+feedItemCols+` FROM feed_items i JOIN feeds f ON f.id=i.feed_id WHERE i.id=?`, id))
}

// MarkFeedItemRead 标记一条为已读。
func (s *Store) MarkFeedItemRead(id string) error {
	_, err := s.db.Exec(`UPDATE feed_items SET read=1 WHERE id=?`, id)
	return err
}

// MarkAllFeedItemsRead 全部标为已读（feedID 非空时只标该源）。
func (s *Store) MarkAllFeedItemsRead(feedID string) error {
	if feedID != "" {
		_, err := s.db.Exec(`UPDATE feed_items SET read=1 WHERE feed_id=?`, feedID)
		return err
	}
	_, err := s.db.Exec(`UPDATE feed_items SET read=1`)
	return err
}

// UnreadFeedItems 返回已启用源的未读条数。
func (s *Store) UnreadFeedItems() int {
	var n int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM feed_items i JOIN feeds f ON f.id=i.feed_id WHERE f.enabled=1 AND i.read=0`).Scan(&n)
	return n
}
