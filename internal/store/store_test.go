package store

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"deskpinmemo/internal/crypto"
	"deskpinmemo/internal/hlc"
)

func open(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func ptr[T any](v T) *T { return &v }

func TestCreateEditHLC(t *testing.T) { // AC-16
	base := time.Now()
	now := base
	s, err := Open(filepath.Join(t.TempDir(), "d.db"), Options{Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	it, err := s.CreateItem(NewItem{Title: "交报告"})
	if err != nil {
		t.Fatal(err)
	}
	f0 := hlc.ParseFieldHLC(it.FieldHLC)
	if f0["title"] != it.HLC || f0["due_at"] != it.HLC {
		t.Fatal("新建时所有字段应为同一 HLC", it.FieldHLC)
	}
	prev := it.HLC
	it, _ = s.UpdateItem(it.ID, ItemPatch{Title: ptr("交周报")})
	if hlc.Compare(it.HLC, prev) <= 0 {
		t.Fatal("hlc 未递增")
	}
	f1 := hlc.ParseFieldHLC(it.FieldHLC)
	if f1["title"] != it.HLC || f1["due_at"] != f0["due_at"] {
		t.Fatalf("只应更新 title 的 field_hlc: %v", f1)
	}
	prev = it.HLC
	now = now.Add(-time.Hour) // 系统时间回调 1 小时
	it, _ = s.UpdateItem(it.ID, ItemPatch{DueAt: ptr(base.UnixMilli())})
	if hlc.Compare(it.HLC, prev) <= 0 {
		t.Fatal("回拨时间后 hlc 未递增")
	}
	f2 := hlc.ParseFieldHLC(it.FieldHLC)
	if f2["due_at"] != it.HLC || f2["title"] != f1["title"] {
		t.Fatalf("%v", f2)
	}
	if it.DeviceID != s.DeviceID() {
		t.Fatal("device_id")
	}
}

func TestTitleValidation(t *testing.T) {
	s := open(t)
	if _, err := s.CreateItem(NewItem{Title: "  "}); err == nil {
		t.Fatal("空标题应报错")
	}
	if _, err := s.CreateItem(NewItem{Title: strings.Repeat("字", 201)}); err == nil {
		t.Fatal("超长标题应报错")
	}
	if _, err := s.CreateItem(NewItem{Title: strings.Repeat("字", 200)}); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteRestoreKeepsEverything(t *testing.T) { // AC-09
	s := open(t)
	due := time.Now().Add(time.Hour).UnixMilli()
	it, _ := s.CreateItem(NewItem{Title: "报销", Note: "发票", DueAt: &due, Priority: ptr(2),
		Tags: []string{"#工作", "财务"}, Reminders: []ReminderInput{{OffsetMinutes: 15, RepeatRule: "FREQ=MONTHLY;BYMONTHDAY=5"}}})
	if err := s.SoftDelete(it.ID); err != nil {
		t.Fatal(err)
	}
	if l, _ := s.ListItems(Filter{}); len(l) != 0 {
		t.Fatal("已删除事项不应出现在列表")
	}
	if l, _ := s.ListItems(Filter{Status: StatusDeleted}); len(l) != 1 {
		t.Fatal("回收站应有 1 条")
	}
	r, err := s.Restore(it.ID)
	if err != nil {
		t.Fatal(err)
	}
	if r.Title != "报销" || r.Note != "发票" || r.Priority != 2 || len(r.Tags) != 2 || len(r.Reminders) != 1 ||
		r.DueAt == nil || *r.DueAt != due || r.Status != StatusTodo {
		t.Fatalf("恢复后字段丢失: %+v", r)
	}
	if *r.Reminders[0].RemindAt != due-15*60000 {
		t.Fatal("截止前 15 分钟提醒时间错误")
	}
}

func TestPurgeExpired(t *testing.T) {
	now := time.Now()
	s, _ := Open(filepath.Join(t.TempDir(), "d.db"), Options{Now: func() time.Time { return now }})
	defer s.Close()
	a, _ := s.CreateItem(NewItem{Title: "a"})
	b, _ := s.CreateItem(NewItem{Title: "b"})
	s.SoftDelete(a.ID)
	now = now.Add(20 * 24 * time.Hour)
	s.SoftDelete(b.ID)
	now = now.Add(11 * 24 * time.Hour) // a 已 31 天，b 11 天
	n, err := s.PurgeExpired()
	if err != nil || n != 1 {
		t.Fatal(n, err)
	}
	if _, err := s.GetItem(a.ID); err == nil {
		t.Fatal("a 应被清除")
	}
	if _, err := s.GetItem(b.ID); err != nil {
		t.Fatal("b 应保留")
	}
}

func TestSortOrder(t *testing.T) { // FR-106
	loc := time.FixedZone("x", 8*3600)
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, loc)
	ms := func(d time.Time) *int64 { v := d.UnixMilli(); return &v }
	items := []Item{
		{ID: "nodue", Priority: 2},
		{ID: "future", DueAt: ms(now.AddDate(0, 0, 3)), Status: StatusTodo},
		{ID: "today-low", DueAt: ms(now.Add(3 * time.Hour)), Priority: 0, Status: StatusTodo},
		{ID: "today-high", DueAt: ms(now.Add(4 * time.Hour)), Priority: 2, Status: StatusTodo},
		{ID: "overdue", DueAt: ms(now.Add(-time.Hour)), Status: StatusTodo},
	}
	SortItems(items, now, loc)
	var got []string
	for _, i := range items {
		got = append(got, i.ID)
	}
	want := "overdue,today-high,today-low,future,nodue"
	if strings.Join(got, ",") != want {
		t.Fatalf("got %v want %s", got, want)
	}
}

func TestGroupsAndInbox(t *testing.T) {
	s := open(t)
	inbox, _ := s.InboxID()
	g, _ := s.CreateGroup("工作", "#3b82f6")
	it, _ := s.CreateItem(NewItem{Title: "x", GroupID: g.ID})
	if err := s.DeleteGroup(inbox); err == nil {
		t.Fatal("收件箱不可删除")
	}
	if err := s.DeleteGroup(g.ID); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetItem(it.ID)
	if got.GroupID != inbox {
		t.Fatal("删除分组后事项应移回收件箱")
	}
	gs, _ := s.ListGroups()
	if len(gs) != 1 || !gs[0].IsDefault {
		t.Fatal(gs)
	}
}

func TestSearchChinese(t *testing.T) {
	s := open(t)
	s.CreateItem(NewItem{Title: "回复 HR 邮件", Note: "下午三点前"})
	s.CreateItem(NewItem{Title: "交报销单"})
	for _, q := range []string{"邮件", "报销单", "下午三点", "hr", "HR 邮"} {
		got, err := s.Search(q, 10)
		if err != nil {
			t.Fatalf("%q: %v", q, err)
		}
		if len(got) != 1 {
			t.Fatalf("%q: 期望 1 条, got %d", q, len(got))
		}
	}
	if got, _ := s.Search("不存在", 10); len(got) != 0 {
		t.Fatal("不应命中")
	}
	// 已完成也应可搜索
	it, _ := s.CreateItem(NewItem{Title: "已完成的旧任务"})
	s.SetDone(it.ID, true)
	if got, _ := s.Search("旧任务", 10); len(got) != 1 || got[0].Status != StatusDone {
		t.Fatal("已完成事项应可搜索")
	}
	// 删除后不可搜索；特殊字符不报错
	s.SoftDelete(it.ID)
	if got, _ := s.Search("旧任务", 10); len(got) != 0 {
		t.Fatal("已删除不应命中")
	}
	if _, err := s.Search(`50% "off" _x\`, 10); err != nil {
		t.Fatal(err)
	}
}

func TestSearch10kUnder200ms(t *testing.T) { // AC-10
	s := open(t)
	err := s.Tx(func(tx *sqlTx) error {
		for i := 0; i < 10000; i++ {
			title := fmt.Sprintf("测试事项 %d 编号", i)
			if i == 7777 {
				title = "需要找到的特别事项"
			}
			id := newID()
			h := s.clock.Next()
			if _, err := tx.Exec(`INSERT INTO items(`+itemCols+`) VALUES(?,?,?,(SELECT id FROM groups LIMIT 1),1,NULL,'todo',NULL,NULL,?,'',?,?,?,?,'{}')`,
				id, title, "备注内容", i, 1, 1, h, s.deviceID); err != nil {
				return err
			}
			if err := s.ftsUpsert(tx, id, title, "备注内容", true); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{"特别事项", "特别", "备注内容"} {
		start := time.Now()
		got, err := s.Search(q, 200)
		el := time.Since(start)
		if err != nil || len(got) == 0 {
			t.Fatalf("%q: %v n=%d", q, err, len(got))
		}
		if el > 200*time.Millisecond {
			t.Fatalf("%q 搜索耗时 %v > 200ms", q, el)
		}
		t.Logf("%q: %d 条, %v", q, len(got), el)
	}
}

func TestBackupAndPrune(t *testing.T) {
	now := time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC)
	s, _ := Open(filepath.Join(t.TempDir(), "d.db"), Options{Now: func() time.Time { return now }})
	defer s.Close()
	s.CreateItem(NewItem{Title: "备份我"})
	dir := filepath.Join(t.TempDir(), "backups")
	for d := 0; d < 20; d++ {
		now = time.Date(2026, 1, 1+d, 8, 0, 0, 0, time.UTC)
		if _, err := s.DailyBackup(dir); err != nil {
			t.Fatal(err)
		}
		if _, err := s.DailyBackup(dir); err != nil { // 同一天第二次不应新增
			t.Fatal(err)
		}
	}
	files, _ := filepath.Glob(filepath.Join(dir, "data-*.db"))
	if len(files) != BackupKeep {
		t.Fatalf("应保留 %d 份, got %d", BackupKeep, len(files))
	}
	b, err := Open(files[len(files)-1])
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	if l, _ := b.ListItems(Filter{}); len(l) != 1 || l[0].Title != "备份我" {
		t.Fatal("备份内容不完整")
	}
}

func TestExportImportOverwrite(t *testing.T) { // AC-13
	s := open(t)
	g, _ := s.CreateGroup("生活", "")
	due := time.Now().Add(time.Hour).UnixMilli()
	a, _ := s.CreateItem(NewItem{Title: "买菜", GroupID: g.ID, DueAt: &due, Tags: []string{"家"},
		Reminders: []ReminderInput{{OffsetMinutes: 5}}})
	b, _ := s.CreateItem(NewItem{Title: "已做完"})
	s.SetDone(b.ID, true)
	d, err := s.Export()
	if err != nil {
		t.Fatal(err)
	}
	if d.FormatVersion != 1 || d.ExportedAt == "" {
		t.Fatal("顶层字段")
	}
	// 经 JSON 往返
	js := mustJSON(t, d)
	d2 := mustParse(t, js)
	// 清空数据
	s.SoftDelete(a.ID)
	s.PurgeExpired()
	s.EmptyTrash()
	s.SetDone(b.ID, false)
	st, err := s.Import(d2, ImportOverwrite)
	if err != nil {
		t.Fatal(err)
	}
	if st.Items != 2 {
		t.Fatalf("%+v", st)
	}
	got, err := s.GetItem(a.ID)
	if err != nil || got.Title != "买菜" || got.GroupID != g.ID || len(got.Tags) != 1 || got.Tags[0] != "家" ||
		len(got.Reminders) != 1 || *got.DueAt != due {
		t.Fatalf("%+v %v", got, err)
	}
	if got, _ := s.GetItem(b.ID); got.Status != StatusDone {
		t.Fatal("完成状态应恢复")
	}
	if r, _ := s.Search("买菜", 5); len(r) != 1 {
		t.Fatal("FTS 应重建")
	}
	gs, _ := s.ListGroups()
	if len(gs) != 2 {
		t.Fatalf("分组数 %d", len(gs))
	}
}

func TestImportMergeByHLC(t *testing.T) {
	s := open(t)
	it, _ := s.CreateItem(NewItem{Title: "旧标题"})
	d, _ := s.Export()
	s.UpdateItem(it.ID, ItemPatch{Title: ptr("本地更新")})
	st, err := s.Import(mustParse(t, mustJSON(t, d)), ImportMerge)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GetItem(it.ID); got.Title != "本地更新" {
		t.Fatalf("合并时较新的本地版本应保留, %+v", st)
	}
	// 导入一个本地没有的默认分组：应映射到本地收件箱而不是新增
	gs, _ := s.ListGroups()
	if len(gs) != 1 {
		t.Fatal("不应出现重复收件箱")
	}
}

func TestEncryption(t *testing.T) { // AC-15 的存储层部分
	s := open(t)
	it, _ := s.CreateItem(NewItem{Title: "秘密标题", Note: "秘密备注", Tags: []string{"机密"}})
	s.CreateGroup("私人分组", "")
	dek, _ := crypto.NewDEK()
	c, _ := crypto.NewCipher(dek)
	if err := s.EnableEncryption(c); err != nil {
		t.Fatal(err)
	}
	// 直接读库：必须是密文
	raw, _ := os.ReadFile(s.Path())
	wal, _ := os.ReadFile(s.Path() + "-wal")
	for _, secret := range []string{"秘密标题", "秘密备注", "机密", "私人分组"} {
		if strings.Contains(string(raw), secret) || strings.Contains(string(wal), secret) {
			t.Fatalf("明文 %q 泄露到磁盘", secret)
		}
	}
	got, _ := s.GetItem(it.ID)
	if got.Title != "秘密标题" || got.Tags[0] != "机密" {
		t.Fatal("解锁后应能读取")
	}
	if r, _ := s.Search("秘密", 5); len(r) != 1 {
		t.Fatal("加密模式内存检索")
	}
	// 新建的也要加密
	n, _ := s.CreateItem(NewItem{Title: "新的秘密"})
	var raw2 string
	s.db.QueryRow(`SELECT title FROM items WHERE id=?`, n.ID).Scan(&raw2)
	if !crypto.IsCipher(raw2) {
		t.Fatal("新建事项标题应加密")
	}
	// 未解锁
	s.SetCodec(nil)
	if !s.Locked() {
		t.Fatal("应处于锁定")
	}
	locked, _ := s.GetItem(it.ID)
	if locked.Title == "秘密标题" || !locked.Locked {
		t.Fatal("未解锁不应显示内容")
	}
	if _, err := s.CreateItem(NewItem{Title: "x"}); err == nil {
		t.Fatal("锁定时不能写入")
	}
	// 关闭加密
	s.SetCodec(c)
	if err := s.DisableEncryption(); err != nil {
		t.Fatal(err)
	}
	s.db.QueryRow(`SELECT title FROM items WHERE id=?`, it.ID).Scan(&raw2)
	if raw2 != "秘密标题" {
		t.Fatal("关闭加密后应为明文")
	}
	if r, _ := s.Search("秘密标题", 5); len(r) != 1 {
		t.Fatal("FTS 应恢复")
	}
}

func TestReminderPendingFlow(t *testing.T) {
	now := time.Now()
	s, _ := Open(filepath.Join(t.TempDir(), "d.db"), Options{Now: func() time.Time { return now }})
	defer s.Close()
	at := now.Add(2 * time.Minute).UnixMilli()
	it, _ := s.CreateItem(NewItem{Title: "开会", Reminders: []ReminderInput{{RemindAt: &at}}})
	p, _ := s.PendingReminders()
	if len(p) != 1 || p[0].FireAt != at {
		t.Fatalf("%+v", p)
	}
	s.MarkFired([]string{p[0].ID}, at)
	if p, _ := s.PendingReminders(); len(p) != 0 {
		t.Fatal("触发后不应再待触发")
	}
	until := at + 10*60000
	s.SnoozeItem(it.ID, until) // AC-05：10 分钟后再次提醒
	p, _ = s.PendingReminders()
	if len(p) != 1 || p[0].FireAt != until {
		t.Fatalf("稍后提醒: %+v", p)
	}
	s.MarkFired([]string{p[0].ID}, until)
	if p, _ := s.PendingReminders(); len(p) != 0 {
		t.Fatal()
	}
	// 完成后不再提醒
	s.SnoozeItem(it.ID, until+1000)
	s.SetDone(it.ID, true)
	if p, _ := s.PendingReminders(); len(p) != 0 {
		t.Fatal("已完成事项不应提醒")
	}
	// 修改截止时间会重算「截止前」提醒
	due := now.Add(time.Hour).UnixMilli()
	it2, _ := s.CreateItem(NewItem{Title: "x", DueAt: &due, Reminders: []ReminderInput{{OffsetMinutes: 30}}})
	due2 := due + 3600_000
	s.UpdateItem(it2.ID, ItemPatch{DueAt: &due2})
	got, _ := s.GetItem(it2.ID)
	if *got.Reminders[0].RemindAt != due2-30*60000 {
		t.Fatal("提醒时间未随截止时间重算")
	}
}

func TestWindowsPersist(t *testing.T) {
	s := open(t)
	inbox, _ := s.InboxID()
	w, err := s.SaveWindow(Window{GroupID: inbox, Mode: "top", X: 10, Y: 20, Width: 300, Height: 420, Opacity: 0.1})
	if err != nil || w.Opacity != 0.3 {
		t.Fatal(err, w.Opacity)
	}
	w.X = 99
	s.SaveWindow(w)
	got, _ := s.GetWindowByGroup(inbox)
	if got.X != 99 || got.Mode != "top" || got.ID != w.ID {
		t.Fatalf("%+v", got)
	}
	if l, _ := s.ListWindows(); len(l) != 1 {
		t.Fatal("窗口不应重复")
	}
}

func TestReopenKeepsDeviceIDAndClock(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "d.db")
	s, _ := Open(p)
	id := s.DeviceID()
	it, _ := s.CreateItem(NewItem{Title: "a"})
	s.Close()
	s2, _ := Open(p)
	defer s2.Close()
	if s2.DeviceID() != id {
		t.Fatal("device_id 必须永不改变")
	}
	it2, _ := s2.UpdateItem(it.ID, ItemPatch{Title: ptr("b")})
	if hlc.Compare(it2.HLC, it.HLC) <= 0 {
		t.Fatal("重启后 hlc 必须继续递增")
	}
}

func TestTempStoreInMemory(t *testing.T) { // 排序 / 临时表不落盘到系统临时目录
	s := open(t)
	var v int
	if err := s.db.QueryRow(`PRAGMA temp_store`).Scan(&v); err != nil || v != 2 {
		t.Fatalf("temp_store 应为 MEMORY(2): %d %v", v, err)
	}
}
