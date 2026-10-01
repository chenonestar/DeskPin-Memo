package store

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"deskpinmemo/internal/crypto"
	"deskpinmemo/internal/hlc"
)

func titlesOf(subs []Subtask) string {
	var out []string
	for _, s := range subs {
		out = append(out, s.Title)
	}
	return strings.Join(out, ",")
}

func TestSubtaskCRUDAndProgress(t *testing.T) { // FR-108
	s := open(t)
	it, _ := s.CreateItem(NewItem{Title: "出差准备"})
	if len(it.Subtasks) != 0 {
		t.Fatal("新事项没有子任务")
	}
	a, err := s.AddSubtask(it.ID, "订机票")
	if err != nil {
		t.Fatal(err)
	}
	s.AddSubtask(it.ID, "订酒店")
	s.AddSubtask(it.ID, "  带充电器  ")
	got, _ := s.GetItem(it.ID)
	if titlesOf(got.Subtasks) != "订机票,订酒店,带充电器" {
		t.Fatalf("按添加顺序: %s", titlesOf(got.Subtasks))
	}
	// 勾选
	done := true
	sub, err := s.UpdateSubtask(a.ID, nil, &done)
	if err != nil || !sub.Done || sub.CompletedAt == nil {
		t.Fatal(sub, err)
	}
	// 改标题
	nt := "订高铁票"
	sub, _ = s.UpdateSubtask(a.ID, &nt, nil)
	if sub.Title != nt || !sub.Done {
		t.Fatalf("%+v", sub)
	}
	// 取消勾选清除完成时间
	undone := false
	sub, _ = s.UpdateSubtask(a.ID, nil, &undone)
	if sub.Done || sub.CompletedAt != nil {
		t.Fatalf("%+v", sub)
	}
	// 删除是软删除：不再出现，但行还在（墓碑）
	if err := s.DeleteSubtask(a.ID); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetItem(it.ID)
	if titlesOf(got.Subtasks) != "订酒店,带充电器" {
		t.Fatal(titlesOf(got.Subtasks))
	}
	var tomb int
	s.db.QueryRow(`SELECT deleted FROM subtasks WHERE id=?`, a.ID).Scan(&tomb)
	if tomb != 1 {
		t.Fatal("应保留墓碑")
	}
	// 撤销删除
	s.RestoreSubtask(a.ID)
	got, _ = s.GetItem(it.ID)
	if len(got.Subtasks) != 3 {
		t.Fatal("恢复后应有 3 条")
	}
}

func TestSubtaskValidation(t *testing.T) {
	s := open(t)
	it, _ := s.CreateItem(NewItem{Title: "x"})
	if _, err := s.AddSubtask(it.ID, "   "); err == nil {
		t.Fatal("空标题应拒绝")
	}
	if _, err := s.AddSubtask(it.ID, strings.Repeat("字", 201)); err == nil {
		t.Fatal("超长应拒绝")
	}
	if _, err := s.AddSubtask("不存在", "x"); err == nil {
		t.Fatal("事项不存在应报错")
	}
	s.SoftDelete(it.ID)
	if _, err := s.AddSubtask(it.ID, "x"); err == nil {
		t.Fatal("已删除事项不能加子任务")
	}
}

func TestSubtaskReorder(t *testing.T) {
	s := open(t)
	it, _ := s.CreateItem(NewItem{Title: "x"})
	var ids []string
	for _, n := range []string{"甲", "乙", "丙"} {
		sub, _ := s.AddSubtask(it.ID, n)
		ids = append(ids, sub.ID)
	}
	other, _ := s.CreateItem(NewItem{Title: "y"})
	foreign, _ := s.AddSubtask(other.ID, "别人的")
	if err := s.ReorderSubtasks(it.ID, []string{ids[2], ids[0], foreign.ID}); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetItem(it.ID)
	if titlesOf(got.Subtasks) != "丙,甲,乙" {
		t.Fatalf("未列出的排在后面，别的事项的 id 被忽略: %s", titlesOf(got.Subtasks))
	}
	if o, _ := s.GetItem(other.ID); titlesOf(o.Subtasks) != "别人的" {
		t.Fatal("不应影响其他事项")
	}
}

