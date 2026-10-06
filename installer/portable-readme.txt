桌面备忘钉（DeskPin Memo）绿色版
================================
双击 DeskPinMemo.exe 即可运行，无需安装、无需管理员权限。

- 本目录下存在 portable.flag 时，所有数据保存在 .\data 子目录（数据库、备份、日志），
  可整个文件夹拷走。删除 portable.flag 则改用 %APPDATA%\DeskPinMemo。
- 需要 Microsoft Edge WebView2 运行时（Windows 10 21H2 及以后通常已预装，
  缺失时程序会提示并引导安装）。
- 本程序会在【当前用户】注册表（HKCU，不需要管理员权限）写入三项：
    1) 通知身份  HKCU\Software\Classes\AppUserModelId\DeskPinMemo.App  （没有它 Win10 不显示通知）
    2) 协议      HKCU\Software\Classes\deskpin                          （通知上的「完成 / 稍后」按钮）
    3) 开机自启  HKCU\Software\Microsoft\Windows\CurrentVersion\Run\DeskPinMemo
  绿色版默认【不】开机自启，需要时在 设置 → 常规 里手动开启。
- 搬动文件夹后：下次手动运行会自动把上面记录的位置更新为新位置，通知按钮和（已开启的）自启随之恢复。
- 不再使用、要删除文件夹之前：请先在 设置 → 数据 → 系统注册表项 点「清除注册表项」，
  再点「退出程序」，然后才删除文件夹，这样不会在注册表里留下残留。
