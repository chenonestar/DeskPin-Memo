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
	"deskpinmemo/internal/datadir"
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
	if os.Getenv("DESKPIN_ALLOW_TEMP") == "" && datadir.RunningFromTempOrArchive(exe, os.TempDir()) {
		// 在压缩包里直接双击 exe：旁边没有 portable.flag，数据和注册表项会散落到系统目录
		fatalDialog("检测到程序正在压缩包或系统临时目录中运行：\n" + filepath.Dir(exe) +
			"\n\n请先把整个压缩包完整解压到一个固定的文件夹（例如 D:\\Tools\\DeskPinMemo），再运行其中的 DeskPinMemo.exe。")
		return
	}
	cfgDir, portable := resolveConfigDir(exe)
	if portable {
		if err := datadir.EnsureWritable(cfgDir); err != nil {
			fatalDialog("绿色版需要在程序所在文件夹里创建 data 子目录保存数据，但该位置不可写：\n" + cfgDir +
				"\n\n原因：" + err.Error() + "\n\n请把程序文件夹复制到可写的位置（如 D:\\Tools）后再运行。")
			return
		}
	}
	// 数据目录（data.db 与 backups）可自定义（FR-605）；日志、WebView 缓存、图标留在配置目录
	res := datadir.Resolution{Dir: cfgDir}
	if !portable {
		res = datadir.Resolve(cfgDir)
	}
	dataDir := res.Dir
	if lw, err := logx.Open(filepath.Join(cfgDir, "logs")); err == nil {
		log.SetOutput(lw)
		defer lw.Close()
	}
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.Printf("启动 DeskPin Memo %s，配置目录 %s，数据目录 %s", version, cfgDir, dataDir)
	if res.Fallback != "" {
		log.Print(res.Fallback)
	}

	opts := store.Options{}
	if res.Custom || portable { // 自定义（可能是网盘）目录、绿色版（可能在 U 盘上）：用单文件回滚日志模式，没有 -wal/-shm 伴生文件，
		// 文件夹整体拷走更安全
		opts.JournalMode = "DELETE"
	}
	st, err := store.Open(filepath.Join(dataDir, "data.db"), opts)
	if err != nil {
		log.Printf("打开数据库失败: %v", err)
		fatalDialog("无法打开数据库：" + err.Error())
		return
	}
	defer st.Close()
	svc, err := service.New(st, dataDir, service.Options{ConfigDir: cfgDir, Portable: portable, Fallback: res.Fallback})
	if err != nil {
		log.Printf("初始化服务失败: %v", err)
		fatalDialog("初始化失败：" + err.Error())
		return
	}
	a := app.New(svc, &app.NopShell{}, version)
	d := &daemon{app: a, svc: svc, exe: exe, dataDir: cfgDir, silent: hasArg("--autostart")}

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
			WebviewUserDataPath:  filepath.Join(cfgDir, "webview"),
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
	dataDir string // 配置目录（图标等）；数据库位置由 service 管理
	silent  bool   // --autostart 启动

	autostartKnown bool // 是否已在启动时同步过自启项
	lastAutostart  bool

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
		OnQuick:        d.app.OpenQuick,
		OnToggle:       func() { d.app.ToggleVisible() },
		OnResume:       func() { log.Print("系统唤醒：重新计算提醒"); d.sched.Kick(); sh.Repin() },
		OnTimeChange:   func() { log.Print("系统时间变更：重新计算提醒"); d.sched.Kick() },
		OnTaskbar:      func() { log.Print("资源管理器重启：重新钉在桌面"); sh.Repin() },
		OnDisplay:      func() { sh.SetBounds(sh.Bounds()) },
		OnMenu:         d.menu,
		OnEndSession:   func() { d.app.Quit() },
		OnCtrl:         sh.OnCtrl,
		ClickThroughOn: d.app.ClickThroughEnabled,
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
	d.app.MaybeShowOverview() // FR-308：当天首次启动时弹出今日概览

	// 冷启动时由协议链接（通知按钮）拉起：处理命令行参数
	for _, arg := range os.Args[1:] {
		if strings.HasPrefix(strings.Trim(arg, `"`), "deskpin://") {
			_ = d.app.HandleProtocolURL(arg)
		}
	}
}

func (d *daemon) applySettings(s service.Settings) {
	switch {
	case !d.autostartKnown:
		// 启动时：让注册表与设置一致。程序目录被移动后，这里会把失效的自启路径更新到当前位置；
		// 但不会动别的副本（如已安装版）留下的有效条目。
		if msg, err := winsys.SyncAutostart(d.exe, s.Autostart); err != nil {
			log.Printf("同步开机自启失败: %v", err)
		} else if msg != "" {
			log.Print(msg)
		}
	case s.Autostart != d.lastAutostart:
		// 用户在设置里明确切换了开机自启：强制写入 / 删除
		if err := winsys.SetAutostart(s.Autostart, d.exe); err != nil {
			log.Printf("设置开机自启失败: %v", err)
		}
	} // 其他设置（主题、字号…）被修改时不碰注册表
	d.autostartKnown, d.lastAutostart = true, s.Autostart
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
	case winsys.CmdClickThrough:
		_, _ = d.app.ToggleClickThrough()
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

// resolveConfigDir 返回配置目录以及是否为绿色版：绿色版（exe 旁存在 portable.flag）使用 exe 目录下的 data，
// 且不允许改数据目录；否则 %APPDATA%\DeskPinMemo；环境变量 DESKPIN_DATA 优先（便于测试）。
func resolveConfigDir(exe string) (string, bool) {
	if v := os.Getenv("DESKPIN_DATA"); v != "" {
		return v, false
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(exe), "portable.flag")); err == nil {
		return filepath.Join(filepath.Dir(exe), "data"), true
	}
	base := os.Getenv("APPDATA")
	if base == "" {
		base, _ = os.UserConfigDir()
	}
	return filepath.Join(base, "DeskPinMemo"), false
}

func hasArg(a string) bool {
	for _, x := range os.Args[1:] {
		if x == a {
			return true
		}
	}
	return false
}