func TestSubtaskFieldHLC(t *testing.T) { // 与其他业务表一致地维护同步预留字段
	now := time.Now()
	s, _ := Open(filepath.Join(t.TempDir(), "d.db"), Options{Now: func() time.Time { return now }})
	defer s.Close()
	it, _ := s.CreateItem(NewItem{Title: "x"})
	sub, _ := s.AddSubtask(it.ID, "标题")
	var h0, fh0, dev string
	s.db.QueryRow(`SELECT hlc,field_hlc,device_id FROM subtasks WHERE id=?`, sub.ID).Scan(&h0, &fh0, &dev)
	if dev != s.DeviceID() {
		t.Fatal("device_id")
	}
	f0 := hlc.ParseFieldHLC(fh0)
	if f0["title"] != h0 || f0["done"] != h0 {
		t.Fatalf("新建时所有字段同一 HLC: %v", f0)
	}
	now = now.Add(-time.Hour) // 回拨系统时间
	done := true
	s.UpdateSubtask(sub.ID, nil, &done)
	var h1, fh1 string
	s.db.QueryRow(`SELECT hlc,field_hlc FROM subtasks WHERE id=?`, sub.ID).Scan(&h1, &fh1)
	f1 := hlc.ParseFieldHLC(fh1)
	if hlc.Compare(h1, h0) <= 0 {
		t.Fatal("hlc 必须递增")
	}
	if f1["done"] != h1 || f1["title"] != f0["title"] {
		t.Fatalf("只应更新 done 相关字段: %v", f1)
	}
}

func TestSubtasksCreatedWithItemAndPurged(t *testing.T) {
	s := open(t)
	it, err := s.CreateItem(NewItem{Title: "带子任务", Subtasks: []NewSubtask{{Title: "一"}, {Title: "二", Done: true}, {ID: "fixed-id-1", Title: "三"}}})
	if err != nil {
		t.Fatal(err)
	}
	if titlesOf(it.Subtasks) != "一,二,三" || !it.Subtasks[1].Done || it.Subtasks[2].ID != "fixed-id-1" {
		t.Fatalf("%+v", it.Subtasks)
	}
	s.SoftDelete(it.ID)
	s.EmptyTrash()
	var n int
	s.db.QueryRow(`SELECT COUNT(*) FROM subtasks`).Scan(&n)
	if n != 0 {
		t.Fatalf("彻底删除事项时应一并清除子任务, 剩 %d", n)
	}
}

func TestSubtasksSurviveRestore(t *testing.T) {
	s := open(t)
	it, _ := s.CreateItem(NewItem{Title: "x", Subtasks: []NewSubtask{{Title: "a"}, {Title: "b", Done: true}}})
	s.SoftDelete(it.ID)
	r, _ := s.Restore(it.ID)
	if len(r.Subtasks) != 2 || !r.Subtasks[1].Done {
		t.Fatalf("恢复事项后子任务应完整: %+v", r.Subtasks)
	}
}

func TestSubtaskEncryption(t *testing.T) {
	s := open(t)
	it, _ := s.CreateItem(NewItem{Title: "x", Subtasks: []NewSubtask{{Title: "机密子任务"}}})
	dek, _ := crypto.NewDEK()
	c, _ := crypto.NewCipher(dek)
	if err := s.EnableEncryption(c); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(s.Path())
	if strings.Contains(string(raw), "机密子任务") {
		t.Fatal("子任务标题不应以明文落盘")
	}
	var stored string
	s.db.QueryRow(`SELECT title FROM subtasks`).Scan(&stored)
	if !crypto.IsCipher(stored) {
		t.Fatal("子任务标题应加密")
	}
	got, _ := s.GetItem(it.ID)
	if got.Subtasks[0].Title != "机密子任务" {
		t.Fatal("解锁后可读")
	}
	n, err := s.AddSubtask(it.ID, "新增的机密")
	if err != nil {
		t.Fatal(err)
	}
	s.db.QueryRow(`SELECT title FROM subtasks WHERE id=?`, n.ID).Scan(&stored)
	if !crypto.IsCipher(stored) {
		t.Fatal("加密模式下新增的子任务也应加密")
	}
	s.SetCodec(nil)
	locked, _ := s.GetItem(it.ID)
	if locked.Subtasks[0].Title == "机密子任务" || !locked.Subtasks[0].Locked {
		t.Fatal("未解锁不应显示内容")
	}
	s.SetCodec(c)
	if err := s.DisableEncryption(); err != nil {
		t.Fatal(err)
	}
	s.db.QueryRow(`SELECT title FROM subtasks WHERE id=?`, n.ID).Scan(&stored)
	if stored != "新增的机密" {
		t.Fatalf("关闭加密后应为明文: %q", stored)
	}
}

