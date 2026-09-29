import { api } from '../api'
import type { Notification } from '../types'
import { t } from '../i18n/zh-CN'

/** 强提醒（FR-305）：置顶弹出，必须手动处理才会关闭（没有「关闭」按钮）。 */
export function StrongAlert({ n, onHandled }: { n: Notification; onHandled: () => void }) {
  const act = async (id: string) => {
    if (n.itemId) await api.handleAction(n.itemId, id)
    onHandled()
  }
  return (
    <div className="alert-backdrop" role="alertdialog" aria-label={t('strongTitle')} data-testid="strong-alert">
      <div className="alert">
        <h3>⏰ {t('strongTitle')}</h3>
        <div className="t">{n.title}</div>
        {n.body && <div className="b">{n.body}</div>}
        <div className="btns">
          {(n.actions.length ? n.actions : [{ id: 'done', label: t('finish') }]).map((a) => (
            <button key={a.id} className={`btn ${a.id === 'done' ? 'primary' : ''}`} onClick={() => void act(a.id)}>{a.label}</button>
          ))}
        </div>
      </div>
    </div>
  )
}
