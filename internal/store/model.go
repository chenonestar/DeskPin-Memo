package store

// 所有时间字段均为 UTC 毫秒时间戳；前端按本地时区显示。

const (
	StatusTodo    = "todo"
	StatusDone    = "done"
	StatusDeleted = "deleted"

	PriorityLow  = 0
	PriorityMid  = 1
	PriorityHigh = 2

	InboxName = "收件箱"
)

// Reminder 是事项的一条提醒。
type Reminder struct {
	ID            string `json:"id"`
	ItemID        string `json:"itemId"`
	RemindAt      *int64 `json:"remindAt"`      // 下次触发时间（绝对）
	OffsetMinutes int    `json:"offsetMinutes"` // >0 表示「截止前 N 分钟」
	RepeatRule    string `json:"repeatRule"`    // RRULE
	SnoozeUntil   *int64 `json:"snoozeUntil"`
	LastFiredAt   *int64 `json:"lastFiredAt"`
}

// ReminderInput 是创建/替换提醒时的输入。
type ReminderInput struct {
	RemindAt      *int64 `json:"remindAt"`
	OffsetMinutes int    `json:"offsetMinutes"`
	RepeatRule    string `json:"repeatRule"`
}

// Item 是一条事项。
type Item struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	Note        string     `json:"note"`
	GroupID     string     `json:"groupId"`
	Priority    int        `json:"priority"`
	DueAt       *int64     `json:"dueAt"`
	Status      string     `json:"status"`
	CompletedAt *int64     `json:"completedAt"`
	DeletedAt   *int64     `json:"deletedAt"`
	SortOrder   float64    `json:"sortOrder"`
	SeriesID    string     `json:"seriesId"`
	CreatedAt   int64      `json:"createdAt"`
	UpdatedAt   int64      `json:"updatedAt"`
	HLC         string     `json:"hlc"`
	DeviceID    string     `json:"deviceId"`
	FieldHLC    string     `json:"fieldHlc"`
	Tags        []string   `json:"tags"`
	Reminders   []Reminder `json:"reminders"`
	Subtasks    []Subtask  `json:"subtasks"`
	Locked      bool       `json:"locked"` // 加密未解锁，标题/备注不可读
}

// Subtask 是事项下挂的一层检查项（FR-108）。
type Subtask struct {
	ID          string  `json:"id"`
	ItemID      string  `json:"itemId"`
	Title       string  `json:"title"`
	Done        bool    `json:"done"`
	SortOrder   float64 `json:"sortOrder"`
	CompletedAt *int64  `json:"completedAt"`
	CreatedAt   int64   `json:"createdAt"`
	Locked      bool    `json:"locked"`
}

// NewSubtask 是新建子任务的输入；ID 非空时使用指定 id（周期事项的确定性 id）。
type NewSubtask struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Done  bool   `json:"done"`
}

// NewItem 是新建事项的输入。
type NewItem struct {
	Title     string          `json:"title"`
	Note      string          `json:"note"`
	GroupID   string          `json:"groupId"`
	Priority  *int            `json:"priority"`
	DueAt     *int64          `json:"dueAt"`
	Tags      []string        `json:"tags"`
	Reminders []ReminderInput `json:"reminders"`
	Subtasks  []NewSubtask    `json:"subtasks"`
	SeriesID  string          `json:"seriesId"`
	// ID 非空时使用指定 id（周期事项的确定性 id）。
	ID string `json:"id"`
}

// ItemPatch 是编辑事项的输入；nil 表示不修改。ClearDue 表示清除截止时间。
type ItemPatch struct {
	Title    *string   `json:"title"`
	Note     *string   `json:"note"`
	GroupID  *string   `json:"groupId"`
	Priority *int      `json:"priority"`
	DueAt    *int64    `json:"dueAt"`
	ClearDue bool      `json:"clearDue"`
	Tags     *[]string `json:"tags"`
	Sort     *float64  `json:"sortOrder"`
}

// Group 是分组。
type Group struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Color     string  `json:"color"`
	SortOrder float64 `json:"sortOrder"`
	IsDefault bool    `json:"isDefault"`
	Locked    bool    `json:"locked"`
}

// Window 是便签窗口状态（设备相关，不同步）。
type Window struct {
	ID        string  `json:"id"`
	GroupID   string  `json:"groupId"`
	Mode      string  `json:"mode"` // desktop | top | normal
	X         int     `json:"x"`
	Y         int     `json:"y"`
	Width     int     `json:"width"`
	Height    int     `json:"height"`
	MonitorID string  `json:"monitorId"`
	Opacity   float64 `json:"opacity"`
	Locked    bool    `json:"locked"`
	Collapsed bool    `json:"collapsed"`
	Color     string  `json:"color"`
	// ClickThrough 开启后便签只显示、不响应鼠标（FR-208）；按住 Ctrl 临时恢复交互。
	ClickThrough bool `json:"clickThrough"`
}

// Filter 用于列出事项。
type Filter struct {
	GroupID string
	Status  string // 空 = 未删除（todo+done）
	Tag     string
	DueFrom *int64
	DueTo   *int64
	NoDue   bool
	Limit   int
}

// SearchHit 是搜索结果。
type SearchHit struct {
	Item  Item   `json:"item"`
	Title string `json:"title"`
}
