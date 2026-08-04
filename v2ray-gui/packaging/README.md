# v2rayG 发行包说明

由 GitHub Actions 自动构建。推送 `v*` 格式 tag(如 `v1.2.3`)触发,产物发布到 GitHub Release。每个便携 zip 内均附带 README.md 与 LICENSE。

## 产物清单

| 产物 | 平台 | 架构 |
| --- | --- | --- |
| `v2ray-gui_<ver>_linux_amd64.zip` | Linux 便携包 | amd64 |
| `v2ray-gui_<ver>_linux_arm64.zip` | Linux 便携包 | arm64 |
| `v2ray-gui_<ver>_linux_arm.zip` | Linux 便携包 | arm(v7) |
| `v2ray-gui_<ver>_linux_386.zip` | Linux 便携包 | 386 |
| `v2ray-gui_<ver>_windows_amd64.zip` | Windows 便携包 | amd64 |
| `v2ray-gui_<ver>_windows_arm64.zip` | Windows 便携包 | arm64 |
| `v2ray-gui_<ver>_windows_386.zip` | Windows 便携包 | 386 |
| `v2ray-gui_<ver>_amd64.deb` | Debian/Ubuntu | amd64 |
| `v2ray-gui_<ver>_arm64.deb` | Debian/Ubuntu | arm64 |
| `v2ray-gui-<ver>.x86_64.rpm` | Fedora/RHEL/openSUSE | amd64 |
| `v2ray-gui-<ver>.aarch64.rpm` | Fedora/RHEL/openSUSE | arm64 |

`<ver>` 为去掉前缀 `v` 的版本号(如 tag `v1.2.3` → `1.2.3`)。

## 安装

### deb(Debian / Ubuntu / Linux Mint 等)

```bash
sudo apt install ./v2ray-gui_<ver>_amd64.deb
```

### rpm(Fedora / RHEL / openSUSE 等)

```bash
sudo dnf install ./v2ray-gui-<ver>.x86_64.rpm
```

deb/rpm 安装后自动注册桌面入口 `/usr/share/applications/v2ray-gui.desktop` 与图标,可从应用菜单启动 v2rayG。

### Linux 便携 zip

```bash
unzip v2ray-gui_<ver>_linux_amd64.zip -d v2ray-gui
./v2ray-gui/v2ray-gui
```

### Windows 便携 zip

解压后运行 `v2ray-gui.exe` 即可,无需安装。

## 说明

- 核心 v2ray-core 以源码内嵌,运行时无需额外下载二进制。
- 首次运行可能需要网络权限;Linux 下 TUN 模式会通过 `pkexec` 请求 root 权限。
