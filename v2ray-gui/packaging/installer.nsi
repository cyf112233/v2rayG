; v2rayG NSIS 安装脚本
;
; 用法(由 GitHub Actions 调用,参数通过 -D 传入):
;   makensis -DVERSION=1.0.0 -DAPP_ROOT=<repo根(绝对路径)> -DOUTDIR=<release目录(绝对路径)> installer.nsi
;
; 默认值便于本地单独编译(相对本脚本所在目录:packaging/ → 仓库根)。

Unicode true

!ifndef VERSION
  !define VERSION "0.0.0"
!endif
!ifndef APP_ROOT
  !define APP_ROOT "..\.."
!endif
!ifndef OUTDIR
  !define OUTDIR "..\..\release"
!endif

!define APP_NAME "v2rayG"
!define APP_EXE "v2ray-gui.exe"
!define APP_PUBLISHER "v2rayG"
!define APP_UNINST_KEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\v2rayG"

Name "${APP_NAME} ${VERSION}"
OutFile "${OUTDIR}\v2rayG-Setup-${VERSION}.exe"
InstallDir "$PROGRAMFILES64\v2rayG"
InstallDirRegKey HKLM "${APP_UNINST_KEY}" "InstallLocation"
RequestExecutionLevel admin

Page directory
Page instfiles
UninstPage uninstConfirm
UninstPage instfiles

Section "MainSection" SEC01
  SetOutPath "$INSTDIR"
  File "${APP_ROOT}\v2ray-gui\v2ray-gui.exe"
  File "${APP_ROOT}\v2ray-gui\README.md"
  File "${APP_ROOT}\v2ray-gui\LICENSE"

  ; 卸载器
  WriteUninstaller "$INSTDIR\uninstall.exe"

  ; 开始菜单快捷方式
  CreateDirectory "$SMPROGRAMS\${APP_NAME}"
  CreateShortcut "$SMPROGRAMS\${APP_NAME}\${APP_NAME}.lnk" "$INSTDIR\${APP_EXE}"
  CreateShortcut "$SMPROGRAMS\${APP_NAME}\Uninstall ${APP_NAME}.lnk" "$INSTDIR\uninstall.exe"

  ; 桌面快捷方式
  CreateShortcut "$DESKTOP\${APP_NAME}.lnk" "$INSTDIR\${APP_EXE}"

  ; 卸载注册表项(HKLM Uninstall\v2rayG)
  WriteRegStr HKLM "${APP_UNINST_KEY}" "DisplayName" "${APP_NAME}"
  WriteRegStr HKLM "${APP_UNINST_KEY}" "DisplayVersion" "${VERSION}"
  WriteRegStr HKLM "${APP_UNINST_KEY}" "UninstallString" '"$INSTDIR\uninstall.exe"'
  WriteRegStr HKLM "${APP_UNINST_KEY}" "Publisher" "${APP_PUBLISHER}"
  WriteRegStr HKLM "${APP_UNINST_KEY}" "InstallLocation" "$INSTDIR"
  WriteRegStr HKLM "${APP_UNINST_KEY}" "DisplayIcon" "$INSTDIR\${APP_EXE}"
  WriteRegDWORD HKLM "${APP_UNINST_KEY}" "NoModify" 1
  WriteRegDWORD HKLM "${APP_UNINST_KEY}" "NoRepair" 1
SectionEnd

Section "Uninstall"
  Delete "$INSTDIR\${APP_EXE}"
  Delete "$INSTDIR\README.md"
  Delete "$INSTDIR\LICENSE"
  Delete "$INSTDIR\uninstall.exe"
  RMDir "$INSTDIR"

  Delete "$DESKTOP\${APP_NAME}.lnk"
  Delete "$SMPROGRAMS\${APP_NAME}\${APP_NAME}.lnk"
  Delete "$SMPROGRAMS\${APP_NAME}\Uninstall ${APP_NAME}.lnk"
  RMDir "$SMPROGRAMS\${APP_NAME}"

  DeleteRegKey HKLM "${APP_UNINST_KEY}"
SectionEnd
