import { useState } from 'react'
import { api } from '../api'
import { t } from '../i18n/zh-CN'

/** 加密开启且未解锁（「每次启动输入密码」或 DPAPI 失败）时显示（FR-607）。 */
export function LockScreen({ onUnlocked }: { onUnlocked: () => void }) {
  const [pw, setPw] = useState('')
  const [rk, setRk] = useState('')
  const [np, setNp] = useState('')
  const [forgot, setForgot] = useState(false)
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)

  const submit = async () => {
    setBusy(true); setErr('')
    try {
      if (forgot) await api.resetPassword(rk, np)
      else await api.unlock(pw)
      onUnlocked()
    } catch (e) { setErr((e as Error).message) } finally { setBusy(false) }
  }
  return (
    <div className="lock-card" data-testid="lock-screen">
      <form style={{ width: '100%' }} onSubmit={(e) => { e.preventDefault(); void submit() }}>
        <h3 style={{ margin: '0 0 4px' }}>🔒 {t('unlockTitle')}</h3>
        <div className="hint" style={{ marginBottom: 8 }}>{t('unlockHint')}</div>
        {forgot ? (
          <>
            <div className="field"><label>{t('recoveryKey')}</label><input value={rk} onChange={(e) => setRk(e.target.value)} autoFocus aria-label={t('recoveryKey')} /></div>
            <div className="field"><label>{t('newPassword')}</label><input type="password" value={np} onChange={(e) => setNp(e.target.value)} aria-label={t('newPassword')} /></div>
          </>
        ) : (
          <div className="field"><label>{t('password')}</label><input type="password" value={pw} onChange={(e) => setPw(e.target.value)} autoFocus aria-label={t('password')} data-testid="unlock-input" /></div>
        )}
        {err && <div className="err" role="alert">{err}</div>}
        <div className="btns" style={{ justifyContent: 'space-between' }}>
          <button type="button" className="btn" onClick={() => { setForgot(!forgot); setErr('') }} style={{ fontSize: 12 }}>{forgot ? t('cancel') : t('forgotPassword')}</button>
          <button className="btn primary" disabled={busy || (forgot ? !rk || !np : !pw)}>{forgot ? t('resetPassword') : t('unlock')}</button>
        </div>
      </form>
    </div>
  )
}
