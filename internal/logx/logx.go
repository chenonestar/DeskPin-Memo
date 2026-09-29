// Package logx 提供本地滚动日志（NFR-12）：单文件 ≤ 5 MB，保留 5 个，绝不记录事项内容。
package logx

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

const (
	MaxSize  = 5 << 20
	MaxFiles = 5
)

// Writer 实现 io.Writer，写满后滚动：app.log → app.1.log → … → app.4.log。
type Writer struct {
	mu   sync.Mutex
	dir  string
	f    *os.File
	size int64
	max  int64
}

// Open 打开日志目录下的 app.log。
func Open(dir string) (*Writer, error) { return open(dir, MaxSize) }

func open(dir string, max int64) (*Writer, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	w := &Writer{dir: dir, max: max}
	return w, w.openFile()
}

func (w *Writer) path(i int) string {
	if i == 0 {
		return filepath.Join(w.dir, "app.log")
	}
	return filepath.Join(w.dir, fmt.Sprintf("app.%d.log", i))
}

func (w *Writer) openFile() error {
	f, err := os.OpenFile(w.path(0), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return err
	}
	w.f, w.size = f, st.Size()
	return nil
}

func (w *Writer) rotate() error {
	w.f.Close()
	_ = os.Remove(w.path(MaxFiles - 1))
	for i := MaxFiles - 2; i >= 0; i-- {
		if _, err := os.Stat(w.path(i)); err == nil {
			if err := os.Rename(w.path(i), w.path(i+1)); err != nil {
				return err
			}
		}
	}
	return w.openFile()
}

// Write 写入日志，必要时滚动。
func (w *Writer) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.size+int64(len(p)) > w.max {
		if err := w.rotate(); err != nil {
			return 0, err
		}
	}
	n, err := w.f.Write(p)
	w.size += int64(n)
	return n, err
}

// Close 关闭文件。
func (w *Writer) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.f.Close()
}
