# 架构与拟实施目录

本文保留初始架构目标，不代表所有模块均已实现。当前代码和阶段边界见 `docs/05-current-implementation.md`；REST 源文件实际采用 `api/openapi.json`。

## 1. 架构决策

| 决策 | 选择 | 理由 |
| --- | --- | --- |
| 运行形态 | 模块化单体、一个 Go 进程 | 单用户工具不需要跨服务部署与一致性成本 |
| 前端 | React 19、TypeScript strict、Vite、Tailwind | 遵循已选栈，界面与原生 WebView 解耦 |
| 终端 | xterm.js + 官方匹配版本 addons | 不自写 VT 解析器或 IME 输入栈 |
| 后端 HTTP | Go `net/http`，显式路由和中间件 | 避免为少量路由引入大型框架 |
| 通信 | 同源 REST + WebSocket | 命令/元数据与高频字节流分开 |
| SSH/SFTP | `golang.org/x/crypto/ssh`、`github.com/pkg/sftp` | 使用成熟协议实现；仍需自己设计 host key 与生命周期 |
| WebSocket | `github.com/coder/websocket` | 使用 context 生命周期；应用层仍需限流和权限校验 |
| 存储 | SQLite，纯 Go 驱动候选 `modernc.org/sqlite` | 本机单用户、事务与迁移；三平台 CGO=0 发布需实测 |
| 前端状态 | Query 管请求数据；轻量 store 管布局 | 终端字节和 xterm 实例不进入 React state/store |
| 契约 | OpenAPI → Go/TS 类型；WS 独立版本化规范 | 避免手写两份 DTO 以及 Wails 生成绑定耦合 |
| 分发 | 前端构建产物经 `go:embed` 内嵌 | 运行时不需要 Node、Vite 或 Docker |

实现时固定 Go/Node 工具链、依赖版本和锁文件，用目标平台 CI 验证，而不是随手使用 `latest`。xterm 的主分支快照只用于阅读，正式依赖选择经过验证的发行版；当前查询到的正式版记录在 `evidence/xterm-release.json`。

不建立面向所有未来协议的巨大统一终端接口。接口定义在消费者模块，只抽象真实存在的 SSH、存储和系统边界。

## 2. 工程目录

```text
ssh-workbench/
├── cmd/
│   └── sshworkbench/
│       └── main.go
├── internal/
│   ├── bootstrap/
│   │   ├── application.go
│   │   └── shutdown.go
│   ├── config/
│   ├── transport/
│   │   └── httpapi/
│   │       ├── router.go
│   │       ├── middleware/
│   │       ├── handlers/
│   │       ├── ws/
│   │       └── generated/
│   ├── auth/
│   ├── credentials/
│   ├── hosts/
│   ├── connections/
│   ├── terminals/
│   │   ├── service.go
│   │   ├── session.go
│   │   ├── registry.go
│   │   ├── stream.go
│   │   └── ports.go
│   ├── files/
│   ├── transfers/
│   ├── tunnels/
│   ├── workspaces/
│   └── adapters/
│       ├── sshtransport/
│       ├── sftpfs/
│       ├── localdocuments/
│       ├── sqlite/
│       │   └── migrations/
│       ├── keyring/
│       └── platform/
├── web/
│   ├── embed.go
│   ├── package.json
│   ├── package-lock.json
│   ├── vite.config.ts
│   ├── dist/
│   └── src/
│       ├── app/
│       │   ├── router.tsx
│       │   ├── providers.tsx
│       │   └── workbench.tsx
│       ├── features/
│       │   ├── hosts/
│       │   ├── terminal/
│       │   │   ├── controller/
│       │   │   ├── model/
│       │   │   └── ui/
│       │   ├── files/
│       │   ├── editor/
│       │   ├── transfers/
│       │   ├── tunnels/
│       │   ├── snippets/
│       │   ├── workspace/
│       │   └── settings/
│       └── shared/
│           ├── api/generated/
│           ├── protocol/
│           ├── ui/
│           ├── styles/
│           └── lib/
├── api/
│   ├── openapi.yaml
│   └── terminal-protocol.md
├── tests/
│   ├── integration/
│   ├── e2e/
│   ├── fixtures/
│   └── manual/
├── scripts/
├── docs/
├── .github/workflows/
├── go.mod
└── go.sum
```

树是模块成长方向，不要求第一天创建所有空目录。先实现能连接一个主机的纵向切片，新增能力时才加入相应模块。单元测试贴近源码；`tests/` 只放跨模块场景和真实环境记录。

### 各层职责

