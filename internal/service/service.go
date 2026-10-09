// Package service 是前端与 Win32 外壳共用的业务层：所有数据读写都经过这里（7.4 架构）。
package service

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"deskpinmemo/internal/nlp"
	"deskpinmemo/internal/recur"
	"deskpinmemo/internal/scheduler"
	"deskpinmemo/internal/store"

	"github.com/google/uuid"
)

// Service 汇集业务逻辑。方法均可被 Wails 绑定到前端。
type Service struct {
	st        *store.Store
	dataDir   string
	configDir string // 配置目录：日志、WebView 缓存、图标、数据目录指针（固定，不随数据迁移）
	portable  bool   // 绿色版：数据固定在程序目录，不允许改数据目录
	fallback  string // 自定义数据目录不可用而回退到默认目录时的说明
	now       func() time.Time
	loc       func() *time.Location

	mu       sync.Mutex
	settings Settings
	undo     []undoEntry

	// 回调（由外壳注入）
	Emit         func(event string, data any) // 通知前端刷新
	Kick         func()                       // 通知调度器重算
	OnOverdue    func(count int)              // 托盘红点
	OnSettings   func(Settings)               // 设置变更（快捷键、自启、透明度…）
	OnStrongShow func(n scheduler.Notification)
}

type undoEntry struct {
	label string
	fn    func() error
}

const maxUndo = 50

// Options 创建服务的可选项。
type Options struct {
	Now func() time.Time
	Loc func() *time.Location
	// ConfigDir 为空时等于 dataDir。
	ConfigDir string
	Portable  bool
	// Fallback 为启动时数据目录回退的说明，界面会提示用户。
	Fallback string
}

// New 创建服务。dataDir 为 %APPDATA%\DeskPinMemo。
func New(st *store.Store, dataDir string, opts ...Options) (*Service, error) {
	s := &Service{st: st, dataDir: dataDir, now: time.Now, loc: func() *time.Location { return time.Local }}
	s.configDir = dataDir
	if len(opts) > 0 {
		if opts[0].ConfigDir != "" {
			s.configDir = opts[0].ConfigDir
		}
		s.portable, s.fallback = opts[0].Portable, opts[0].Fallback
		if opts[0].Now != nil {
			s.now = opts[0].Now
		}
		if opts[0].Loc != nil {
			s.loc = opts[0].Loc
		}
	}
	s.Emit, s.Kick, s.OnOverdue, s.OnSettings = func(string, any) {}, func() {}, func(int) {}, func(Settings) {}
	s.OnStrongShow = func(scheduler.Notification) {}
	cfg := DefaultSettings()
	if s.portable {
		// 绿色版默认不自启：开机自启要写注册表，且路径固定为当前 exe，搬动文件夹会失效；
		// 拿到 zip 只想试用的人不应被悄悄加上开机启动。用户仍可在设置里手动开启。
		cfg.Autostart = false
	}
	if ok, err := st.GetSetting("app", &cfg); err != nil {
		return nil, err
	} else if !ok {
		if err := st.SetSetting("app", cfg); err != nil {
			return nil, err
		}
	}
	s.settings = cfg.normalized()
	return s, nil
}

// Store 返回底层存储（供外壳使用）。
func (s *Service) Store() *store.Store { return s.st }

// DataDir 返回数据目录。
func (s *Service) DataDir() string { return s.dataDir }

// BackupDir 返回备份目录。
func (s *Service) BackupDir() string { return filepath.Join(s.dataDir, "backups") }

func (s *Service) changed() {
	s.Emit("data:changed", nil)
	s.Kick()
	s.OnOverdue(s.st.CountOverdue())
}

func (s *Service) pushUndo(label string, fn func() error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.undo = append(s.undo, undoEntry{label, fn})
	if len(s.undo) > maxUndo {
		s.undo = s.undo[len(s.undo)-maxUndo:]
	}
}

// Undo 撤销最近一次操作（Ctrl+Z，AC-08）；返回被撤销操作的名称，无可撤销时为空串。
func (s *Service) Undo() (string, error) {
	s.mu.Lock()
	if len(s.undo) == 0 {
		s.mu.Unlock()
		return "", nil
	}
	e := s.undo[len(s.undo)-1]
	s.undo = s.undo[:len(s.undo)-1]
	s.mu.Unlock()
	if err := e.fn(); err != nil {
		return "", err
	}
	s.changed()
	return e.label, nil
}

