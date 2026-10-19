# 总体架构

> 状态：设计稿，尚未实现。标为“待验证”的部分需要原型或真机测试确认。

## 统一还是多套？

**一套核心代码 + 一套界面代码，平台差异只在少数模块里。** 不做每个平台一套前后端。

| 层 | 技术 | 跨平台方式 |
|---|---|---|
| 核心（组网、协议、工具、存储） | Go | 同一份代码，交叉编译出 Windows、Linux、macOS 版本 |
| 平台相关的非桌面功能（会话、进程、自启等） | Go | 按平台分文件（`_windows.go`、`_linux.go`、`_darwin.go`），用构建标签选择 |
| Windows 桌面操作（截图、输入、界面元素、控制权提示） | Rust 桌面辅助进程 | 只有 Windows 版；由 Go 主程序启动，通过标准输入输出或命名管道传 JSON。其他平台以后按需选择语言 |
| 桌面客户端界面 | Flutter（Dart） | 先做 Windows，同一套代码再做安卓控制端；启动 Go 程序作为子进程，经标准输入输出传 JSON。见 [ADR-0002 界面](decisions/adr-0002-ui-framework.md) |
| 网页控制台（可选） | 暂不做 | 需要时再评估 |

语言已确定为 Go 主程序 + Rust 写的 Windows 桌面辅助进程，见 [ADR-0001 语言](decisions/adr-0001-language-and-framework.md)。客户端界面选用 Flutter，见 [ADR-0002 界面](decisions/adr-0002-ui-framework.md)。

## 组件

```
┌──────────────┐   MCP (stdio)    ┌─────────────────────────────┐
│  AI Agent    │ ───────────────▶ │ deskbridge mcp              │
│ (Claude Code)│                  │ 把设备和工具暴露为 MCP 工具   │
└──────────────┘                  └──────────────┬──────────────┘
┌──────────────┐                                 │
│ 人：桌面客户端│ ── 同一个客户端库 ──────────────┤
│ 人：命令行    │                                 │
└──────────────┘                                 ▼
                                  ┌─────────────────────────────┐
                                  │ 连接层（可替换）              │
                                  │ Tailscale（tsnet），以后更多  │
                                  └──────────────┬──────────────┘
                                                 │ 加密的内网连接
                                                 ▼
                                  ┌─────────────────────────────┐
                                  │ deskbridge 节点（每台设备）   │
                                  │ 工具：命令、文件、桌面……       │
                                  │ 本地存储：SQLite              │
                                  └─────────────────────────────┘
```

所有入口都是同一个程序 `deskbridge` 的子命令：

| 子命令 | 作用 |
|---|---|
| `deskbridge node` | 节点：加入内网，对外提供工具。每台被控设备运行一个。 |
| `deskbridge <工具命令>` | 命令行客户端，例如 `deskbridge exec win-pc "dir"`。 |
| `deskbridge mcp` | MCP 服务器，供 AI Agent 使用。 |
| `deskbridge ui-core` | 供 Flutter 客户端启动的子进程，经标准输入输出传 JSON（名称待定）。 |

一台设备可以同时是“控制端”和“被控端”，两种模式由同一个程序完成。

## 连接层与身份

上层（节点协议、设备列表、权限、MCP、界面）只依赖连接层接口：登录状态、对端列表（地址、路径、延迟）、拨号和监听、身份识别、事件。Tailscale 是第一种实现；局域网发现与配对在初版一起做（ADR-0009）；WireGuard 直连在后续（任务 19）。每台设备有自己的 DeskBridge 设备 ID，用于身份、配对和同一连接方式内部去重。各连接方式互相独立、不跨连接方式合并：同一台设备可以同时出现在多种连接方式里，从哪种方式连接就走哪种方式。详见 [ADR-0007](decisions/adr-0007-pluggable-connection-layer.md)、[ADR-0010](decisions/adr-0010-independent-connection-channels.md)。

Tailscale 实现（[ADR-0003](decisions/adr-0003-network-tsnet.md)）：

