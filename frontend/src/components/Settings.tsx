import { useCallback, useEffect, useRef, useState } from 'react'
import { api, on } from '../api'
import type { DataDirChange, DataDirStatus, RegTrace, DirInfo, EncStatus, Group, ImportInfo, Settings as S, WindowState } from '../types'
import { t } from '../i18n/zh-CN'
import { hotkeyFromEvent } from '../format'
import { ConfirmDialog, Modal } from './Dialog'
import { Icon } from './Icon'

type Tab = 'general' | 'appearance' | 'reminder' | 'hotkey' | 'data' | 'about'
const TABS: [Tab, string][] = [['general', '常规'], ['appearance', '外观'], ['reminder', '提醒'], ['hotkey', '快捷键'], ['data', '数据'], ['about', '关于']]
const COLORS = ['#FFF3B0', '#D9F2C9', '#CFE8FF', '#FFD9E4', '#E4D9FF', '#E8E8E8']

interface Props {
  settings: S
  groups: Group[]
  onSettings: (s: S) => void
  reloadGroups: () => void
  onClose: () => void
  /** 当前便签的窗口状态，用于提示「穿透开启时自动变淡不生效」 */
  win?: WindowState | null
}

export function Settings({ settings, groups, onSettings, reloadGroups, onClose, win }: Props) {
  const [tab, setTab] = useState<Tab>('general')
  const [conflict, setConflict] = useState<string[]>([])
  // 设置保存必须串行：连续快速修改时，如果请求乱序到达后端，后到的旧值会覆盖新值。
  // 因此 ① 始终基于最新的本地状态合并（latest），② 保存请求排队依次发出，③ 只有队列清空后才采用后端规范化的结果。
  const latest = useRef(settings)
  const pending = useRef(0)
  const queue = useRef<Promise<unknown>>(Promise.resolve())
  useEffect(() => { if (pending.current === 0) latest.current = settings }, [settings])
  const patch = useCallback((p: Partial<S> | ((cur: S) => Partial<S>)) => {
    latest.current = { ...latest.current, ...(typeof p === 'function' ? p(latest.current) : p) }
    const snapshot = latest.current
    onSettings(snapshot) // 乐观更新：控件立即反映
    pending.current++
    const job = queue.current.then(() => api.saveSettings(snapshot))
    queue.current = job.catch(() => undefined)
    return job.then((next) => { if (pending.current === 1) { latest.current = next; onSettings(next) } }).finally(() => { pending.current-- })
  }, [onSettings])

  useEffect(() => on('hotkey:conflict', (d) => setConflict(d as string[])), [])
  useEffect(() => {
    const h = (e: KeyboardEvent) => e.key === 'Escape' && !(e.target as HTMLElement).closest('.modal') && onClose()
    window.addEventListener('keydown', h)
    return () => window.removeEventListener('keydown', h)
  }, [onClose])

  return (
    <div className="settings" data-testid="settings">
      <div className="head"
        onMouseDown={(e) => { if (e.button === 0 && !(e.target as HTMLElement).closest('button')) void api.beginDrag() }}>
        {t('settings')}<span className="spacer" />
        <button className="iconbtn" onClick={onClose} aria-label={t('close')} data-testid="settings-close"><Icon name="x" /></button>
      </div>
      <div className="wrap">
        <nav className="tabs" role="tablist">
          {TABS.map(([k, label]) => (
            <button key={k} role="tab" aria-selected={tab === k} className={tab === k ? 'on' : ''} onClick={() => setTab(k)} data-testid={`tab-${k}`}>{label}</button>
          ))}
        </nav>
        <div className="pane" role="tabpanel">
          {tab === 'general' && <General s={settings} groups={groups} patch={patch} reloadGroups={reloadGroups} />}
          {tab === 'appearance' && <Appearance s={settings} patch={patch} clickThrough={!!win?.clickThrough} />}
          {tab === 'reminder' && <Reminder s={settings} patch={patch} />}
          {tab === 'hotkey' && <Hotkeys s={settings} patch={patch} conflict={conflict} />}
          {tab === 'data' && <DataPane />}
          {tab === 'about' && <About />}
        </div>
      </div>
    </div>
  )
}

type P = { s: S; patch: (p: Partial<S> | ((cur: S) => Partial<S>)) => Promise<void> }

