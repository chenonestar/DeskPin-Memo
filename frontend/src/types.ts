// 与 Go 侧 store / service 的 JSON 结构一一对应。时间均为 UTC 毫秒时间戳。

export interface Reminder {
  id: string
  itemId: string
  remindAt: number | null
  offsetMinutes: number
  repeatRule: string
  snoozeUntil: number | null
  lastFiredAt: number | null
}

export interface ReminderInput {
  remindAt?: number | null
  offsetMinutes: number
  repeatRule: string
}

export type Status = 'todo' | 'done' | 'deleted'

export interface Subtask {
  id: string
  itemId: string
  title: string
  done: boolean
  sortOrder: number
  completedAt: number | null
  createdAt: number
  locked: boolean
}

export interface Item {
  id: string
  title: string
  note: string
  groupId: string
  priority: number // 0 低 1 中 2 高
  dueAt: number | null
  status: Status
  completedAt: number | null
  deletedAt: number | null
  sortOrder: number
  seriesId: string
  tags: string[]
  reminders: Reminder[]
  subtasks: Subtask[]
  subDone: number
  subTotal: number
  locked: boolean
  overdue: boolean
  rank: number
  repeatText: string
  hasAlarm: boolean
  groupName?: string
}

export interface NewItem {
  title: string
  note?: string
  groupId?: string
  priority?: number | null
  dueAt?: number | null
  tags?: string[]
  reminders?: ReminderInput[]
}

export interface ItemPatch {
  title?: string | null
  note?: string | null
  groupId?: string | null
  priority?: number | null
  dueAt?: number | null
  clearDue?: boolean
  tags?: string[] | null
  sortOrder?: number | null
}

export interface Group {
  id: string
  name: string
  color: string
  sortOrder: number
  isDefault: boolean
  locked: boolean
}

export interface GroupView {
  group: Group
  todo: Item[]
  done: Item[]
  overdue: number
}

export interface DND {
  enabled: boolean
  start: string
  end: string
}

export interface Settings {
  autostart: boolean
  language: string
  theme: 'system' | 'light' | 'dark'
  fontSize: number
  opacity: number
  fadeOnLeave: boolean
  stickyColor: string
  defaultRemindTime: string
  sound: boolean
  strongReminder: boolean
  dnd: DND
  hotkeyQuick: string
  hotkeyToggle: string
  dailyOverview: boolean
}

export interface EncStatus {
  enabled: boolean
  locked: boolean
  mode: string
  dpapiAvailable: boolean
}

export interface Bootstrap {
  groups: Group[]
  settings: Settings
  tags: string[]
  locked: boolean
  overdue: number
  encryption: EncStatus
}

export type WinMode = 'desktop' | 'top' | 'normal'

export interface WindowState {
  id: string
  groupId: string
  mode: WinMode
  x: number
  y: number
  width: number
  height: number
  opacity: number
  locked: boolean
  collapsed: boolean
  color: string
  clickThrough: boolean
  visible: boolean
  /** 外壳用窗口级 alpha 处理透明度 / 变淡：前端不要再叠加 CSS opacity（否则便签颜色会叠在黑色窗口底色上变黑） */
  nativeOpacity: boolean
}

export interface QuickPreview {
  title: string
  dueAt: number | null
  hasTime: boolean
  dueText: string
  tags: string[]
  group: string
  priority: number | null
  repeatRule: string
  repeatText: string
  groupId: string
  groupName: string
  repeatDesc: string
}

export interface Notification {
  tag: string
  title: string
  body: string
  itemId: string
  actions: { id: string; label: string }[]
  strong: boolean
  summary: boolean
  missed: boolean
  sound?: boolean
}

export interface ImportInfo {
  encrypted: boolean
  items: number
  groups: number
}

export interface ImportStats {
  items: number
  groups: number
  reminders: number
  tags: number
  skipped: number
}

export interface Overview {
  date: string
  overdue: Item[]
  today: Item[]
  total: number
}

export interface DataDirStatus {
  dir: string
  configDir: string
  custom: boolean
  portable: boolean
  fallback: string
}

export interface DirInfo {
  path: string
  valid: boolean
  error: string
  hasData: boolean
  isDefault: boolean
  warning: string
}

export interface DataDirChange {
  newDir: string
  oldDir: string
  restartRequired: boolean
  kept: string
}

export interface RegTrace {
  key: string
  desc: string
  detail: string
}