- **内置 Tailscale（`tsnet`）：** 程序自己成为内网里的一个节点，用户不需要另装 Tailscale。首次运行时打开登录链接，用 Google、GitHub、Microsoft 等账号登录。
- **设备列表：** 从 `tsnet` 的本地状态直接读取同一账号下的所有节点，再探测哪些节点运行着 DeskBridge。不需要 Tailscale API 密钥。
- **客户端类型：** DeskBridge 内置节点在设备信息（Hostinfo）里把 App 设为 `deskbridge`，用来和系统 Tailscale 客户端节点区分（`mesh.Peer.Client`：`deskbridge`、`tsnet`、`tailscale`）；不用 ACL 标签，因为打标签会把设备归属从账号改成标签，破坏“同账号才信任”。Tailscale 后台网页是否显示这个字段：待验证。
- **身份：** 节点收到请求时，由连接层查出对方是谁；Tailscale 实现用 `WhoIs` 查出账号和设备。同一账号的设备默认互信，**不再需要共享密钥**（原型里的 HMAC 密钥会被淘汰）。
- **可替换的控制服务器：** 登录服务器地址可配置，可以改成自建的 Headscale。
- **中继：** 打不通直连时走 Tailscale 的公共中继（DERP），只承载工具类功能。自建中继**暂时搁置**，见 [ADR-0006](decisions/adr-0006-feature-tiers-and-lan-scope.md)。
- **网络场景：** 局域网直连、跨网直连、中继、中国大陆网络、只允许 HTTPS 的网络、断开外网、网络切换都要考虑，各客户端要显示连接方式和延迟。见 ADR-0003 的“必须考虑的网络场景”。
- **功能分层：** 截图、点击、输入等工具类功能在任何内网路径下可用；实时远程桌面画面暂时只在局域网直连时开放，其他路径的远程桌面降级为截图模式，见 ADR-0006。

局域网实现（[ADR-0009](decisions/adr-0009-first-release-scope.md)，初版）：

- **发现：** DeskBridge 在局域网中广播自己（如 mDNS），设置里可以关闭。
- **配对：** 一方显示 6 位配对码（局域网页的“本机配对码”），另一方输入；双方保存对方的设备公钥（DeskBridge 设备 ID），以后自动连接。原型暂定最多尝试 5 次、5 分钟有效，任务 28 中确定（待验证）。
- **连接：** 直接用局域网地址，传输加密并校验对方的设备公钥（具体协议待定）。没有配对的设备只能看到名字。

设备连接方式（界面上按连接方式横向切换，ADR-0009）：

| 标签 | 来源 | 初版权限 |
|---|---|---|
| 局域网 | 同一局域网中发现的 DeskBridge 设备（也在 Tailscale 里的设备同样列出）；配对后才能使用 | 配对后全部工具 |
| Tailscale | 同一 Tailscale 账号的节点；用户主动添加后显示 | 全部工具 |
| WireGuard、自建中继 | 后续（任务 19、13） | — |

界面默认只显示局域网标签；Tailscale 由用户从“添加连接方式”加入，也可以退出登录或移除。各连接方式互不影响：Tailscale 断开或移除只影响 Tailscale 标签，局域网发现关闭只影响局域网标签（ADR-0010）。设备上同时装了 Tailscale 客户端、Tailscale 里出现两个节点时，按 DeskBridge 设备 ID 去重。桌面控制权、命令和操作记录按设备 ID 记录，与经哪种方式连上无关。外部设备（他人共享）和细分权限推迟到初版之后（ADR-0008）。

## 节点协议

沿用原型的思路，换成更简单的承载方式：

- **承载：** 内网里的 HTTP + JSON；需要持续推送的（进度、事件、实时远程桌面信令）用 WebSocket。
- **能力报告：** `GET /v1/capability` 返回平台、版本、工具列表和每个工具的 JSON Schema。客户端据此决定能做什么。
- **工具调用：** `POST /v1/tools/{name}`，带 `request_id`、参数、超时。支持取消（`DELETE /v1/requests/{id}`）。
- **操作记录和实时事件：** `GET /v1/records`、`GET /v1/requests/{id}/events`（WebSocket），见“操作记录”一节。
  - `request_id` 可省略（节点生成）；指定时不超过 128 字节、不含控制字符，否则返回 `invalid_args`。
  - `timeout_ms` 缺省（0）时节点用该工具自己的默认超时（通用默认 60 秒）；指定值和工具默认值都被截到工具上限，且任何工具的上限不超过全局上限 10 分钟。客户端库不知道工具的默认值，所以总是设置等待截止时间：`timeout_ms` 缺省时取全局上限加 15 秒余量，指定时取 `timeout_ms`（同样截到全局上限）加 15 秒余量，保证能收到节点的超时响应。
