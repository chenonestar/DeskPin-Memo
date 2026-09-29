import { useEffect, useRef, useState, type ReactNode } from 'react'
import { t } from '../i18n/zh-CN'

export function Modal({ title, children, onClose, footer }: { title?: string; children: ReactNode; onClose?: () => void; footer?: ReactNode }) {
  useEffect(() => {
    if (!onClose) return
    const h = (e: KeyboardEvent) => e.key === 'Escape' && (e.stopPropagation(), onClose())
    window.addEventListener('keydown', h, true)
    return () => window.removeEventListener('keydown', h, true)
  }, [onClose])
  return (
    <div className="modal-backdrop" onMouseDown={(e) => e.target === e.currentTarget && onClose?.()}>
      <div className="modal" role="dialog" aria-label={title}>
        {title && <h3>{title}</h3>}
        {children}
        {footer && <div className="btns">{footer}</div>}
      </div>
    </div>
  )
}

/** 文本输入对话框（WebView 的 window.prompt 体验差，且不可样式化）。 */
export function PromptDialog({ title, label, initial = '', onOk, onCancel }: { title: string; label?: string; initial?: string; onOk: (v: string) => void; onCancel: () => void }) {
  const [v, setV] = useState(initial)
  const ref = useRef<HTMLInputElement>(null)
  useEffect(() => { ref.current?.focus(); ref.current?.select() }, [])
  const ok = () => v.trim() && onOk(v.trim())
  return (
    <Modal
      title={title}
      onClose={onCancel}
      footer={<>
        <button className="btn" onClick={onCancel}>{t('cancel')}</button>
        <button className="btn primary" disabled={!v.trim()} onClick={ok}>{t('confirm')}</button>
      </>}
    >
      <div className="field">
        {label && <label>{label}</label>}
        <input ref={ref} value={v} onChange={(e) => setV(e.target.value)} onKeyDown={(e) => e.key === 'Enter' && ok()} />
      </div>
    </Modal>
  )
}

/** 二次确认（仅用于不可撤销的操作：清空回收站、覆盖导入，见 4.4）。 */
export function ConfirmDialog({ title, message, danger = true, okText, onOk, onCancel }: { title: string; message: ReactNode; danger?: boolean; okText?: string; onOk: () => void; onCancel: () => void }) {
  return (
    <Modal
      title={title}
      onClose={onCancel}
      footer={<>
        <button className="btn" onClick={onCancel}>{t('cancel')}</button>
        <button className={`btn ${danger ? 'danger' : 'primary'}`} onClick={onOk}>{okText ?? t('confirm')}</button>
      </>}
    >
      <div>{message}</div>
    </Modal>
  )
}
