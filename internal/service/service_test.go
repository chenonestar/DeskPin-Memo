package service

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"deskpinmemo/internal/store"
)

var cst = time.FixedZone("CST", 8*3600)

type env struct {
	svc *Service
	st  *store.Store
	now *time.Time
	dir string
}

func setup(t *testing.T, start time.Time) *env {
	t.Helper()
	dir := t.TempDir()
	now := start
	st, err := store.Open(filepath.Join(dir, "data.db"), store.Options{Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	svc, err := New(st, dir, Options{Now: func() time.Time { return now }, Loc: func() *time.Location { return cst }})
	if err != nil {
		t.Fatal(err)
	}
	return &env{svc, st, &now, dir}
}

func TestQuickCreateAC01(t *testing.T) {
	e := setup(t, time.Date(2026, 9, 29, 10, 0, 0, 0, cst))
	inbox, _ := e.st.InboxID()
	v, err := e.svc.QuickCreate("明天下午3点 交报告", "")
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 9, 30, 15, 0, 0, 0, cst).UnixMilli()
	if v.Title != "交报告" || v.GroupID != inbox || v.DueAt == nil || *v.DueAt != want {
		t.Fatalf("%+v", v)
	}
	if len(v.Reminders) != 1 || *v.Reminders[0].RemindAt != want {
		t.Fatal("应同时设置到点提醒")
	}
}

func TestQuickPreviewGroupAndDefaultTime(t *testing.T) {
	e := setup(t, time.Date(2026, 9, 29, 10, 0, 0, 0, cst))
	g, _ := e.svc.CreateGroup("工作", "#3b82f6")
	s := e.svc.GetSettings()
	s.DefaultRemindTime = "08:30"
	e.svc.SaveSettings(s)
	p, _ := e.svc.QuickParse("周五 买菜 @工作 #家", "")
	if p.GroupID != g.ID || p.GroupName != "工作" || len(p.Tags) != 1 {
		t.Fatalf("%+v", p)
	}
	if p.DueAt.Hour() != 8 || p.DueAt.Minute() != 30 {
		t.Fatalf("应套用默认提醒时刻: %v", p.DueAt)
	}
}

func TestRecurringWorkdayAC07(t *testing.T) {
	// 周五 09:00 到期，周五当天完成 → 下一次为下周一 09:00
	fri := time.Date(2026, 10, 2, 9, 0, 0, 0, cst)
	e := setup(t, fri.Add(-time.Hour))
	v, err := e.svc.QuickCreate("每个工作日 09:00 站会", "")
	if err != nil {
		t.Fatal(err)
	}
	if !time.UnixMilli(*v.DueAt).Equal(fri) {
		t.Fatalf("首次应为周五 09:00: %v", time.UnixMilli(*v.DueAt).In(cst))
	}
	*e.now = fri.Add(6 * time.Hour) // 周五下午完成
	if _, err := e.svc.Toggle(v.ID, true); err != nil {
		t.Fatal(err)
	}
	items, _ := e.st.ListItems(store.Filter{Status: store.StatusTodo})
	if len(items) != 1 {
		t.Fatalf("应生成下一次, got %d", len(items))
	}
	next := items[0]
	if want := time.Date(2026, 10, 5, 9, 0, 0, 0, cst); !time.UnixMilli(*next.DueAt).Equal(want) {
		t.Fatalf("下一次应为下周一 09:00, got %v", time.UnixMilli(*next.DueAt).In(cst))
	}
	if next.Title != "站会" || len(next.Reminders) != 1 || next.Reminders[0].RepeatRule == "" || next.SeriesID == "" {
		t.Fatalf("%+v", next)
	}
	// 确定性 id：撤销后重新完成应得到相同 id
	nid := next.ID
	if _, err := e.svc.Undo(); err != nil {
		t.Fatal(err)
	}
	if e.st.ItemExists(nid) {
		t.Fatal("撤销后应移除自动生成的下一次")
	}
	e.svc.Toggle(v.ID, true)
	items, _ = e.st.ListItems(store.Filter{Status: store.StatusTodo})
	if len(items) != 1 || items[0].ID != nid {
		t.Fatal("下一次实例 id 应确定性生成")
	}
}

func TestMonthlyLateCompletion(t *testing.T) {
	e := setup(t, time.Date(2026, 1, 3, 8, 0, 0, 0, cst))
	v, _ := e.svc.QuickCreate("每月5日 09:00 交报销单", "")
	*e.now = time.Date(2026, 3, 20, 12, 0, 0, 0, cst) // 拖到 3 月 20 日才完成
	e.svc.Toggle(v.ID, true)
	items, _ := e.st.ListItems(store.Filter{Status: store.StatusTodo})
	if len(items) != 1 || !time.UnixMilli(*items[0].DueAt).Equal(time.Date(2026, 4, 5, 9, 0, 0, 0, cst)) {
		t.Fatalf("迟完成应跳到下一个未来的 5 日: %+v", items)
	}
}

func TestUndoCompleteAC08(t *testing.T) {
	e := setup(t, time.Now())
	v, _ := e.svc.CreateItem(store.NewItem{Title: "x"})
	e.svc.Toggle(v.ID, true)
	if it, _ := e.st.GetItem(v.ID); it.Status != store.StatusDone {
		t.Fatal()
	}
	label, err := e.svc.Undo()
	if err != nil || label != "完成" {
		t.Fatal(label, err)
	}
	if it, _ := e.st.GetItem(v.ID); it.Status != store.StatusTodo || it.CompletedAt != nil {
		t.Fatal("应恢复为未完成")
	}
	if l, _ := e.svc.Undo(); l != "新建" {
		t.Fatalf("继续撤销应撤销新建: %q", l)
	}
	if e.st.ItemExists(v.ID) {
		t.Fatal()
	}
	if l, _ := e.svc.Undo(); l != "" {
		t.Fatal("栈空")
	}
}

func TestUndoDeleteAndEdit(t *testing.T) {
	e := setup(t, time.Now())
	due := time.Now().Add(time.Hour).UnixMilli()
	v, _ := e.svc.CreateItem(store.NewItem{Title: "原标题", DueAt: &due, Tags: []string{"a"}})
	nt := "新标题"
	e.svc.UpdateItem(v.ID, store.ItemPatch{Title: &nt, ClearDue: true, Tags: &[]string{"b"}})
	got, _ := e.st.GetItem(v.ID)
	if got.Title != nt || got.DueAt != nil || got.Tags[0] != "b" {
		t.Fatalf("%+v", got)
	}
	e.svc.Undo()
	got, _ = e.st.GetItem(v.ID)
	if got.Title != "原标题" || got.DueAt == nil || *got.DueAt != due || got.Tags[0] != "a" {
		t.Fatalf("撤销编辑失败: %+v", got)
	}
	e.svc.Delete(v.ID)
	e.svc.Undo()
	if got, _ := e.st.GetItem(v.ID); got.Status != store.StatusTodo {
		t.Fatal("撤销删除")
	}
}

func TestGroupContentOrderAndDone(t *testing.T) {
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, cst)
	e := setup(t, now)
	inbox, _ := e.st.InboxID()
	ms := func(d time.Duration) *int64 { v := now.Add(d).UnixMilli(); return &v }
	e.svc.CreateItem(store.NewItem{Title: "无日期"})
	e.svc.CreateItem(store.NewItem{Title: "未来", DueAt: ms(72 * time.Hour)})
	e.svc.CreateItem(store.NewItem{Title: "今天", DueAt: ms(3 * time.Hour)})
	e.svc.CreateItem(store.NewItem{Title: "逾期", DueAt: ms(-2 * time.Hour)})
	d, _ := e.svc.CreateItem(store.NewItem{Title: "做完"})
	e.svc.Toggle(d.ID, true)
	gv, err := e.svc.GroupContent(inbox)
	if err != nil {
		t.Fatal(err)
	}
	var titles []string
	for _, v := range gv.Todo {
		titles = append(titles, v.Title)
	}
	if strings.Join(titles, ",") != "逾期,今天,未来,无日期" || gv.Overdue != 1 || !gv.Todo[0].Overdue {
		t.Fatalf("%v overdue=%d", titles, gv.Overdue)
	}
	if len(gv.Done) != 1 || gv.Done[0].Title != "做完" {
		t.Fatal("已完成区")
	}
}

