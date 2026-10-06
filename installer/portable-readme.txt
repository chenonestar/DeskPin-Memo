桌面备忘钉（DeskPin Memo）绿色版
================================
双击 DeskPinMemo.exe 即可运行，无需安装、无需管理员权限。

- 本目录下存在 portable.flag 时，所有数据保存在 .\data 子目录（数据库、备份、日志），
  可整个文件夹拷走。删除 portable.flag 则改用 %APPDATA%\DeskPinMemo。
- 需要 Microsoft Edge WebView2 运行时（Windows 10 21H2 及以后通常已预装，
  缺失时程序会提示并引导安装）。
- 请先把压缩包【完整解压】到一个固定文件夹再运行（如 D:\Tools\DeskPinMemo）。直接在压缩包里双击
  exe 时，程序会检测到并提示你先解压——否则数据和注册表项会散落到系统目录，就不算绿色版了。
- 数据库使用单文件模式，正常退出后文件夹里只有 data.db，没有 -wal / -shm 之类的伴生文件，
  整个文件夹可以直接拷到另一台电脑或 U 盘。程序所在位置必须可写（不能放在只读光盘 / 只读共享上）。
- 本程序会在【当前用户】注册表（HKCU，不需要管理员权限）写入三项：
    1) 通知身份  HKCU\Software\Classes\AppUserModelId\DeskPinMemo.App  （没有它 Win10 不显示通知）
    2) 协议      HKCU\Software\Classes\deskpin                          （通知上的「完成 / 稍后」按钮）
    3) 开机自启  HKCU\Software\Microsoft\Windows\CurrentVersion\Run\DeskPinMemo
    4) 通知设置记录（Windows 在首次弹出通知后自动生成）
    5) Windows 11 的托盘图标记录 HKCU\Control Panel\NotifyIconSettings\<编号>（Windows 自动生成）
  绿色版默认【不】开机自启，需要时在 设置 → 常规 里手动开启。
- 搬动文件夹后：下次手动运行会自动把上面记录的位置更新为新位置，通知按钮和（已开启的）自启随之恢复。
- 不再使用、要删除文件夹之前：请先在 设置 → 数据 → 系统注册表项 点「清除注册表项」，
  再点「退出程序」，然后才删除文件夹，这样不会在注册表里留下残留。

- 关于「退出」：关闭窗口只是隐藏到托盘，进程仍在运行并占用文件夹。要删除文件夹请先在托盘图标右键菜单里选「退出」，
  或在上面的「清除注册表项」对话框里点「退出程序」。

仍会留下、但程序无法清除的系统自动记录（任何 Windows 程序运行后都会有，与是否绿色版无关）：
  - 系统的程序执行记录：Prefetch、UserAssist、BAM、AppCompat（兼容性助手）、MUI 缓存
  - 通过「导出 / 导入」文件对话框使用过的文件，会出现在「最近使用的文件」和文件对话框的历史记录里
  - 发送通知时会调用 PowerShell，Windows 会在 %LOCALAPPDATA%\Microsoft\Windows\PowerShell 下维护它自己的缓存
  这些不含你的事项内容，也不由本程序写入。