function Check({ label, checked, onChange, disabled }: { label: string; checked: boolean; onChange: (v: boolean) => void; disabled?: boolean }) {
  return (
    <div className="rowline">
      <label htmlFor={`c-${label}`}>{label}</label>
      <input id={`c-${label}`} type="checkbox" checked={checked} disabled={disabled} onChange={(e) => onChange(e.target.checked)} />
    </div>
  )
}

function General({ s, groups, patch, reloadGroups }: P & { groups: Group[]; reloadGroups: () => void }) {
  const [names, setNames] = useState<Record<string, string>>({})
  const [portable, setPortable] = useState(false)
  useEffect(() => { void api.dataDirStatus().then((d) => setPortable(d.portable)) }, [])
  const [delId, setDelId] = useState<string | null>(null)
  return (
    <>
      <h4>启动</h4>
      <Check label="开机自动启动（静默显示便签）" checked={s.autostart} onChange={(v) => void patch({ autostart: v })} />
      {portable && <div className="hint" data-testid="portable-autostart-hint">绿色版默认不开机自启。开启后会在当前用户注册表（HKCU\...\Run）写入本程序当前所在位置；之后搬动文件夹，下次手动启动时会自动更新为新位置。删除文件夹前可在「数据 → 系统注册表项」里清除。</div>}
      <div className="rowline"><label htmlFor="lang">界面语言</label>
        <select id="lang" value={s.language} disabled onChange={() => undefined}><option value="zh-CN">简体中文</option></select></div>
      <h4>分组</h4>
      <div className="hint" style={{ marginBottom: 6 }}>「收件箱」为默认分组，不能删除；删除其他分组时其中的事项会移回收件箱。</div>
      {groups.map((g) => (
        <div className="grouprow" key={g.id}>
          <input type="color" value={/^#[0-9a-f]{6}$/i.test(g.color) ? g.color : '#f5c542'} aria-label={`${g.name} 颜色`}
            onChange={(e) => void api.updateGroup(g.id, null, e.target.value).then(reloadGroups)} />
          <input type="text" value={names[g.id] ?? g.name} aria-label="分组名称"
            onChange={(e) => setNames({ ...names, [g.id]: e.target.value })}
            onBlur={() => { const n = (names[g.id] ?? g.name).trim(); if (n && n !== g.name) void api.updateGroup(g.id, n, null).then(reloadGroups) }} />
          <button className="btn" disabled={g.isDefault} onClick={() => setDelId(g.id)} aria-label={`删除 ${g.name}`}><Icon name="trash" /></button>
        </div>
      ))}
      <div className="grouprow">
        <button className="btn" onClick={async () => { await api.createGroup('新分组', ''); reloadGroups() }}>+ 新建分组</button>
      </div>
      {delId && <ConfirmDialog title="删除分组" message="分组内的事项会移回「收件箱」。" onCancel={() => setDelId(null)}
        onOk={async () => { const id = delId; setDelId(null); await api.deleteGroup(id); reloadGroups() }} />}
    </>
  )
}

function Appearance({ s, patch, clickThrough }: P & { clickThrough: boolean }) {
  return (
    <>
      <h4>主题</h4>
      <div className="rowline"><label htmlFor="theme">颜色主题</label>
        <select id="theme" value={s.theme} onChange={(e) => void patch({ theme: e.target.value as S['theme'] })}>
          <option value="system">跟随系统</option><option value="light">浅色</option><option value="dark">深色</option></select></div>
      <div className="rowline"><label htmlFor="fs">字号</label>
        <select id="fs" value={s.fontSize} onChange={(e) => void patch({ fontSize: Number(e.target.value) })}>
          {[12, 14, 16].map((n) => <option key={n} value={n}>{n} px</option>)}</select></div>
      <h4>便签</h4>
      <div className="rowline"><label htmlFor="op">默认透明度 {Math.round(s.opacity * 100)}%</label>
        <input id="op" type="range" min={30} max={100} value={Math.round(s.opacity * 100)} onChange={(e) => void patch({ opacity: Number(e.target.value) / 100 })} /></div>
      <Check label="鼠标离开后自动变淡" checked={s.fadeOnLeave} onChange={(v) => void patch({ fadeOnLeave: v })} />
      {s.fadeOnLeave && (
        <div className="rowline"><label htmlFor="fop">变淡后的透明度 {Math.round(s.fadedOpacity * 100)}%</label>
          <input id="fop" type="range" min={20} max={80} step={5} value={Math.round(s.fadedOpacity * 100)} onChange={(e) => void patch({ fadedOpacity: Number(e.target.value) / 100 })} /></div>
      )}
      {s.fadeOnLeave && clickThrough && (
        <div className="hint" data-testid="fade-ignored-note">当前便签已开启鼠标穿透，此时「自动变淡」不生效（便签保持原透明度）；关闭穿透后恢复。</div>
      )}
      <div className="rowline"><label>默认便签颜色</label>
        <div className="swatches" style={{ padding: 0 }}>{COLORS.map((c) => (
          <button key={c} className={`swatch ${s.stickyColor === c ? 'on' : ''}`} style={{ background: c }} aria-label={c} onClick={() => void patch({ stickyColor: c })} />))}</div></div>
      <div className="hint">单个便签的透明度、颜色、位置可在便签标题栏「菜单」中单独设置。</div>
    </>
  )
}

function Reminder({ s, patch }: P) {
  return (
    <>
      <h4>默认值</h4>
      <div className="rowline"><label htmlFor="drt">只有日期时的默认提醒时刻</label>
        <input id="drt" type="time" value={s.defaultRemindTime} onChange={(e) => e.target.value && void patch({ defaultRemindTime: e.target.value })} /></div>
      <Check label="播放提示音" checked={s.sound} onChange={(v) => void patch({ sound: v })} />
      <Check label="高优先级事项使用强提醒（置顶窗口，必须手动处理）" checked={s.strongReminder} onChange={(v) => void patch({ strongReminder: v })} />
      <Check label="每天首次启动时弹出今日事项概览" checked={s.dailyOverview} onChange={(v) => void patch({ dailyOverview: v })} />
      <h4>免打扰</h4>
      <Check label="启用免打扰时段（提醒延后到时段结束）" checked={s.dnd.enabled} onChange={(v) => void patch((c) => ({ dnd: { ...c.dnd, enabled: v } }))} />
      <div className="rowline"><label htmlFor="dnd1">开始</label>
        <input id="dnd1" type="time" value={s.dnd.start} disabled={!s.dnd.enabled} onChange={(e) => e.target.value && void patch((c) => ({ dnd: { ...c.dnd, start: e.target.value } }))} /></div>
      <div className="rowline"><label htmlFor="dnd2">结束</label>
        <input id="dnd2" type="time" value={s.dnd.end} disabled={!s.dnd.enabled} onChange={(e) => e.target.value && void patch((c) => ({ dnd: { ...c.dnd, end: e.target.value } }))} /></div>
      <div className="hint">同时会尊重 Windows「专注助手」设置：专注助手开启时系统会自行静音通知。</div>
    </>
  )
}

function HotkeyInput({ label, value, onChange }: { label: string; value: string; onChange: (v: string) => void }) {
  const [rec, setRec] = useState(false)
  return (
    <div className="rowline">
      <label>{label}</label>
      <button className={`kbd ${rec ? 'rec' : ''}`} aria-label={label}
        onClick={() => setRec(true)} onBlur={() => setRec(false)}
        onKeyDown={(e) => {
          if (!rec) return
          e.preventDefault()
          if (e.key === 'Escape') return setRec(false)
          const hk = hotkeyFromEvent(e)
          if (hk) { onChange(hk); setRec(false) }
        }}>
        {rec ? '请按下新的快捷键…' : value}
      </button>
    </div>
  )
}

function Hotkeys({ s, patch, conflict }: P & { conflict: string[] }) {
  return (
    <>
      <h4>全局快捷键</h4>
      <HotkeyInput label="快速新建" value={s.hotkeyQuick} onChange={(v) => void patch({ hotkeyQuick: v })} />
      <HotkeyInput label="显示/隐藏全部便签" value={s.hotkeyToggle} onChange={(v) => void patch({ hotkeyToggle: v })} />
      <HotkeyInput label="切换鼠标穿透" value={s.hotkeyClickThrough} onChange={(v) => void patch({ hotkeyClickThrough: v })} />
      {conflict.length > 0 && <div className="err" role="alert" data-testid="hotkey-conflict">{conflict.map((c) => t('hotkeyConflict', { msg: c })).join('；')}</div>}
      <div className="hint">点击后按下组合键（需含 Ctrl / Alt / Shift / Win 之一）。注册失败即表示与其他程序冲突。</div>
      <h4>窗口内快捷键</h4>
      <div className="hint">Ctrl+N 新建 · Ctrl+F 搜索 · Ctrl+Z 撤销 · Delete 删除选中事项 · Esc 取消 / 返回</div>
    </>
  )
}

function DataPane() {
  const [backups, setBackups] = useState<string[]>([])
  const [msg, setMsg] = useState('')
  const [err, setErr] = useState('')
  const [expDlg, setExpDlg] = useState(false)
  const [impDlg, setImpDlg] = useState<{ path: string; info: ImportInfo; pw: string } | null>(null)
  const [confirmOverwrite, setConfirmOverwrite] = useState<{ path: string; pw: string } | null>(null)

  const refresh = useCallback(() => { void api.listBackups().then((b) => setBackups(b ?? [])) }, [])
  useEffect(refresh, [refresh])
  const run = async (f: () => Promise<string | void>) => {
    setErr(''); setMsg('')
    try { const m = await f(); if (m) setMsg(m) } catch (e) { setErr((e as Error).message) }
  }

  const pickImport = () => run(async () => {
    const path = await api.pickImportFile()
    if (!path) return
    const info = await api.inspectImport(path, '')
    setImpDlg({ path, info, pw: '' })
  })
  const doImport = (overwrite: boolean, path: string, pw: string) => run(async () => {
    const st = await api.importJSON(path, pw, overwrite)
    setImpDlg(null); setConfirmOverwrite(null); refresh()
    return `导入完成：事项 ${st.items}，分组 ${st.groups}，提醒 ${st.reminders}，跳过 ${st.skipped}`
  })

  return (
    <>
      <DataDirSection onChanged={refresh} />
      <h4>备份</h4>
      <div className="hint">每天首次启动自动备份一次，保留最近 14 份。</div>
      <div className="rowline">
        <button className="btn" onClick={() => void run(async () => { const p = await api.backupNow(); refresh(); return '已备份：' + p })}>立即备份</button>
        <button className="btn" onClick={() => void run(() => api.openBackupDir())}>打开备份目录</button>
      </div>
      {backups.length > 0 && <div className="list-plain">{backups.slice(0, 5).map((b) => <div key={b}>{b.split(/[\\/]/).pop()}</div>)}</div>}
      <h4>导出与导入</h4>
      <div className="rowline">
        <button className="btn" onClick={() => setExpDlg(true)} data-testid="export-json">导出 JSON（完整数据）</button>
        <button className="btn" onClick={() => void run(async () => { const p = await api.exportMarkdown(); return p ? '已导出：' + p : undefined })}>导出 Markdown（可读清单）</button>
      </div>
      <div className="rowline"><button className="btn" onClick={pickImport} data-testid="import-json">从 JSON 导入…</button></div>
      {msg && <div className="hint" role="status" style={{ color: 'var(--ok)' }}>{msg}</div>}
      {err && <div className="err" role="alert">{err}</div>}
      <RegistrySection />
      <Encryption onChanged={refresh} />

      {expDlg && <ExportDialog onClose={() => setExpDlg(false)} onExport={(pw) => { setExpDlg(false); void run(async () => { const p = await api.exportJSON(pw); return p ? '已导出：' + p : undefined }) }} />}
      {impDlg && (
        <Modal title="导入数据" onClose={() => setImpDlg(null)} footer={<>
          <button className="btn" onClick={() => setImpDlg(null)}>{t('cancel')}</button>
          {impDlg.info.encrypted && impDlg.info.items === 0 ? (
            <button className="btn primary" disabled={!impDlg.pw} onClick={() => void run(async () => { const info = await api.inspectImport(impDlg.path, impDlg.pw); setImpDlg({ ...impDlg, info }) })}>解密</button>
          ) : <>
            <button className="btn" onClick={() => void doImport(false, impDlg.path, impDlg.pw)}>合并</button>
            <button className="btn danger" onClick={() => setConfirmOverwrite({ path: impDlg.path, pw: impDlg.pw })} data-testid="import-overwrite">覆盖</button>
          </>}
        </>}>
          {impDlg.info.encrypted && impDlg.info.items === 0 ? (
            <div className="field"><label>该文件已加密，请输入导出密码</label>
              <input type="password" value={impDlg.pw} onChange={(e) => setImpDlg({ ...impDlg, pw: e.target.value })} aria-label="导出密码" /></div>
          ) : <div>文件包含 {impDlg.info.items} 条事项、{impDlg.info.groups} 个分组。<br />
            <b>合并</b>：按修改时间保留较新的版本；<b>覆盖</b>：先清空现有数据再导入（会先自动备份）。</div>}
          {err && <div className="err" role="alert">{err}</div>}
        </Modal>
      )}
      {confirmOverwrite && (
        <ConfirmDialog title="覆盖导入" message="将清空当前所有事项并用导入文件替换，此操作不可撤销（导入前会自动备份当前数据库）。确定继续吗？"
          okText="确定覆盖" onCancel={() => setConfirmOverwrite(null)} onOk={() => void doImport(true, confirmOverwrite.path, confirmOverwrite.pw)} />
      )}
    </>
  )
}

function ExportDialog({ onClose, onExport }: { onClose: () => void; onExport: (pw: string) => void }) {
  const [encrypt, setEncrypt] = useState(false)
  const [pw, setPw] = useState('')
  return (
    <Modal title="导出 JSON" onClose={onClose} footer={<>
      <button className="btn" onClick={onClose}>{t('cancel')}</button>
      <button className="btn primary" disabled={encrypt && pw.length < 6} onClick={() => onExport(encrypt ? pw : '')}>导出</button>
    </>}>
      <div className="rowline"><label htmlFor="enc-exp">加密导出（单独设置导出密码）</label>
        <input id="enc-exp" type="checkbox" checked={encrypt} onChange={(e) => setEncrypt(e.target.checked)} /></div>
      {encrypt ? (
        <div className="field"><label>导出密码（至少 6 位，请牢记，无法找回）</label>
          <input type="password" value={pw} onChange={(e) => setPw(e.target.value)} aria-label="导出密码" /></div>
      ) : <div className="err" role="note">⚠ 明文导出：文件中的标题、备注、标签均未加密，请妥善保管。</div>}
    </Modal>
  )
}

function Encryption({ onChanged }: { onChanged: () => void }) {
  const [st, setSt] = useState<EncStatus | null>(null)
  const [wizard, setWizard] = useState(false)
  const [recovery, setRecovery] = useState('')
  const [dlg, setDlg] = useState<'disable' | 'change' | 'mode' | null>(null)
  const [msg, setMsg] = useState('')
  const load = useCallback(() => void api.encryptionStatus().then(setSt), [])
  useEffect(load, [load])
  if (!st) return null
  return (
    <>
      <h4>加密存储</h4>
      <div className="hint">开启后事项标题、备注、标签名、分组名以 AES-256-GCM 密文落盘；时间与提醒规则不加密，保证提醒照常运行。默认关闭。</div>
      {!st.enabled ? (
        <div className="rowline"><button className="btn" onClick={() => setWizard(true)} data-testid="enable-encryption">开启加密…</button></div>
      ) : (
        <>
          <div className="rowline"><span>状态：已开启（{st.mode === 'dpapi' ? '本机账户自动解锁' : '每次启动输入密码'}）</span></div>
          <div className="rowline">
            <button className="btn" onClick={() => setDlg('mode')}>切换解锁方式</button>
            <button className="btn" onClick={() => setDlg('change')}>修改主密码</button>
            <button className="btn danger" onClick={() => setDlg('disable')}>关闭加密</button>
          </div>
        </>
      )}
      {msg && <div className="hint" role="status" style={{ color: 'var(--ok)' }}>{msg}</div>}
      {wizard && <EnableWizard st={st} recovery={recovery} setRecovery={setRecovery}
        onClose={(done) => { setWizard(false); setRecovery(''); load(); onChanged(); if (done) setMsg('加密已开启。建议删除开启前生成的明文备份。') }} />}
      {dlg && <PasswordDialog kind={dlg} st={st} onClose={(m) => { setDlg(null); load(); onChanged(); if (m) setMsg(m) }} />}
      {msg.includes('明文备份') && <div className="rowline"><button className="btn" onClick={async () => { const n = await api.deleteOldBackups(); setMsg(`已删除 ${n} 个旧备份`); onChanged() }}>删除旧的明文备份</button></div>}
    </>
  )
}

function EnableWizard({ st, recovery, setRecovery, onClose }: { st: EncStatus; recovery: string; setRecovery: (r: string) => void; onClose: (done: boolean) => void }) {
  const [pw, setPw] = useState('')
  const [pw2, setPw2] = useState('')
  const [mode, setMode] = useState(st.dpapiAvailable ? 'dpapi' : 'password')
  const [saved, setSaved] = useState(false)
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)
  const fmt = (k: string) => k.replace(/(.{4})/g, '$1 ').trim()

  if (recovery) {
    return (
      <Modal title="请保存恢复密钥">
        <div className="hint" style={{ marginBottom: 8 }}>忘记主密码时，只能用它重设。请抄写或另存到安全的地方——它只会显示这一次。</div>
        <div className="code" data-testid="recovery-key">{fmt(recovery)}</div>
        <div className="rowline"><label htmlFor="saved">我已抄写 / 另存了恢复密钥</label>
          <input id="saved" type="checkbox" checked={saved} onChange={(e) => setSaved(e.target.checked)} data-testid="recovery-saved" /></div>
        <div className="btns"><button className="btn primary" disabled={!saved} onClick={() => onClose(true)}>完成</button></div>
      </Modal>
    )
  }
  return (
    <Modal title="开启加密" onClose={() => onClose(false)} footer={<>
      <button className="btn" onClick={() => onClose(false)}>{t('cancel')}</button>
      <button className="btn primary" disabled={busy || pw.length < 6 || pw !== pw2} data-testid="enable-confirm"
        onClick={async () => {
          setBusy(true); setErr('')
          try { setRecovery(await api.enableEncryption(pw, mode)) } catch (e) { setErr((e as Error).message) } finally { setBusy(false) }
        }}>开启</button>
    </>}>
      <div className="field"><label>主密码（至少 6 位）</label><input type="password" value={pw} onChange={(e) => setPw(e.target.value)} aria-label="主密码" /></div>
      <div className="field"><label>再次输入</label><input type="password" value={pw2} onChange={(e) => setPw2(e.target.value)} aria-label="确认主密码" /></div>
      <div className="field"><label>解锁方式</label>
        <select value={mode} onChange={(e) => setMode(e.target.value)} aria-label="解锁方式">
          <option value="dpapi" disabled={!st.dpapiAvailable}>本机账户自动解锁（默认，无感使用）</option>
          <option value="password">每次启动输入密码</option></select></div>
      <div className="hint">开启前会自动备份；开启后将生成 24 位恢复密钥。</div>
      {err && <div className="err" role="alert">{err}</div>}
    </Modal>
  )
}

