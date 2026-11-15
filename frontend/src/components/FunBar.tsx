import { useCallback, useEffect, useState } from 'react'
import { api, on } from '../api'
import type { NewsItem, Settings } from '../types'
import { ago, countdown, earnedToday, formatDuration } from '../fun'

/** 便签底部的一行：下班倒计时 + 新闻轮播。 */
export function FunBar({ settings, onOpenNews }: { settings: Settings; onOpenNews: () => void }) {
  const showNews = settings.newsEnabled
  const showCd = settings.showCountdown
  if (!showNews && !showCd) return null
  return (
    <div className="funbar" data-testid="funbar">
      {showCd && <CountdownChip settings={settings} />}
      {showNews && <NewsTicker settings={settings} onOpenNews={onOpenNews} />}
    </div>
  )
}

function CountdownChip({ settings }: { settings: Settings }) {
  const [now, setNow] = useState(() => new Date())
  useEffect(() => {
    const id = window.setInterval(() => { if (!document.hidden) setNow(new Date()) }, 1000)
    return () => clearInterval(id)
  }, [])
  const c = countdown(now, settings.workStart, settings.workEnd)
  const earned = earnedToday(settings.monthlySalary, c.fraction)
  let text: string
  switch (c.state) {
    case 'weekend': text = '周末愉快'; break
    case 'before': text = `距上班 ${formatDuration(c.remainingMs)}`; break
    case 'after': text = '已下班 🎉'; break
    default: text = `距下班 ${formatDuration(c.remainingMs)}`
  }
  const money = settings.monthlySalary > 0 && c.state !== 'weekend' ? ` · 已赚 ¥${earned.toFixed(2)}` : ''
  return <span className="cdchip" data-testid="countdown" title={`上班 ${settings.workStart} · 下班 ${settings.workEnd}`}>{text}{money}</span>
}

const ROTATE_MS = 8000

function NewsTicker({ settings, onOpenNews }: { settings: Settings; onOpenNews: () => void }) {
  const [items, setItems] = useState<NewsItem[]>([])
  const [unread, setUnread] = useState(0)
  const [idx, setIdx] = useState(0)
  const [paused, setPaused] = useState(false)

  const load = useCallback(async () => {
    try {
      const [it, n] = await Promise.all([api.newsItems('', 20), api.newsUnread()])
      setItems(it); setUnread(n)
      setIdx((i) => (i < it.length ? i : 0))
    } catch { /* 忽略 */ }
  }, [])
  useEffect(() => { void load() }, [load])
  useEffect(() => on('news:changed', () => void load()), [load])
  useEffect(() => {
    if (paused || items.length < 2) return
    const id = window.setInterval(() => { if (!document.hidden) setIdx((i) => (i + 1) % items.length) }, ROTATE_MS)
    return () => clearInterval(id)
  }, [paused, items.length])

  const cur = items[idx]
  return (
    <span className="ticker" onMouseEnter={() => setPaused(true)} onMouseLeave={() => setPaused(false)} data-testid="news-ticker">
      {cur ? (
        <button className="tickerline" key={cur.id} title={`${cur.title}\n${cur.feedTitle} · ${ago(cur.publishedAt)}${cur.summary ? '\n\n' + cur.summary : ''}`}
          onClick={() => void api.openNewsItem(cur.id)} data-testid="news-title">
          <span className={cur.read ? 'read' : ''}>{cur.title}</span>
          <span className="src">{cur.feedTitle} · {ago(cur.publishedAt)}</span>
        </button>
      ) : (
        <span className="tickerline empty" data-testid="news-empty">还没有新闻（每 {settings.newsInterval} 分钟更新）</span>
      )}
      <button className="newscount" onClick={onOpenNews} title="打开新闻列表" aria-label="打开新闻列表" data-testid="news-open-list">
        {unread > 0 ? unread : '≡'}
      </button>
    </span>
  )
}
