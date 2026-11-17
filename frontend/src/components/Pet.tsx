import { useCallback, useEffect, useRef, useState, type CSSProperties, type MouseEvent as RMouseEvent, type PointerEvent as RPointerEvent } from 'react'
import { api, on } from '../api'
import type { Pet as PetT, PetAccessory, PetEvent, PetGlasses, PetStatus, Settings } from '../types'
import {
  anniversary, dueGreeting, GREETING_LINE, holidayOutfit, isSneezeCombo, petLine, petMood, TALK_INTERVAL_MS, WATER_LINE, waterDue,
  type Greeting, type PetMood,
} from '../fun'
import { Dog, type PetAction, type PetEffect } from './Dog'
import { Menu, type MenuEntry } from './Menu'
import { ConfirmDialog, Modal } from './Dialog'

export const ACCESSORY_LABEL: Record<PetAccessory, string> = { '': '不戴', bow: '领结', scarf: '围巾', hat: '毛线帽', crown: '皇冠' }
export const GLASSES_LABEL: Record<PetGlasses, string> = { round: '圆框眼镜', square: '方框眼镜', sunglasses: '墨镜', gold: '金边眼镜' }
/** 各等级解锁的装扮（与后端 service/pet.go 一致，用于提示「下一级解锁什么」）。 */
const UNLOCKS: Record<number, string> = { 2: '领结', 3: '方框眼镜', 4: '围巾', 5: '墨镜', 6: '毛线帽', 7: '金边眼镜', 9: '皇冠' }
export const NAME_SUGGESTIONS = ['博士', '摸摸', '鱼丸']

/** 只在本机浏览器里记的小状态（每天的问候 / 纪念日 / 喝水提醒时间）。存储不可用时退化为不记（最多重复说一次）。 */
const store = {
  get: (k: string) => { try { return localStorage.getItem('deskpin.pet.' + k) } catch { return null } },
  set: (k: string, v: string) => { try { localStorage.setItem('deskpin.pet.' + k, v) } catch { /* 忽略 */ } },
}
const todayKey = (d = new Date()) => `${d.getFullYear()}-${d.getMonth() + 1}-${d.getDate()}`

const SINGLE_CLICK_DELAY = 250 // 区分单击（摸头）与双击（握手）
const DRAG_THRESHOLD = 5

type Play = { phase: 'throw' | 'run' | 'back'; dx: number; ballX: number } | null

