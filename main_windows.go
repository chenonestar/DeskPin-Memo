//go:build windows

package main

import (
	"context"
	"embed"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"deskpinmemo/internal/app"
	"deskpinmemo/internal/logx"
	"deskpinmemo/internal/scheduler"
	"deskpinmemo/internal/service"
	"deskpinmemo/internal/store"
	"deskpinmemo/internal/winsys"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed all:frontend/dist
var assets embed.FS

// windowTitle 是窗口唯一标题，用于在进程内定位 HWND。
const windowTitle = "DeskPinMemo-Sticky"

func main() {
	exe, _ := os.Executable()
	dataDir := resolveDataDir(exe)
	if lw, err := logx.Open(filepath.Join(dataDir, "logs")); err == nil {
		log.SetOutput(lw)
		defer lw.Close()
	}
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.Printf("启动 DeskPin Memo %s，数据目录 %s", version, dataDir)

	st, err := store.Open(filepath.Join(dataDir, "data.db"))
	if err != nil {
		log.Printf("打开数据库失败: %v", err)
		fatalDialog("无法打开数据库：" + err.Error())
		return
	}
	defer st.Close()
	svc, err := service.New(st, dataDir)
	if err != nil {
		log.Printf("初始化服务失败: %v", err)
		fatalDialog("初始化失败：" + err.Error())
		return
	}
	a := app.New(svc, &app.NopShell{}, version)
	d := &daemon{app: a, svc: svc, exe: exe, dataDir: dataDir, silent: hasArg("--autostart")}

	err = wails.Run(&options.App{
		Title:             windowTitle,
		Width:             app.DefaultW,
		Height:            app.DefaultH,
		DisableResize:     true, // 缩放由前端缩放柄 + Go 层完成
		Frameless:         true,
		StartHidden:       true,
		HideWindowOnClose: true,
		BackgroundColour:  &options.RGBA{R: 0, G: 0, B: 0, A: 0},
		AssetServer:       &assetserver.Options{Assets: assets},
		Bind:              []interface{}{a},
		OnStartup:         d.startup,
		OnDomReady:        d.domReady,
		OnShutdown:        d.shutdown,
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId:               "9b6f3c1e-0d55-4d0a-9d0e-DeskPinMemo",
			OnSecondInstanceLaunch: d.secondInstance, // NFR-07：重复启动时激活已有实例
		},
		Windows: &windows.Options{
			WebviewIsTransparent: true,
			WindowIsTranslucent:  false,
			DisableWindowIcon:    true,
			WebviewUserDataPath:  filepath.Join(dataDir, "webview"),
		},
	})
	if err != nil {
		log.Printf("运行失败: %v", err)
	}
}

// daemon 把 Wails 生命周期与 Win32 外壳、调度器、服务连接起来。
type daemon struct {
	app     *app.App
	svc     *service.Service
	exe     string
	dataDir string
	silent  bool // --autostart 启动

	ctx    context.Context
	cancel context.CancelFunc
	shell  *winsys.Shell
	loop   *winsys.Loop
	sched  *scheduler.Scheduler
	once   sync.Once
}

func (d *daemon) startup(ctx context.Context) {
	d.ctx, d.cancel = context.WithCancel(ctx)
	emit := func(ev string, data any) { runtime.EventsEmit(ctx, ev, data) }
	d.app.Emit = emit
	d.svc.Emit = emit
}

func (d *daemon) domReady(ctx context.Context) {
	d.once.Do(func() { d.init(ctx) }) // DomReady 在页面刷新时会重复触发，初始化只做一次
}