function PasswordDialog({ kind, st, onClose }: { kind: 'disable' | 'change' | 'mode'; st: EncStatus; onClose: (msg?: string) => void }) {
  const [pw, setPw] = useState('')
  const [np, setNp] = useState('')
  const [mode, setMode] = useState(st.mode === 'dpapi' ? 'password' : 'dpapi')
  const [err, setErr] = useState('')
  const title = kind === 'disable' ? '关闭加密' : kind === 'change' ? '修改主密码' : '切换解锁方式'
  const go = async () => {
    setErr('')
    try {
      if (kind === 'disable') { await api.disableEncryption(pw); onClose('加密已关闭。建议删除此前的备份，以免残留密文之外的旧数据。') }
      else if (kind === 'change') { await api.changePassword(pw, np); onClose('主密码已修改') }
      else { await api.setUnlockMode(mode, pw); onClose('解锁方式已更新') }
    } catch (e) { setErr((e as Error).message) }
  }
  return (
    <Modal title={title} onClose={() => onClose()} footer={<>
      <button className="btn" onClick={() => onClose()}>{t('cancel')}</button>
      <button className={`btn ${kind === 'disable' ? 'danger' : 'primary'}`} disabled={!pw || (kind === 'change' && np.length < 6)} onClick={go}>{t('confirm')}</button>
    </>}>
      <div className="field"><label>{kind === 'change' ? '当前主密码' : '主密码'}</label><input type="password" value={pw} onChange={(e) => setPw(e.target.value)} autoFocus aria-label="主密码" /></div>
      {kind === 'change' && <div className="field"><label>新主密码（至少 6 位）</label><input type="password" value={np} onChange={(e) => setNp(e.target.value)} aria-label="新主密码" /></div>}
      {kind === 'mode' && <div className="field"><label>解锁方式</label>
        <select value={mode} onChange={(e) => setMode(e.target.value)}>
          <option value="dpapi" disabled={!st.dpapiAvailable}>本机账户自动解锁</option><option value="password">每次启动输入密码（删除本机自动解锁副本）</option></select></div>}
      {err && <div className="err" role="alert">{err}</div>}
    </Modal>
  )
}

