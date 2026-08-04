#!/bin/sh
# v2rayG 安装后:刷新菜单/图标缓存,并为安装者创建桌面快捷方式。
# 菜单缓存:KDE 需要 kbuildsycoca6(update-desktop-database 只管 GTK)。
if command -v kbuildsycoca6 >/dev/null 2>&1; then
  kbuildsycoca6 --noincremental >/dev/null 2>&1 || true
fi
update-desktop-database /usr/share/applications >/dev/null 2>&1 || true
gtk-update-icon-cache -f /usr/share/icons/hicolor >/dev/null 2>&1 || true

# 桌面快捷方式:优先 SUDO_USER,其次 logname,再退回当前登录用户。
INSTALLER="${SUDO_USER:-}"
if [ -z "$INSTALLER" ]; then
  INSTALLER=$(logname 2>/dev/null || true)
fi
if [ -n "$INSTALLER" ]; then
  UHOME=$(getent passwd "$INSTALLER" 2>/dev/null | cut -d: -f6)
  if [ -n "$UHOME" ] && [ -d "$UHOME" ]; then
    for D in "$UHOME/Desktop" "$UHOME/桌面"; do
      if [ -d "$D" ]; then
        cp -f /usr/share/applications/v2ray-gui.desktop "$D/v2ray-gui.desktop"
        chown "$INSTALLER" "$D/v2ray-gui.desktop" 2>/dev/null || true
        chmod +x "$D/v2ray-gui.desktop"
        break
      fi
    done
  fi
fi
exit 0
