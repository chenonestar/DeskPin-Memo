import { useCallback, useEffect, useRef, useState } from 'react'
import { api, on } from '../api'
import type { Pet as PetT, PetAccessory, PetEvent, PetGlasses, PetStatus, Settings } from '../types'
import { petLine, petMood, TALK_INTERVAL_MS, type PetMood } from '../fun'
import { Dog, type PetEffect } from './Dog'
import { Menu, type MenuEntry } from './Menu'
import { ConfirmDialog, PromptDialog } from './Dialog'

export const ACCESSORY_LABEL: Record<PetAccessory, string> = { '': '不戴', bow: '领结', scarf: '围巾', hat: '毛线帽', crown: '皇冠' }
export const GLASSES_LABEL: Record<PetGlasses, string> = { round: '圆框眼镜', square: '方框眼镜', sunglasses: '墨镜', gold: '金边眼镜' }
/** 各等级解锁的装扮（与后端 service/pet.go 一致，用于提示「下一级解锁什么」）。 */
const UNLOCKS: Record<number, string> = { 2: '领结', 3: '方框眼镜', 4: '围巾', 5: '墨镜', 6: '毛线帽', 7: '金边眼镜', 9: '皇冠' }

/** 坐在便签角落的狗狗（FR-801~808）。 */
export function PetCorner({ settings, onHide }: { settings: Settings; onHide: () => void }) {
  const [status, setStatus] = useState<PetStatus | null>(null)
  const [transient, setTransient] = useState<'happy' | 'celebrate' | null>(null)
  const [effect, setEffect] = useState<PetEffect>(null)
  const [bubble, setBubble] = useState('')
  const [idle, setIdle] = useState(0)
  const [now, setNow] = useState(() => new Date())
  const [menu, setMenu] = useState<{ x: number; y: number } | null>(null)
  const [rename, setRename] = useState(false)
  const [confirmHide, setConfirmHide] = useState(false)
  const lastTalk = useRef(Date.now()) // 刚出现时不急着说话
  const newNews = useRef(false)
  const timers = useRef<number[]>([])
  const prevMood = useRef<PetMood>('normal')

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

  // 完成事项：叼走一根骨头；升级 / 今天的事全部做完：撒花（FR-804、FR-807）
  useEffect(() => on('pet:event', async (d) => {
    const ev = d as PetEvent
    flash('bone', 'happy', 2500)
    const s = await load()
    if (ev.levelUp) {
      later(() => { flash('confetti', 'celebrate', 3500); say(petLine({ mood: 'celebrate', overdue: 0, levelUp: true, newNews: false, todayDone: false, name: ev.pet.name }, 0)) }, 1200)
    } else if (s && s.todayTotal > 0 && s.todayLeft === 0) {
      later(() => { flash('confetti', 'celebrate', 3500); say(petLine({ mood: 'celebrate', overdue: 0, levelUp: false, newNews: false, todayDone: true, name: ev.pet.name }, 0)) }, 1200)
    }
  }), [load, say])

  // 时间相关的状态（下班、免打扰、打盹）每 30 秒重新判断；系统空闲时间每分钟取一次
  useEffect(() => {
    const tick = () => { setNow(new Date()); void api.idleSeconds().then(setIdle).catch(() => undefined) }
    tick()
    const id = window.setInterval(tick, 30_000)
    return () => clearInterval(id)
  }, [])

  const mood = petMood({ now, transient, overdue: status?.overdue ?? 0, worryEnabled: settings.petWorry, dnd: settings.dnd, workEnd: settings.workEnd, idleSeconds: idle })

  // 说话：情境变化时立刻说（到点下班），平时最多每小时闲聊一句
  useEffect(() => {
    const ctx = { mood, overdue: status?.overdue ?? 0, levelUp: false, newNews: newNews.current, todayDone: false, name: status?.pet.name ?? '' }
    if (mood === 'offwork' && prevMood.current !== 'offwork') say(petLine(ctx, Math.random()))
    else if (!transient && mood !== 'sleeping' && Date.now() - lastTalk.current >= TALK_INTERVAL_MS) {
      say(petLine(ctx, Math.random()))
      newNews.current = false
    }
    prevMood.current = mood
  }, [mood, now, status, transient, say])

  if (!status) return null
  const pet: PetT = status.pet

  const pat = async () => {
    flash('hearts', 'happy', 1800)
    try { await api.patPet() } catch { /* 忽略 */ }
  }
  const snack = async () => {
    flash('bone', 'happy', 2500)
    say('好吃！谢谢～', 3000)
    try { await api.givePetSnack() } catch { /* 忽略 */ }
  }
  const look = async (acc: PetAccessory, gl: PetGlasses) => { try { await api.setPetLook(acc, gl); await load() } catch { /* 未解锁 */ } }
  const nextUnlock = Object.entries(UNLOCKS).find(([lv]) => Number(lv) > pet.level)

  const entries: MenuEntry[] = [
    { kind: 'header', label: `${pet.name} · Lv.${pet.level}` },
    { kind: 'custom', node: (
      <div className="petstats" data-testid="pet-stats">
        <div>经验 {pet.levelNext ? `${pet.exp - pet.levelFloor} / ${pet.levelNext - pet.levelFloor}` : '已满级'}</div>
        <div>陪伴 {pet.days} 天 · 帮你完成 {pet.completed} 件事</div>
        {nextUnlock && <div className="hint">Lv.{nextUnlock[0]} 解锁：{nextUnlock[1]}</div>}
      </div>) },
    { kind: 'sep' },
    { kind: 'item', label: '摸摸头', onClick: () => void pat() },
    { kind: 'item', label: '喂零食', onClick: () => void snack() },
    { kind: 'item', label: '改名…', onClick: () => setRename(true) },
    { kind: 'group', label: '换装', children: [
      { kind: 'header', label: '配饰' },
      ...pet.accessories.map((a): MenuEntry => ({ kind: 'item', label: ACCESSORY_LABEL[a], checked: pet.accessory === a, onClick: () => void look(a, pet.glasses) })),
      { kind: 'header', label: '眼镜' },
      ...pet.glassesList.map((g): MenuEntry => ({ kind: 'item', label: GLASSES_LABEL[g], checked: pet.glasses === g, onClick: () => void look(pet.accessory, g) })),
    ] },
    { kind: 'item', label: settings.petCorner === 'left' ? '换到右下角' : '换到左下角', onClick: () => void api.saveSettings({ ...settings, petCorner: settings.petCorner === 'left' ? 'right' : 'left' }) },
    { kind: 'item', label: '暂时收起', onClick: () => setConfirmHide(true) },
  ]

  return (
    <>
    <div className={`pet ${settings.petCorner === 'left' ? 'left' : 'right'}`} data-testid="pet">
      {bubble && <div className="petbubble" role="status" data-testid="pet-bubble" onClick={() => setBubble('')}>{bubble}</div>}
      <button className="petbtn" title={`${pet.name}（Lv.${pet.level}）：点我摸头，右键更多`} aria-label={`狗狗 ${pet.name}`} data-testid="pet-dog"
        onClick={() => void pat()} onContextMenu={(e) => { e.preventDefault(); setMenu({ x: e.clientX, y: e.clientY }) }}>
        <Dog mood={mood} accessory={pet.accessory} glasses={pet.glasses} effect={effect} />
      </button>
    </div>
    {/* 菜单 / 对话框放在角落容器外面：容器是 pointer-events: none（不挡住下面的事项），放在里面会点不到 */}
      {menu && <Menu x={menu.x} y={menu.y} entries={entries} onClose={() => setMenu(null)} />}
      {rename && <PromptDialog title="给狗狗改名" label="名字（1–12 个字）" initial={pet.name}
        onOk={async (v) => { try { await api.renamePet(v); await load(); setRename(false) } catch { /* 名字不合法：保留对话框 */ } }} onCancel={() => setRename(false)} />}
      {confirmHide && <ConfirmDialog title="暂时收起狗狗" danger={false} okText="收起"
        message={<>狗狗会躲起来，直到下次启动程序。<br />想彻底不养，可以在 设置 → 摸鱼 里关掉。</>}
        onOk={() => { setConfirmHide(false); onHide() }} onCancel={() => setConfirmHide(false)} />}
    </>
  )
}