func (d *daemon) init(ctx context.Context) {
	h, err := winsys.FindWindow(windowTitle, 5*time.Second)
	if err != nil {
		log.Printf("定位窗口失败: %v", err)
		return
	}
	sh := winsys.NewShell(func() { runtime.Quit(ctx) })
	sh.Attach(h)
	d.shell = sh
	d.app.SetShell(sh)

	if err := winsys.RegisterIdentity(d.exe, d.dataDir); err != nil {
		log.Printf("注册 AUMID/协议失败: %v", err)
	}
	settings := d.svc.GetSettings()
	d.applySettings(settings)

	loop, err := winsys.StartLoop(winsys.Callbacks{
		OnQuick:      d.app.OpenQuick,
		OnToggle:     func() { d.app.ToggleVisible() },
		OnResume:     func() { log.Print("系统唤醒：重新计算提醒"); d.sched.Kick(); sh.Repin() },
		OnTimeChange: func() { log.Print("系统时间变更：重新计算提醒"); d.sched.Kick() },
		OnTaskbar:    func() { log.Print("资源管理器重启：重新钉在桌面"); sh.Repin() },
		OnDisplay:    func() { sh.SetBounds(sh.Bounds()) },
		OnMenu:       d.menu,
		OnEndSession: func() { d.app.Quit() },
	}, d.dataDir)
	if err != nil {
		log.Printf("启动消息循环失败: %v", err)
	} else {
		d.loop = loop
		sh.SetLoop(loop)
		d.registerHotkeys(settings)
	}

	// 通知与调度
	toaster := &winsys.Toaster{IconPath: filepath.Join(d.dataDir, "icons", "notify.png")}
	d.svc.Kick = func() {
		if d.sched != nil {
			d.sched.Kick()
		}
	}
	d.svc.OnOverdue = sh.SetOverdueBadge
	d.svc.OnSettings = d.applySettings
	d.sched = scheduler.New(scheduler.Config{
		Source:        d.svc.Store(),
		Notifier:      notifier{toaster: toaster, app: d.app},
		DND:           d.svc.DND,
		StrongEnabled: func() bool { return d.svc.GetSettings().StrongReminder },
		OnFired:       func() { d.svc.Emit("data:changed", nil); sh.SetOverdueBadge(d.svc.Store().CountOverdue()) },
	})

	// 加密：DPAPI 自动解锁；否则前端显示解锁界面
	d.svc.TryAutoUnlock()
	if err := d.svc.Housekeeping(); err != nil {
		log.Printf("启动维护失败: %v", err)
	}
	if _, err := d.app.ApplyWindow(); err != nil {
		log.Printf("恢复窗口失败: %v", err)
	}
	sh.SetVisible(true) // 开机自启（--autostart）与手动启动一样静默显示便签（SW_SHOWNOACTIVATE，不抢焦点）
	sh.SetOverdueBadge(d.svc.Store().CountOverdue())
	go d.sched.Run(d.ctx)

	// 冷启动时由协议链接（通知按钮）拉起：处理命令行参数
	for _, arg := range os.Args[1:] {
		if strings.HasPrefix(strings.Trim(arg, `"`), "deskpin://") {
			_ = d.app.HandleProtocolURL(arg)
		}
	}
}

func (d *daemon) applySettings(s service.Settings) {
	if err := winsys.SetAutostart(s.Autostart, d.exe); err != nil {
		log.Printf("设置开机自启失败: %v", err)
	}
	if d.loop != nil {
		d.registerHotkeys(s)
	}
}

func (d *daemon) registerHotkeys(s service.Settings) {
	if conflicts := d.loop.RegisterHotkeys(s.HotkeyQuick, s.HotkeyToggle); len(conflicts) > 0 {
		log.Printf("快捷键冲突: %v", conflicts)
		d.svc.Emit("hotkey:conflict", conflicts) // 冲突时提示（FR-501）
	}
}

func (d *daemon) menu(cmd int) {
	switch cmd {
	case winsys.CmdNew:
		d.app.OpenQuick()
	case winsys.CmdToggle:
		d.app.ToggleVisible()
	case winsys.CmdSettings:
		d.app.OpenSettings()
	case winsys.CmdQuit:
		d.app.Quit()
	}
}

func (d *daemon) secondInstance(data options.SecondInstanceData) {
	handled := false
	for _, arg := range data.Args {
		if strings.HasPrefix(strings.Trim(arg, `"`), "deskpin://") {
			if err := d.app.HandleProtocolURL(arg); err != nil {
				log.Printf("处理通知操作失败: %v", err)
			}
			handled = true
		}
	}
	if !handled && d.shell != nil { // 重复启动：激活已有实例
		d.shell.SetVisible(true)
		d.shell.Focus()
	}
}

func (d *daemon) shutdown(ctx context.Context) {
	if d.cancel != nil {
		d.cancel()
	}
	if d.loop != nil {
		d.loop.Close()
	}
}

// notifier 组合系统 Toast 与强提醒窗口。
type notifier struct {
	toaster *winsys.Toaster
	app     *app.App
}

func (n notifier) Notify(nt scheduler.Notification) error {
	if nt.Strong {
		n.app.ShowStrongAlert(nt)
	}
	return n.toaster.Notify(nt)
}

// resolveDataDir：绿色版（exe 旁存在 portable.flag）使用 exe 目录下的 data；
// 否则 %APPDATA%\DeskPinMemo；环境变量 DESKPIN_DATA 优先（便于测试）。
func resolveDataDir(exe string) string {
	if v := os.Getenv("DESKPIN_DATA"); v != "" {
		return v
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(exe), "portable.flag")); err == nil {
		return filepath.Join(filepath.Dir(exe), "data")
	}
	base := os.Getenv("APPDATA")
	if base == "" {
		base, _ = os.UserConfigDir()
	}
	return filepath.Join(base, "DeskPinMemo")
}

func hasArg(a string) bool {
	for _, x := range os.Args[1:] {
		if x == a {
			return true
		}
	}
	return false
}