| 区域 | 负责 | 禁止 |
| --- | --- | --- |
| `cmd` | 解析 CLI、选择启动/打开/编辑命令 | 连接 SSH、拼 SQL、处理 HTTP 业务 |
| `bootstrap` | 构造依赖、启动和优雅停止 | 堆积业务逻辑、成为新的巨型 App |
| `httpapi` | 认证入口、参数校验、DTO、错误映射、连接升级 | 直接持有数据库或 `ssh.Client` 实现业务 |
| `hosts` / `credentials` / `connections` | 主机配置、凭据句柄、拨号策略与身份检查 | 返回明文私钥给普通查询接口 |
| `terminals` | 终端状态机、订阅、流控与关闭 | 引用 HTTP request 或 React 视图状态 |
| `files` / `transfers` | 文件语义、编辑版本、任务状态与取消 | 用 HTTP 请求寿命代替独立任务寿命 |
| `tunnels` | 转发配置、监听、计数和资源释放 | 随标签页 React 卸载隐式退出 |
| `adapters` | SSH/SFTP/数据库/系统具体实现 | 反向依赖 HTTP/UI 或业务全局单例 |
| 前端 `controller` | xterm、socket、订阅、resize、dispose | 把每一块终端输出推入 React state |
| 前端 `ui` | 展示、交互意图、可访问性 | 自己创建第二套 SSH/WS 生命周期 |

`bootstrap → transport/业务/适配器`；`transport → 业务`；适配器实现业务定义的窄接口。`connections` 可以使用主机与凭据服务；`terminals/files/tunnels` 依赖连接能力，不反向被连接模块引用。`transfers` 使用文件接口，文件模块不反向依赖任务 UI。禁止跨 feature 深入导入私有实现与循环依赖。

## 3. 核心模型与资源所有权

| 模型 | 寿命 | 含义 |
| --- | --- | --- |
| `HostProfile` | 持久化 | 地址、端口、身份引用、跳板、终端偏好 |
| `CredentialRef` | 持久化句柄 | 指向加密凭据，不是前端可读取的秘密 |
| `ConnectionLease` | 运行期 | 一个用途明确的 SSH transport 与跳板资源 |
| `TerminalSession` | 运行期 | SSH channel、PTY、流缓存、订阅和退出状态 |
| `TransferJob` | 运行期 + 状态记录 | 有界任务队列、进度、取消；不等于浏览器组件 |
| `TunnelSpec/Runtime` | 配置持久化、运行期分离 | 端口转发计划与实际监听状态 |
| `WorkspaceLayout` | 持久化 | 标签和分屏布局，不保存存活 SSH 对象 |

- V1 不设计跨主机、跨用途的全局连接池。交互终端、批量 SFTP 传输和隧道按用途独立租用 transport，避免大文件传输挤占终端输出。
- 可以在单个文件能力内复用经过验证的 SFTP client；所有租约有单一拥有者和幂等关闭。将来优化池化要先有测量数据。
- 每个 terminal/transfer/tunnel 有自己的 context、关闭路径与失败状态；应用 context 是它们的共同上级。
- HTTP 创建请求结束不应直接取消终端；WebSocket 断开也不等于立即结束 SSH。显式停止、保留期结束、注销与应用退出有不同策略。
- 关闭 SSH channel、底层连接或 deadline 负责解除阻塞读；仅取消 context 不假定能中断任意 `io.Read`。
- 一个终端只允许一个可写浏览器租约。第二个窗口请求显式接管，旧窗口变为不可写，避免两个窗口重复输入。

```text
created → connecting → awaiting-trust/auth → running
                 └──→ failed
running → detached → running
    │         └──→ closing → closed
    └────────────→ closing → closed
running → exited
```

初始建议：同页 socket 断线保留 60 秒；断线缓存每终端最多 8 MiB，全局另有总上限；终端数量默认上限 8。都是待压测的有界配置，不是无限保活承诺。

整页刷新恢复布局，不自动把新建 xterm 接上环形缓存中间并声称屏幕完整恢复。原会话到期清理，有明确提示与提前关闭入口。后端崩溃/SSH 断开后，不承诺远端 shell 持续可恢复。

## 4. REST 与流协议

### REST 负责有边界的操作

拟议资源：`/api/v1/hosts`、`/terminals`、`/files`、`/documents`、`/transfers`、`/tunnels`、`/workspaces`。文件二进制上传下载走 HTTP 流，不塞进终端 WebSocket，也不先把整个大文件读成浏览器 Blob。

创建和取消任务具备去重键及明确状态。终端命令发送是有副作用操作，不能像普通 GET 那样自动重试；广播也不承诺跨目标事务或 exactly-once 执行结果。

