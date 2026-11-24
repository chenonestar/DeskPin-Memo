import { useCallback, useEffect, useRef, useState } from 'react'
import { api, on } from '../api'
import type { NewsItem, Settings } from '../types'
import { ago, countdown, countdownText, tossEvent, TWO_ROW_WIDTH } from '../fun'

/** 便签底部的一行：下班倒计时 + 新闻轮播。 */
export function FunBar({ settings, onOpenNews }: { settings: Settings; onOpenNews: () => void }) {
  const showNews = settings.newsEnabled
  const showCd = settings.showCountdown
  const ref = useRef<HTMLDivElement>(null)
  const [narrow, setNarrow] = useState(false)
  // 两者都开、宽度不够时分两行：上一行倒计时，下一行新闻
  useEffect(() => {
    const el = ref.current
    if (!el) return
    const ro = new ResizeObserver(() => setNarrow(el.clientWidth < TWO_ROW_WIDTH))
    ro.observe(el)
    return () => ro.disconnect()
  }, [showNews, showCd])
  if (!showNews && !showCd) return null
  const two = showNews && showCd && narrow
  return (
    <div ref={ref} className={`funbar ${two ? 'tworow' : ''}`} data-testid="funbar" data-rows={two ? 2 : 1}>
      {showCd && <CountdownChip settings={settings} />}
      {showNews && <NewsTicker settings={settings} onOpenNews={onOpenNews} />}
    </div>
  )
}

function CountdownChip({ settings }: { settings: Settings }) {
  const [now, setNow] = useState(() => new Date())
  const [expanded, setExpanded] = useState(false)
  // 紧凑显示只到分钟：每 15 秒刷新一次就够，不会每秒跳动；悬停展开后每秒刷新
  useEffect(() => {
    setNow(new Date())
    const id = window.setInterval(() => { if (!document.hidden) setNow(new Date()) }, expanded ? 1000 : 15_000)
    return () => clearInterval(id)
  }, [expanded])
  // 跨过整点 / 半点：通知狗狗把过去的时间扔掉（狗狗没开时没人接，倒计时照常走）
  const prev = useRef(now)
  useEffect(() => {
    const t = tossEvent(prev.current, now, settings.workStart, settings.workEnd, settings.countdownToss ?? 'hour')
    prev.current = now
    if (t) window.dispatchEvent(new CustomEvent('deskpin:countdown-toss', { detail: t }))
  }, [now, settings.workStart, settings.workEnd, settings.countdownToss])
  const c = countdown(now, settings.workStart, settings.workEnd)
  return (
    <span className={`cdchip ${expanded ? 'open' : ''}`} data-testid="countdown" title={`上班 ${settings.workStart} · 下班 ${settings.workEnd}`}
      onMouseEnter={() => setExpanded(true)} onMouseLeave={() => setExpanded(false)}>
      {countdownText(c, expanded, settings.monthlySalary, settings.countdownMoney)}
    </span>
  )
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

  // 通知狗狗：新闻换标题了（它会偶尔推一下眼镜）
  useEffect(() => { if (items.length > 1) window.dispatchEvent(new Event('deskpin:news-rotate')) }, [idx, items.length])
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
