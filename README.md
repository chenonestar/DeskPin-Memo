# 桌面备忘钉（DeskPin Memo）

运行在 Windows 10/11（x64）上的单机桌面备忘 / 待办程序：**钉在桌面、一眼可见**，到点主动提醒，数据完全本地、离线。

需求依据：仓库中的《桌面备忘钉（DeskPin Memo）开发需求说明书.docx》。

## 已实现（V1）

| 领域 | 功能 |
| --- | --- |
| 事项 | 新建（标题 ≤200 字、备注、截止、提醒、分组、标签、优先级）、行内编辑、完成 / 撤销（3 秒过渡）、软删除 + 回收站（30 天自动清除）、FR-106 排序 + 拖拽手动排序、逾期标红置顶 |
| 快速输入 | 中文自然语言解析（`明天下午3点 交报告`、`周五 买菜`、`每月5日 09:00 交报销单`、`#标签 @分组 !高`），回车前预览；Tab 进入详细字段 |
| 桌面窗口 | 钉在桌面 / 置顶 / 普通三选一、位置尺寸记忆（多显示器、越界回主屏）、锁定、透明度、鼠标离开变淡、折叠、托盘、Win+D 后仍可见 |
| 提醒 | 一次性、截止前 N 分钟、周期（每天 / 工作日 / 每周 / 每月 / 每年 / 每 N 天，RRULE）、Toast 通知按钮（完成 / 10 分钟后 / 1 小时后 / 明天）、强提醒、睡眠唤醒后汇总补发、免打扰时段 |
| 检索 | 分组、标签、智能视图（今天 / 逾期 / 未来 7 天 / 无日期 / 已完成）、FTS5 trigram 中文全文搜索（1 万条 < 10 ms） |
| 数据 | SQLite（WAL，事务）、每日自动备份保留 14 份、导出 JSON / Markdown、导入（合并 / 覆盖）、可选字段级 AES-256-GCM 加密（Argon2id + DPAPI + 恢复密钥）、加密导出 |
| 鼠标穿透（FR-208） | 标题栏菜单 / 托盘菜单开启后便签只显示、不响应鼠标（点击落到下层窗口），按住 Ctrl 临时恢复交互；每个分组便签单独保存；设置 / 快速输入 / 概览 / 强提醒期间自动暂停；开启时提示恢复方式，标题栏有「穿透」标记 |
| 子任务（FR-108） | 事项下挂一层检查项：行内显示进度 `☑ x/y`，点进度展开，悬停「＋」或右键「添加子任务…」添加，回车连续录入，双击改名、勾选、删除、全部可撤销；周期事项完成后下一次带上子任务并重置；导入导出 / Markdown / 加密均覆盖；数据库升到 v2（升级前自动备份） |
| 每日概览（FR-308） | 每天首次启动弹出「逾期 + 今天到期」概览，可直接勾选完成；当天只认领一次，设置里可关；加密未解锁时不弹 |
| 数据目录（FR-605） | 设置 → 数据 → 更改目录：校验可写、网盘 / 网络位置提示、复制到新目录或使用目录里已有数据、重启后生效、可恢复默认；原数据不删除 |
| 同步预留 | `hlc` / `device_id` / `field_hlc` / `deleted` 从 V1 起建表并正确维护（`internal/hlc`，V3 直接复用） |

## 目录结构

```
main_windows.go      Wails 入口：生命周期、单实例、托盘 / 热键 / 调度器接线
internal/
  hlc/               混合逻辑时钟（AC-16）
  store/             SQLite 存储、FTS5、备份、导入导出、加密迁移
  service/           业务层：快速创建、周期事项、撤销栈、加密、导入导出
  scheduler/         提醒调度（单定时器、补发、免打扰）
  recur/ nlp/        RRULE 封装 · 中文时间解析
  datadir/           数据目录指针（location.json）、校验、网盘提示
  crypto/            AES-GCM、Argon2id、DPAPI
  app/               绑定给前端的 API + Shell 抽象
  winsys/            Win32：WorkerW/Progman 钉桌面、托盘、热键、Toast、自启（仅 Windows）
  logx/              滚动日志（≤5 MB × 5）
cmd/devserver/       在任意平台把业务层暴露为 HTTP，用于浏览器预览和 E2E
frontend/            React 18 + Vite + TypeScript + dnd-kit
tools/genicon/       生成应用 / 托盘图标
installer/           NSIS 安装脚本（免管理员）+ 绿色版说明
```

## 开发

```bash
go test ./...                      # 业务层全部单元测试（任意平台）
cd frontend && npm ci && npm test  # 前端单元测试
npm run build && npm run e2e       # 前端构建 + Playwright 端到端（真实 Go 后端 + SQLite）

# 在浏览器里预览界面（非 Windows 也可）：
go run ./cmd/devserver -seed       # http://127.0.0.1:8787
```

