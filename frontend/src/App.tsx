import { useCallback, useEffect, useState } from 'react'
import { api, on } from './api'
import type { Bootstrap, Group, Notification, Settings as SettingsT, WindowState } from './types'
import { Sticky } from './components/Sticky'
import { Quick } from './components/Quick'
import { Settings } from './components/Settings'
import { LockScreen } from './components/LockScreen'
import { StrongAlert } from './components/StrongAlert'

type Overlay = '' | 'quick' | 'settings'

export default function App() {
  const [boot, setBoot] = useState<Bootstrap | null>(null)
  const [groups, setGroups] = useState<Group[]>([])
  const [settings, setSettings] = useState<SettingsT | null>(null)
  const [win, setWin] = useState<WindowState | null>(null)
  const [overlay, setOverlay] = useState<Overlay>('')
  const [locked, setLocked] = useState(false)
  const [alerts, setAlerts] = useState<Notification[]>([])
  const [groupId, setGroupId] = useState('')
  const [fatal, setFatal] = useState('')

  const reloadGroups = useCallback(() => { void api.groups().then(setGroups) }, [])

  const init = useCallback(async () => {
    try {
      const b = await api.bootstrap()
      setBoot(b); setGroups(b.groups); setSettings(b.settings); setLocked(b.locked)
      setGroupId(await api.currentGroup())
      setWin(await api.applyWindow())
      setOverlay((await api.overlay()) as Overlay)
    } catch (e) { setFatal((e as Error).message) }
  }, [])

  useEffect(() => { void init() }, [init])
  useEffect(() => on('overlay:open', (k) => setOverlay(k as Overlay)), [])
  useEffect(() => on('overlay:close', () => setOverlay('')), [])
  useEffect(() => on('settings:changed', (s) => setSettings(s as SettingsT)), [])
  useEffect(() => on('lock:changed', (l) => { setLocked(!!l); if (!l) void init() }), [init])
  useEffect(() => on('data:changed', reloadGroups), [reloadGroups])
  useEffect(() => on('reminder:strong', (n) => setAlerts((a) => [...a, n as Notification])), [])

  // 主题 / 字号（4.4）
  useEffect(() => {
    if (!settings) return
    const root = document.documentElement
    if (settings.theme === 'system') root.removeAttribute('data-theme')
    else root.setAttribute('data-theme', settings.theme)
    root.style.setProperty('--font-size', `${settings.fontSize}px`)
  }, [settings])

  // 切换分组时 Sticky 内部会调用 showGroup；这里只需同步当前分组给快速输入框
  useEffect(() => { if (win) setGroupId(win.groupId) }, [win])

  if (fatal) return <div className="banner">{fatal}</div>
  if (!boot || !settings) return null

  const closeSettings = () => void api.closeSettings()

  return (
    <>
      {overlay === 'quick' ? (
        <Quick groups={groups} groupId={groupId} />
      ) : overlay === 'settings' ? (
        <Settings settings={settings} groups={groups} onSettings={setSettings} reloadGroups={reloadGroups} onClose={closeSettings} />
      ) : locked ? (
        <LockScreen onUnlocked={() => setLocked(false)} />
      ) : (
        <Sticky groups={groups} settings={settings} win={win} setWin={setWin} reloadGroups={reloadGroups}
          onOpenSettings={() => void api.openSettings()} />
      )}
      {alerts.length > 0 && <StrongAlert n={alerts[0]} onHandled={() => setAlerts((a) => a.slice(1))} />}
    </>
  )
}
