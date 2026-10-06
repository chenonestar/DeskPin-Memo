; 桌面备忘钉 安装包（NFR-09）：无需管理员权限，安装到用户目录 %LOCALAPPDATA%\Programs\DeskPinMemo
; 用法（在仓库根目录）：makensis /DVERSION=1.0.0 installer\deskpin.nsi
Unicode true
!include "MUI2.nsh"

!ifndef VERSION
  !define VERSION "1.0.0"
!endif
!define APPNAME "桌面备忘钉"
!define EXE "DeskPinMemo.exe"
!define UNINSTKEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\DeskPinMemo"

Name "${APPNAME} DeskPin Memo"
OutFile "..\build\bin\DeskPinMemo-${VERSION}-setup.exe"
RequestExecutionLevel user
InstallDir "$LOCALAPPDATA\Programs\DeskPinMemo"
InstallDirRegKey HKCU "Software\DeskPinMemo" "InstallDir"
SetCompressor /SOLID lzma
BrandingText "${APPNAME} ${VERSION}"

!define MUI_ICON "..\build\windows\icon.ico"
!define MUI_UNICON "..\build\windows\icon.ico"
!define MUI_FINISHPAGE_RUN "$INSTDIR\${EXE}"
!define MUI_FINISHPAGE_RUN_TEXT "立即运行 ${APPNAME}"
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "SimpChinese"

VIProductVersion "${VERSION}.0"
VIAddVersionKey /LANG=2052 "ProductName" "${APPNAME}"
VIAddVersionKey /LANG=2052 "FileVersion" "${VERSION}"
VIAddVersionKey /LANG=2052 "LegalCopyright" "DeskPin Memo"
VIAddVersionKey /LANG=2052 "FileDescription" "${APPNAME} 安装程序"

Section "Install"
  ; 升级安装：先结束正在运行的实例
  nsExec::Exec 'taskkill /F /IM ${EXE}'
  SetOutPath "$INSTDIR"
  File "..\build\bin\${EXE}"
  WriteUninstaller "$INSTDIR\Uninstall.exe"
  CreateShortCut "$SMPROGRAMS\${APPNAME}.lnk" "$INSTDIR\${EXE}"
  WriteRegStr HKCU "Software\DeskPinMemo" "InstallDir" "$INSTDIR"
  WriteRegStr HKCU "${UNINSTKEY}" "DisplayName" "${APPNAME} DeskPin Memo"
  WriteRegStr HKCU "${UNINSTKEY}" "DisplayVersion" "${VERSION}"
  WriteRegStr HKCU "${UNINSTKEY}" "DisplayIcon" "$INSTDIR\${EXE}"
  WriteRegStr HKCU "${UNINSTKEY}" "UninstallString" '"$INSTDIR\Uninstall.exe"'
  WriteRegDWORD HKCU "${UNINSTKEY}" "NoModify" 1
  WriteRegDWORD HKCU "${UNINSTKEY}" "NoRepair" 1
SectionEnd

Section "Uninstall"
  nsExec::Exec 'taskkill /F /IM ${EXE}'
  Delete "$INSTDIR\${EXE}"
  Delete "$INSTDIR\Uninstall.exe"
  RMDir "$INSTDIR"
  Delete "$SMPROGRAMS\${APPNAME}.lnk"
  ; 清理程序自己写入 HKCU 的注册项（自启、通知身份、协议）；用户数据 %APPDATA%\DeskPinMemo 保留
  DeleteRegValue HKCU "Software\Microsoft\Windows\CurrentVersion\Run" "DeskPinMemo"
  DeleteRegKey HKCU "Software\Classes\AppUserModelId\DeskPinMemo.App"
  DeleteRegKey HKCU "Software\Classes\deskpin"
  ; Windows 在弹出通知后自动生成的通知设置记录
  DeleteRegKey HKCU "Software\Microsoft\Windows\CurrentVersion\Notifications\Settings\DeskPinMemo.App"
  ; Windows 11 为托盘图标生成的记录（NotifyIconSettings\<编号>，ExecutablePath 指向本程序）：只删除指向本程序的
  StrCpy $R0 0
  trayloop:
    EnumRegKey $R1 HKCU "Control Panel\NotifyIconSettings" $R0
    StrCmp $R1 "" traydone
    ReadRegStr $R2 HKCU "Control Panel\NotifyIconSettings\$R1" "ExecutablePath"
    StrCmp $R2 "$INSTDIR\${EXE}" 0 traynext
      DeleteRegKey HKCU "Control Panel\NotifyIconSettings\$R1"
      Goto trayloop ; 删除后同一索引上是下一个子项，不递增
    traynext:
      IntOp $R0 $R0 + 1
      Goto trayloop
  traydone:
  DeleteRegKey HKCU "${UNINSTKEY}"
  DeleteRegKey HKCU "Software\DeskPinMemo"
SectionEnd
