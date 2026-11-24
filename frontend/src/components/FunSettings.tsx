import { useCallback, useEffect, useState } from 'react'
import { api, on } from '../api'
import type { Feed, FeedPreset, FeedTest, Group, Pet, Settings as S } from '../types'
import { ago } from '../fun'
import { Check, type P } from './Settings'
import { ConfirmDialog, Modal, PromptDialog } from './Dialog'
import { NameDialog } from './Pet'

/** 设置 → 摸鱼：新闻订阅、狗狗、下班倒计时、正经模式。 */
export function FunPane({ s, patch, groups, onSettings }: P & { groups: Group[]; onSettings: (s: S) => void }) {
  return (
    <>
      <News s={s} patch={patch} onSettings={onSettings} />
      <PetSection s={s} patch={patch} />
      <NotifyTest />
      <h4>下班倒计时</h4>
      <Check label="在便签底部显示下班倒计时" checked={s.showCountdown} onChange={(v) => void patch({ showCountdown: v })} />
      <div className="rowline"><label htmlFor="ws">上班时间</label>
        <input id="ws" type="time" value={s.workStart} onChange={(e) => e.target.value && void patch({ workStart: e.target.value })} /></div>
      <div className="rowline"><label htmlFor="we">下班时间</label>
        <input id="we" type="time" value={s.workEnd} onChange={(e) => e.target.value && void patch({ workEnd: e.target.value })} /></div>
      <div className="rowline"><label htmlFor="sal">月薪（可不填）</label>
        <input id="sal" type="number" min={0} step={100} style={{ width: 110 }} value={s.monthlySalary || ''} placeholder="不显示已赚"
          onChange={(e) => void patch({ monthlySalary: Math.max(0, Number(e.target.value) || 0) })} /></div>
      <div className="rowline"><label htmlFor="cm">金额显示</label>
        <select id="cm" value={s.countdownMoney} onChange={(e) => void patch({ countdownMoney: e.target.value as S['countdownMoney'] })}>
          <option value="hover">鼠标悬停时显示</option><option value="always">一直显示</option><option value="never">不显示</option>
        </select></div>
      <div className="rowline"><label htmlFor="ctoss">狗狗扔掉过去的时间</label>
        <select id="ctoss" value={s.countdownToss} onChange={(e) => void patch({ countdownToss: e.target.value as S['countdownToss'] })} data-testid="countdown-toss">
          <option value="hour">每小时</option><option value="half">每 30 分钟</option><option value="off">关闭</option>
        </select></div>
      <div className="hint">平时只显示到分钟（如「下班 2:13 · ¥ ••」），鼠标放上去展开秒数和金额。填了月薪才有「今天已赚」（按每月 21.75 个计薪日折算），只保存在本机。狗狗到下班时间会伸懒腰；上班时间里每过一小时（或 30 分钟），狗狗会把过去的时间从倒计时里叼出来扔掉（需要同时开着狗狗）。周末不倒计时。便签较窄时，倒计时和新闻自动分成两行。</div>
      <h4>正经模式（老板键）</h4>
      <div className="rowline"><label htmlFor="bossg">同时切换到分组</label>
        <select id="bossg" value={s.bossGroupId} onChange={(e) => void patch({ bossGroupId: e.target.value })}>
          <option value="">不切换</option>
          {groups.map((g) => <option key={g.id} value={g.id}>{g.name}</option>)}
        </select></div>
      <div className="hint">按 {s.hotkeyBoss}（可在「快捷键」页修改）或托盘菜单「正经模式」：新闻条、狗狗、倒计时一键收起；再按一次恢复。</div>
    </>
  )
}

// ---------------------------------------------------------------- 新闻订阅

