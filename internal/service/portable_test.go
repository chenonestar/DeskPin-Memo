package service

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"deskpinmemo/internal/logx"
	"deskpinmemo/internal/store"
)

// 绿色版审计：模拟一次完整使用（含备份、导出导入、加密开关、大批量事项排序/搜索），
// 断言程序没有往「数据目录」以外的地方写任何文件——系统临时目录、用户目录、当前目录都必须保持为空——
// 并且正常关闭后数据目录里不残留 -wal / -shm / -journal 伴生文件。
func TestPortableLeavesNoFilesOutsideItsFolder(t *testing.T) {
	root := t.TempDir()
	exeDir := filepath.Join(root, "Tools", "DeskPinMemo")
	dataDir := filepath.Join(exeDir, "data")
	os.MkdirAll(exeDir, 0o755)
	os.WriteFile(filepath.Join(exeDir, "portable.flag"), nil, 0o644)

	// 把所有「系统位置」指向空目录哨兵，之后检查它们仍然是空的
	sentinels := map[string]string{}
	for _, name := range []string{"TMPDIR", "TEMP", "TMP", "HOME", "USERPROFILE", "APPDATA", "LOCALAPPDATA", "XDG_CONFIG_HOME", "XDG_CACHE_HOME", "XDG_DATA_HOME"} {
		d := filepath.Join(root, "sentinel-"+name)
		os.MkdirAll(d, 0o755)
		t.Setenv(name, d)
		sentinels[name] = d
	}
	cwd := filepath.Join(root, "cwd")
	os.MkdirAll(cwd, 0o755)
	old, _ := os.Getwd()
	os.Chdir(cwd)
	defer os.Chdir(old)

	lw, err := logx.Open(filepath.Join(dataDir, "logs"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(filepath.Join(dataDir, "data.db"), store.Options{JournalMode: "DELETE"}) // 绿色版的配置
	if err != nil {
		t.Fatal(err)
	}
	svc, err := New(st, dataDir, Options{Portable: true})
	if err != nil {
		t.Fatal(err)
	}
	lw.Write([]byte("started\n"))

	// 大批量写入 + 排序 + 搜索（SQLite 可能在这里使用临时文件）
	now := time.Now()
	for i := 0; i < 600; i++ {
		due := now.Add(time.Duration(i-300) * time.Minute).UnixMilli()
		it, err := svc.CreateItem(store.NewItem{Title: "批量事项 " + strings.Repeat("字", i%30), DueAt: &due, Tags: []string{"t" + string(rune('a'+i%5))}})
		if err != nil {
			t.Fatal(err)
		}
		if i%100 == 0 {
			svc.AddSubtask(it.ID, "子任务")
		}
	}
	inbox, _ := st.InboxID()
	if _, err := svc.GroupContent(inbox); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Search("批量"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Overview(); err != nil {
		t.Fatal(err)
	}
	if err := svc.Housekeeping(); err != nil { // 每日备份
		t.Fatal(err)
	}
	// 导出 / 导入（用户选择保存位置；这里放进数据目录）
	exp := filepath.Join(dataDir, "export.json")
	if err := svc.ExportJSON(exp, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ImportJSON(exp, "", true); err != nil { // 覆盖导入会先备份
		t.Fatal(err)
	}
	if err := svc.ExportMarkdown(filepath.Join(dataDir, "list.md")); err != nil {
		t.Fatal(err)
	}
	// 加密开 / 关（含 VACUUM 清除残留明文）
	if _, err := svc.EnableEncryption("secret-1", UnlockPassword); err != nil {
		t.Fatal(err)
	}
	if err := svc.DisableEncryption("secret-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.BackupNow(); err != nil {
		t.Fatal(err)
	}
	lw.Close()
	st.Close() // 正常退出

	// 1) 系统位置必须保持为空
	for name, dir := range sentinels {
		entries, _ := os.ReadDir(dir)
		if len(entries) != 0 {
			var names []string
			for _, e := range entries {
				names = append(names, e.Name())
			}
			t.Errorf("%s (%s) 里出现了程序写入的文件: %v", name, dir, names)
		}
	}
	if entries, _ := os.ReadDir(cwd); len(entries) != 0 {
		t.Errorf("当前工作目录里出现了文件: %v", entries)
	}
	// 2) 程序文件夹里除了 portable.flag 和 data 之外不应出现任何东西
	top, _ := os.ReadDir(exeDir)
	for _, e := range top {
		if e.Name() != "portable.flag" && e.Name() != "data" {
			t.Errorf("程序文件夹顶层出现了多余内容: %s", e.Name())
		}
	}
	// 3) 数据目录里没有 WAL 伴生文件 / 回滚日志 / 临时文件残留
	filepath.WalkDir(exeDir, func(p string, d fs.DirEntry, err error) error {
		for _, bad := range []string{"-wal", "-shm", "-journal", ".tmp"} {
			if strings.HasSuffix(d.Name(), bad) {
				t.Errorf("残留文件: %s", p)
			}
		}
		return nil
	})
	if _, err := os.Stat(filepath.Join(dataDir, "data.db")); err != nil {
		t.Fatal("数据库应在数据目录内")
	}
}