/** 坐在便签角落的狗狗（FR-801~810 与二期互动）。 */
export function PetCorner({ settings, boss = false, onHide }: { settings: Settings; boss?: boolean; onHide: () => void }) {
  const [status, setStatus] = useState<PetStatus | null>(null)
  const [transient, setTransient] = useState<'happy' | 'celebrate' | null>(null)
  const [effect, setEffect] = useState<PetEffect>(null)
  const [action, setAction] = useState<PetAction>(null)
  const [bubble, setBubble] = useState('')
  const [reminder, setReminder] = useState<'water' | 'stand' | null>(null)
  const [idle, setIdle] = useState(0)
  const [now, setNow] = useState(() => new Date())
  const [awakeUntil, setAwakeUntil] = useState(0)
  const [look, setLook] = useState<{ x: number; y: number } | undefined>(undefined)
  const [menu, setMenu] = useState<{ x: number; y: number } | null>(null)
  const [naming, setNaming] = useState(false)
  const [confirmHide, setConfirmHide] = useState(false)
  const [drag, setDrag] = useState<number | null>(null) // 拖动中：相对起点的水平位移
  const [play, setPlay] = useState<Play>(null)
  const lastTalk = useRef(Date.now()) // 刚出现时不急着说话
  const newNews = useRef(false)
  const timers = useRef<number[]>([])
  const prevMood = useRef<PetMood>('normal')
  const clicks = useRef<number[]>([])
  const clickTimer = useRef<number | null>(null)
  const sneezeAt = useRef(0)
  const pointer = useRef<{ x: number; id: number; moved: boolean } | null>(null)
  const justDragged = useRef(false) // 拖动松手后浏览器还会补发一次 click，不能当成摸头
  const rootRef = useRef<HTMLDivElement>(null)
  const btnRef = useRef<HTMLButtonElement>(null)

  const later = (fn: () => void, ms: number) => { timers.current.push(window.setTimeout(fn, ms)) }
  useEffect(() => () => timers.current.forEach(clearTimeout), [])

  const load = useCallback(() => api.petStatus().then((s) => { setStatus(s); return s }).catch(() => null), [])
  useEffect(() => { void load() }, [load])
  useEffect(() => on('data:changed', () => void load()), [load])
  useEffect(() => on('pet:changed', () => void load()), [load])
  useEffect(() => on('news:changed', () => { newNews.current = true }), [])

  const say = useCallback((text: string, ms = 6000) => {
    if (!settings.petBubbles) return
    setBubble(text)
    lastTalk.current = Date.now()
    later(() => setBubble((b) => (b === text ? '' : b)), ms)
  }, [settings.petBubbles])

  const flash = (fx: PetEffect, mood: 'happy' | 'celebrate', ms: number) => {
    setEffect(fx); setTransient(mood)
    later(() => { setEffect((e) => (e === fx ? null : e)); setTransient((t) => (t === mood ? null : t)) }, ms)
  }
  const doAction = (a: PetAction, ms: number) => {
    setAction(a)
    later(() => setAction((x) => (x === a ? null : x)), ms)
  }

  // 完成事项：叼走一根骨头；升级 / 今天的事全部做完：撒花
  useEffect(() => on('pet:event', async (d) => {
    const ev = d as PetEvent
    flash('bone', 'happy', 2500)
    const s = await load()
    const ctx = { mood: 'celebrate' as const, overdue: 0, newNews: false, name: ev.pet.name }
    if (ev.levelUp) later(() => { flash('confetti', 'celebrate', 3500); say(petLine({ ...ctx, levelUp: true, todayDone: false }, 0)) }, 1200)
    else if (s && s.todayTotal > 0 && s.todayLeft === 0) later(() => { flash('confetti', 'celebrate', 3500); say(petLine({ ...ctx, levelUp: false, todayDone: true }, 0)) }, 1200)
  }), [load, say])

  // 新闻条换标题时，偶尔推一下眼镜
  useEffect(() => {
    const h = () => { if (Math.random() < 1 / 3) doAction('push', 900) }
    window.addEventListener('deskpin:news-rotate', h)
    return () => window.removeEventListener('deskpin:news-rotate', h)
  }, [])

  // 时间相关的状态每 30 秒重新判断；系统空闲时间同时取一次
  useEffect(() => {
    const tick = () => { setNow(new Date()); void api.idleSeconds().then(setIdle).catch(() => undefined) }
    tick()
    const id = window.setInterval(tick, 30_000)
    return () => clearInterval(id)
  }, [])

  const awake = now.getTime() < awakeUntil
  const mood = petMood({ now, transient, overdue: status?.overdue ?? 0, worryEnabled: settings.petWorry, dnd: settings.dnd, workEnd: settings.workEnd, idleSeconds: idle, awake })
  const sleepy = mood === 'sleeping' || mood === 'dozing'

  // 说话：情境变化时立刻说（到点下班），平时最多每小时闲聊一句
  useEffect(() => {
    if (!status) return
    const ctx = { mood, overdue: status.overdue, levelUp: false, newNews: newNews.current, todayDone: false, name: status.pet.name }
    if (mood === 'offwork' && prevMood.current !== 'offwork') say(petLine(ctx, Math.random()))
    else if (!transient && !reminder && mood !== 'sleeping' && Date.now() - lastTalk.current >= TALK_INTERVAL_MS) {
      say(petLine(ctx, Math.random()))
      newNews.current = false
    }
    prevMood.current = mood
  }, [mood, now, status, transient, reminder, say])

  // 时段问候、陪伴纪念日：每天各一次
  useEffect(() => {
    if (!status || boss || mood === 'sleeping') return
    const day = todayKey(now)
    const said = new Set<Greeting>((['morning', 'lunch'] as Greeting[]).filter((g) => store.get(`greet.${g}`) === day))
    const g = dueGreeting(now, settings.workStart, said)
    if (g) { store.set(`greet.${g}`, day); say(GREETING_LINE[g]); return }
    if (store.get('anniv') !== day) {
      const text = anniversary(new Date(status.pet.createdAt), now, status.pet.days)
      store.set('anniv', day)
      if (text) { flash('confetti', 'celebrate', 3500); say(text) }
    }
  }, [now, status?.pet.createdAt, boss])

  // 喝水 / 起身提醒
  useEffect(() => {
    if (!status || reminder) return
    const lastRaw = store.get('water.last')
    const due = waterDue({ now, enabled: settings.petWater, intervalMin: settings.petWaterInterval, lastAt: lastRaw ? Number(lastRaw) : null,
      workStart: settings.workStart, workEnd: settings.workEnd, dnd: settings.dnd, idleSeconds: idle, boss })
    if (!due) return
    const kind = store.get('water.kind') === 'water' ? 'stand' : 'water' // 喝水和起身交替
    store.set('water.kind', kind)
    store.set('water.last', String(now.getTime()))
    setReminder(kind)
    setAction(kind === 'water' ? 'water' : null)
    void api.petNotify(WATER_LINE[kind]).catch(() => undefined)
    later(() => { setReminder((r) => (r === kind ? null : r)); setAction((a) => (a === 'water' ? null : a)) }, 120_000) // 两分钟没理会就收起
  }, [now, idle, status, reminder, boss, settings.petWater, settings.petWaterInterval, settings.workStart, settings.workEnd, settings.dnd])

  const answerReminder = async (snooze: boolean) => {
    const kind = reminder
    setReminder(null); setAction(null)
    if (snooze) { store.set('water.last', String(Date.now() - settings.petWaterInterval * 60_000 + 10 * 60_000)); return }
    if (kind === 'water') { try { await api.drinkWater(); await load() } catch { /* 忽略 */ } }
    flash(null, 'happy', 1500)
    say(kind === 'water' ? '咕嘟咕嘟，舒服～' : '伸个懒腰，舒服多了', 3000)
  }

  // 眼睛跟着鼠标（醒着、没在做动作时）
  const tracking = !sleepy && mood !== 'happy' && mood !== 'celebrate' && !action && !play && !menu
  useEffect(() => {
    if (!tracking) { setLook(undefined); return }
    let raf = 0
    const move = (e: MouseEvent) => {
      if (raf) return
      raf = requestAnimationFrame(() => {
        raf = 0
        const r = btnRef.current?.getBoundingClientRect()
        if (!r) return
        const dx = e.clientX - (r.left + r.width / 2)
        const dy = e.clientY - (r.top + r.height * 0.4)
        const len = Math.max(1, Math.hypot(dx, dy))
        setLook({ x: dx / Math.max(len, 60), y: dy / Math.max(len, 60) })
      })
    }
    const leave = () => setLook(undefined)
    window.addEventListener('mousemove', move)
    document.addEventListener('mouseleave', leave)
    return () => { window.removeEventListener('mousemove', move); document.removeEventListener('mouseleave', leave); if (raf) cancelAnimationFrame(raf) }
  }, [tracking])

  if (!status) return null
  const pet: PetT = status.pet
  const holiday = settings.petHoliday ? holidayOutfit(now) : null
  const accessory = holiday ? holiday.id : pet.accessory

  // ---- 互动 ----
  const pat = async () => {
    flash('hearts', 'happy', 1800)
    try { await api.patPet() } catch { /* 忽略 */ }
  }
  const shake = () => { doAction('shake', 1600); flash(null, 'happy', 1600); say('握手！', 2000) }
  const wake = () => {
    doAction('rub', 1500)
    say('再睡五分钟…', 3000)
    setAwakeUntil(Date.now() + (mood === 'sleeping' ? 10_000 : 5 * 60_000))
    later(() => setNow(new Date()), mood === 'sleeping' ? 10_500 : 0)
  }
  const sneeze = () => {
    sneezeAt.current = Date.now()
    clicks.current = []
    doAction('sneeze', 3000)
    say('阿……阿嚏！眼镜都歪了', 3000)
  }
  const snack = async () => {
    flash('bone', 'happy', 2500)
    say('好吃！谢谢～', 3000)
    try { await api.givePetSnack() } catch { /* 忽略 */ }
  }
  const throwBall = () => {
    const parent = rootRef.current?.offsetParent as HTMLElement | null
    const width = parent?.clientWidth ?? 300
    const dir = settings.petCorner === 'left' ? 1 : -1
    const dist = Math.max(40, width - 64 - 24) * dir
    setPlay({ phase: 'throw', dx: 0, ballX: dist })
    later(() => setPlay({ phase: 'run', dx: dist, ballX: dist }), 700)
    later(() => { setPlay({ phase: 'back', dx: 0, ballX: dist }); setAction('carry') }, 1900)
    later(async () => {
      setPlay(null); setAction(null)
      flash(null, 'happy', 1500)
      say('再来一次！', 2500)
      try { await api.playBall(); await load() } catch { /* 忽略 */ }
    }, 3100)
  }

  /** 单击 / 双击 / 连点的判定：连点 5 次优先；双击握手；单击延迟一点再摸头。 */
  const onClickDog = () => {
    if (play) return
    const t = Date.now()
    clicks.current = [...clicks.current.filter((x) => t - x <= 2000), t]
    if (isSneezeCombo(clicks.current, t) && t - sneezeAt.current > 60_000) {
      if (clickTimer.current) { clearTimeout(clickTimer.current); clickTimer.current = null }
      sneeze()
      return
    }
    if (sleepy) { wake(); return }
    if (clickTimer.current) { // 第二下：双击
      clearTimeout(clickTimer.current); clickTimer.current = null
      shake()
      return
    }
    clickTimer.current = window.setTimeout(() => { clickTimer.current = null; void pat() }, SINGLE_CLICK_DELAY)
  }

  // ---- 拖到另一边 ----
  const onPointerDown = (e: RPointerEvent<HTMLButtonElement>) => {
    if (e.button !== 0 || play) return
    pointer.current = { x: e.clientX, id: e.pointerId, moved: false }
  }
  const onPointerMove = (e: RPointerEvent<HTMLButtonElement>) => {
    const p = pointer.current
    if (!p || p.id !== e.pointerId) return
    const dx = e.clientX - p.x
    if (!p.moved && Math.abs(dx) < DRAG_THRESHOLD) return
    if (!p.moved) { p.moved = true; e.currentTarget.setPointerCapture(e.pointerId) }
    setDrag(dx)
  }
  const onPointerUp = (e: RPointerEvent<HTMLButtonElement>) => {
    const p = pointer.current
    pointer.current = null
    if (!p || !p.moved) return
    justDragged.current = true
    setDrag(null)
    const parent = rootRef.current?.offsetParent as HTMLElement | null
    const r = parent?.getBoundingClientRect()
    const side = r && e.clientX < r.left + r.width / 2 ? 'left' : 'right'
    if (side !== settings.petCorner) void api.saveSettings({ ...settings, petCorner: side })
  }
  const swallowClick = (e: RMouseEvent) => { if (justDragged.current) { justDragged.current = false; e.preventDefault(); return } onClickDog() }

  const look2 = (acc: PetAccessory, gl: PetGlasses) => void api.setPetLook(acc, gl).then(load).catch(() => undefined)
  const nextUnlock = Object.entries(UNLOCKS).find(([lv]) => Number(lv) > pet.level)
  const entries: MenuEntry[] = [
    { kind: 'header', label: `${pet.name} · Lv.${pet.level}${holiday ? ` · ${holiday.name}装扮中` : ''}` },
    { kind: 'custom', node: (
      <div className="petstats" data-testid="pet-stats">
        <div>经验 {pet.levelNext ? `${pet.exp - pet.levelFloor} / ${pet.levelNext - pet.levelFloor}` : '已满级'}</div>
        <div>陪伴 {pet.days} 天 · 帮你完成 {pet.completed} 件事</div>
        <div>一起玩了 {pet.plays} 次球{settings.petWater ? ` · 今天喝了 ${pet.waterToday} 杯` : ''}</div>
        {nextUnlock && <div className="hint">Lv.{nextUnlock[0]} 解锁：{nextUnlock[1]}</div>}
      </div>) },
    { kind: 'sep' },
    { kind: 'item', label: '摸摸头', onClick: () => void pat() },
    { kind: 'item', label: '握个手', onClick: shake },
    { kind: 'item', label: '扔个球', onClick: throwBall },
    { kind: 'item', label: '喂零食', onClick: () => void snack() },
    { kind: 'sep' },
    { kind: 'item', label: '改名…', onClick: () => setNaming(true) },
    { kind: 'group', label: '换装', children: [
      ...(holiday ? [{ kind: 'header', label: `${holiday.name}期间戴节日装扮` } as MenuEntry] : []),
      { kind: 'header', label: '配饰' },
      ...pet.accessories.map((a): MenuEntry => ({ kind: 'item', label: ACCESSORY_LABEL[a], checked: pet.accessory === a, onClick: () => look2(a, pet.glasses) })),
      { kind: 'header', label: '眼镜' },
      ...pet.glassesList.map((g): MenuEntry => ({ kind: 'item', label: GLASSES_LABEL[g], checked: pet.glasses === g, onClick: () => look2(pet.accessory, g) })),
    ] },
    { kind: 'item', label: settings.petCorner === 'left' ? '换到右下角' : '换到左下角', onClick: () => void api.saveSettings({ ...settings, petCorner: settings.petCorner === 'left' ? 'right' : 'left' }) },
    { kind: 'item', label: '暂时收起', onClick: () => setConfirmHide(true) },
  ]

  const left = settings.petCorner === 'left'
  const dx = drag ?? play?.dx ?? 0
  const facing = play ? (play.phase === 'back' ? (left ? -1 : 1) : (left ? 1 : -1)) : 1 // 狗狗默认朝右看；跑动时朝跑的方向
  return (
    <>
      <div ref={rootRef} className={`pet ${left ? 'left' : 'right'}${drag !== null ? ' dragging' : ''}${play ? ' playing' : ''}`} data-testid="pet"
        style={{ transform: dx ? `translateX(${dx}px)` : undefined }} data-play={play?.phase ?? ''}>
        {(bubble || reminder) && (
          <div className="petbubble" role="status" data-testid="pet-bubble" onClick={() => !reminder && setBubble('')}>
            {reminder ? (
              <div data-testid="pet-reminder">
                <div>{pet.name}提醒你：{WATER_LINE[reminder]}</div>
                <div className="petbtns">
                  <button className="btn" onClick={() => void answerReminder(false)} data-testid="reminder-done">好了</button>
                  <button className="btn" onClick={() => void answerReminder(true)} data-testid="reminder-snooze">10 分钟后</button>
                </div>
              </div>
            ) : bubble}
          </div>
        )}
        <button ref={btnRef} className="petbtn" title={`${pet.name}（Lv.${pet.level}）：点我摸头，双击握手，拖动换位置，右键更多`} aria-label={`狗狗 ${pet.name}`} data-testid="pet-dog"
          onClick={swallowClick} onContextMenu={(e) => { e.preventDefault(); setMenu({ x: e.clientX, y: e.clientY }) }}
          onPointerDown={onPointerDown} onPointerMove={onPointerMove} onPointerUp={onPointerUp} onPointerCancel={() => { pointer.current = null; setDrag(null) }}>
          <span className="petface" style={{ transform: facing < 0 ? 'scaleX(-1)' : undefined }}>
            <Dog mood={mood} accessory={accessory} glasses={pet.glasses} effect={effect} action={action} look={look} />
          </span>
        </button>
      </div>
      {play && play.phase !== 'back' && <div className={`petball ${left ? 'left' : 'right'} ${play.phase}`} data-testid="pet-ball" style={{ '--ball-x': `${play.ballX}px` } as CSSProperties} />}
      {/* 菜单 / 对话框放在角落容器外面：容器是 pointer-events: none（不挡住下面的事项），放在里面会点不到 */}
      {menu && <Menu x={menu.x} y={menu.y} entries={entries} onClose={() => setMenu(null)} />}
      {(naming || !pet.named) && <NameDialog initial={pet.name} first={!pet.named}
        onDone={async (name) => { try { await api.renamePet(name); await load(); setNaming(false) } catch { /* 名字不合法：保留对话框 */ } }}
        onCancel={() => { if (pet.named) setNaming(false); else void api.renamePet(pet.name).then(load) }} />}
      {confirmHide && <ConfirmDialog title="暂时收起狗狗" danger={false} okText="收起"
        message={<>狗狗会躲起来，直到下次启动程序。<br />想彻底不养，可以在 设置 → 摸鱼 里关掉。</>}
        onOk={() => { setConfirmHide(false); onHide() }} onCancel={() => setConfirmHide(false)} />}
    </>
  )
}