function News({ s, patch, onSettings }: P & { onSettings: (s: S) => void }) {
  const [feeds, setFeeds] = useState<Feed[]>([])
  const [picker, setPicker] = useState<FeedPreset[] | null>(null)
  const [url, setUrl] = useState('')
  const [busy, setBusy] = useState(false)
  const [msg, setMsg] = useState('')
  const [err, setErr] = useState('')
  const [tests, setTests] = useState<Record<string, FeedTest | 'pending'>>({})
  const [renaming, setRenaming] = useState<Feed | null>(null)
  const [deleting, setDeleting] = useState<Feed | null>(null)

  const load = useCallback(() => { void api.listFeeds().then(setFeeds).catch(() => undefined) }, [])
  useEffect(load, [load])
  useEffect(() => on('feeds:changed', load), [load])

  const openPicker = async () => setPicker(await api.feedPresets())
  const enable = async (v: boolean) => {
    if (v && !s.newsSetupDone) { await openPicker(); return } // 首次开启：先选预置订阅源
    await patch({ newsEnabled: v })
  }
  const add = async () => {
    setErr(''); setMsg(''); setBusy(true)
    try {
      const f = await api.addFeed(url)
      setUrl('')
      setMsg(f.lastError ? `已添加，但首次拉取失败：${f.lastError}` : `已添加「${f.title || f.url}」`)
      load()
    } catch (e) { setErr((e as Error).message) } finally { setBusy(false) }
  }
  const refresh = async () => {
    setBusy(true); setMsg('')
    try { const n = await api.refreshNews(); setMsg(n > 0 ? `新增 ${n} 条` : '没有新内容'); load() } catch (e) { setErr((e as Error).message) } finally { setBusy(false) }
  }
  const test = async (f: Feed) => {
    setTests((t) => ({ ...t, [f.id]: 'pending' }))
    const r = await api.testFeed(f.url)
    setTests((t) => ({ ...t, [f.id]: r }))
  }

  return (
    <>
      <h4>新闻订阅（RSS）</h4>
      <Check label="启用新闻订阅（会联网）" checked={s.newsEnabled} onChange={(v) => void enable(v)} />
      <div className="hint">关闭时程序不发出任何网络请求。开启后只访问下面列出的订阅地址，每隔一段时间拉取一次（RSS 不是实时推送）。</div>
      {s.newsEnabled && (
        <>
          <div className="rowline"><label htmlFor="niv">更新间隔</label>
            <select id="niv" value={s.newsInterval} onChange={(e) => void patch({ newsInterval: Number(e.target.value) })}>
              {[15, 30, 60].map((n) => <option key={n} value={n}>每 {n} 分钟</option>)}
            </select></div>
          <div className="rowline"><label htmlFor="npx">网络代理</label>
            <select id="npx" value={s.newsProxy} onChange={(e) => void patch({ newsProxy: e.target.value as S['newsProxy'] })}>
              <option value="system">跟随系统</option><option value="none">不使用代理</option><option value="manual">手动设置</option>
            </select></div>
          {s.newsProxy === 'manual' && (
            <div className="rowline"><label htmlFor="npu">代理地址</label>
              <input id="npu" defaultValue={s.newsProxyUrl} placeholder="127.0.0.1:7890" onBlur={(e) => void patch({ newsProxyUrl: e.target.value.trim() })} /></div>
          )}
          <div className="hint">公司网络需要代理时选「跟随系统」（读取 Windows 设置里的手动代理）。只配置了自动代理脚本（PAC）的网络暂不支持，请手动填写代理地址。</div>

          <div className="rowline" style={{ marginTop: 8 }}>
            <input value={url} onChange={(e) => setUrl(e.target.value)} placeholder="粘贴 RSS 订阅地址" aria-label="订阅地址" data-testid="feed-url"
              onKeyDown={(e) => { if (e.key === 'Enter' && url.trim()) void add() }} style={{ flex: 1 }} />
            <button className="btn" disabled={!url.trim() || busy} onClick={() => void add()} data-testid="feed-add">添加</button>
          </div>
          <div className="rowline">
            <button className="btn" onClick={() => void openPicker()} data-testid="feed-presets">预置订阅源…</button>
            <button className="btn" disabled={busy || feeds.length === 0} onClick={() => void refresh()} data-testid="feed-refresh">立即刷新</button>
          </div>
          {err && <div className="err" role="alert">{err}</div>}
          {msg && <div className="hint" data-testid="feed-msg">{msg}</div>}
          {feeds.length === 0 && <div className="hint">还没有订阅。可以从「预置订阅源」里挑，或粘贴自己常看的网站的 RSS 地址。</div>}
          {feeds.map((f) => {
            const tr = tests[f.id]
            return (
              <div key={f.id} className="feedrow" data-testid="feed-row">
                <div className="top">
                  <input type="checkbox" checked={f.enabled} aria-label={`启用 ${f.title || f.url}`} onChange={(e) => void api.updateFeed(f.id, null, e.target.checked).then(load)} />
                  <b title={f.title}>{f.title || '（尚未获取标题）'}</b>
                  <button className="iconlink" onClick={() => void test(f)}>测试</button>
                  <button className="iconlink" onClick={() => setRenaming(f)}>改名</button>
                  <button className="iconlink" onClick={() => setDeleting(f)}>删除</button>
                </div>
                <div className="url">{f.url}</div>
                <div data-testid="feed-status">
                  {!f.enabled ? <span className="hint">已停用</span>
                    : f.lastError ? <span className="bad">✗ {f.lastError}{f.failCount > 1 ? `（连续 ${f.failCount} 次）` : ''}</span>
                    : f.lastOkAt ? <span className="ok">✓ {ago(f.lastOkAt)}更新 · {f.itemCount} 条{f.unread ? `，${f.unread} 条未读` : ''}</span>
                    : <span className="hint">等待第一次拉取</span>}
                </div>
                {tr && <div data-testid="feed-test">{tr === 'pending' ? '正在测试…' : tr.ok ? <span className="ok">连接正常：{tr.title}，{tr.count} 条{tr.latest ? `，最新「${tr.latest}」` : ''}</span> : <span className="bad">连接失败：{tr.error}</span>}</div>}
              </div>
            )
          })}
        </>
      )}
      {picker && <PresetPicker presets={picker} first={!s.newsSetupDone} onCancel={() => setPicker(null)}
        onOk={async (urls) => { setPicker(null); const next = await api.setupNews(urls); onSettings(next); load() }} />}
      {renaming && <PromptDialog title="订阅源改名" label="名字（留空恢复使用网站自己的标题）" initial={renaming.customTitle ? renaming.title : ''}
        onOk={async (v) => { await api.updateFeed(renaming.id, v, null); setRenaming(null); load() }} onCancel={() => setRenaming(null)} />}
      {deleting && <ConfirmDialog title="删除订阅源" message={`删除「${deleting.title || deleting.url}」及其全部新闻？`} okText="删除"
        onOk={async () => { await api.deleteFeed(deleting.id); setDeleting(null); load() }} onCancel={() => setDeleting(null)} />}
    </>
  )
}

