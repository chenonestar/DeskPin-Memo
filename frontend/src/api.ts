// 前端与 Go 的唯一接口层。
// - 在 Wails 中：调用 window.go.app.App.*（由 Wails 注入），事件走 window.runtime.EventsOn。
// - 在浏览器开发/测试中：改用 Go 开发服务器的 /rpc 与 /events（SSE）。
import type {
  Bootstrap, DataDirChange, DataDirStatus, DirInfo, Overview, EncStatus, Group, GroupView, ImportInfo, ImportStats, Item, ItemPatch, NewItem,
  Notification, QuickPreview, ReminderInput, Settings, Subtask, WindowState, WinMode,
} from './types'

type Fn = (...args: unknown[]) => Promise<unknown>

interface WailsWindow {
  go?: { app?: { App?: Record<string, Fn> } }
  runtime?: { EventsOn: (name: string, cb: (...d: unknown[]) => void) => () => void }
}

const w = window as unknown as WailsWindow

export const inWails = (): boolean => !!w.go?.app?.App

async function call<T>(method: string, ...args: unknown[]): Promise<T> {
  const native = w.go?.app?.App?.[method]
  if (native) {
    try {
      return (await native(...args)) as T
    } catch (e) {
      throw new Error(typeof e === 'string' ? e : (e as Error)?.message ?? String(e))
    }
  }
  const res = await fetch(`/rpc/${method}`, { method: 'POST', body: JSON.stringify(args) })
  const text = await res.text()
  let body: { result?: T; error?: string } = {}
  try { body = JSON.parse(text) } catch { /* 非 JSON */ }
  if (!res.ok) throw new Error(body.error ?? (text || res.statusText))
  return body.result as T
}

type Handler = (data: unknown) => void
const handlers = new Map<string, Set<Handler>>()
let sse: EventSource | null = null

function ensureSSE() {
  if (sse || inWails()) return
  sse = new EventSource('/events')
  // 连接（重连）建立后通知界面重新同步状态：连接建立前发生的事件不会被重放
  sse.onopen = () => handlers.get('sse:open')?.forEach((h) => h(null))
  sse.onmessage = (m) => {
    try {
      const { event, data } = JSON.parse(m.data)
      handlers.get(event)?.forEach((h) => h(data))
    } catch { /* 忽略 */ }
  }
}

/** 订阅后端事件；返回取消订阅函数。 */
export function on(event: string, h: Handler): () => void {
  if (inWails()) {
    const off = w.runtime!.EventsOn(event, (d) => h(d))
    return off
  }
  ensureSSE()
  let s = handlers.get(event)
  if (!s) handlers.set(event, (s = new Set()))
  s.add(h)
  return () => s!.delete(h)
}