function About() {
  const [v, setV] = useState('')
  useEffect(() => { void api.appVersion().then(setV) }, [])
  return (
    <>
      <h4>{t('appName')} · DeskPin Memo</h4>
      <div>版本 {v}</div>
      <div className="hint" style={{ marginTop: 8 }}>钉在桌面、一眼可见的备忘 / 待办。完全离线运行，数据只保存在本机，不上传任何内容，不含统计埋点。</div>
      <h4>快捷键</h4>
      <div className="hint">全局：Ctrl+Alt+N 快速新建 · Ctrl+Alt+M 显示/隐藏全部便签</div>
    </>
  )
}

function DataDirSection({ onChanged }: { onChanged: () => void }) {
  const [st, setSt] = useState<DataDirStatus | null>(null)
  const [dlg, setDlg] = useState(false)
  const [target, setTarget] = useState('')
  const [info, setInfo] = useState<DirInfo | null>(null)
  const [mode, setMode] = useState<'use' | 'replace'>('use')
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)
  const [result, setResult] = useState<DataDirChange | null>(null)
  const load = useCallback(() => { void api.dataDirStatus().then(setSt) }, [])
  useEffect(load, [load])

  // 输入路径后（防抖）校验
  useEffect(() => {
    if (!dlg || !target.trim()) { setInfo(null); return }
    let alive = true
    const id = window.setTimeout(() => { void api.inspectDataDir(target).then((i) => alive && setInfo(i)) }, 250)
    return () => { alive = false; window.clearTimeout(id) }
  }, [target, dlg])

  if (!st) return null
  const open = (path = '') => { setTarget(path); setInfo(null); setErr(''); setResult(null); setMode('use'); setDlg(true) }
  const close = () => { setDlg(false); load(); onChanged() }
  const apply = async () => {
    if (!info?.valid) return
    setBusy(true); setErr('')
    try {
      const m = info.hasData ? mode : 'copy'
      setResult(await api.changeDataDir(info.path, m))
    } catch (e) { setErr((e as Error).message) } finally { setBusy(false) }
  }

  return (
    <>
      <h4>数据目录</h4>
      <div className="rowline">
        <code style={{ flex: 1, wordBreak: 'break-all' }} data-testid="data-dir">{st.dir}</code>
        <button className="btn" onClick={() => void api.openDataDir()}>打开</button>
      </div>
      <div className="hint">{st.portable ? '绿色版：数据固定保存在程序目录下的 data 文件夹，不能更改。' : st.custom ? '当前使用自定义数据目录。日志和缓存仍保存在默认位置。' : '当前使用默认位置，可以改到其他磁盘或网盘同步目录。'}</div>
      {st.fallback && <div className="err" role="alert" data-testid="datadir-fallback">⚠ {st.fallback}</div>}
      {!st.portable && (
        <div className="rowline">
          <button className="btn" onClick={() => open()} data-testid="datadir-change">更改目录…</button>
          {st.custom && <button className="btn" onClick={() => open(st.configDir)} data-testid="datadir-reset">恢复默认位置</button>}
        </div>
      )}
      {dlg && (
        <Modal title="更改数据目录" onClose={result ? undefined : close}
          footer={result ? <>
            <button className="btn" onClick={close}>稍后重启</button>
            <button className="btn primary" onClick={() => void api.restartApp()} data-testid="datadir-restart">立即重启</button>
          </> : <>
            <button className="btn" onClick={close}>{t('cancel')}</button>
            <button className="btn primary" disabled={busy || !info?.valid} onClick={() => void apply()} data-testid="datadir-apply">确定</button>
          </>}>
          {result ? (
            <div data-testid="datadir-done">
              <div>数据目录已设置为：<code style={{ wordBreak: 'break-all' }}>{result.newDir}</code></div>
              <div className="hint" style={{ marginTop: 6 }}>重启后生效。原目录里的数据没有被删除，确认新目录正常后可手动清理：<br /><code style={{ wordBreak: 'break-all' }}>{result.oldDir}</code></div>
              {result.kept && <div className="hint" style={{ marginTop: 6 }}>目标目录里原有的数据库已改名保留为：<code style={{ wordBreak: 'break-all' }}>{result.kept}</code></div>}
            </div>
          ) : (
            <>
              <div className="field">
                <label>新的数据目录（完整路径）</label>
                <div className="inline">
                  <input value={target} onChange={(e) => setTarget(e.target.value)} placeholder={'D:\\Sync\\DeskPinMemo'} aria-label="数据目录路径" data-testid="datadir-input" />
                  <button className="btn" style={{ flex: 'none' }} onClick={async () => { const p = await api.pickDataDir(); if (p) setTarget(p) }}>浏览…</button>
                </div>
              </div>
              {info && !info.valid && <div className="err" role="alert" data-testid="datadir-error">{info.error}</div>}
              {info?.valid && info.warning && <div className="err" style={{ color: 'var(--accent)' }} data-testid="datadir-warning">⚠ {info.warning}</div>}
              {info?.valid && !info.hasData && <div className="hint" data-testid="datadir-copy-note">会把当前全部数据（事项、设置、备份）复制到该目录，原目录里的数据保留不动。</div>}
              {info?.valid && info.hasData && (
                <div className="field" data-testid="datadir-conflict">
                  <label>该目录里已经有一份 data.db：</label>
                  <label><input type="radio" name="dd-mode" checked={mode === 'use'} onChange={() => setMode('use')} /> 使用目录里已有的数据（当前数据不会带过去）</label>
                  <label><input type="radio" name="dd-mode" checked={mode === 'replace'} onChange={() => setMode('replace')} /> 用当前数据替换（目录里的旧数据库改名保留）</label>
                </div>
              )}
              {err && <div className="err" role="alert">{err}</div>}
            </>
          )}
        </Modal>
      )}
    </>
  )
}

