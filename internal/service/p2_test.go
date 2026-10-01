package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"deskpinmemo/internal/datadir"
	"deskpinmemo/internal/store"
)

func TestOverviewContentAndOrder(t *testing.T) { // FR-308
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, cst)
	e := setup(t, now)
	ms := func(d time.Duration) *int64 { v := now.Add(d).UnixMilli(); return &v }
	hi := 2
	e.svc.CreateItem(store.NewItem{Title: "昨天的", DueAt: ms(-20 * time.Hour)})
	e.svc.CreateItem(store.NewItem{Title: "今早的", DueAt: ms(-3 * time.Hour), Priority: &hi})
	e.svc.CreateItem(store.NewItem{Title: "下午的", DueAt: ms(4 * time.Hour)})
	e.svc.CreateItem(store.NewItem{Title: "明天的", DueAt: ms(20 * time.Hour)}) // 不在概览里
	e.svc.CreateItem(store.NewItem{Title: "无日期"})                            // 不在概览里
	d, _ := e.svc.CreateItem(store.NewItem{Title: "做完了", DueAt: ms(2 * time.Hour)})
	e.svc.Toggle(d.ID, true) // 已完成不在概览里

	ov, err := e.svc.Overview()
	if err != nil {
		t.Fatal(err)
	}
	titles := func(vs []ItemView) string {
		var s []string
		for _, v := range vs {
			s = append(s, v.Title)
		}
		return strings.Join(s, ",")
	}
	if titles(ov.Overdue) != "今早的,昨天的" || titles(ov.Today) != "下午的" || ov.Total != 3 || ov.Date != "2026-09-29" {
		t.Fatalf("overdue=%s today=%s total=%d", titles(ov.Overdue), titles(ov.Today), ov.Total)
	}
	if ov.Overdue[0].GroupName != "收件箱" {
		t.Fatal("概览应带分组名")
	}
}

func TestClaimDailyOverviewOncePerDay(t *testing.T) {
	now := time.Date(2026, 9, 29, 9, 0, 0, 0, cst)
	e := setup(t, now)
	due := now.Add(time.Hour).UnixMilli()
	e.svc.CreateItem(store.NewItem{Title: "今天要做", DueAt: &due})
	if ok, err := e.svc.ClaimDailyOverview(); err != nil || !ok {
		t.Fatalf("当天首次应弹出: %v %v", ok, err)
	}
	if ok, _ := e.svc.ClaimDailyOverview(); ok {
		t.Fatal("同一天第二次启动不应再弹")
	}
	*e.now = now.Add(24 * time.Hour) // 第二天
	if ok, _ := e.svc.ClaimDailyOverview(); !ok {
		t.Fatal("第二天首次启动应再次弹出（昨天的事项已逾期）")
	}
}

func TestClaimDailyOverviewSkips(t *testing.T) {
	now := time.Date(2026, 9, 29, 9, 0, 0, 0, cst)
	// 没有可展示内容：不弹，但当天已认领，之后新增事项也不会弹
	e := setup(t, now)
	if ok, _ := e.svc.ClaimDailyOverview(); ok {
		t.Fatal("没有内容不应弹出")
	}
	due := now.Add(time.Hour).UnixMilli()
	e.svc.CreateItem(store.NewItem{Title: "白天新建", DueAt: &due})
	if ok, _ := e.svc.ClaimDailyOverview(); ok {
		t.Fatal("当天已认领，白天新建事项不应再弹")
	}
	// 设置关闭
	e2 := setup(t, now)
	e2.svc.CreateItem(store.NewItem{Title: "x", DueAt: &due})
	s := e2.svc.GetSettings()
	s.DailyOverview = false
	e2.svc.SaveSettings(s)
	if ok, _ := e2.svc.ClaimDailyOverview(); ok {
		t.Fatal("设置关闭时不应弹出")
	}
	// 加密未解锁：不弹，也不占用名额
	e3 := setup(t, now)
	e3.svc.CreateItem(store.NewItem{Title: "x", DueAt: &due})
	e3.st.SetEncrypted(true)
	if ok, _ := e3.svc.ClaimDailyOverview(); ok {
		t.Fatal("未解锁不应弹出")
	}
	if v, _ := e3.st.GetMeta(metaOverviewDate); v != "" {
		t.Fatal("未解锁时不应占用当天名额")
	}
}