// ---- 视图 ----

// ItemView 是给前端的事项视图。
type ItemView struct {
	store.Item
	Overdue    bool   `json:"overdue"`
	Rank       int    `json:"rank"`
	RepeatText string `json:"repeatText"`
	HasAlarm   bool   `json:"hasAlarm"`
	GroupName  string `json:"groupName,omitempty"`
	SubDone    int    `json:"subDone"`  // 已完成的子任务数（FR-108：显示 x/y）
	SubTotal   int    `json:"subTotal"` // 子任务总数
}

func (s *Service) views(items []store.Item) []ItemView {
	now, loc := s.now(), s.loc()
	out := make([]ItemView, 0, len(items))
	for _, it := range items {
		v := ItemView{Item: it, Rank: store.Rank(it, now, loc)}
		v.Overdue = v.Rank == 0
		v.SubTotal = len(it.Subtasks)
		for _, sub := range it.Subtasks {
			if sub.Done {
				v.SubDone++
			}
		}
		for _, r := range it.Reminders {
			if r.RepeatRule != "" && v.RepeatText == "" {
				v.RepeatText = recur.Describe(r.RepeatRule)
			}
			v.HasAlarm = true
		}
		out = append(out, v)
	}
	return out
}

// GroupView 是单个分组便签的内容。
type GroupView struct {
	Group   store.Group `json:"group"`
	Todo    []ItemView  `json:"todo"`
	Done    []ItemView  `json:"done"`
	Overdue int         `json:"overdue"`
}

// Bootstrap 是前端启动时一次性拉取的数据。
type Bootstrap struct {
	Groups   []store.Group `json:"groups"`
	Settings Settings      `json:"settings"`
	Tags     []string      `json:"tags"`
	Locked   bool          `json:"locked"`
	Overdue  int           `json:"overdue"`
	Encrypt  EncStatus     `json:"encryption"`
}

// Bootstrap 返回初始数据。
func (s *Service) Bootstrap() (Bootstrap, error) {
	gs, err := s.st.ListGroups()
	if err != nil {
		return Bootstrap{}, err
	}
	tags, _ := s.st.ListTags()
	return Bootstrap{Groups: gs, Settings: s.GetSettings(), Tags: tags, Locked: s.st.Locked(),
		Overdue: s.st.CountOverdue(), Encrypt: s.EncryptionStatus()}, nil
}

// GroupContent 返回分组便签内容：未完成（FR-106 排序）与已完成（按完成时间倒序）。
func (s *Service) GroupContent(groupID string) (GroupView, error) {
	gs, err := s.st.ListGroups()
	if err != nil {
		return GroupView{}, err
	}
	var gv GroupView
	found := false
	for _, g := range gs {
		if g.ID == groupID {
			gv.Group, found = g, true
		}
	}
	if !found {
		return gv, errors.New("分组不存在")
	}
	items, err := s.st.ListItems(store.Filter{GroupID: groupID})
	if err != nil {
		return gv, err
	}
	var todo, done []store.Item
	for _, it := range items {
		if it.Status == store.StatusDone {
			done = append(done, it)
		} else {
			todo = append(todo, it)
		}
	}
	store.SortItems(todo, s.now(), s.loc())
	sortDone(done)
	gv.Todo, gv.Done = s.views(todo), s.views(done)
	for _, v := range gv.Todo {
		if v.Overdue {
			gv.Overdue++
		}
	}
	return gv, nil
}

func sortDone(done []store.Item) {
	for i := 1; i < len(done); i++ { // 数量小，插入排序足够
		for j := i; j > 0 && val(done[j].CompletedAt) > val(done[j-1].CompletedAt); j-- {
			done[j], done[j-1] = done[j-1], done[j]
		}
	}
}

func val(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}

