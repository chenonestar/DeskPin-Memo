package store

import (
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
)

func TestFeedsCRUDAndItems(t *testing.T) {
	s := open(t)
	f, err := s.AddFeed("https://a.com/feed", "", true)
	if err != nil || !f.Enabled || !f.Preset {
		t.Fatal(f, err)
	}
	if _, err := s.AddFeed("https://a.com/feed", "", false); !errors.Is(err, ErrFeedExists) {
		t.Fatal("重复地址应报错", err)
	}
	// 首次拉取：写入条目，按 guid 去重
	n, err := s.SaveFeedItems(f.ID, []NewFeedItem{{GUID: "1", Title: "一", Link: "https://a.com/1", PublishedAt: 1000}, {GUID: "2", Title: "二", Link: "https://a.com/2"}})
	if err != nil || n != 2 {
		t.Fatal(n, err)
	}
	n, _ = s.SaveFeedItems(f.ID, []NewFeedItem{{GUID: "1", Title: "一（改）", Link: "https://a.com/1"}, {GUID: "3", Title: "三", Link: "https://a.com/3", PublishedAt: 2000}})
	if n != 1 {
		t.Fatalf("只应新增 1 条: %d", n)
	}
	items, _ := s.ListFeedItems("", 10)
	if len(items) != 3 || items[2].Title != "一" {
		t.Fatalf("按发布时间倒序，已有条目不被覆盖: %+v", items)
	}
	if s.UnreadFeedItems() != 3 {
		t.Fatal(s.UnreadFeedItems())
	}
	_ = s.MarkFeedItemRead(items[0].ID)
	if s.UnreadFeedItems() != 2 {
		t.Fatal("标记已读")
	}
	// 记录拉取结果：标题跟随源，用户改名后不再覆盖
	_ = s.RecordFetch(f.ID, FetchRecord{OK: true, Title: "源标题", ETag: "e1", NextFetchAt: 5})
	g, _ := s.GetFeed(f.ID)
	if g.Title != "源标题" || g.ETag != "e1" || g.LastOKAt == nil || g.FailCount != 0 {
		t.Fatalf("%+v", g)
	}
	name := "我的名字"
	_ = s.UpdateFeed(f.ID, &name, nil)
	_ = s.RecordFetch(f.ID, FetchRecord{OK: true, Title: "源标题2", NextFetchAt: 5})
	g, _ = s.GetFeed(f.ID)
	if g.Title != "我的名字" || !g.CustomTitle {
		t.Fatalf("用户改过的名字不应被覆盖: %+v", g)
	}
	_ = s.RecordFetch(f.ID, FetchRecord{Error: "超时", NextFetchAt: 9})
	_ = s.RecordFetch(f.ID, FetchRecord{Error: "超时", NextFetchAt: 9})
	g, _ = s.GetFeed(f.ID)
	if g.FailCount != 2 || g.LastError != "超时" || g.NextFetchAt != 9 {
		t.Fatalf("%+v", g)
	}
	// 停用的源不出现在新闻里，也不到期
	off := false
	_ = s.UpdateFeed(f.ID, nil, &off)
	if items, _ := s.ListFeedItems("", 10); len(items) != 0 {
		t.Fatal("停用的源不显示条目")
	}
	if due, _ := s.DueFeeds(1 << 50); len(due) != 0 {
		t.Fatal("停用的源不拉取")
	}
	list, _ := s.ListFeeds()
	if len(list) != 1 || list[0].ItemCount != 3 || list[0].Unread != 2 {
		t.Fatalf("%+v", list)
	}
	if err := s.DeleteFeed(f.ID); err != nil {
		t.Fatal(err)
	}
	var cnt int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM feed_items`).Scan(&cnt)
	if cnt != 0 {
		t.Fatal("删除源时一并删除条目")
	}
}

func TestPruneFeedItems(t *testing.T) {
	s := open(t)
	f, _ := s.AddFeed("https://a.com/feed", "A", false)
	var in []NewFeedItem
	for i := 1; i <= 120; i++ {
		in = append(in, NewFeedItem{GUID: fmt.Sprint(i), Title: "t", Link: "https://a.com", PublishedAt: int64(i) * 1000})
	}
	_, _ = s.SaveFeedItems(f.ID, in)
	if err := s.PruneFeedItems(100, 15_000); err != nil {
		t.Fatal(err)
	}
	items, _ := s.ListFeedItems(f.ID, 500)
	if len(items) != 100 || items[len(items)-1].PublishedAt != 21_000 {
		t.Fatalf("保留最新 100 条: %d %d", len(items), items[len(items)-1].PublishedAt)
	}
}

func TestPetAndExportRoundTrip(t *testing.T) {
	s := open(t)
	if _, ok, _ := s.GetPet(); ok {
		t.Fatal("初始没有宠物")
	}
	if _, ok, _ := s.AddPetProgress(10, 1, 0, 0); ok {
		t.Fatal("没有宠物时不做任何事")
	}
	_, _ = s.SavePet(Pet{Name: "眼镜"})
	p, ok, _ := s.AddPetProgress(10, 1, 2, 0)
	if !ok || p.Exp != 10 || p.Completed != 1 || p.Pats != 2 || p.Glasses != "round" {
		t.Fatalf("%+v", p)
	}
	p, _, _ = s.AddPetProgress(-50, -5, 0, 0)
	if p.Exp != 0 || p.Completed != 0 {
		t.Fatal("不会小于 0")
	}
	_, _ = s.AddFeed("https://a.com/feed", "A", true)
	d, err := s.Export()
	if err != nil || len(d.Feeds) != 1 || d.Pet == nil || d.Pet.Name != "眼镜" {
		t.Fatalf("%+v %v", d, err)
	}
	t2 := open(t)
	if _, err := t2.Import(mustParse(t, mustJSON(t, d)), ImportMerge); err != nil {
		t.Fatal(err)
	}
	fs, _ := t2.ListFeeds()
	pp, ok, _ := t2.GetPet()
	if len(fs) != 1 || fs[0].Title != "A" || !ok || pp.Name != "眼镜" {
		t.Fatalf("%+v %+v", fs, pp)
	}
	// 合并导入不覆盖本机已有的宠物
	_, _ = t2.SavePet(Pet{Name: "本机狗", Exp: 99, CreatedAt: pp.CreatedAt})
	_, _ = t2.Import(mustParse(t, mustJSON(t, d)), ImportMerge)
	if pp, _, _ := t2.GetPet(); pp.Name != "本机狗" {
		t.Fatal(pp.Name)
	}
	_, _ = t2.Import(mustParse(t, mustJSON(t, d)), ImportOverwrite)
	if pp, _, _ := t2.GetPet(); pp.Name != "眼镜" {
		t.Fatal("覆盖导入替换宠物", pp.Name)
	}
}

// 从 v5 升级：还叫旧默认名「眼镜」的狗狗自动改名为「博士」，已有的狗狗视为已起名；改过名的保持不变。
func TestMigrateV6RenamesDefaultPet(t *testing.T) {
	for _, tc := range []struct{ before, after string }{{"眼镜", "博士"}, {"旺财", "旺财"}} {
		db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(filepath.Join(t.TempDir(), "old.db")))
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 5; i++ {
			if _, err := db.Exec(migrations[i]); err != nil {
				t.Fatal(i, err)
			}
		}
		_, _ = db.Exec(`PRAGMA user_version = 5`)
		if _, err := db.Exec(`INSERT INTO pet(id,name,exp,created_at) VALUES(1,?,120,1)`, tc.before); err != nil {
			t.Fatal(err)
		}
		if err := migrate(db, nil); err != nil {
			t.Fatal(err)
		}
		s := &Store{db: db}
		p, ok, err := s.GetPet()
		if err != nil || !ok || p.Name != tc.after || !p.Named || p.Exp != 120 || p.Plays != 0 {
			t.Fatalf("%+v %v", p, err)
		}
		db.Close()
	}
}

func TestPetPlaysAndWater(t *testing.T) {
	s := open(t)
	_, _ = s.SavePet(Pet{Name: "博士"})
	p, _, _ := s.AddPetPlay()
	if p.Plays != 1 || p.Named {
		t.Fatalf("新建的狗狗默认未起名: %+v", p)
	}
	p, _, _ = s.AddPetWater("2026-11-16")
	p, _, _ = s.AddPetWater("2026-11-16")
	if p.WaterCount != 2 || p.WaterDate != "2026-11-16" {
		t.Fatal(p)
	}
	if p, _, _ = s.AddPetWater("2026-11-17"); p.WaterCount != 1 {
		t.Fatal("换了一天从 1 开始", p)
	}
}