- **工具命名不带平台前缀：** 例如 `fs.read`、`proc.exec`、`screen.capture`，同名工具在各平台语义一致；平台不支持时在能力报告中不出现，调用时返回明确错误 `unsupported_platform`。
- **输出统一为 UTF-8：** Windows 上的命令输出按代码页（如 GBK）解码后再返回。原型里中文报错变乱码的问题必须在这里解决。

- **端口：** 节点协议在内网地址的 7465 端口上提供。
- **错误格式：** `{"ok":false,"error":{"code":"...","message":"..."}}`。错误码：`invalid_args`、`unknown_tool`、`not_found`、`device_not_found`、`permission_denied`、`duplicate_request`、`payload_too_large`、`cancelled`、`internal`、`unsupported_platform`、`unavailable`、`timeout`。
- **发起者：** 调用请求带 `initiator`（人工或 AI、经命令行 / MCP / 界面、会话），由调用方声明；调用者的身份由连接层查出，不由调用方声明。两者一起写进操作记录，供以后的权限判断（任务 17）。
  - 校验规则：`type` 只能是 `ai` 或 `human`，`via` 只能是 `cli`、`mcp` 或 `ui`（空串视为缺省）；`session` 不超过 128 字节且不含控制字符。不合法返回 `invalid_args`。
  - 缺省值按字段补：整个 `initiator` 缺省、或其中 `type` 为空补 `human`，`via` 为空补 `cli`；`session` 保持原样。因此只给 `session` 的请求按“人工经命令行”记录。
- **不在连接层之上再做一次双向认证（任务 4 决定）：** Tailscale 已经用 WireGuard 密钥认证节点，`WhoIs` 结果可靠。能力报告返回 DeskBridge 设备 ID 和公钥；没有连接层身份的局域网配对（任务 28）再用设备密钥签名认证。

## 节点进程和本机接口

每台设备只运行一个长期运行的 `deskbridge node`，连接方式（如 tsnet）只在节点里启动。桌面客户端是例外的启动方式：本机没有节点时，客户端的后台（ui-core）自己在进程内运行节点，见下文“桌面客户端的后台”。命令行、MCP 服务器和桌面客户端都不直接加入内网，而是经**本机接口**调用本机节点，由本机节点经连接层转发到对端节点。这样每条命令不需要重新启动 tsnet（需要几秒），内网里也不会因为客户端多出节点。

```
deskbridge exec / mcp / ui-core ──本机接口──▶ 本机 deskbridge node ──连接层──▶ 对端 deskbridge node
```