// SmartView 返回智能视图（FR-403）：today / overdue / next7 / nodate / done。
func (s *Service) SmartView(name string) ([]ItemView, error) {
	all, err := s.st.ListItems(store.Filter{})
	if err != nil {
		return nil, err
	}
	now, loc := s.now(), s.loc()
	y, m, d := now.In(loc).Date()
	startToday := time.Date(y, m, d, 0, 0, 0, 0, loc)
	startTomorrow := startToday.AddDate(0, 0, 1)
	endNext7 := startToday.AddDate(0, 0, 8)
	var out []store.Item
	for _, it := range all {
		switch name {
		case "done":
			if it.Status == store.StatusDone {
				out = append(out, it)
			}
			continue
		}
		if it.Status != store.StatusTodo {
			continue
		}
		switch name {
		case "today":
			if it.DueAt != nil && *it.DueAt >= startToday.UnixMilli() && *it.DueAt < startTomorrow.UnixMilli() {
				out = append(out, it)
			}
		case "overdue":
			if it.DueAt != nil && *it.DueAt < now.UnixMilli() {
				out = append(out, it)
			}
		case "next7":
			if it.DueAt != nil && *it.DueAt >= startTomorrow.UnixMilli() && *it.DueAt < endNext7.UnixMilli() {
				out = append(out, it)
			}
		case "nodate":
			if it.DueAt == nil {
				out = append(out, it)
			}
		default:
			return nil, fmt.Errorf("未知视图 %q", name)
		}
	}
	if name == "done" {
		sortDone(out)
	} else {
		store.SortItems(out, now, loc)
	}
	return s.decorateGroupNames(s.views(out)), nil
}

func (s *Service) decorateGroupNames(vs []ItemView) []ItemView {
	gs, _ := s.st.ListGroups()
	names := map[string]string{}
	for _, g := range gs {
		names[g.ID] = g.Name
	}
	for i := range vs {
		vs[i].GroupName = names[vs[i].GroupID]
	}
	return vs
}

// ByTag 返回带某标签的事项。
func (s *Service) ByTag(tag string) ([]ItemView, error) {
	items, err := s.st.ListItems(store.Filter{Tag: tag})
	if err != nil {
		return nil, err
	}
	store.SortItems(items, s.now(), s.loc())
	return s.decorateGroupNames(s.views(items)), nil
}

// Search 全局搜索（FR-404）。
func (s *Service) Search(q string) ([]ItemView, error) {
	items, err := s.st.Search(q, 200)
	if err != nil {
		return nil, err
	}
	return s.decorateGroupNames(s.views(items)), nil
}

// ---- 事项操作 ----

// CreateItem 新建事项（FR-101），可撤销。
func (s *Service) CreateItem(in store.NewItem) (ItemView, error) {
	it, err := s.st.CreateItem(in)
	if err != nil {
		return ItemView{}, err
	}
	s.pushUndo("新建", func() error { return s.st.PurgeItem(it.ID) })
	s.changed()
	return s.views([]store.Item{it})[0], nil
}

// QuickPreview 是快速输入框的实时预览（回车前展示解析结果）。
type QuickPreview struct {
	nlp.Result
	GroupID    string `json:"groupId"`
	GroupName  string `json:"groupName"`
	RepeatDesc string `json:"repeatDesc"`
}

// QuickParse 解析快速输入文本（FR-102）。groupID 为当前分组，@分组名 可覆盖。
func (s *Service) QuickParse(text, groupID string) (QuickPreview, error) {
	r := nlp.Parse(text, s.now(), s.loc())
	s.applyDefaultTime(&r)
	p := QuickPreview{Result: r}
	if r.RepeatRule != "" {
		p.RepeatDesc = recur.Describe(r.RepeatRule)
	}
	gs, err := s.st.ListGroups()
	if err != nil {
		return p, err
	}
	p.GroupID = groupID
	if p.GroupID == "" {
		p.GroupID, _ = s.st.InboxID()
	}
	if r.Group != "" {
		for _, g := range gs {
			if strings.EqualFold(g.Name, r.Group) {
				p.GroupID = g.ID
			}
		}
	}
	for _, g := range gs {
		if g.ID == p.GroupID {
			p.GroupName = g.Name
		}
	}
	return p, nil
}