func TestReorder(t *testing.T) {
	e := setup(t, time.Now())
	inbox, _ := e.st.InboxID()
	a, _ := e.svc.CreateItem(store.NewItem{Title: "a"})
	b, _ := e.svc.CreateItem(store.NewItem{Title: "b"})
	c, _ := e.svc.CreateItem(store.NewItem{Title: "c"})
	if err := e.svc.Reorder(c.ID, a.ID); err != nil { // c 放到 a 前
		t.Fatal(err)
	}
	gv, _ := e.svc.GroupContent(inbox)
	var got []string
	for _, v := range gv.Todo {
		got = append(got, v.Title)
	}
	if strings.Join(got, "") != "cab" {
		t.Fatal(got)
	}
	e.svc.Reorder(c.ID, "") // 放到末尾
	gv, _ = e.svc.GroupContent(inbox)
	got = nil
	for _, v := range gv.Todo {
		got = append(got, v.Title)
	}
	if strings.Join(got, "") != "abc" {
		t.Fatal(got)
	}
	_ = b
}

func TestSmartViews(t *testing.T) {
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, cst)
	e := setup(t, now)
	ms := func(d time.Duration) *int64 { v := now.Add(d).UnixMilli(); return &v }
	e.svc.CreateItem(store.NewItem{Title: "逾期", DueAt: ms(-20 * time.Hour)})
	e.svc.CreateItem(store.NewItem{Title: "今天", DueAt: ms(2 * time.Hour)})
	e.svc.CreateItem(store.NewItem{Title: "明天", DueAt: ms(20 * time.Hour)})
	e.svc.CreateItem(store.NewItem{Title: "十天后", DueAt: ms(240 * time.Hour)})
	e.svc.CreateItem(store.NewItem{Title: "无日期"})
	d, _ := e.svc.CreateItem(store.NewItem{Title: "已完成"})
	e.svc.Toggle(d.ID, true)
	for view, want := range map[string]string{"today": "今天", "overdue": "逾期", "next7": "明天", "nodate": "无日期", "done": "已完成"} {
		got, err := e.svc.SmartView(view)
		if err != nil || len(got) != 1 || got[0].Title != want {
			t.Errorf("%s: %v %+v", view, err, got)
		}
	}
	if _, err := e.svc.SmartView("bogus"); err == nil {
		t.Fatal()
	}
}