export const api = {
  bootstrap: () => call<Bootstrap>('Bootstrap'),
  groupContent: (id: string) => call<GroupView>('GroupContent', id),
  smartView: (name: string) => call<Item[]>('SmartView', name),
  byTag: (tag: string) => call<Item[]>('ByTag', tag),
  search: (q: string) => call<Item[]>('Search', q),
  trash: () => call<Item[]>('Trash'),
  emptyTrash: () => call<number>('EmptyTrash'),
  undo: () => call<string>('Undo'),
  remove: (id: string) => call<void>('Delete', id),
  restore: (id: string) => call<Item>('Restore', id),
  reorder: (id: string, beforeId: string) => call<void>('Reorder', id, beforeId),
  createItem: (n: NewItem) => call<Item>('CreateItem', n),
  updateItem: (id: string, p: ItemPatch) => call<Item>('UpdateItem', id, p),
  toggle: (id: string, done: boolean) => call<Item>('Toggle', id, done),
  setReminders: (id: string, r: ReminderInput[]) => call<Item>('SetReminders', id, r),
  buildRepeat: (kind: string, arg: number) => call<string>('BuildRepeat', kind, arg),
  quickParse: (text: string, groupId: string) => call<QuickPreview>('QuickParse', text, groupId),
  quickCreate: (text: string, groupId: string) => call<Item>('QuickCreate', text, groupId),
  addSubtask: (itemId: string, title: string) => call<Subtask>('AddSubtask', itemId, title),
  toggleSubtask: (id: string, done: boolean) => call<Subtask>('ToggleSubtask', id, done),
  renameSubtask: (id: string, title: string) => call<Subtask>('RenameSubtask', id, title),
  deleteSubtask: (id: string) => call<void>('DeleteSubtask', id),
  reorderSubtasks: (itemId: string, ids: string[]) => call<void>('ReorderSubtasks', itemId, ids),
  handleAction: (itemId: string, action: string) => call<void>('HandleAction', itemId, action),

  groups: () => call<Group[]>('Groups'),
  createGroup: (name: string, color: string) => call<Group>('CreateGroup', name, color),
  updateGroup: (id: string, name: string | null, color: string | null) => call<void>('UpdateGroup', id, name, color),
  deleteGroup: (id: string) => call<void>('DeleteGroup', id),

  getSettings: () => call<Settings>('GetSettings'),
  saveSettings: (s: Settings) => call<Settings>('SaveSettings', s),

  currentGroup: () => call<string>('CurrentGroup'),
  showGroup: (id: string) => call<WindowState>('ShowGroup', id),
  applyWindow: () => call<WindowState>('ApplyWindow'),
  setWindowMode: (m: WinMode) => call<WindowState>('SetWindowMode', m),
  setWindowLocked: (l: boolean) => call<WindowState>('SetWindowLocked', l),
  setWindowOpacity: (o: number) => call<WindowState>('SetWindowOpacity', o),
  setWindowColor: (c: string) => call<WindowState>('SetWindowColor', c),
  setClickThrough: (on: boolean) => call<WindowState>('SetClickThrough', on),
  strongAlertDone: () => call<void>('StrongAlertDone'),
  toggleCollapse: () => call<WindowState>('ToggleCollapse'),
  beginDrag: () => call<void>('BeginDrag'),
  beginResize: () => call<void>('BeginResize'),
  toggleVisible: () => call<boolean>('ToggleVisible'),

  openQuick: () => call<void>('OpenQuick'),
  closeQuick: () => call<void>('CloseQuick'),
  openSettings: () => call<void>('OpenSettings'),
  closeSettings: () => call<void>('CloseSettings'),
  overlay: () => call<string>('Overlay'),
  overview: () => call<Overview>('Overview'),
  openOverview: () => call<void>('OpenOverview'),
  closeOverview: () => call<void>('CloseOverview'),
  maybeShowOverview: () => call<boolean>('MaybeShowOverview'),

  dataDir: () => call<string>('DataDir'),
  dataDirStatus: () => call<DataDirStatus>('DataDirStatus'),
  inspectDataDir: (p: string) => call<DirInfo>('InspectDataDir', p),
  pickDataDir: () => call<string>('PickDataDir'),
  changeDataDir: (target: string, mode: string) => call<DataDirChange>('ChangeDataDir', target, mode),
  restartApp: () => call<void>('RestartApp'),
  openDataDir: () => call<void>('OpenDataDir'),
  backupNow: () => call<string>('BackupNow'),
  listBackups: () => call<string[]>('ListBackups'),
  openBackupDir: () => call<void>('OpenBackupDir'),
  appVersion: () => call<string>('AppVersion'),
  exportJSON: (password: string) => call<string>('ExportJSONDialog', password),
  exportMarkdown: () => call<string>('ExportMarkdownDialog'),
  pickImportFile: () => call<string>('PickImportFile'),
  inspectImport: (path: string, password: string) => call<ImportInfo>('InspectImport', path, password),
  importJSON: (path: string, password: string, overwrite: boolean) => call<ImportStats>('ImportJSON', path, password, overwrite),

  encryptionStatus: () => call<EncStatus>('EncryptionStatus'),
  enableEncryption: (pw: string, mode: string) => call<string>('EnableEncryption', pw, mode),
  unlock: (pw: string) => call<void>('Unlock', pw),
  resetPassword: (rk: string, pw: string) => call<void>('ResetPasswordWithRecovery', rk, pw),
  changePassword: (o: string, n: string) => call<void>('ChangePassword', o, n),
  setUnlockMode: (mode: string, pw: string) => call<void>('SetUnlockMode', mode, pw),
  disableEncryption: (pw: string) => call<void>('DisableEncryption', pw),
  deleteOldBackups: () => call<number>('DeleteOldBackups'),

  quit: () => call<void>('Quit'),
}

export type StrongAlert = Notification