// 「只有日期没有时刻」时套用设置里的默认提醒时刻。
func (s *Service) applyDefaultTime(r *nlp.Result) {
	if r.DueAt == nil || r.HasTime {
		return
	}
	var h, m int
	if _, err := fmt.Sscanf(s.GetSettings().DefaultRemindTime, "%d:%d", &h, &m); err != nil {
		return
	}
	t := *r.DueAt
	t = time.Date(t.Year(), t.Month(), t.Day(), h, m, 0, 0, t.Location())
	r.DueAt = &t
	ms := t.UnixMilli()
	r.DueMs = &ms
}

// QuickCreate 由快速输入文本创建事项：解析出的时间同时设为提醒时间，周期规则写入提醒。
func (s *Service) QuickCreate(text, groupID string) (ItemView, error) {
	p, err := s.QuickParse(text, groupID)
	if err != nil {
		return ItemView{}, err
	}
	in := store.NewItem{Title: p.Title, GroupID: p.GroupID, Priority: p.Priority, DueAt: p.DueMs, Tags: p.Tags}
	if p.DueMs != nil {
		in.Reminders = []store.ReminderInput{{RemindAt: p.DueMs, RepeatRule: p.RepeatRule}}
	}
	return s.CreateItem(in)
}

// UpdateItem 编辑事项，可撤销。
func (s *Service) UpdateItem(id string, p store.ItemPatch) (ItemView, error) {
	old, err := s.st.GetItem(id)
	if err != nil {
		return ItemView{}, err
	}
	it, err := s.st.UpdateItem(id, p)
	if err != nil {
		return ItemView{}, err
	}
	s.pushUndo("编辑", func() error {
		back := store.ItemPatch{Title: &old.Title, Note: &old.Note, GroupID: &old.GroupID, Priority: &old.Priority,
			DueAt: old.DueAt, ClearDue: old.DueAt == nil, Tags: &old.Tags, Sort: &old.SortOrder}
		_, err := s.st.UpdateItem(id, back)
		return err
	})
	s.changed()
	return s.views([]store.Item{it})[0], nil
}

// Reorder 拖拽手动排序（FR-106）：把事项放到 beforeID 之前（空串 = 放到末尾）。
func (s *Service) Reorder(id, beforeID string) error {
	it, err := s.st.GetItem(id)
	if err != nil {
		return err
	}
	items, err := s.st.ListItems(store.Filter{GroupID: it.GroupID, Status: store.StatusTodo})
	if err != nil {
		return err
	}
	store.SortItems(items, s.now(), s.loc())
	var order []store.Item
	for _, x := range items {
		if x.ID != id {
			order = append(order, x)
		}
	}
	pos := len(order)
	for i, x := range order {
		if x.ID == beforeID {
			pos = i
			break
		}
	}
	order = append(order[:pos], append([]store.Item{it}, order[pos:]...)...)
	// 重新分配整段 sort_order，保证与显示顺序一致
	for i, x := range order {
		v := float64(i + 1)
		if x.SortOrder != v {
			if _, err := s.st.UpdateItem(x.ID, store.ItemPatch{Sort: &v}); err != nil {
				return err
			}
		}
	}
	s.changed()
	return nil
}

// Toggle 勾选 / 取消完成（FR-104）。完成周期事项时自动生成下一次（FR-302）。可撤销（AC-08）。
func (s *Service) Toggle(id string, done bool) (ItemView, error) {
	before, err := s.st.GetItem(id)
	if err != nil {
		return ItemView{}, err
	}
	it, err := s.st.SetDone(id, done)
	if err != nil {
		return ItemView{}, err
	}
	var nextID string
	if done && before.Status != store.StatusDone {
		if nextID, err = s.spawnNext(before); err != nil {
			return ItemView{}, err
		}
	}
	s.pushUndo("完成", func() error {
		if _, err := s.st.SetDone(id, before.Status == store.StatusDone); err != nil {
			return err
		}
		if nextID != "" {
			return s.st.PurgeItem(nextID)
		}
		return nil
	})
	s.changed()
	return s.views([]store.Item{it})[0], nil
}