错误包含稳定 `code`、可翻译信息、`requestId` 和可选字段错误；底层错误只进脱敏诊断，不把密码、密钥、连接串或完整终端输出写日志。

### 每个终端一条 WebSocket

- 控制消息是版本化 JSON：`hello/attach/resize/ack/state/error/close`。
- 输出采用二进制 envelope：1 字节类型 + 8 字节输出字节偏移 + 原始 payload。偏移使用 unsigned 64-bit，JSON 表示为十进制字符串，避免 JS Number 精度歧义。
- 输入采用独立二进制类型。文本 `onData` 只 UTF-8 编码一次；xterm `onBinary` 的 bytestring 按单字节映射，不能再次 UTF-8 编码。
- UTF-8/ANSI 数据在 Go 侧不经过 JSON 字符串、base64 或逐块 `string()` 清洗。前端去掉 envelope 后按顺序调用 `xterm.write(Uint8Array, callback)`。
- `resize` 校验列/行范围，隐藏面板不发送 0×0；通过 ResizeObserver 与帧节流减少抖动。
- 服务端输出顺序经过一个有界队列与单一 socket writer；stdout/stderr 保留各自顺序，不宣称重建远端两条流的绝对发生时间。

### 背压与重连

- ACK 在 xterm 的写入解析回调后发送，不在 socket `onmessage` 刚收到时就确认消费。
- 建议起始窗口为 256 KiB、单块 payload ≤32 KiB；达到未确认上限时暂停上游读取，不能无限 `append`。
- 同页重连前等待已排队解析完成，携带最后已应用偏移。重连代际和偏移用于避免旧 socket 回调与重放数据造成重复。
- 缓存不覆盖未确认输出。达到限额后施加背压；保留期结束再关闭。界面提示进程可能因终端未消费而暂时阻塞。
- 不重放未确认的键盘输入：断线时不接受新的终端输入，重连后提示发送结果不确定的情况。
- 心跳、消息大小、写入超时、每终端内存和全局内存均有界。日志/传输状态使用另一条低频事件连接，防止抢占终端字节流。

协议冻结前必须用切包、丢 ACK、重连、慢消费者和非法消息测试，不因这里列出了字段就宣称协议已验证。

## 5. 中文与终端渲染

中文问题分四层定位：

1. **IME 事件**：由 xterm 和浏览器处理 composition。快捷键遇到 `isComposing`、组合输入或非终端焦点时让行；不另建文本框转发一套输入，不搬旧 WKWebView 补丁。
2. **字节与编码**：同一个 xterm 解码器连续接收原始 Uint8Array；测试 UTF-8 每一个字节位置切分。首版远端终端契约为 UTF-8，不静默猜测 GBK。
3. **字宽与字体**：匹配字体 fallback、字形测量、Unicode 宽度与远端程序行为；混合中英文、emoji、组合字符、全角符号要单独验收。不能只增加字体就声称解决编码。
4. **PTY 与远端环境**：请求 `xterm-256color`，在实际尺寸变化时 resize。远端不支持的 locale 只能提示诊断，不强写可能不存在的 `LC_ALL=en_US.UTF-8`。

WebGL 是可选优化，有明确关闭与 DOM renderer 回退；不混用不匹配的旧 canvas addon。字体加载完成再 fit，主题/缩放变化不重建 SSH 会话。xterm 的创建、挂载、订阅、dispose 全由 controller 统一负责，React StrictMode 重挂载也不能产生双重输入订阅。

## 6. 文件与任务边界

- 远端 SFTP 路径与本机路径使用不同类型/处理器，不能在 Windows Go 后端用 `filepath.Join` 随意拼 Unix 远端路径。
- 中文文件名完整保留；遇到非 UTF-8 文件名字节，首版明确标记不支持编辑/重命名等操作，不经 JSON 替换字符后继续破坏性操作。
- 上传相对路径检查 `..`、绝对路径、分隔符、NUL 与重复名字；文件夹上传和打包下载有数量、总量与深度上限。
- 服务端临时文件有明确根目录、大小配额和清理生命周期；本机文件授权检查符号链接与 TOCTOU 风险，不能只用字符串前缀判断路径归属。
- 文件删除、覆盖与目录递归操作展示目标；首次实现不默认跟随符号链接递归。
- 传输并发拟设为 2。关闭终端不取消传输；显式取消或注销取消。浏览器上传源在关闭页面后不可继续读，任务必须报告中断，不伪装成服务端持久任务。
- 后端重启后将运行中的传输标为 interrupted；首版重试从头开始，不把持久化任务表误称为断点续传。
- 下载文件名正确编码 `Content-Disposition`，不将用户路径直接拼入 header；直接下载流避免内存随文件大小增长。

