package logx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRotate(t *testing.T) {
	dir := t.TempDir()
	w, err := open(dir, 100)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	line := strings.Repeat("x", 40) + "\n"
	for i := 0; i < 60; i++ {
		w.Write([]byte(line))
	}
	files, _ := filepath.Glob(filepath.Join(dir, "app*.log"))
	if len(files) != MaxFiles {
		t.Fatalf("应保留 %d 个文件, got %d", MaxFiles, len(files))
	}
	for _, f := range files {
		if st, _ := os.Stat(f); st.Size() > 100 {
			t.Fatalf("%s 超过上限: %d", f, st.Size())
		}
	}
}