// spawnNext 为带周期提醒的事项生成下一次实例。
// 下一次的 id 由「系列 id + 发生时刻」确定性生成，两台设备各自生成也会合并为同一条（11.5）。
func (s *Service) spawnNext(it store.Item) (string, error) {
	var rule string
	for _, r := range it.Reminders {
		if r.RepeatRule != "" {
			rule = r.RepeatRule
			break
		}
	}
	if rule == "" {
		return "", nil
	}
	var base time.Time
	switch {
	case it.DueAt != nil:
		base = time.UnixMilli(*it.DueAt)
	default:
		for _, r := range it.Reminders {
			if r.RemindAt != nil {
				base = time.UnixMilli(*r.RemindAt)
			}
		}
	}
	if base.IsZero() {
		return "", nil
	}
	after := base
	if now := s.now(); now.After(after) { // 迟完成：从现在起找下一次，避免生成已过期实例
		after = now
	}
	next, ok, err := recur.Next(rule, base, after, s.loc())
	if err != nil || !ok {
		return "", err
	}
	series := it.SeriesID
	if series == "" {
		series = it.ID
		if err := s.st.SetSeries(it.ID, series); err != nil {
			return "", err
		}
	}
	nid := uuid.NewSHA1(uuid.NameSpaceOID, []byte(series+"@"+next.UTC().Format(time.RFC3339))).String()
	if s.st.ItemExists(nid) {
		return "", nil
	}
	nms := next.UnixMilli()
	delta := nms - base.UnixMilli()
	in := store.NewItem{ID: nid, Title: it.Title, Note: it.Note, GroupID: it.GroupID, Priority: &it.Priority,
		DueAt: &nms, Tags: it.Tags, SeriesID: series}
	// 子任务一并带到下一次，并重置为未完成；id 同样确定性生成，多设备各自生成会合并为同一条
	for i, sub := range it.Subtasks {
		sid := uuid.NewSHA1(uuid.NameSpaceOID, []byte(fmt.Sprintf("%s/sub/%d", nid, i))).String()
		in.Subtasks = append(in.Subtasks, store.NewSubtask{ID: sid, Title: sub.Title})
	}
	for _, r := range it.Reminders {
		ri := store.ReminderInput{OffsetMinutes: r.OffsetMinutes, RepeatRule: r.RepeatRule}
		if r.OffsetMinutes == 0 && r.RemindAt != nil {
			t := *r.RemindAt + delta
			ri.RemindAt = &t
		}
		in.Reminders = append(in.Reminders, ri)
	}
	if _, err := s.st.CreateItem(in); err != nil {
		return "", err
	}
	return nid, nil
}

// Delete 删除到回收站（FR-105），可撤销。
func (s *Service) Delete(id string) error {
	if err := s.st.SoftDelete(id); err != nil {
		return err
	}
	s.pushUndo("删除", func() error { _, err := s.st.Restore(id); return err })
	s.changed()
	return nil
}

// Restore 从回收站恢复。
func (s *Service) Restore(id string) (ItemView, error) {
	it, err := s.st.Restore(id)
	if err != nil {
		return ItemView{}, err
	}
	s.changed()
	return s.views([]store.Item{it})[0], nil
}

// Trash 返回回收站内容。
func (s *Service) Trash() ([]ItemView, error) {
	items, err := s.st.ListItems(store.Filter{Status: store.StatusDeleted})
	if err != nil {
		return nil, err
	}
	for i := 0; i < len(items); i++ {
		for j := i; j > 0 && val(items[j].DeletedAt) > val(items[j-1].DeletedAt); j-- {
			items[j], items[j-1] = items[j-1], items[j]
		}
	}
	return s.views(items), nil
}

// EmptyTrash 清空回收站（不可撤销，前端需二次确认）。
func (s *Service) EmptyTrash() (int, error) {
	n, err := s.st.EmptyTrash()
	if err == nil {
		s.changed()
	}
	return n, err
}

// SetReminders 替换事项提醒。
func (s *Service) SetReminders(id string, ins []store.ReminderInput) (ItemView, error) {
	for _, in := range ins {
		if in.RepeatRule != "" {
			if err := recur.Validate(in.RepeatRule); err != nil {
				return ItemView{}, fmt.Errorf("周期规则无效: %w", err)
			}
		}
	}
	old, err := s.st.GetItem(id)
	if err != nil {
		return ItemView{}, err
	}
	it, err := s.st.SetReminders(id, ins)
	if err != nil {
		return ItemView{}, err
	}
	s.pushUndo("设置提醒", func() error {
		var back []store.ReminderInput
		for _, r := range old.Reminders {
			back = append(back, store.ReminderInput{RemindAt: r.RemindAt, OffsetMinutes: r.OffsetMinutes, RepeatRule: r.RepeatRule})
		}
		_, err := s.st.SetReminders(id, back)
		return err
	})
	s.changed()
	return s.views([]store.Item{it})[0], nil
}

