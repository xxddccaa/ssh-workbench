# 同类项目与旧工程调研

## 1. 方法与结论边界

核验日期：2026-11-25，Asia/Shanghai。

- 通过 GitHub API 核对 16 个仓库的描述、许可证标识、星数与归档状态。
- 浅克隆 8 个产品/架构参考仓库，另取 Termix SSH 插件与 xterm.js 源码，共 10 份源码快照。
- 对重点文件做静态抽查；没有安装参考产品，没有连接用户主机，没有运行其中的安装脚本或测试。
- 浏览检索工具未返回可用结果，实际证据来自 GitHub API、官方仓库文件及本地旧工程。可复查信息保存在 `evidence/`。
- 星数只表示核验时的关注度，不代表代码质量；`pushed_at` 不是默认分支提交时间，也不是最新发行日期。
- 下文“观察”是源码/文档事实，“判断”是本方案的取舍；README 声称的功能未经运行验收。

## 2. 需要修正的背景信息

1. **Termix 不应再只理解成一个 Node SSH 页面。** 当前 README 将其描述为插件化服务器管理平台，终端、文件、隧道等进入独立插件。主仓快照包含 `packages/plugin-sdk/`、`src/backend/plugins/`，另取了 `Plugin-SSH-Terminal` 核对终端实现入口。
2. **`uiff/ssh-app` 当前产品名是 Keyway，许可证为 MIT。** 它的 Go 后端加浏览器终端/SFTP/编辑/会话保留与本需求高度重合。2 星不是忽略它的理由。
3. **File Browser 已不是可默认信任其持续维护的底座。** 官方 README 写明在 2026-09-01 归档，API 核验 `archived=true`，并列出未修复的命令执行与会话问题。只参考历史文件操作组织，不嵌入其服务，也不把它作为新项目运行依赖。
4. **输入法问题需要按具体 issue 判断。** Netcatty 的中文首输丢失报告仍开放；electerm 既有开放报告，#4765 则已于 2026-11-25 关闭。不能把这些状态概括成“所有中文问题都没解决”，更不能推导成“换浏览器就一定解决”。

证据：各仓库 `README.md`、根许可证、`evidence/repositories.json`、`evidence/input-issues.json`，以及下文源码索引。

## 3. 仓库筛查

许可证列除重点仓库根文件外，其余以 API 标识为线索，并非逐文件许可审计。所有星数均为当天查询快照。

| 项目 | 星数 | 许可证标识 | 本轮深度 | 对本项目的用途 |
| --- | ---: | --- | --- | --- |
| Licoy/sshyard | 321 | AGPL-3.0 | 元数据 | 桌面 SSH/SFTP 产品参考，不作为 Web 工程底座 |
| t8y2/motty | 1,847 | Apache-2.0 | 源码抽查 | Go 连接、认证、agent 与跳板的职责划分 |
| binaricat/Netcatty | 7,574 | GPL-3.0 | 元数据、issue | 使用场景与中文回归案例 |
| nyakang/nyaterm | 2,623 | Apache-2.0 | 元数据 | 桌面工作台定位参考；未审查实现 |
| electerm/electerm | 15,709 | MIT | 元数据、issue | 多协议边界与输入法测试案例 |
| Eugeny/tabby | 75,577 | MIT | 元数据 | 成熟桌面终端对照；未审查实现 |
| kingToolbox/WindTerm | 32,739 | API 未识别 | 元数据 | 仅产品体验候选，不视作已核准可复用源码 |
| Termix-SSH/Termix | 16,115 | Apache-2.0 | 主仓、SSH 插件 | Web 工作台基线，不照搬插件平台 |
| gnmyt/Nexterm | 5,152 | MIT | 元数据、package.json | Web 多协议产品基线，不满足 Go 后端选择 |
| bifrost0x/webssh | 340 | Apache-2.0 | 源码/文档抽查 | 文件工作区、安全边界与操作确认设计 |
| nirui/sshwifty | 3,130 | AGPL-3.0 | 源码抽查 | Go WebSSH 与 WebSocket 来源检查 |
| uiff/ssh-app（Keyway） | 2 | MIT | 源码抽查 | 最接近的 Go WebSSH 产品对照 |
| huoguoji/shellcove | 0 | Apache-2.0 | 源码抽查 | Go 内嵌 Web、传输与部署组织；不可直接认定安全 |
| o8oo8o/WebSSH | 387 | MIT | 元数据 | 小型 Go WebSSH 候选；未审查实现 |
| wavetermdev/waveterm | 22,739 | Apache-2.0 | 源码抽查 | Go 后端与 Web 视图、控制器和生命周期拆分 |
| filebrowser/filebrowser | 35,833 | Apache-2.0 | 源码/公告 | 已归档；仅历史设计与安全反例 |