func TestOverviewDefaultOn(t *testing.T) {
	if !DefaultSettings().DailyOverview {
		t.Fatal("每日概览默认应开启")
	}
}

// ---- FR-605 ----

func setupDirs(t *testing.T) (*env, string) {
	t.Helper()
	e := setup(t, time.Date(2026, 9, 29, 12, 0, 0, 0, cst))
	return e, e.dir // setup 中 configDir == dataDir == e.dir
}

func TestChangeDataDirCopy(t *testing.T) {
	e, cfg := setupDirs(t)
	e.svc.CreateItem(store.NewItem{Title: "要带走的事项", Note: "备注"})
	if _, err := e.svc.BackupNow(); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "Sync", "DeskPin")
	if i := e.svc.InspectDataDir(target); !i.Valid || i.HasData {
		t.Fatalf("%+v", i)
	}
	res, err := e.svc.ChangeDataDir(target, DataDirCopy)
	if err != nil || !res.RestartRequired || res.NewDir != target {
		t.Fatal(res, err)
	}
	// 原数据保留
	if !e.st.ItemExists(mustFirstItemID(t, e.st)) {
		t.Fatal("原库应保持可用")
	}
	// 指针生效：重启后解析到新目录
	if r := datadir.Resolve(cfg); r.Dir != target || !r.Custom {
		t.Fatalf("%+v", r)
	}
	// 新目录里的数据库完整，备份也一起带过去
	st2, err := store.Open(filepath.Join(target, "data.db"), store.Options{JournalMode: "DELETE"})
	if err != nil {
		t.Fatal(err)
	}
	defer st2.Close()
	items, _ := st2.ListItems(store.Filter{})
	if len(items) != 1 || items[0].Title != "要带走的事项" || items[0].Note != "备注" {
		t.Fatalf("%+v", items)
	}
	if st2.DeviceID() != e.st.DeviceID() {
		t.Fatal("device_id 必须随数据库一起迁移而不变")
	}
	if b, _ := filepath.Glob(filepath.Join(target, "backups", "*.db")); len(b) != 1 {
		t.Fatalf("备份应复制过去: %v", b)
	}
	// 回滚日志模式下不产生 -wal 文件
	st2.CreateItem(store.NewItem{Title: "再写一条"})
	if _, err := os.Stat(filepath.Join(target, "data.db-wal")); err == nil {
		t.Fatal("DELETE 模式不应有 -wal 文件")
	}
}

func mustFirstItemID(t *testing.T, st *store.Store) string {
	t.Helper()
	items, _ := st.ListItems(store.Filter{})
	if len(items) == 0 {
		t.Fatal("no items")
	}
	return items[0].ID
}