// BuildRepeat 由 UI 选项生成 RRULE。
func (s *Service) BuildRepeat(kind string, arg int) (string, error) {
	return recur.Build(recur.Kind(kind), arg)
}

// HandleAction 处理通知按钮（FR-304）：done | snooze10 | snooze60 | tomorrow。
func (s *Service) HandleAction(itemID, action string) error {
	now := s.now()
	switch action {
	case "done":
		_, err := s.Toggle(itemID, true)
		return err
	case "snooze10":
		return s.Snooze(itemID, now.Add(10*time.Minute).UnixMilli())
	case "snooze60":
		return s.Snooze(itemID, now.Add(time.Hour).UnixMilli())
	case "tomorrow":
		loc := s.loc()
		var h, m int
		fmt.Sscanf(s.GetSettings().DefaultRemindTime, "%d:%d", &h, &m)
		t := now.In(loc).AddDate(0, 0, 1)
		return s.Snooze(itemID, time.Date(t.Year(), t.Month(), t.Day(), h, m, 0, 0, loc).UnixMilli())
	}
	return fmt.Errorf("未知操作 %q", action)
}

// Snooze 稍后提醒到 untilMs（UTC 毫秒）。
func (s *Service) Snooze(itemID string, untilMs int64) error {
	if err := s.st.SnoozeItem(itemID, untilMs); err != nil {
		return err
	}
	s.changed()
	return nil
}

// ---- 分组 ----

// Groups 返回全部分组。
func (s *Service) Groups() ([]store.Group, error) { return s.st.ListGroups() }

// CreateGroup 新建分组。
func (s *Service) CreateGroup(name, color string) (store.Group, error) {
	g, err := s.st.CreateGroup(name, color)
	if err == nil {
		s.changed()
	}
	return g, err
}

// UpdateGroup 重命名 / 改色。
func (s *Service) UpdateGroup(id string, name, color *string) error {
	if err := s.st.UpdateGroup(id, name, color, nil); err != nil {
		return err
	}
	s.changed()
	return nil
}

// DeleteGroup 删除分组（事项移回收件箱）。
func (s *Service) DeleteGroup(id string) error {
	if err := s.st.DeleteGroup(id); err != nil {
		return err
	}
	s.changed()
	return nil
}

// ---- 窗口状态 ----

// GetWindow 返回分组的窗口状态；不存在则返回默认值。
func (s *Service) GetWindow(groupID string) (store.Window, error) {
	w, err := s.st.GetWindowByGroup(groupID)
	if err != nil {
		// 颜色 / 透明度留空 = 跟随设置里的默认值，这样之后修改默认值会立刻作用到所有没单独设置过的便签
		return store.Window{GroupID: groupID, Mode: "desktop", Width: 300, Height: 420}, nil
	}
	return w, nil
}

// SaveWindow 保存窗口状态（位置、尺寸、模式、透明度、锁定、折叠）。
func (s *Service) SaveWindow(w store.Window) (store.Window, error) { return s.st.SaveWindow(w) }

// ---- 设置 ----

// GetSettings 返回当前设置。
func (s *Service) GetSettings() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.settings
}

// SaveSettings 保存设置并通知外壳应用。
func (s *Service) SaveSettings(in Settings) (Settings, error) {
	in = in.normalized()
	if err := s.st.SetSetting("app", in); err != nil {
		return in, err
	}
	s.mu.Lock()
	s.settings = in
	s.mu.Unlock()
	s.OnSettings(in)
	s.Emit("settings:changed", in)
	s.Kick()
	return in, nil
}

// DND 供调度器读取。
func (s *Service) DND() scheduler.DND { return s.GetSettings().DND }

// Housekeeping 启动时执行：清理过期回收站、每日备份（FR-105/602）。
func (s *Service) Housekeeping() error {
	if _, err := s.st.PurgeExpired(); err != nil {
		return err
	}
	_, err := s.st.DailyBackup(s.BackupDir())
	return err
}
