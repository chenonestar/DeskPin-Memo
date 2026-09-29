// devserver 在任意平台上把业务层以 HTTP 暴露出来，用于在浏览器中预览/调试前端，
// 也用于端到端测试（Playwright）。前端在没有 Wails 运行时的情况下自动改用这里的 /rpc 接口。
//
//	go run ./cmd/devserver -addr :8787 -seed
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"time"

	"deskpinmemo/internal/app"
	"deskpinmemo/internal/scheduler"
	"deskpinmemo/internal/service"
	"deskpinmemo/internal/store"
)

type hub struct {
	mu   sync.Mutex
	subs map[chan []byte]struct{}
}

func (h *hub) emit(event string, data any) {
	b, _ := json.Marshal(map[string]any{"event": event, "data": data})
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- b:
		default:
		}
	}
}

type devNotifier struct{ h *hub }

func (n devNotifier) Notify(nt scheduler.Notification) error {
	n.h.emit("dev:notify", nt)
	return nil
}

func main() {
	addr := flag.String("addr", "127.0.0.1:8787", "监听地址")
	dir := flag.String("data", "", "数据目录（默认临时目录）")
	static := flag.String("static", "frontend/dist", "前端构建产物目录")
	seed := flag.Bool("seed", false, "写入演示数据")
	flag.Parse()

	if *dir == "" {
		d, err := os.MkdirTemp("", "deskpin-dev-")
		if err != nil {
			log.Fatal(err)
		}
		*dir = d
	}
	st, err := store.Open(filepath.Join(*dir, "data.db"))
	if err != nil {
		log.Fatal(err)
	}
	defer st.Close()
	svc, err := service.New(st, *dir)
	if err != nil {
		log.Fatal(err)
	}
	h := &hub{subs: map[chan []byte]struct{}{}}
	a := app.New(svc, &app.NopShell{R: app.Rect{X: 100, Y: 100, W: 300, H: 420}, Mode: "desktop"}, "dev")
	svc.Emit, a.Emit = h.emit, h.emit
	sched := scheduler.New(scheduler.Config{Source: st, Notifier: devNotifier{h}, DND: svc.DND,
		StrongEnabled: func() bool { return svc.GetSettings().StrongReminder },
		OnFired:       func() { h.emit("data:changed", nil) }})
	svc.Kick = sched.Kick
	go sched.Run(context.Background())
	if *seed {
		seedData(svc)
	}

	mux := http.NewServeMux()
	rv := reflect.ValueOf(a)
	errType := reflect.TypeOf((*error)(nil)).Elem()
	mux.HandleFunc("/rpc/", func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Path[len("/rpc/"):]
		m := rv.MethodByName(name)
		if !m.IsValid() {
			http.Error(w, jsonErr("未知方法 "+name), http.StatusNotFound)
			return
		}
		body, _ := io.ReadAll(r.Body)
		var raw []json.RawMessage
		if len(body) > 0 {
			if err := json.Unmarshal(body, &raw); err != nil {
				http.Error(w, jsonErr("参数必须是 JSON 数组"), http.StatusBadRequest)
				return
			}
		}
		mt := m.Type()
		if len(raw) != mt.NumIn() {
			http.Error(w, jsonErr(fmt.Sprintf("%s 需要 %d 个参数，收到 %d", name, mt.NumIn(), len(raw))), http.StatusBadRequest)
			return
		}
		args := make([]reflect.Value, mt.NumIn())
		for i := range args {
			p := reflect.New(mt.In(i))
			if err := json.Unmarshal(raw[i], p.Interface()); err != nil {
				http.Error(w, jsonErr(fmt.Sprintf("参数 %d 无效: %v", i, err)), http.StatusBadRequest)
				return
			}
			args[i] = p.Elem()
		}
		out := m.Call(args)
		var result any
		for _, o := range out {
			if o.Type().Implements(errType) {
				if !o.IsNil() {
					http.Error(w, jsonErr(o.Interface().(error).Error()), http.StatusInternalServerError)
					return
				}
				continue
			}
			result = o.Interface()
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"result": result})
	})
	mux.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
		fl, ok := w.(http.Flusher)
		if !ok {
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		ch := make(chan []byte, 16)
		h.mu.Lock()
		h.subs[ch] = struct{}{}
		h.mu.Unlock()
		defer func() { h.mu.Lock(); delete(h.subs, ch); h.mu.Unlock() }()
		fmt.Fprint(w, ": ok\n\n")
		fl.Flush()
		for {
			select {
			case b := <-ch:
				fmt.Fprintf(w, "data: %s\n\n", b)
				fl.Flush()
			case <-r.Context().Done():
				return
			}
		}
	})
	mux.Handle("/", http.FileServer(http.Dir(*static)))
	log.Printf("devserver: http://%s （数据目录 %s）", *addr, *dir)
	log.Fatal(http.ListenAndServe(*addr, mux))
}

func jsonErr(msg string) string {
	b, _ := json.Marshal(map[string]string{"error": msg})
	return string(b)
}

func seedData(svc *service.Service) {
	now := time.Now()
	ms := func(d time.Duration) *int64 { v := now.Add(d).UnixMilli(); return &v }
	hi := 2
	svc.CreateItem(store.NewItem{Title: "回复 HR 邮件", DueAt: ms(-2 * time.Hour), Priority: &hi,
		Reminders: []store.ReminderInput{{RemindAt: ms(-2 * time.Hour)}}})
	svc.CreateItem(store.NewItem{Title: "下午 3 点前提交周报", DueAt: ms(3 * time.Hour), Tags: []string{"工作"},
		Reminders: []store.ReminderInput{{OffsetMinutes: 15}}})
	svc.CreateItem(store.NewItem{Title: "买牛奶和鸡蛋", DueAt: ms(30 * time.Hour)})
	svc.CreateItem(store.NewItem{Title: "整理桌面文件", Note: "把下载目录里的安装包清一下"})
	d, _ := svc.CreateItem(store.NewItem{Title: "预约体检"})
	svc.Toggle(d.ID, true)
	svc.CreateGroup("生活", "#34a853")
}
