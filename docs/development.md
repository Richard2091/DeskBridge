# 开发方式

## 两份代码，Git 同步，重活在 Windows

| | Linux 服务器 | Windows 开发机 |
|---|---|---|
| 位置 | `/root/projects/deskbridge` | `D:/data/AI/Projects/deskbridge` |
| 用途 | AI 在这里编辑代码和文档、提交 | 下载依赖、编译、检查、测试、交叉编译、运行桌面功能 |
| 原因 | 资源很少（2 核、2GB 内存），只放源代码 | 资源充足，而且是主要目标平台 |

### 同步方式：GitHub 为中心

```
Linux（编辑、提交） ──push──▶ GitHub ──pull──▶ Windows（编译、测试）
        ▲                                           │
        └──────────── pull ◀── GitHub ◀── push ─────┘（Windows 生成的文件，如 go.sum）
```

- **GitHub 仓库：** `Richard2091/DeskBridge`（目前私有，有可运行代码后再公开）。两边的 `origin` 都指向它。
- **Linux 的 GitHub 权限：** 一把只对这个仓库有读写权限的部署密钥（Deploy Key，`~/.ssh/deskbridge_github`，GitHub 上标题为 `linux-dev-server`）。Linux 上不保存 GitHub 账号令牌，不能访问其他仓库，也不能调用 GitHub API（建仓库、开 PR 等需要在 Windows 上用 `gh` 完成）。仓库配置了 `core.sshCommand` 使用这把密钥。
- **Windows 的 GitHub 权限：** 用户的 GitHub CLI 登录（`gh`，账号 `Richard2091`）。
- **备用通道：** GitHub 偶尔连不上时（从中国大陆访问可能出现 TLS 握手失败），Windows 可以直接和 Linux 同步：Windows 上的远程 `linux` 指向 `ssh://root@<Linux 内网 IP>/root/projects/deskbridge`。
  - Windows 使用专用密钥 `%USERPROFILE%\.ssh\deskbridge_sync`，Linux 上对它做了限制：只接受 Tailscale 内网（`100.64.0.0/10`）的连接，只能对这一个仓库执行 Git 拉取和推送（`/usr/local/bin/deskbridge-git-only`），不能开终端或转发端口。
  - Linux 仓库设置了 `receive.denyCurrentBranch=updateInstead`，所以 Windows 可以直接推送到 Linux，前提是 Linux 工作区没有未提交的修改。

### 日常流程

1. AI 在 Linux 上修改代码、提交，并推送到 GitHub：`git push`。
2. 通过远程执行让 Windows 拉取：`git -C D:/data/AI/Projects/deskbridge pull --ff-only`。
3. 在 Windows 上编译和测试，读取结果。
4. 如果 Windows 上产生了需要入库的文件（如 `go.sum`），在 Windows 上提交并推送到 GitHub，Linux 再 `git pull`。
5. 长时间的命令（首次下载依赖、完整编译）放到后台运行，结果写入日志文件，再读取日志。原型的单条命令最长 120 秒。
6. 仓库目前是私有的，日常推送不需要逐次确认；**改为公开或推送到其他远程仓库前，必须先征得用户同意。**

## 目录结构

| 路径 | 内容 |
|---|---|
| `cmd/deskbridge/` | 唯一的程序入口，子命令见 `deskbridge help` |
| `internal/cli/` | 命令行解析和子命令分派 |
| `internal/mesh/` | 内置组网（tsnet）。不叫 `net`，避免和标准库重名 |
| `internal/node/` | 节点，对外提供工具 |
| `internal/tools/` | 工具实现 |
| `internal/client/` | 命令行、MCP、桌面客户端共用的客户端库 |
| `internal/mcp/` | MCP 服务器 |
| `internal/store/` | 本地存储 |
| `internal/platform/` | 平台差异，按 `_windows.go`、`_linux.go`、`_darwin.go` 分文件 |
| `internal/version/` | 版本号，构建时用 `-ldflags` 注入 |
| `desktop-helper/` | Rust 写的 Windows 桌面辅助进程 |
| `ui/` | Flutter 客户端（任务 12 开始时建立） |
| `scripts/` | 检查和构建脚本 |
| `spikes/` | 技术验证代码，各自是独立的模块，不参与检查和 CI |

## 检查和构建

日常检查和编译都在 Windows 上做（通过远程执行），Linux 上不跑。

| 命令（Windows） | 做什么 |
|---|---|
| `powershell -ExecutionPolicy Bypass -File scripts\check.ps1` | gofmt、go vet、staticcheck、go test；Rust 的 fmt、clippy、test；构建两个 Windows 程序 |
| `powershell -ExecutionPolicy Bypass -File scripts\build.ps1 [-Version v0.1.0]` | 交叉编译 Windows/Linux/macOS 的 amd64 和 arm64，输出到 `dist/` |