/** 起名框：第一次开启时弹出（预填「博士」，可一键选候选名或自己输入），也用于改名。 */
export function NameDialog({ initial, first, onDone, onCancel }: { initial: string; first: boolean; onDone: (name: string) => void; onCancel: () => void }) {
  const [v, setV] = useState(initial)
  const ok = v.trim().length > 0 && [...v.trim()].length <= 12
  return (
    <Modal title={first ? '给狗狗起个名字' : '给狗狗改名'} onClose={onCancel} footer={<>
      <button className="btn" onClick={onCancel}>{first ? '就用默认的' : '取消'}</button>
      <button className="btn primary" disabled={!ok} onClick={() => onDone(v.trim())} data-testid="name-ok">就叫这个</button>
    </>}>
      <input autoFocus value={v} maxLength={12} onChange={(e) => setV(e.target.value)} aria-label="狗狗的名字" data-testid="name-input"
        onKeyDown={(e) => { if (e.key === 'Enter' && ok && !e.nativeEvent.isComposing) onDone(v.trim()) }} style={{ width: '100%' }} />
      <div className="namechips">
        {NAME_SUGGESTIONS.map((n) => <button key={n} className={`chip ${v === n ? 'on' : ''}`} onClick={() => setV(n)}>{n}</button>)}
      </div>
      <div className="hint">1–12 个字，之后可以在狗狗右键菜单或设置里改。</div>
    </Modal>
  )
}