func TestHandleActions(t *testing.T) { // FR-304
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, cst)
	e := setup(t, now)
	at := now.UnixMilli()
	v, _ := e.svc.CreateItem(store.NewItem{Title: "x", Reminders: []store.ReminderInput{{RemindAt: &at}}})
	e.svc.HandleAction(v.ID, "snooze10")
	p, _ := e.st.PendingReminders()
	if len(p) != 1 || p[0].FireAt != now.Add(10*time.Minute).UnixMilli() {
		t.Fatalf("%+v", p)
	}
	e.svc.HandleAction(v.ID, "tomorrow")
	p, _ = e.st.PendingReminders()
	if want := time.Date(2026, 9, 30, 9, 0, 0, 0, cst).UnixMilli(); p[0].FireAt != want {
		t.Fatalf("明天应为次日 09:00: %v", time.UnixMilli(p[0].FireAt).In(cst))
	}
	e.svc.HandleAction(v.ID, "done")
	if it, _ := e.st.GetItem(v.ID); it.Status != store.StatusDone {
		t.Fatal()
	}
	if err := e.svc.HandleAction(v.ID, "bogus"); err == nil {
		t.Fatal()
	}
}

func TestSettingsPersist(t *testing.T) {
	e := setup(t, time.Now())
	s := e.svc.GetSettings()
	s.FontSize = 99
	s.Theme = "dark"
	got, _ := e.svc.SaveSettings(s)
	if got.FontSize != 14 || got.Theme != "dark" {
		t.Fatalf("非法字号应回退: %+v", got)
	}
	svc2, _ := New(e.st, e.dir)
	if svc2.GetSettings().Theme != "dark" {
		t.Fatal("设置应持久化")
	}
}