- **数据目录：** 环境变量 `DESKBRIDGE_HOME`；否则 Windows 为 `%AppData%\DeskBridge`，Linux 为 `~/.config/deskbridge`。tsnet 状态在其中的 `tailscale/`，目录权限只允许当前用户。
- **本机接口：** 数据目录下的 unix socket `node.sock` 上的 HTTP（Windows 10 1803 起支持 AF_UNIX）。节点启动时先把数据目录收紧为只有当前用户能访问（Linux、macOS 上权限 0700；Windows 上设受保护的 DACL，只授予当前用户和 SYSTEM，目录里的文件继承同样的权限），再创建 socket，并对 socket 再单独设置一次（0600 或同样的 DACL）。连上本机接口就获得本机身份，所以访问控制只靠这层文件权限。Windows 上已验证 DACL 设置正确、当前用户能连接；其他用户被拒绝尚未实测（待验证）。以后如果改为系统服务运行（见下文“服务模式”），节点进程的用户不再是桌面用户，这里要相应调整。
- **接口：** `GET /local/status`；`POST /local/channels/{连接方式}/login`、`logout`；`GET /local/channels/{连接方式}/peers`；`GET /local/devices[?refresh=1]` 返回设备列表：按连接方式分组（不跨方式合并）、只列同账号的设备、本机排第一，并对在线设备探测是否运行 DeskBridge（取能力报告，探测结果缓存 30 秒，`refresh=1` 忽略缓存，离线设备不探测，同一连接方式内设备 ID 相同的节点合并为一条）；`/local/devices/{设备}/v1/...` 转发到对端节点的节点协议。设备按名称、系统主机名或内网 IP 查找，可写成 `连接方式:设备` 指定连接方式；`self` 表示本机，不经过内网，身份为 `local`；本机在连接方式里的名称、主机名或内网 IP（可带 `连接方式:` 前缀）也按 `self` 处理。
- **登录：** 交互式登录返回登录链接；也可以用预授权密钥（`--auth-key` 或环境变量 `DESKBRIDGE_AUTH_KEY`），适合没有浏览器的服务器。
- **设备的两个名字（2026-10-14 用户决定）：** “设备名称”（`--name`）只在 DeskBridge 里显示，可以用中文；“主机名”（`--hostname`）就是这台设备在 Tailscale 里的名字，只能用英文、数字和连字符（Tailscale 会丢弃其他字符，中文名会变成空）。两个名字分开设置，不自动转换；界面上统一叫“设备名称”和“主机名”，不叫“备注”或“设备编码”，以免和设备 ID 混淆。以后在 DeskBridge 里修改主机名，就是修改这台设备在 Tailscale 里的名字（节点运行中修改自己的主机名，改别的设备时经节点协议由对方修改自己；新名字要保存到配置，因为 tsnet 每次启动都会用配置覆盖。待实现）。
- **设备 ID：** ed25519 密钥对，ID 为公钥 SHA-256 前 20 字节的 base32 小写编码。私钥放在系统密钥库；系统密钥库不可用时（如没有 Secret Service 的 Linux 服务器）退回数据目录下权限为 0600 的文件。

第一批工具（命令和文件工具的参数和返回值见 `docs/tools.md`）：

| 工具 | 说明 |
|---|---|
| `sys.info` | 系统、版本、桌面会话状态 |
| `fs.list` / `fs.read` / `fs.write` / `fs.stat` | 文件 |
| `proc.exec` | 执行命令，结构化返回退出码、标准输出、标准错误、是否超时 |
| `proc.list` / `proc.kill` | 进程 |
| `screen.capture` | 截图，可选显示器和缩放 |
| `input.click` / `input.type` / `input.key` / `input.scroll` | 鼠标键盘 |
| `window.list` / `window.focus` | 窗口 |
| `ui.tree` / `ui.find` / `ui.invoke` | 界面元素（Windows UI Automation） |

## MCP 服务器

`deskbridge mcp` 经标准输入输出提供 MCP，供 Claude Code 等 AI Agent 使用。它和命令行一样经本机接口调用本机节点，自己不加入内网。SDK 用官方的 `github.com/modelcontextprotocol/go-sdk`。

**工具形态（任务 7 决定）：** 固定的一组通用工具，设备作为参数，不按“设备 × 工具”动态生成。设备会上下线、各设备工具不同，动态生成会让工具列表频繁变化、数量膨胀，Agent 也难以缓存。常用操作另给专用工具，参数更直观，AI 不用先查 schema。

| MCP 工具 | 参数 | 对应 |
|---|---|---|
| `list_devices` | `refresh`（可选） | 本机接口 `GET /local/devices` |
| `device_tools` | `device` | 能力报告：该设备的工具及参数 schema |
| `call_tool` | `device`、`tool`、`args`（对象，可选）、`timeout_ms`（可选） | 调用任意工具，返回 result |
| `exec` | `device`、`command`、`shell`、`cwd`、`stdin`、`timeout_ms`、`max_output_bytes`（后五个可选） | `proc.exec` |
| `read_file` | `device`、`path`、`offset`、`max_bytes`（后两个可选） | `fs.read`（`encoding` 为 `auto`） |
| `write_file` | `device`、`path`、`content`、`append`（可选） | `fs.write` |
| `list_dir` | `device`、`path`、`recursive`、`max_entries`（后两个可选） | `fs.list` |