## 4. 重点项目：具体学什么、不学什么

### 4.1 Keyway：最需要正面比较的替代方案

观察：

- `go.mod` 使用 Go、`coder/websocket`、`pkg/sftp`、`x/crypto` 和纯 Go SQLite。
- `internal/term/term.go` 管理终端与目录信息；`internal/server/ws.go` 处理连接及交互式确认；`internal/vault/`、`internal/store/` 分开组织。
- `web/static/js/terminal.js` 将二进制 WebSocket 数据交给 `term.write(Uint8Array)`，并区分 `onData` 和 `onBinary`。
- 前端是静态 JavaScript 模块，不是本需求已确定的 React 19 + TypeScript。

判断：它证明“Go + 浏览器 + SSH/SFTP”本身不构成独特价值。直接使用它的成本最低；整体 fork 后替换前端、本机授权、接口与工作区模型仍是实质改造。将它作为功能基线和会话参考，但不因星数低就贬低其代码，也不声称自研必然比 fork 省时间。

### 4.2 Termix：产品基线，不是要复制的平台规模

观察：主仓依赖包含 `express`、`ssh2`、`ws`、数据库工具；README 列出独立的 SSH Terminal、File Manager、Tunnels、Workspaces 等插件。SSH 插件另有 `src/backend/live-sessions.ts`、`src/backend/shell-cwd.ts` 和终端设置契约。

判断：学习终端/文件/传输的工作区组织与设置体验。不引入其用户分享、插件隔离、升级市场和多数据库形态；这些不是单用户本机工具的首版问题。

### 4.3 Motty：参考 Go 能力边界，不 fork 原生 UI

观察：README 说明界面使用 Viva 原生渲染，不是浏览器。实际源码把 SSH 认证、host key、agent 平台实现、代理等拆在 `internal/sshconn/` 的独立文件，并配套测试。`go.mod` 含 Viva 的相邻目录替换与第三方替换。

判断：学习按协议职责和平台划分文件的方式。不要把“Go 写的”误认为“能直接改成 Web”；其原生输入与绘制层不符合这里的产品方向。README 的目录示意也需要同实际路径核对，不能当作源码事实直接照抄。

### 4.4 Wave：参考控制器，不复制远程运行时

观察：`pkg/blockcontroller/`、`pkg/shellexec/`、`pkg/web/ws.go` 与 `pkg/wshrpc/` 把视图对应的进程控制、运行、Web 通道和 RPC 拆开；文档另有 durable sessions 与 wsh 的说明。

判断：采用“面板标识 ≠ SSH transport ≠ 终端生命周期”的思想。不引入远端 wsh 安装、整套 RPC 框架、AI 和可执行小组件；首版远端只需要已有 SSH/SFTP 服务。

### 4.5 Sshwifty：参考 WebSSH 基础与安全边界

观察：项目定位是浏览器 SSH/Telnet；`application/` 下分别有配置、网络、controller、I/O 等包，`application/controller/socket.go` 具有 Origin 检查。README 和根许可证标为 AGPL。

判断：它是网络通道参考，不是完整的终端+文件工作流底座。不直接搬运实现，也不把其配置默认值原样移植到本机授权模型。

### 4.6 ShellCove：接近部署目标，但不是拿来即用的安全底座

观察：README 声明 Go 内嵌前端、SQLite、无 CGO；`web/package.json` 使用 Vue 3，产品偏 Linux 服务器、多用户、审计与录像。抽查提交中的 `internal/ssh/ssh.go:57` 使用 `ssh.InsecureIgnoreHostKey()`。

判断：可以参考发布与传输模块；但“Go 单二进制”不等于满足本需求的 React 技术栈、三平台发布和主机身份验证。这个具体 host-key 策略不能进入新工程。该观察只针对固定快照，不是对项目全部版本的安全审计。

### 4.7 bifrost WebSSH 与 File Browser

观察：bifrost WebSSH 的 README 和目录体现 Python 后端，包含文件工作区、文件保存冲突及信任边界文档。File Browser 的当前 README 明确标记归档和遗留安全问题。

