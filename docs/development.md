# 开发方式

## 两份代码，Git 同步，重活在 Windows

| | Linux 服务器 | Windows 开发机 |
|---|---|---|
| 位置 | `/root/projects/deskbridge` | `D:/data/AI/Projects/deskbridge` |
| 用途 | AI 在这里编辑代码和文档、提交；跑轻量的 Go 检查和交叉编译 | 下载依赖、完整编译、测试、Rust 构建、运行桌面功能 |
| 原因 | 资源很少（2 核、2GB 内存），只装了 Go，没有 Rust 和 Flutter | 资源充足，而且是主要目标平台 |

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

| 命令 | 在哪里运行 | 做什么 |
|---|---|---|
| `scripts/check.sh` | Linux、CI | gofmt、go vet、staticcheck、go test |
| `scripts/build.sh [版本号]` | Linux、CI | 交叉编译 Windows/Linux/macOS 的 amd64 和 arm64，输出到 `dist/` |
| `powershell -ExecutionPolicy Bypass -File scripts\check.ps1` | Windows、CI | Go 检查和测试，Rust 的 fmt、clippy、test，构建两个 Windows 程序 |

- Go 版本由 `go.mod` 决定，本地 Go 较旧时会自动下载对应工具链（`GOTOOLCHAIN=auto`）。
- `check.ps1` 必须保存为带 BOM 的 UTF-8，否则 Windows PowerShell 5.1 会按 GBK 读取中文。
- CI 见 `.github/workflows/ci.yml`：Linux 任务跑 `check.sh` 和 `build.sh`，Windows 任务跑 `check.ps1`。Flutter 的构建和冒烟测试在任务 12 加入。

## 远程执行通道

- **现在：** 使用原型 AgentDesk（`agentdesk exec ...`），经 Tailscale 连接 Windows。
- **以后：** DeskBridge 里程碑一完成后，改用 DeskBridge 自己连接和开发 DeskBridge。开发过程中遇到的不便，记录为 DeskBridge 的需求。

## 已知风险

- AgentDesk 的执行器是手动启动的。Windows 重启、睡眠或执行器崩溃后，远程通道会断开，需要用户在 Windows 上重新启动。
- 两台机器之间走香港中继，每条命令有约 230ms 的往返延迟。
- 推送到公开的远程仓库（如 GitHub）前，需要用户同意。