- **保护 AI 的上下文：** `exec` 的 `max_output_bytes` 默认 65536，`read_file` 的 `max_bytes` 默认 262144，`list_dir` 的 `max_entries` 默认 500，都写在工具描述里；截断时结果末尾注明，`read_file` 没读完会给出继续读的 `offset`。`exec` 的 `timeout_ms` 默认 300000、上限 600000（节点按工具截断），描述里告诉 AI 耗时长的命令放到后台写日志、用 `read_file` 轮询。
- **返回格式：** `exec` 返回给人读的文本（退出码、标准输出、标准错误，超时和截断有标注）；`read_file` 返回文件内容（base64 时注明）；其余返回两空格缩进的 JSON 文本。
- 发起者为 `{"type":"ai","via":"mcp","session":<MCP 会话 ID>}`。
- 工具失败（协议错误）作为 `isError` 的工具结果返回，文本为“错误（code）：message”，让 Agent 能读到原因；节点没运行也这样返回，MCP 服务器本身不退出。
- Agent 取消调用时，经 `DELETE /v1/requests/{id}` 通知对端取消。
- 以后（任务 22）转发被控端的 MCP 工具，仍经 `call_tool` 调用，不改变本节的工具形态。

## 调用被控端的 AI 能力：Agent、Skill、MCP（规划中）

被控设备上往往已经有自己的 AI 环境：Claude Code、Codex 等 Agent，它们的 Skill，以及本机配置的 MCP 服务器（例如只能在 Windows 本机运行的数据库、浏览器或桌面控制工具）。DeskBridge 应该能让控制端使用这些能力，而不是只能用 DeskBridge 自带的工具。

| 能力 | 做法 | 例子 |
|---|---|---|
| 调用被控端的 Agent | 节点在被控端启动一个无界面的 Agent 会话（如 `claude -p`），把任务交给它，流式返回过程和结果，可以取消 | Linux 上的 Agent 把“在 Windows 上编译并修复报错”整个交给 Windows 上的 Agent |
| 使用被控端的 MCP 服务器 | 节点读取被控端的 MCP 配置，在本机启动这些服务器，把它们的工具经内网转发给控制端，工具名带前缀（如 `mcp.<服务器名>.<工具名>`） | Linux 上的 Agent 直接使用 Windows 本机的 MCP 工具 |
| 使用被控端的 Skill | 列出被控端可用的 Skill；通过被控端的 Agent 调用，或者同步到控制端使用 | 复用 Windows 上已配置好的工作流 |

**对现在设计的影响（任务 4 节点协议必须预留）：**

1. **工具来源可扩展：** 能力报告里的工具分来源：内置工具、MCP 转发工具、Agent 运行器。来源在运行时变化（MCP 服务器启动或停止），能力报告要能刷新。
2. **长时间任务和流式输出：** Agent 任务可能运行几十分钟，需要任务 ID、进度推送（WebSocket）、中途取消、断线后重新查看结果。
3. **嵌套调用的身份记录：** 操作记录要能看出“Linux 的 Agent → 调用 Windows 的 Agent → 它又调用了某个 MCP 工具”这条链。
4. **权限：** 被控端的 Agent 和 MCP 工具能力很强，后续权限设计（任务 17）要能单独控制它们。

## 桌面控制权仲裁

本地用户、远程人工和 AI 可能同时使用同一台设备。优先级为**本地用户 > 远程人工 > AI**；只有模拟鼠标键盘这类“前台输入”需要控制权，执行命令、读写文件、截图、读取界面元素等操作不受影响。本地物理输入会被自动识别并立即抢占 AI。详见 [ADR-0005](decisions/adr-0005-desktop-control-arbitration.md)。

## 桌面模式：真实桌面和虚拟桌面

每台 Windows 设备有两种桌面：本地用户正在使用的**真实桌面**，和给 AI 专用、不影响本地用户的**虚拟桌面**。截图、输入、窗口、界面元素工具和远程桌面都要指定桌面（参数暂定 `desktop: real | virtual`）。控制权仲裁只在真实桌面上和本地用户争夺；虚拟桌面上有多方操作时仍按同样的优先级仲裁。

AI 拿到的画面在两端都能看到：发起端可以实时查看和回看截图；被控端的真实桌面本来就在眼前，虚拟桌面通过查看窗口观看和接管。两端看到的就是 AI 拿到的那张截图，截图存进本地操作记录。

虚拟桌面的实现方式**待验证**（任务 29）：先验证 Win32 隐藏桌面，不通过时改用 Windows 沙盒或虚拟机。详见 [ADR-0011](decisions/adr-0011-real-and-virtual-desktop.md)。

## 权限与确认