## 7. 认证与安全模式

### 本机模式

- 只绑定 `127.0.0.1`，不隐式绑定 `::` 或所有网卡。先创建 listener，再打开浏览器，避免先探测空闲端口再关闭重开的竞争窗口。
- 启动时生成短期、一次性 bootstrap capability，通过 URL fragment 交给页面；页面立即清除 fragment，用 POST 换取 HttpOnly、SameSite=Strict 的会话 Cookie。禁止长效令牌放 query/localStorage。
- 校验 Host 和精确 Origin；写接口有 CSRF 防护，WebSocket 升级同样验证认证和 Origin。不能使用 `CheckOrigin=true`。
- 本机 HTTP Cookie 不假装具备远程 TLS 的安全属性；Cookie 不按端口隔离。额外依赖短期会话、应用特定命名/路径、Origin/Host 检查；不承诺防住已控制本机当前用户的恶意进程。
- CLI 文件注册等高权限操作走仅当前 OS 用户可访问的 Unix socket / Windows named pipe，不向浏览器提供任意本机路径授权接口。

### 远程模式

- 监听非 loopback 或显式 remote 时进入远程策略；缺少初始化的所有者凭据或 TLS 配置则拒绝启动，不能静默退回裸 HTTP。
- V1 优先支持 Go 直接终止 TLS。受信任反代模式只能在显式配置、受控上游地址与部署检查下启用，不能信任任意 `X-Forwarded-Proto/Host`。
- 初始化所有者必须通过本机控制入口完成；不向公网暴露无认证“抢占第一个管理员”页面。
- 密码以 Argon2id 等经评审配置存储，认证限速；使用可吊销的服务端会话、Secure/HttpOnly/SameSite Cookie，而非不可撤销的长期 JWT。
- 注销/锁定会吊销会话，关闭活动 WS、终端和隧道并取消传输，锁定凭据；改密与会话到期也处理已有长连接，不能仅阻止下一次 HTTP 请求。
- 远程模式禁用任意本机文件浏览/编辑、浏览器触发本地 shell 和配置导入执行指令。访问本机 agent 或服务端私钥由所有者明确配置，浏览器不能任意选择本机路径。

### 凭据、主机身份与内容

- 凭据默认只在当前会话内使用。保存时使用独立随机数据密钥加密；密钥由系统凭据存储保护，不同数据库一起明文落盘。
- Linux 无可用系统 keyring/无头运行时，要求显式主密码解锁加密库；不偷偷降级为 plaintext 或把解密密钥和库放一起冒充安全。
- 浏览器只拿到凭据引用/标签。登录密码与 SSH 私钥/主密码职责分开；重置登录密码不应自动绕过加密库。
- 未知 host key 要确认；变更默认阻断；读取/解析信任存储失败则失败关闭。每级跳板各验证一次，后台任务不能隐式接受新密钥。
- 终端内容是非可信数据。限制 OSC 扩展；不自动把 OSC 52 写入系统剪贴板，不执行终端输出中的链接或命令；外链仅允许受控协议并隔离 opener。
- 无默认终端录屏/逐键日志/遥测。日志记录生命周期、错误码和脱敏诊断，认证挑战与终端内容不被收集。

## 8. 持久化与发布

SQLite 存放 profiles、加密 credentials、known host keys、布局、隧道配置、任务状态与会话撤销数据；运行中 socket/PTY/client 不写数据库。迁移有版本、事务与升级前备份策略；备份包含加密数据时仍按秘密材料管理。

数据目录遵循各 OS 的用户配置/数据目录，凭据文件使用用户专属权限/ACL。应用退出按“停止接受新任务 → 吊销输入 → 关闭终端/隧道/任务 → 等待限时回收 → 关闭存储”顺序执行。

构建顺序固定为前端安装锁定依赖、类型/测试检查、前端产物构建、生成接口、Go 测试、Go 内嵌构建。开发时 Vite 仅提供同源代理，不将生产 CORS 放开来迁就热更新。发布至少覆盖 macOS arm64/amd64、Windows amd64、Linux amd64/arm64；交叉编译通过不等于真实系统运行通过。

## 9. 实现落地时的取舍（0.2.0-beta.1）

以下是按上文目标实现时做出的具体决定，代码中有对应测试；与原文不同之处在此显式记录。