## 构建 Windows 发行包

需要 Windows、Go 1.26、Node 22、[Wails v2](https://wails.io)、NSIS：

```bash
wails build -clean -skipbindings -platform windows/amd64 -ldflags "-s -w -X main.version=1.0.0"
makensis /DVERSION=1.0.0 installer/deskpin.nsi     # 安装版（免管理员，装到 %LOCALAPPDATA%\Programs）
# 绿色版：DeskPinMemo.exe + 空文件 portable.flag（数据放 .\data）
```

CI（`.github/workflows/ci.yml`）在 Ubuntu 上跑全部测试与 E2E，在 Windows 上产出安装包和绿色版 zip。

## 绿色版与注册表

绿色版（exe 旁有 `portable.flag`）的数据放在 `.\data`，但**不是完全不碰系统**：Toast 通知必须有通知身份，所以仍会写当前用户注册表（HKCU，无需管理员权限）的三项——通知身份、`deskpin://` 协议、开机自启。为此：

* 绿色版**默认不开机自启**，需要时在设置里手动开启；
* 设置 → 数据 → **系统注册表项** 如实列出这些项，并提供「清除注册表项」（同时关闭自启设置），删除文件夹前使用；
* 程序目录被移动后，下次启动会自动把协议命令和失效的自启路径更新到新位置；但**不会抢走**另一个副本（例如已安装版）仍然有效的自启项，也不会误删它；
* 逻辑集中在 `internal/regtrace`（与真实注册表解耦，可在任意平台测试）。

## 已知限制与后续

* **多便签窗口（FR-206，P1）**：Wails v2 只支持单个窗口。当前用一个窗口 + 标题栏分组切换实现，每个分组各自保存位置 / 尺寸 / 模式 / 颜色（`windows` 表已按分组建模）。真正的多窗口需要 Wails v3（目前仍是 beta）或多进程方案，留待评估。
* 快速输入框、设置窗口以「临时变形的同一窗口」实现（记住原位置与模式，关闭后恢复）。
* **钉在桌面（FR-201）** 通过把窗口 `SetParent` 到承载图标层 `SHELLDLL_DefView` 的桌面宿主（Progman / WorkerW）实现，并监听 `TaskbarCreated` 在资源管理器重启后重挂；失败时降级为「置底窗口」。这是未公开行为，**必须在 Win10 22H2 与 Win11 上实机验证**（需求书 M0 / AC-02 / AC-03）。本仓库的 Windows 专有代码只在 Linux 上做过交叉编译，没有实机运行过。
* V3 同步与移动端 PWA（第 11 章）未做。至此需求书 V1 的 P0 / P1 / P2 功能均已实现（FR-206 多便签窗口受 Wails v2 单窗口限制除外）。
* **鼠标穿透**为 `WS_EX_LAYERED|WS_EX_TRANSPARENT`，同时施加到主窗口及 WebView2 的全部子窗口，关闭时按记录的原样式还原。按住 Ctrl 的检测用键盘原始输入（`RIDEV_INPUTSINK`，WM_INPUT）事件驱动，没有轮询；功能关闭时不注册，零开销。**这是整个项目里最依赖真机行为的功能之一**：WebView2 子窗口是否接受分层样式、与「钉在桌面」的 `SetParent` 组合后点击是否确实落到桌面图标，都必须在 Win10 22H2 / Win11 上实测。若 WebView2 在分层子窗口下出现空白，需要改为只对顶层窗口施加样式或改用 `WM_NCHITTEST` 方案。
* **子任务**的约定：完成子任务不会自动完成父事项，完成父事项也不改动子任务；子任务标题暂不进入全文搜索（需求书 FR-404 只要求标题和备注）；子任务暂不支持拖拽排序（后端 `ReorderSubtasks` 已就绪）。
* **数据目录**：日志、WebView 缓存、图标和指针文件 `location.json` 始终留在 `%APPDATA%\DeskPinMemo`（不应被网盘同步），只有 `data.db` 与 `backups\` 跟随自定义目录；自定义目录下数据库改用 SQLite 回滚日志（DELETE）模式，避免 `-wal/-shm` 被同步工具拷到不一致状态。自定义目录不可用时回退到默认目录并在界面顶部提示。绿色版不支持更改。切换后的重启由一个隐藏的 `cmd` 延迟拉起新进程完成（需要 Windows 实机验证）。
* **每日概览**在 Wails v2 单窗口下以叠加界面实现（与快速输入框、设置窗口同一机制）。
* 自然语言时间解析用 Go 实现（需求书写的是前端 chrono-node），便于与业务层一起测试。