**推迟到初版之后（ADR-0009）。** 初版只接受同一 Tailscale 账号的设备和已配对的局域网设备，它们拥有全部工具权限。以后的设计：被控端按发起者（身份 + 人工或 AI）的权限立即放行或拒绝，不弹窗等人批准。权限项包括查看屏幕、鼠标、键盘、剪贴板、执行命令、读写删除文件和文件范围、调用本机 Agent 和 MCP、重启关机。危险操作（删除文件、修改注册表等）的确认在发起端完成：AI 客户端、本账号的 DeskBridge 窗口或手机。注意：允许执行命令或键盘以后，文件范围和删除限制可以被绕过，界面要提醒。详见 [ADR-0008](decisions/adr-0008-permissions-and-initiator-confirmation.md)。

## Windows 的会话模型

截图和输入必须在**用户的桌面会话**里执行。Windows 服务运行在看不到桌面的会话 0 里，所以分两步：

1. **早期：用户模式。** 用户登录后自动启动节点（计划任务），节点就在桌面会话里。简单，但锁屏、UAC 弹窗、开机未登录时无法操作。
2. **后期：服务模式。** 一个系统服务负责常驻和组网，在用户会话里启动一个辅助进程（`WTSQueryUserToken` + `CreateProcessAsUser`）专门做桌面操作，两者通过命名管道通信。这是 RustDesk、MeshCentral 都采用的思路（只参考设计）。

## 本地存储

- **SQLite（`modernc.org/sqlite`，纯 Go，不需要 C 编译器）：** 设备信息、设置、操作记录。
- **系统密钥库（`go-keyring`）：** 需要保存的凭据，例如 WireGuard 私钥。不写进 SQLite 和配置文件。
- **连接层状态（如 `tsnet` 状态目录）：** 登录状态和节点密钥，权限只允许当前用户访问。
- **DeskBridge 设备 ID 的私钥：** 放在系统密钥库（ADR-0007）。
- **Git 仓库里只放脱敏的示例配置。**

## 操作记录

每次工具调用记录：`request_id`、操作者类型（AI / 人）、操作者身份（内网账号、MCP 会话）、目标设备、工具名、参数摘要（敏感字段脱敏）、结果状态、开始和结束时间。审计日志行中调方可控的字符串字段（`request_id`、`tool`、发起者各字段）都用带引号转义的形式输出，防止换行等字符伪造日志行。哈希链防篡改和双端存储留到以后（任务 17）。

用户要能看到 AI 在各台设备上做了什么，并能插手（2026-10-15 用户决定提前到里程碑二，任务 11、12）。实现如下（2026-10-16，任务 11 完成）：

