import { useCallback, useEffect, useState } from 'react'
import { api, on } from '../api'
import type { Feed, NewsItem } from '../types'
import { ago } from '../fun'

/** 便签里的「新闻」视图（FR-706）：按时间倒序，可按来源筛选，点开用浏览器看原文。 */
export function NewsList() {
  const [feeds, setFeeds] = useState<Feed[]>([])
  const [feedId, setFeedId] = useState('')
  const [items, setItems] = useState<NewsItem[]>([])
  const load = useCallback(async () => {
    try {
      const [f, it] = await Promise.all([api.listFeeds(), api.newsItems(feedId, 100)])
      setFeeds(f.filter((x) => x.enabled)); setItems(it)
    } catch { /* 忽略 */ }
  }, [feedId])
  useEffect(() => { void load() }, [load])
  useEffect(() => on('news:changed', () => void load()), [load])
  useEffect(() => on('feeds:changed', () => void load()), [load])

  return (
    <div className="newslist" data-testid="news-list">
      <div className="newstools">
        <select value={feedId} onChange={(e) => setFeedId(e.target.value)} aria-label="按来源筛选" data-testid="news-filter">
          <option value="">全部来源</option>
          {feeds.map((f) => <option key={f.id} value={f.id}>{f.title || f.url}{f.unread ? `（${f.unread}）` : ''}</option>)}
        </select>
        <span style={{ flex: 1 }} />
        <button className="btn" onClick={() => void api.markAllNewsRead(feedId)} data-testid="news-mark-all">全部标为已读</button>
      </div>
      {items.length === 0 ? <div className="empty">还没有新闻</div> : (
        <ul className="list">
          {items.map((i) => (
            <li key={i.id} className={`newsrow ${i.read ? 'read' : ''}`} data-testid="news-row" onClick={() => void api.openNewsItem(i.id)} title={i.summary || i.title}>
              <div className="title">{i.title}</div>
              <div className="meta"><span>{i.feedTitle}</span><span>{ago(i.publishedAt)}</span></div>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