判断：前者主要学习用户可见的风险提示与文件流程；后者只作为历史参考。二者都不作为新 Go 服务的基础框架。

## 5. 中文 issue 的核验方式

GitHub issue 状态只证明报告目前的处理状态，不能代替在目标版本复现。

| 项目与编号 | 核验时状态 | 对测试设计的意义 |
| --- | --- | --- |
| Netcatty #3185 | open | 中文输入法开启后首次输入丢失，需要“首次聚焦即输入”场景 |
| electerm #4133 | open（搜索 API） | Linux Rime 输入中文变为英文，需要 Linux 真实 IME 场景 |
| electerm #3367 | open（搜索 API） | 早期中文输入报告，不能据此断言当前版仍可复现 |
| electerm #4765 | closed（2026-11-25） | Shift 中英切换/预编辑提交问题；关闭状态也要进入回归样例 |

未对这些问题做本地复现。原始状态和抓取层次记录于 `evidence/input-issues.json`；未取得完整 issue 内容的条目明确标注为搜索结果。

## 6. 旧工程：保留需求，不迁移耦合

只读检查的旧工程提交：`d6a732fe79312011419e932b65c108e787143a22`。

观察：

- 后端大部分实现集中在 `internal/app/`，终端输出通过 Wails runtime 事件进入界面。
- `frontend/src/components/terminal/Terminal.tsx` 为 1,300 行，`TerminalTab.tsx` 为 1,915 行；输入、平台兼容、交互与生命周期集中在大组件中。
- 已经有 UTF-8 缓冲、IME、配置解析和渲染相关测试，并非所有工程工作都缺失。
- `Terminal.tsx` 有原生 macOS 文本输入与 composition 补偿；`internal/app/terminal_handler.go` 有分段 UTF-8 缓冲与远端 locale 设置。
- `internal/app/ssh_manager.go` 包含 known_hosts 策略及若干读取/解析失败容忍路径。新项目要求显式信任与失败关闭，不照搬这些默认行为。

判断：

1. 旧项目提供真实场景清单：配置导入、密钥口令、文件编辑、拖拽、中文输入。
2. 不迁移 Wails 绑定、事件传输、原生窗口、输入法补丁及整个大组件。
3. 将这些历史问题重新编写成独立测试；浏览器与原生 WebView 的差异只是风险减少的机会，不是“中文自动修复”的保证。

## 7. 最终取舍

| 路径 | 优点 | 代价 | 建议 |
| --- | --- | --- | --- |
| 直接使用 Termix / Keyway | 最快验证是否真的需要自研 | 不完全满足既定技术栈、部署和交互取舍 | 作为对照基线，不宣称现有工具无意义 |
| 整仓 fork Keyway | 已有 Go SSH/SFTP 与会话能力 | 仍需改造前端、API、本机权限与工作区；长期跟进上游 | 不作为默认路径 |
| 整仓 fork ShellCove / Motty | 已有较多 Go 能力 | Vue/服务器平台或原生 UI 方向不一致，另有需重新审查的策略 | 不采用 |
| 新建模块化单体 | 边界、技术栈和本机工作流一致 | 必须自己承担协议边缘情况、测试与维护 | 有条件采用，以验证门槛约束范围 |

自研的理由是这些要求的组合，不是“GitHub 没有人做过”。第一阶段若不能改善中文和核心流程，不继续增加协议或页面来掩盖问题。

## 8. 来源与复查

- `evidence/repositories.json`：16 个仓库的 API 快照，包含原始仓库地址和归档状态。
- `evidence/source-snapshots.json`：10 个克隆的完整 commit、来源、路径及审阅文件；不把分支名作为可复现版本。
- `evidence/input-issues.json`：issue 状态、来源和核验粒度。
- `evidence/xterm-release.json`：查询时最新正式版 API 结果，与源码主分支快照分开保存。

原始克隆保存在本地独立参考目录（不随本仓库提交）。例如可使用 `git -C <路径> show <完整commit>:<文件>` 复查证据。未安装依赖、未构建参考产品，不能把静态阅读写成“已实测”。

本方案不复制第三方实现。将来确需引入代码时，逐文件记录出处、许可证与 NOTICE，另行审查；MIT/Apache 标识也不等于可以删除署名。未核准的代码不进入新工程。