- **存储：** 数据目录下的 `records.db`（SQLite，WAL）。被控端把收到的每次调用写进去（`role=target`），控制端把自己发出的请求也写一条（`role=source`，在本机接口转发时记录，请求里没有 `request_id` 就先补上），两条用同一个 `request_id` 关联。调用开始时写入 `running`，结束时更新为 `ok` 或错误码；节点启动时把上次遗留的 `running` 标为 `interrupted`。保留 90 天。表结构用 `PRAGMA user_version` 迁移。
- **参数摘要脱敏（`internal/redact`）：** 键名含 password、token、secret、api_key、authorization、cookie 等的值整个隐藏；`content`、`stdin` 只记字节数；`env` 只留变量名；字符串里的 `password=…`、`--token …`、`Bearer …` 替换为 `***`；单个字符串最多 200 字符，整个摘要最多 1 KiB。宁可多遮：命令行里 `password=` 后面的内容可能连带遮掉同一个词后面的部分。
- **跨设备查询：** `GET /v1/records?since=&until=&operator_type=&tool=&status=&role=&request_id=&before=&limit=`（同账号设备可调用，按开始时间从新到旧，`next_before` 翻页；`tool` 以 `*` 结尾为前缀匹配，`status` 可用 `failed` 表示所有失败）。客户端库的 `Activity` 向本机和所有在线的 DeskBridge 设备查询（对其他设备只取 `role=target`，每台 8 秒超时，失败的设备单独列出），按 `request_id` 去重（被控端记录优先）后按时间合并。离线设备的记录在它上线后才能看到；版本较旧、没有这个接口的设备显示“对端不支持这个接口”。
- **实时事件：** `GET /v1/requests/{id}/events` 升级为 WebSocket，每条消息是一个 `protocol.Event`：`start`（工具名、`dropped_bytes`）、`output`（`stream`=stdout/stderr，已解码为 UTF-8，`proc.exec` 按行推送，没有换行的内容攒到 4 KiB 或命令结束）、`end`（结果码、耗时）。每个调用保留最近 256 KiB 输出，新订阅者先收到缓冲再接实时片段；超出上限丢弃最早的部分并在 `start.dropped_bytes` 标明；订阅者太慢（待发队列 256 条满）被断开。调用结束后订阅返回 `not_found`，界面改用调用结果。只有执行中的调用能订阅，实时事件目前只有 `proc.exec` 推送输出，其他工具只有 `start` 和 `end`。
- **取消和订阅的权限：** 同账号的任何设备都可以按 `request_id` 取消或订阅（`DELETE /v1/requests/{id}`）；本机的本地身份（本机用户经命令行或桌面客户端）也可以，它的 Owner 只是占位值，所以单独放行（实测发现：不放行时本机用户无法取消其他设备发起的调用）。初版信任同账号设备（ADR-0009），以后按权限细分（ADR-0008）。
- **命令行：** `deskbridge log [-d 设备 | --all] [--ai|--human] [--tool] [--status] [--since] [-n] [-v] [--json]`、`deskbridge watch -d 设备 <request_id>`（实时看输出）、`deskbridge cancel -d 设备 <request_id>`。
- **桌面客户端的后台（`deskbridge ui-core`）：** 客户端启动它作为子进程，它在 `127.0.0.1` 随机端口上提供带随机令牌的 HTTP 服务（启动时向标准输出打印 `{"addr","token"}` 一行，标准输入关闭时退出），把 `/local/...` 原样转给本机节点的 unix socket（含 WebSocket），并提供 `GET /api/activity`（跨设备合并的记录）。这样 Dart 不需要支持 unix socket，节点的权限模型不变。这替代了 ui-design 第 7 节早先设想的“标准输入输出传 JSON”。
  - **本机节点的两种模式（2026-10-19 用户决定）：** 参数 `-node=auto`（默认）时，ui-core 先用 1 秒超时连本机接口 socket。连得上就**复用**（地址行 `"node":"external"`，只转发，客户端退出时不关这个节点）；连不上就在 ui-core 进程内**内置运行**节点（`"node":"embedded"`，tsnet 和工具执行都在 ui-core 进程里），客户端退出（含崩溃，标准输入关闭）时节点一起退出，ui-core 最多等 10 秒让它关闭连接和记录库。内置节点等本机接口可连接后才打印地址行，最多等 15 秒；启动失败时向标准错误输出“本机节点启动失败：原因（日志：路径）”并以非零码退出，客户端在启动失败页显示这段文字。`-node=off`（地址行 `"node":"off"`）只转发。内置节点的日志在数据目录 `logs/ui-node.log`（按大小轮转）；预授权密钥取环境变量 `DESKBRIDGE_AUTH_KEY`。内置节点运行中途异常退出时 ui-core 继续转发，界面提示本机节点没有响应。内置节点暂时用默认的设备名称和主机名（不读取自启项的 `--name`、`--hostname`；设备名称持久化还没做）。

## 远程桌面（后期实时画面）

实时画面暂时只支持局域网直连（ADR-0006）。被控端用 DXGI 桌面复制采集、系统自带的 H.264 编码器编码（任务 24.2 实测），经内网传给控制端解码显示，同时转发鼠标键盘。传输方式（WebRTC 或自定义流）随任务 24 决定。客户端里每个远程会话是一个独立窗口（用户提出参考 ToDesk、UU 远程），可以同时操作多台设备；Flutter 的多窗口方案待验证（任务 12、15）。必须做码率自适应和只保留最新帧；路径不是局域网直连时不启用实时画面，远程桌面降级为截图模式，复用工具层的截图和输入能力。是否启用实时画面由一个可替换的判断规则决定，以后支持中继时只改规则。只参考 RustDesk 的设计，不复制代码。

## macOS（预留）

代码结构上为 `_darwin.go` 预留平台模块。未实现的工具在能力报告中不出现。真正适配时需要处理“屏幕录制”和“辅助功能”权限，必须由用户在系统设置中手动授予。