func TestEncryptionFlowAC15(t *testing.T) {
	e := setup(t, time.Now())
	e.svc.CreateItem(store.NewItem{Title: "机密事项"})
	if _, err := e.svc.EnableEncryption("123", ""); err == nil {
		t.Fatal("过短密码应拒绝")
	}
	rk, err := e.svc.EnableEncryption("main-pass", UnlockPassword)
	if err != nil || len(rk) != 24 {
		t.Fatal(rk, err)
	}
	st := e.svc.EncryptionStatus()
	if !st.Enabled || st.Locked || st.Mode != UnlockPassword {
		t.Fatalf("%+v", st)
	}
	// 「重启」：新 Store 实例，未解锁
	path := e.st.Path()
	e.st.Close()
	st2, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st2.Close()
	svc2, _ := New(st2, e.dir)
	if !st2.Locked() || svc2.TryAutoUnlock() {
		t.Fatal("每次输入密码模式重启后必须保持锁定")
	}
	if err := svc2.Unlock("wrong-pass"); err == nil {
		t.Fatal("错误密码应失败")
	}
	if err := svc2.Unlock("main-pass"); err != nil {
		t.Fatal(err)
	}
	items, _ := st2.ListItems(store.Filter{})
	if len(items) != 1 || items[0].Title != "机密事项" {
		t.Fatalf("解锁后应正常显示 %+v", items)
	}
	// 用恢复密钥重设主密码（支持带空格/连字符/小写）
	spaced := strings.ToLower(rk[:4] + " " + rk[4:8] + "-" + rk[8:])
	st2.SetCodec(nil)
	if err := svc2.ResetPasswordWithRecovery(spaced, "new-pass"); err != nil {
		t.Fatal(err)
	}
	if err := svc2.ChangePassword("new-pass", "third-pass"); err != nil {
		t.Fatal(err)
	}
	st2.SetCodec(nil)
	if svc2.Unlock("main-pass") == nil || svc2.Unlock("third-pass") != nil {
		t.Fatal("旧密码应失效，新密码可用")
	}
	if err := svc2.DisableEncryption("bad"); err == nil {
		t.Fatal()
	}
	if err := svc2.DisableEncryption("third-pass"); err != nil {
		t.Fatal(err)
	}
	if s := svc2.EncryptionStatus(); s.Enabled || s.Locked {
		t.Fatalf("%+v", s)
	}
	if r, _ := st2.Search("机密", 5); len(r) != 1 {
		t.Fatal("关闭加密后 FTS 应恢复")
	}
}

func TestExportImportEncryptedEnvelopeAC13(t *testing.T) {
	e := setup(t, time.Now())
	g, _ := e.svc.CreateGroup("生活", "")
	due := time.Now().Add(time.Hour).UnixMilli()
	a, _ := e.svc.CreateItem(store.NewItem{Title: "买菜", GroupID: g.ID, DueAt: &due, Tags: []string{"家"}})
	b, _ := e.svc.CreateItem(store.NewItem{Title: "已做完", Note: "备注"})
	e.svc.Toggle(b.ID, true)
	plain := filepath.Join(e.dir, "plain.json")
	enc := filepath.Join(e.dir, "enc.json")
	if err := e.svc.ExportJSON(plain, ""); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.ExportJSON(enc, "exp-pass"); err != nil {
		t.Fatal(err)
	}
	if info, _ := e.svc.InspectImport(enc, ""); !info.Encrypted {
		t.Fatal("应识别为加密导出")
	}
	if _, err := e.svc.ImportJSON(enc, "wrong", true); err == nil {
		t.Fatal("错误导出密码应失败")
	}
	// 清空数据
	e.st.SoftDelete(a.ID)
	e.st.SoftDelete(b.ID)
	e.st.EmptyTrash()
	if l, _ := e.st.ListItems(store.Filter{}); len(l) != 0 {
		t.Fatal()
	}
	st, err := e.svc.ImportJSON(enc, "exp-pass", true)
	if err != nil || st.Items != 2 {
		t.Fatal(st, err)
	}
	got, _ := e.st.GetItem(a.ID)
	if got.Title != "买菜" || got.GroupID != g.ID || got.Tags[0] != "家" || *got.DueAt != due {
		t.Fatalf("%+v", got)
	}
	if got, _ := e.st.GetItem(b.ID); got.Status != store.StatusDone || got.Note != "备注" {
		t.Fatal("完成状态/备注应恢复")
	}
	if len(e.svc.ListBackups()) == 0 {
		t.Fatal("覆盖导入前应自动备份")
	}
	// 明文导入也可
	if _, err := e.svc.ImportJSON(plain, "", false); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.ExportMarkdown(filepath.Join(e.dir, "x.md")); err != nil {
		t.Fatal(err)
	}
}

func TestHousekeeping(t *testing.T) {
	e := setup(t, time.Now())
	v, _ := e.svc.CreateItem(store.NewItem{Title: "旧"})
	e.svc.Delete(v.ID)
	*e.now = e.now.Add(31 * 24 * time.Hour)
	if err := e.svc.Housekeeping(); err != nil {
		t.Fatal(err)
	}
	if e.st.ItemExists(v.ID) {
		t.Fatal("超过 30 天的回收站事项应清除")
	}
	if len(e.svc.ListBackups()) != 1 {
		t.Fatal("每日备份")
	}
}