function PresetPicker({ presets, first, onOk, onCancel }: { presets: FeedPreset[]; first: boolean; onOk: (urls: string[]) => void; onCancel: () => void }) {
  const [sel, setSel] = useState<Set<string>>(() => new Set(presets.filter((p) => !p.added && first && p.default).map((p) => p.url)))
  const toggle = (u: string) => setSel((s) => { const n = new Set(s); if (n.has(u)) n.delete(u); else n.add(u); return n })
  return (
    <Modal title={first ? '开启新闻订阅：选择订阅源' : '添加预置订阅源'} onClose={onCancel} footer={<>
      <button className="btn" onClick={onCancel}>取消</button>
      <button className="btn primary" onClick={() => onOk([...sel])} data-testid="preset-ok">{first ? '开启' : '添加'}{sel.size ? `（${sel.size}）` : ''}</button>
    </>}>
      <div className="presetlist" data-testid="preset-list">
        {presets.map((p) => (
          <label key={p.url}>
            <input type="checkbox" disabled={p.added} checked={p.added || sel.has(p.url)} onChange={() => toggle(p.url)} aria-label={p.name} />
            <span className="cat">{p.category}</span><span>{p.name}{p.unverified ? '（待验证）' : ''}{p.added ? '（已订阅）' : ''}</span>
          </label>
        ))}
      </div>
      <div className="hint" style={{ marginTop: 6 }}>这些地址来自各网站公开的 RSS，是否能打开取决于你的网络；添加后可在列表里「测试」。也可以一个都不选，之后再添加自己的订阅。</div>
    </Modal>
  )
}

// ---------------------------------------------------------------- 狗狗

