桌面备忘钉（DeskPin Memo）绿色版
================================
双击 DeskPinMemo.exe 即可运行，无需安装、无需管理员权限。

- 本目录下存在 portable.flag 时，所有数据保存在 .\data 子目录（数据库、备份、日志），
  可整个文件夹拷走。删除 portable.flag 则改用 %APPDATA%\DeskPinMemo。
- 需要 Microsoft Edge WebView2 运行时（Windows 10 21H2 及以后通常已预装，
  缺失时程序会提示并引导安装）。
- 开机自启、通知等设置会写入当前用户注册表（HKCU），不需要管理员权限。
