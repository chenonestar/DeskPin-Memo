import { useCallback, useEffect, useState } from 'react'
import { api, on } from '../api'
import type { EncStatus, Group, ImportInfo, Settings as S } from '../types'
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
}

export function Settings({ settings, groups, onSettings, reloadGroups, onClose }: Props) {
  const [tab, setTab] = useState<Tab>('general')
  const [conflict, setConflict] = useState<string[]>([])
  const patch = useCallback(async (p: Partial<S>) => {
    onSettings({ ...settings, ...p }) // 乐观更新：控件立即反映，随后以后端规范化后的结果为准
    onSettings(await api.saveSettings({ ...settings, ...p }))
  }, [settings, onSettings])

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
          {tab === 'appearance' && <Appearance s={settings} patch={patch} />}
          {tab === 'reminder' && <Reminder s={settings} patch={patch} />}
          {tab === 'hotkey' && <Hotkeys s={settings} patch={patch} conflict={conflict} />}
          {tab === 'data' && <DataPane />}
          {tab === 'about' && <About />}
        </div>
      </div>
    </div>
  )
}

type P = { s: S; patch: (p: Partial<S>) => Promise<void> }

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
  const [delId, setDelId] = useState<string | null>(null)
  return (
    <>
      <h4>启动</h4>
      <Check label="开机自动启动（静默显示便签）" checked={s.autostart} onChange={(v) => void patch({ autostart: v })} />
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

function Appearance({ s, patch }: P) {
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
      <h4>免打扰</h4>
      <Check label="启用免打扰时段（提醒延后到时段结束）" checked={s.dnd.enabled} onChange={(v) => void patch({ dnd: { ...s.dnd, enabled: v } })} />
      <div className="rowline"><label htmlFor="dnd1">开始</label>
        <input id="dnd1" type="time" value={s.dnd.start} disabled={!s.dnd.enabled} onChange={(e) => e.target.value && void patch({ dnd: { ...s.dnd, start: e.target.value } })} /></div>
      <div className="rowline"><label htmlFor="dnd2">结束</label>
        <input id="dnd2" type="time" value={s.dnd.end} disabled={!s.dnd.enabled} onChange={(e) => e.target.value && void patch({ dnd: { ...s.dnd, end: e.target.value } })} /></div>
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
      {conflict.length > 0 && <div className="err" role="alert" data-testid="hotkey-conflict">{conflict.map((c) => t('hotkeyConflict', { msg: c })).join('；')}</div>}
      <div className="hint">点击后按下组合键（需含 Ctrl / Alt / Shift / Win 之一）。注册失败即表示与其他程序冲突。</div>
      <h4>窗口内快捷键</h4>
      <div className="hint">Ctrl+N 新建 · Ctrl+F 搜索 · Ctrl+Z 撤销 · Delete 删除选中事项 · Esc 取消 / 返回</div>
    </>
  )
}

function DataPane() {
  const [dir, setDir] = useState('')
  const [backups, setBackups] = useState<string[]>([])
  const [msg, setMsg] = useState('')
  const [err, setErr] = useState('')
  const [expDlg, setExpDlg] = useState(false)
  const [impDlg, setImpDlg] = useState<{ path: string; info: ImportInfo; pw: string } | null>(null)
  const [confirmOverwrite, setConfirmOverwrite] = useState<{ path: string; pw: string } | null>(null)

  const refresh = useCallback(() => { void api.dataDir().then(setDir); void api.listBackups().then((b) => setBackups(b ?? [])) }, [])
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
      <h4>数据目录</h4>
      <div className="rowline"><code style={{ flex: 1, wordBreak: 'break-all' }} data-testid="data-dir">{dir}</code>
        <button className="btn" onClick={() => void run(() => api.openDataDir())}>打开</button></div>
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