function PetSection({ s, patch }: P) {
  const [pet, setPet] = useState<Pet | null>(null)
  const [reset, setReset] = useState(false)
  const [naming, setNaming] = useState(false)
  const load = useCallback(() => { if (s.petEnabled) void api.getPet().then(setPet).catch(() => undefined) }, [s.petEnabled])
  useEffect(load, [load])
  useEffect(() => on('pet:changed', load), [load])
  // 第一次开启：先起名（预填「博士」）
  useEffect(() => { if (s.petEnabled && pet && !pet.named) setNaming(true) }, [s.petEnabled, pet])
  return (
    <>
      <h4>狗狗</h4>
      <Check label="养一只戴眼镜的狗狗" checked={s.petEnabled} onChange={(v) => void patch({ petEnabled: v })} />
      {s.petEnabled && pet && (
        <>
          <div className="hint" data-testid="pet-summary">{pet.name} · Lv.{pet.level} · 陪伴 {pet.days} 天 · 帮你完成 {pet.completed} 件事 · 一起玩了 {pet.plays} 次球。完成事项会给它加经验，升级解锁新装扮。在便签上：单击摸头、双击握手、按住拖到另一边、右键扔球 / 换装。</div>
          <div className="rowline"><label>名字</label><span style={{ flex: 1 }}>{pet.name}</span><button className="btn" onClick={() => setNaming(true)}>改名…</button></div>
          <div className="rowline"><label htmlFor="pc">位置</label>
            <select id="pc" value={s.petCorner} onChange={(e) => void patch({ petCorner: e.target.value as S['petCorner'] })}>
              <option value="right">右下角</option><option value="left">左下角</option>
            </select></div>
          <Check label="偶尔说句话（每小时最多一次）" checked={s.petBubbles} onChange={(v) => void patch({ petBubbles: v })} />
          <Check label="有逾期事项时露出担心的表情" checked={s.petWorry} onChange={(v) => void patch({ petWorry: v })} />
          <div className="rowline"><label htmlFor="pact">活跃程度</label>
            <select id="pact" value={s.petActivity} onChange={(e) => void patch({ petActivity: e.target.value as S['petActivity'] })}>
              <option value="quiet">安静（不做随机小动作）</option><option value="normal">正常（1–3 分钟一个小动作）</option><option value="lively">活泼（30–90 秒一个小动作）</option>
            </select></div>
          <Check label="节日自动装扮（春节、端午、中秋等）" checked={s.petHoliday} onChange={(v) => void patch({ petHoliday: v })} />
          <Check label="喝水 / 起身提醒" checked={s.petWater} onChange={(v) => void patch({ petWater: v })} />
          {s.petWater && (
            <>
              <div className="rowline"><label htmlFor="pwi">提醒间隔</label>
                <select id="pwi" value={s.petWaterInterval} onChange={(e) => void patch({ petWaterInterval: Number(e.target.value) })}>
                  {[30, 45, 60, 90].map((n) => <option key={n} value={n}>每 {n} 分钟</option>)}
                </select></div>
              <div className="rowline"><label htmlFor="pws">提醒方式</label>
                <select id="pws" value={s.petWaterStyle} onChange={(e) => void patch({ petWaterStyle: e.target.value as S['petWaterStyle'] })} data-testid="pet-water-style">
                  <option value="roll">卷起便签（确认后再展开）</option><option value="bubble">只冒气泡</option>
                </select></div>
              {s.petWaterStyle === 'roll' && <Check label="卷起时保持便签的透明度（默认卷起期间不透明）" checked={s.petRollKeepOpacity} onChange={(v) => void patch({ petRollKeepOpacity: v })} />}
              <Check label="同时弹系统通知（便签被挡住时也能看到）" checked={s.petWaterNotify} onChange={(v) => void patch({ petWaterNotify: v })} />
              <div className="hint">喝水和起身交替提醒，只在工作日的上班时间（{s.workStart}–{s.workEnd}）；免打扰时段、正经模式、离开电脑超过 5 分钟时不提醒。卷起便签时如果你正在打字，会等你停手 10 秒；便签折叠成一行时只发系统通知。今天喝了 {pet.waterToday} 杯。</div>
            </>
          )}
          <div className="rowline"><button className="btn" onClick={() => setReset(true)}>重置成长进度…</button></div>
        </>
      )}
      {naming && pet && <NameDialog initial={pet.name} first={!pet.named}
        onDone={async (name) => { try { setPet(await api.renamePet(name)); setNaming(false) } catch { /* 名字不合法 */ } }}
        onCancel={async () => { if (!pet.named) setPet(await api.renamePet(pet.name)); setNaming(false) }} />}
      {reset && <ConfirmDialog title="重置成长进度" message="等级、经验、装扮和名字都会恢复初始状态，之后重新给它起名。" okText="重置"
        onOk={async () => { setPet(await api.resetPet()); setReset(false) }} onCancel={() => setReset(false)} />}
    </>
  )
}

// ---------------------------------------------------------------- 系统通知测试

function NotifyTest() {
  const [state, setState] = useState<'' | 'sending' | 'sent' | string>('')
  const send = async () => {
    setState('sending')
    try { await api.testNotification(); setState('sent') } catch (e) { setState((e as Error).message || '发送失败') }
  }
  return (
    <>
      <h4>系统通知</h4>
      <div className="rowline"><button className="btn" disabled={state === 'sending'} onClick={() => void send()} data-testid="notify-test">发送一条测试通知</button></div>
      {state === 'sent' && <div className="hint" data-testid="notify-test-result">已发送。如果屏幕右下角没有弹出，请检查 Windows 设置 → 系统 → 通知；每次发送的结果都记在 logs\app.log 里（搜「系统通知」）。</div>}
      {state && state !== 'sent' && state !== 'sending' && <div className="err" role="alert" data-testid="notify-test-result">发送失败：{state}</div>}
      <div className="hint">事项到点提醒、狗狗的喝水 / 起身提醒都会用到系统通知。</div>
    </>
  )
}