| 主题 | 实现 | 理由 |
| --- | --- | --- |
| 连接流程 | `connections.Attempts`：`POST /attempts` 创建异步尝试，长轮询状态；主机指纹确认与 keyboard-interactive 提问都在**同一次握手内**作为 prompt 发给发起它的登录会话，回答后继续 | 跳板与目标各需一次确认，MFA 验证码通常一次性有效，不能靠“失败后重试”实现 |
| 后台重拨 | 文件操作等后台重拨的 `Target` 不带 `Interaction`：未知主机密钥直接失败关闭，不能代答 MFA | “后台任务不能隐式接受新密钥” |
| 交互式认证主机的 SFTP | 使用 keyboard-interactive 的主机（含其跳板）无法在后台重新认证，文件操作改为在终端已认证的连接上新开 SFTP channel；终端断开后文件操作明确报错 | 其余认证方式仍按原设计每次独立 transport，隔离大文件传输 |
| 单级跳板 | 每跳单独 `ssh.NewClientConn`，目标连接经跳板 `direct-tcpip`；跳板机本身不能再有跳板，被用作跳板的主机不能再设置跳板 | 与 V1 合同一致；嵌套关系在保存时即拒绝 |
| re-key | 主机密钥回调在每次重新协商时都会被调用；首次验证通过后在本连接内固定密钥字节，之后不再查库或询问，变化即断开 | 拨号上下文在连接建立后即取消，不能再用于后续 re-key |
| 握手超时 | 计时器关闭连接而非 `SetDeadline`（经跳板的 channel 不支持 deadline），等待用户回答时暂停计时 | 保证慢握手不会挂起，也不因用户核对指纹而超时 |
| 本机私钥路径 | 仅本机模式；配置值只能是 `~/.ssh/...` 形式，连接时 `EvalSymlinks` 后必须仍在 `~/.ssh` 内、为普通文件且 ≤256 KiB | “访问服务端私钥由所有者明确配置，浏览器不能任意选择本机路径” |
| known_hosts | 本机模式只读比对 `~/.ssh/known_hosts`，结果仅作为指纹旁的提示；信任仍只写入应用自己的存储，并须用户确认 | “系统 known_hosts 只读参考” |
| 凭据库 | XChaCha20-Poly1305，随机数据密钥；保护方式二选一：系统凭据存储（`zalando/go-keyring`，macOS 经 `security -i` 标准输入传参）或 Argon2id（t=3, m=64 MiB, p=4）主密码；5 次错误锁 1 分钟；注销即锁定；重置会删除全部密文 | 不降级为明文；Linux 无 Secret Service 时提示改用主密码 |
| 控制通道 | 数据目录下 `control.sock`（0600，目录 0700）；Windows 命名管道 DACL 只授当前用户 SID，且客户端/服务端在使用前核对管道服务进程的用户 SID（防抢注），CLI 只打开 `http://127.0.0.1:` 链接。仅支持 `open`（新的一次性授权）与 `edit`（登记本机文件，返回不透明 ID） | Web API 永远拿不到本机路径授权入口 |
| 本机文件编辑 | 登记时解析符号链接，之后只操作真实路径；同目录临时文件 + fsync + 版本复核 + `rename` | 与远端编辑共用 `textdoc` 规则 |
| OpenSSH 配置导入 | `kevinburke/ssh_config` 负责词法、匹配与取值；`Include` 由本项目预处理并限制在 `~/.ssh`（真实路径、深度 5、32 个文件、防循环）；不支持的 `Match` 条件改写为永不匹配的块并给出诊断；`Match exec` 永不执行 | 该库会自行读取任意路径的 Include，并在 Match exec 处整体失败 |
| 目录树操作 | 递归删除/打包前完整清点（≤10,000 项、≤32 层、不跟随符号链接、遇到非 UTF-8 名称整体停止）；递归删除要求用户确认过的数量与服务端再次清点一致 | 不在不完整或已变化的视图上执行破坏性操作 |
| 工作区 | 只持久化标签对应的主机、分屏方式与比例、终端偏好与快捷键；刷新后以“未连接”占位恢复，需用户重新连接 | 不伪装恢复原 Shell |
| 隧道 | 先绑定回环端口（立即报告端口占用），再建立独立 SSH transport；只允许 127.0.0.1 / ::1；SSH 断开即停止并标记失败，不自动重连 | 重连可能需要再次 MFA |
| 广播 | 浏览器通过各终端现有写入租约逐台发送一次整条命令；服务端不新增写入通道 | 保持“一终端一写入方”，不重放、不重试 |
| 日志 | `slog` 统一经过脱敏处理器：敏感键名整体替换，bootstrap、私钥 PEM、会话 Cookie、CSRF 头按模式擦除 | 第二道防线；代码本身仍不记录秘密 |
