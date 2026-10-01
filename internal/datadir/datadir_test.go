package datadir

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveDefaultAndCustom(t *testing.T) {
	cfg := t.TempDir()
	if r := Resolve(cfg); r.Dir != cfg || r.Custom || r.Fallback != "" {
		t.Fatalf("无指针应使用默认目录: %+v", r)
	}
	target := filepath.Join(t.TempDir(), "sync", "DeskPin")
	if err := Set(cfg, target); err != nil {
		t.Fatal(err)
	}
	r := Resolve(cfg)
	if r.Dir != target || !r.Custom {
		t.Fatalf("%+v", r)
	}
	// 恢复默认：删除指针
	if err := Set(cfg, cfg); err != nil {
		t.Fatal(err)
	}
	if r := Resolve(cfg); r.Custom || r.Dir != cfg {
		t.Fatalf("%+v", r)
	}
	if _, err := os.Stat(filepath.Join(cfg, pointerFile)); !os.IsNotExist(err) {
		t.Fatal("恢复默认后应删除指针文件")
	}
}

func TestResolveFallsBackWhenUnavailable(t *testing.T) {
	cfg := t.TempDir()
	// 指向一个无法创建的位置：父路径是一个文件
	f := filepath.Join(t.TempDir(), "afile")
	os.WriteFile(f, []byte("x"), 0o644)
	if err := Set(cfg, filepath.Join(f, "sub")); err != nil {
		t.Fatal(err)
	}
	r := Resolve(cfg)
	if r.Dir != cfg || r.Custom || r.Fallback == "" {
		t.Fatalf("不可用的自定义目录应回退并给出原因: %+v", r)
	}
	// 损坏的指针文件也回退，不报错
	os.WriteFile(filepath.Join(cfg, pointerFile), []byte("{bad"), 0o644)
	if r := Resolve(cfg); r.Dir != cfg || r.Custom {
		t.Fatalf("%+v", r)
	}
}

func TestCheck(t *testing.T) {
	cfg, cur := t.TempDir(), t.TempDir()
	good := filepath.Join(t.TempDir(), "new")
	if i := Check(cfg, cur, good); !i.Valid || i.HasData || i.Error != "" {
		t.Fatalf("%+v", i)
	}
	for in, want := range map[string]string{
		"":              "请输入",
		"relative/path": "完整路径",
		cur:             "已经是当前",
	} {
		if i := Check(cfg, cur, in); i.Valid || !strings.Contains(i.Error, want) {
			t.Errorf("%q: %+v", in, i)
		}
	}
	// 目标是文件
	f := filepath.Join(t.TempDir(), "f")
	os.WriteFile(f, nil, 0o644)
	if i := Check(cfg, cur, f); i.Valid || !strings.Contains(i.Error, "文件") {
		t.Fatalf("%+v", i)
	}
	// 已有数据
	d := t.TempDir()
	os.WriteFile(filepath.Join(d, "data.db"), []byte("x"), 0o644)
	if i := Check(cfg, cur, d); !i.Valid || !i.HasData {
		t.Fatalf("%+v", i)
	}
	// 默认目录标记
	if i := Check(cfg, cur, cfg); !i.Valid || !i.IsDefault {
		t.Fatalf("%+v", i)
	}
}

func TestSyncFolderWarning(t *testing.T) {
	base := t.TempDir()
	for _, name := range []string{"OneDrive", "Dropbox", "坚果云", "Google Drive"} {
		i := Check(t.TempDir(), t.TempDir(), filepath.Join(base, name, "DeskPin"))
		if !i.Valid || i.Warning == "" {
			t.Errorf("%s 应有网盘提示: %+v", name, i)
		}
	}
	if i := Check(t.TempDir(), t.TempDir(), filepath.Join(base, "plain")); i.Warning != "" {
		t.Fatalf("普通目录不应有提示: %+v", i)
	}
}

func TestCopyFile(t *testing.T) {
	d := t.TempDir()
	src, dst := filepath.Join(d, "a"), filepath.Join(d, "b")
	os.WriteFile(src, []byte("hello"), 0o644)
	if err := CopyFile(src, dst); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(dst); string(b) != "hello" {
		t.Fatal(string(b))
	}
	if _, err := os.Stat(dst + ".tmp"); !os.IsNotExist(err) {
		t.Fatal("不应留下临时文件")
	}
}