`scripts/check.sh` 和 `scripts/build.sh` 是 CI 的 Linux 任务用的同等脚本。

- Go 版本由 `go.mod` 决定，本地 Go 较旧时会自动下载对应工具链（`GOTOOLCHAIN=auto`）。
- `.ps1` 脚本必须保存为带 BOM 的 UTF-8，否则 Windows PowerShell 5.1 会按 GBK 读取中文。
- CI 见 `.github/workflows/ci.yml`，运行在 GitHub 的运行器上：Linux 任务跑 `check.sh` 和 `build.sh`，Windows 任务跑 `check.ps1`。Flutter 的构建和冒烟测试在任务 12 加入。

## 远程执行通道

- **现在（2026-10-14 起）：** 用 DeskBridge 自己连接和开发 DeskBridge：Linux 上运行 `deskbridge exec -d desktop -- ...` 等命令，或让 Agent 经 `deskbridge mcp` 调用。开发过程中遇到的不便，记录为 DeskBridge 的需求。
- **以前：** 原型 AgentDesk（`agentdesk exec ...`），经系统 Tailscale 连接 Windows。两台 Windows 都装上新版 DeskBridge 后，于 2026-10-15 退役，执行器已停止。

## 更新已部署的节点

两台机器都运行在 Windows 上编译的程序，Linux 上不编译：

1. Windows 上 `git pull`，运行 `D:\data\AI\Projects\dbwin-run\build-m1.cmd`（先把里面的版本号改成当前提交）：生成 `deskbridge-new.exe` 和 `GOOS=linux` 的 `deskbridge-linux`。
2. 台式机：后台运行同目录的 `restart-node.cmd`（结束旧节点、替换程序、重新运行计划任务 `DeskBridgeNodeDev`）。当前连接会断开，等一会儿再连。
3. Linux：把 `deskbridge-linux` 拉回来，核对 SHA256 一致后装到 `/usr/local/bin/deskbridge`，`systemctl restart deskbridge-node`。经中继拉取约 6 分钟；`deskbridge read --out` 每块 4 MiB，慢的时候单块会超时，可改用按 1 MiB 分块并重试的脚本（`/root/m1-deploy/bin/fetch.sh`，不在仓库里）。
4. 笔记本（2026-10-15 起）：台式机到笔记本经中继传文件太慢，改为在笔记本上本地编译。`D:\Program\AIProject\dbwin-run\build.cmd` 经本机代理 `git pull`，用 `D:\Program\AIProject\go` 编译出 `deskbridge-new.exe`（GOPATH、GOCACHE 都放在 D 盘）。要脱离当前连接运行：用 `schtasks /create /sc once` 建一次性任务，再 `schtasks /run`。编译好后替换 `deskbridge.exe`，运行计划任务 `DeskBridgeNode`。
5. 没有备用通道：新节点如果起不来，远程连接会断开，需要用户在 Windows 上换回旧程序并重新运行计划任务。所以替换前先保留旧程序。

## TaskMaster 的模型

TaskMaster 的 AI 命令（`update-subtask`、`expand` 等）用 GLM，三个角色都配成 `openai-compatible` 服务商，模型 `glm-5.3-flash`（`.taskmaster/config.json`）。TaskMaster 没有 opencode 服务商；不用 `codex-cli`，因为 Codex 的默认服务商不是 GLM，模型名会被发到别的接口而报 404。

接口地址和密钥不进仓库，都写在 Linux 上项目根目录的 `.env` 里（已在 `.gitignore` 中，权限 600），TaskMaster 启动时自己读取，不依赖 shell 环境：

```
OPENAI-COMPATIBLE_BASE_URL=<GLM 接口地址>/v1
OPENAI_COMPATIBLE_API_KEY=<GLM 密钥>
```

地址的变量名带连字符，只能写在 `.env` 里。TaskMaster 没有直接改子任务标题的命令；对父任务用 `update-task` 会让模型重写所有子任务，GLM 的输出过不了格式校验，不要用。

## 已知风险

- Windows 上的 DeskBridge 节点暂时由计划任务手动启动（任务 8 之前）。Windows 重启、睡眠或进程崩溃后，远程通道会断开，需要用户在 Windows 上重新启动。
- 经 DERP 中继时，大文件分块传输（每块 4 MiB，约 74 KB/s）进行期间，同一路径上的其他请求可能延迟几十秒甚至超时；没有传输时空闲后再调用正常（2026-10-14 实测）。节点刚启动的一两分钟内也会出现连接卡住，单次请求卡 10–40 秒；2026-10-15 观察到卡顿期间路径在“跨网直连”和“中继”之间切换，切回稳定路径后恢复正常（推测与 Tailscale 直连路径不稳定、回退到中继有关，待验证）。
- 两台机器之间走香港中继，每条命令有约 230ms 的往返延迟。
- 推送到公开的远程仓库（如 GitHub）前，需要用户同意。
