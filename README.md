# SSH Workbench (`sshw`)

开箱即用的 Web SSH / SFTP 工作台。Go 单进程内嵌 React 19 / TypeScript / xterm.js / Monaco 界面，零运行时依赖，支持直接读取本机 `~/.ssh/config` 一键直连，命令行使用简短的 `sshw` 启动。

## 一键安装与升级

### macOS / Linux 一键安装（与升级）

```sh
curl -fsSL https://raw.githubusercontent.com/xxddccaa/ssh-workbench/main/install.sh | sh
```

已安装后，可随时直接升级到 GitHub 最新 Release：

```sh
sshw upgrade
```

### Windows

从 [GitHub Releases](https://github.com/xxddccaa/ssh-workbench/releases/latest) 下载 `sshw_*_windows_amd64.zip`，解压后直接运行 `sshw.exe`，或使用内置升级命令：

```powershell
.\sshw.exe upgrade
```

## 快速使用

```sh
# 本机启动（默认后台运行并自动打开浏览器，如 7331 被非 sshw 占用则自动顺延端口）
sshw

# 查看后台状态 / 停止后台服务 / 前台运行
sshw status
sshw stop
sshw --foreground

# 局域网 / 服务器部署（监听所有网卡，HTTP 直接访问，不弹出登录拦截）
sshw --listen 0.0.0.0:7331

# 如需后台无浏览器启动
sshw --listen 0.0.0.0:7331 --no-browser

# 在本地终端中打开某个文本文件到浏览器 Monaco 编辑器
sshw edit ./app.conf
```

### 核心特性

- **开箱直连**：默认直接打开工作区，支持 `localhost`、`127.0.0.1`、局域网 IP 与反代 HTTP 直接访问。如需对外暴露并加密码/HTTPS，可按需启用 `--auth` 或 `--remote --origin https://... --tls-cert ... --tls-key ...`。
- **内置本地终端（macOS / Linux）**：支持在工作区直接新建并分屏使用本机默认 Shell（PTY）。
- **完整兼容 Mac / Linux `~/.ssh/config`**：
  - 侧边栏与快速启动台自动列出 `~/.ssh/config` 主机（支持 `Include` 与外部软链接，如 OrbStack、Colima、dotfiles），点击即连。
  - 支持 `ProxyCommand`（如 `cloudflared`、`aws ssm`、`ssh -W`）与 `ProxyJump` 单级跳板。
  - 按 OpenSSH 优先级自动匹配认证：配置私钥列表 → `~/.ssh/id_ed25519` / `id_ecdsa` / `id_rsa` → `SSH_AUTH_SOCK` agent → 密码 / MFA 交互提示。
  - 支持 `~/.ssh/known_hosts` 自动匹配与首次连接自动信任（TOFU），仅在密钥冲突时告警。
- **高效终端与工作区**：
  - 紧凑单行顶栏 + 可拖拽标签页 + 单屏 / 左右 / 上下 / 四宫格分屏。
  - 原生 UTF-8 二进制流与输入法组合保护，不干扰中文/IME 输入。
  - 右侧 SFTP 文件面板默认收起、按需展开，支持目录树、拖拽上传文件夹、打包 zip 下载与内置 Monaco 编辑器。
  - 支持 `-L` 本地端口转发隧道、命令片段库与多主机整条命令广播、可选加密凭据库。

## 从源码构建与发版

开发与构建需要 Go 1.26+ 和 Node 24+（运行编译产物无需 Node）：

```sh
make install   # 安装前后端依赖
make run       # 构建并启动
make test      # 契约漂移检查、Go -race / vet、TypeScript 与 Vitest 单测
make e2e       # Playwright 浏览器端到端验收
make build     # 输出单可执行文件 bin/sshw
make dist      # 交叉编译 macOS/Linux/Windows 五平台归档与 SHA256SUMS
```

### GitHub Actions 自动编译与发布 Release

推送 `v*` 标签即可触发 [`.github/workflows/release.yml`](.github/workflows/release.yml)，自动运行测试并交叉编译 `darwin/arm64`、`darwin/amd64`、`linux/amd64`、`linux/arm64`、`windows/amd64` 产物及 `SHA256SUMS` 发布到 GitHub Releases：

```sh
git tag v0.3.0
git push origin v0.3.0
```