func TestSubtaskExportImportAndMarkdown(t *testing.T) {
	s := open(t)
	it, _ := s.CreateItem(NewItem{Title: "旅行", Subtasks: []NewSubtask{{Title: "订票", Done: true}, {Title: "收拾行李"}}})
	d, err := s.Export()
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Tables["subtasks"]) != 2 {
		t.Fatal("导出应包含 subtasks 表")
	}
	md, _ := s.Markdown(time.UTC)
	if !strings.Contains(md, "- [ ] 旅行") || !strings.Contains(md, "  - [x] 订票") || !strings.Contains(md, "  - [ ] 收拾行李") {
		t.Fatalf("Markdown 应缩进列出子任务:\n%s", md)
	}
	// 清空后覆盖导入
	s.SoftDelete(it.ID)
	s.EmptyTrash()
	st, err := s.Import(mustParse(t, mustJSON(t, d)), ImportOverwrite)
	if err != nil || st.Subtasks != 2 {
		t.Fatal(st, err)
	}
	got, _ := s.GetItem(it.ID)
	if titlesOf(got.Subtasks) != "订票,收拾行李" || !got.Subtasks[0].Done {
		t.Fatalf("%+v", got.Subtasks)
	}
	// 合并导入：本地较新的子任务修改保留
	nt := "订高铁票"
	s.UpdateSubtask(got.Subtasks[0].ID, &nt, nil)
	if _, err := s.Import(mustParse(t, mustJSON(t, d)), ImportMerge); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetItem(it.ID)
	if got.Subtasks[0].Title != nt {
		t.Fatal("合并时较新的本地版本应保留")
	}
	// 父事项缺失的子任务被跳过而不是报外键错误
	d.Tables["subtasks"] = append(d.Tables["subtasks"], map[string]any{"id": "orphan", "item_id": "nope", "title": "x", "hlc": "9", "device_id": "d", "field_hlc": "{}", "created_at": float64(1)})
	st, err = s.Import(mustParse(t, mustJSON(t, d)), ImportMerge)
	if err != nil || st.Skipped == 0 {
		t.Fatal(st, err)
	}
}

// 从 v1 数据库升级到 v2：迁移前自动备份，已有数据完整保留（NFR-11）。
func TestMigrateV1ToV2(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.db")
	raw, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(migrations[0]); err != nil {
		t.Fatal(err)
	}
	raw.Exec(`PRAGMA user_version = 1`)
	raw.Exec(`INSERT INTO meta(key,value) VALUES('device_id','0190a1b2-c3d4-7000-8000-000000000000')`)
	raw.Exec(`INSERT INTO groups(id,name,color,sort_order,is_default,hlc,device_id,field_hlc) VALUES('g1','收件箱','',1,1,'1','d','{}')`)
	raw.Exec(`INSERT INTO items(id,title,group_id,created_at,updated_at,hlc,device_id) VALUES('i1','v1 时代的事项','g1',1,1,'1','d')`)
	raw.Close()

	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var v int
	s.db.QueryRow(`PRAGMA user_version`).Scan(&v)
	if v != CurrentSchemaVersion {
		t.Fatalf("版本应升级到 %d, got %d", CurrentSchemaVersion, v)
	}
	it, err := s.GetItem("i1")
	if err != nil || it.Title != "v1 时代的事项" {
		t.Fatal(it, err)
	}
	if _, err := s.AddSubtask("i1", "升级后新增的子任务"); err != nil {
		t.Fatal(err)
	}
	if b, _ := filepath.Glob(filepath.Join(dir, "backups", "premigrate-*.db")); len(b) != 1 {
		t.Fatalf("迁移前应自动备份: %v", b)
	}
	// 备份是升级前的 v1 数据库
	bk, _ := filepath.Glob(filepath.Join(dir, "backups", "premigrate-*.db"))
	old, _ := sql.Open("sqlite", "file:"+filepath.ToSlash(bk[0]))
	defer old.Close()
	var ov int
	old.QueryRow(`PRAGMA user_version`).Scan(&ov)
	if ov != 1 {
		t.Fatalf("备份应是升级前的版本, got %d", ov)
	}
}

// v2 → v3：windows 表新增 click_through 列，已有窗口记录保留，默认不穿透。
func TestMigrateV2ToV3ClickThrough(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.db")
	raw, _ := sql.Open("sqlite", "file:"+filepath.ToSlash(path))
	for i := 0; i < 2; i++ {
		if _, err := raw.Exec(migrations[i]); err != nil {
			t.Fatal(err)
		}
	}
	raw.Exec(`PRAGMA user_version = 2`)
	raw.Exec(`INSERT INTO meta(key,value) VALUES('device_id','0190a1b2-c3d4-7000-8000-000000000000')`)
	raw.Exec(`INSERT INTO groups(id,name,color,sort_order,is_default,hlc,device_id,field_hlc) VALUES('g1','收件箱','',1,1,'1','d','{}')`)
	raw.Exec(`INSERT INTO windows(id,group_id,mode,x,y,width,height,locked) VALUES('w1','g1','top',10,20,300,420,1)`)
	raw.Close()

	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	w, err := s.GetWindowByGroup("g1")
	if err != nil || w.Mode != "top" || w.X != 10 || !w.Locked || w.ClickThrough {
		t.Fatalf("升级后已有窗口记录应保留且默认不穿透: %+v %v", w, err)
	}
	w.ClickThrough = true
	s.SaveWindow(w)
	if got, _ := s.GetWindowByGroup("g1"); !got.ClickThrough {
		t.Fatal("click_through 应可保存")
	}
	if b, _ := filepath.Glob(filepath.Join(dir, "backups", "premigrate-*.db")); len(b) != 1 {
		t.Fatalf("迁移前应自动备份: %v", b)
	}
}
