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
wails build -clean -platform windows/amd64 -ldflags "-s -w -X main.version=1.0.0"
makensis /DVERSION=1.0.0 installer/deskpin.nsi     # 安装版（免管理员，装到 %LOCALAPPDATA%\Programs）
# 绿色版：DeskPinMemo.exe + 空文件 portable.flag（数据放 .\data）
```

CI（`.github/workflows/ci.yml`）在 Ubuntu 上跑全部测试与 E2E，在 Windows 上产出安装包和绿色版 zip。

## 已知限制与后续

* **多便签窗口（FR-206，P1）**：Wails v2 只支持单个窗口。当前用一个窗口 + 标题栏分组切换实现，每个分组各自保存位置 / 尺寸 / 模式 / 颜色（`windows` 表已按分组建模）。真正的多窗口需要 Wails v3（目前仍是 beta）或多进程方案，留待评估。
* 快速输入框、设置窗口以「临时变形的同一窗口」实现（记住原位置与模式，关闭后恢复）。
* **钉在桌面（FR-201）** 通过把窗口 `SetParent` 到承载图标层 `SHELLDLL_DefView` 的桌面宿主（Progman / WorkerW）实现，并监听 `TaskbarCreated` 在资源管理器重启后重挂；失败时降级为「置底窗口」。这是未公开行为，**必须在 Win10 22H2 与 Win11 上实机验证**（需求书 M0 / AC-02 / AC-03）。本仓库的 Windows 专有代码只在 Linux 上做过交叉编译，没有实机运行过。
* 每日概览（FR-308）、子任务（FR-108）、鼠标穿透（FR-208）、自定义数据目录（FR-605）为 P2，未做；V3 同步与移动端 PWA（第 11 章）未做。
* 自然语言时间解析用 Go 实现（需求书写的是前端 chrono-node），便于与业务层一起测试。