function RegistrySection() {
  const [traces, setTraces] = useState<RegTrace[] | null>(null)
  const [confirm, setConfirm] = useState(false)
  const [removed, setRemoved] = useState<string[] | null>(null)
  const [err, setErr] = useState('')
  const load = useCallback(() => { void api.registryTraces().then(setTraces) }, [])
  useEffect(load, [load])
  if (!traces) return null
  const clear = async () => {
    setErr('')
    try { setRemoved(await api.clearRegistryTraces()); setConfirm(false); load() } catch (e) { setErr((e as Error).message) }
  }
  return (
    <>
      <h4>系统注册表项</h4>
      <div className="hint">本程序只会写入<b>当前用户</b>注册表（HKCU，不需要管理员权限），用于开机自启和 Toast 通知。绿色版删除文件夹前建议先清除，否则这些项会残留。</div>
      {traces.length === 0 ? (
        <div className="hint" data-testid="reg-empty" style={{ marginTop: 6 }}>当前没有任何注册表项。</div>
      ) : (
        <ul className="reglist" data-testid="reg-list">
          {traces.map((t) => (
            <li key={t.key} data-testid="reg-item">
              <div><b>{t.desc}</b></div>
              <code>{t.key}</code>
              {t.detail && <div className="hint" style={{ wordBreak: 'break-all' }}>{t.detail}</div>}
            </li>
          ))}
        </ul>
      )}
      <div className="rowline"><button className="btn" disabled={traces.length === 0} onClick={() => setConfirm(true)} data-testid="reg-clear">清除注册表项…</button></div>
      {removed && (
        <Modal title="已清除" footer={<>
          <button className="btn" onClick={() => setRemoved(null)}>继续使用</button>
          <button className="btn primary" onClick={() => void api.quit()} data-testid="reg-quit">退出程序</button>
        </>}>
          <div data-testid="reg-done">已删除 {removed.length} 项，「开机自启」设置也已关闭。现在可以退出程序并删除文件夹。</div>
          <div className="hint" style={{ marginTop: 6 }}>如果继续使用，通知身份和协议会在下次启动时重新写入（Toast 通知需要）。</div>
        </Modal>
      )}
      {confirm && (
        <ConfirmDialog title="清除注册表项" okText="清除" danger={false} onCancel={() => setConfirm(false)} onOk={() => void clear()}
          message={<>将删除上面列出的 {traces.length} 项，并关闭「开机自启」。<br />清除后，通知上的「完成 / 稍后」按钮在程序下次启动前不可用。</>} />
      )}
      {err && <div className="err" role="alert">{err}</div>}
    </>
  )
}