func TestChangeDataDirExistingData(t *testing.T) {
	e, _ := setupDirs(t)
	e.svc.CreateItem(store.NewItem{Title: "当前库的事项"})
	// 目标目录里已有另一份数据
	target := t.TempDir()
	other, _ := store.Open(filepath.Join(target, "data.db"))
	other.CreateItem(store.NewItem{Title: "目标库的事项"})
	other.Close()

	if i := e.svc.InspectDataDir(target); !i.Valid || !i.HasData {
		t.Fatalf("应识别出已有数据: %+v", i)
	}
	if _, err := e.svc.ChangeDataDir(target, DataDirCopy); err == nil {
		t.Fatal("目标已有数据时，复制模式必须拒绝，避免静默覆盖")
	}
	// 使用已有数据：不复制
	if _, err := e.svc.ChangeDataDir(target, DataDirUse); err != nil {
		t.Fatal(err)
	}
	st, _ := store.Open(filepath.Join(target, "data.db"))
	items, _ := st.ListItems(store.Filter{})
	st.Close()
	if len(items) != 1 || items[0].Title != "目标库的事项" {
		t.Fatalf("use 模式应保留目标库: %+v", items)
	}
	// 替换：旧文件改名保留
	res, err := e.svc.ChangeDataDir(target, DataDirReplace)
	if err != nil || res.Kept == "" {
		t.Fatal(res, err)
	}
	if _, err := os.Stat(res.Kept); err != nil {
		t.Fatal("被替换的旧库应改名保留")
	}
	st, _ = store.Open(filepath.Join(target, "data.db"))
	items, _ = st.ListItems(store.Filter{})
	st.Close()
	if len(items) != 1 || items[0].Title != "当前库的事项" {
		t.Fatalf("replace 模式应写入当前数据: %+v", items)
	}
}

func TestChangeDataDirRejects(t *testing.T) {
	e, cfg := setupDirs(t)
	if _, err := e.svc.ChangeDataDir(cfg, DataDirCopy); err == nil {
		t.Fatal("不能切换到当前目录")
	}
	if _, err := e.svc.ChangeDataDir(filepath.Join(cfg, "sub"), DataDirCopy); err == nil || !strings.Contains(err.Error(), "子文件夹") {
		t.Fatalf("不能选当前目录的子文件夹: %v", err)
	}
	if _, err := e.svc.ChangeDataDir("relative", DataDirCopy); err == nil {
		t.Fatal("必须是绝对路径")
	}
	if _, err := e.svc.ChangeDataDir(t.TempDir(), "bogus"); err == nil {
		t.Fatal("未知模式")
	}
	if _, err := e.svc.ChangeDataDir(t.TempDir(), DataDirUse); err == nil {
		t.Fatal("空目录不能 use")
	}
	if r := datadir.Resolve(cfg); r.Custom {
		t.Fatal("失败的切换不应写入指针")
	}
}

func TestChangeDataDirPortableBlocked(t *testing.T) {
	dir := t.TempDir()
	st, _ := store.Open(filepath.Join(dir, "data.db"))
	defer st.Close()
	svc, _ := New(st, dir, Options{Portable: true})
	if !svc.DataDirStatus().Portable {
		t.Fatal()
	}
	if _, err := svc.ChangeDataDir(t.TempDir(), DataDirCopy); err == nil || !strings.Contains(err.Error(), "绿色版") {
		t.Fatalf("绿色版不允许改数据目录: %v", err)
	}
}

func TestRestoreDefaultDataDir(t *testing.T) {
	cfg := t.TempDir()
	custom := t.TempDir()
	st, _ := store.Open(filepath.Join(custom, "data.db"), store.Options{JournalMode: "DELETE"})
	defer st.Close()
	st.CreateItem(store.NewItem{Title: "在自定义目录"})
	datadir.Set(cfg, custom)
	svc, _ := New(st, custom, Options{ConfigDir: cfg})
	status := svc.DataDirStatus()
	if !status.Custom || status.Dir != custom || status.ConfigDir != cfg {
		t.Fatalf("%+v", status)
	}
	// 切回默认目录（默认目录里没有 data.db → 复制）
	res, err := svc.ChangeDataDir(cfg, DataDirCopy)
	if err != nil || res.NewDir != cfg {
		t.Fatal(res, err)
	}
	if r := datadir.Resolve(cfg); r.Custom || r.Dir != cfg {
		t.Fatalf("恢复默认后应清除指针: %+v", r)
	}
	if _, err := os.Stat(filepath.Join(cfg, "data.db")); err != nil {
		t.Fatal("数据应已复制回默认目录")
	}
}
