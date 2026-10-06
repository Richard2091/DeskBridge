# 开发方式

## 两份代码，Git 同步，重活在 Windows

| | Linux 服务器 | Windows 开发机 |
|---|---|---|
| 位置 | `/root/projects/deskbridge` | `D:/data/AI/Projects/deskbridge` |
| 用途 | AI 在这里编辑代码和文档、提交 | 下载依赖、编译、测试、运行桌面功能 |
| 原因 | 资源很少（2 核、2GB 内存），只放源代码 | 资源充足，而且是主要目标平台 |

### 同步方式

- Linux 上的仓库是主仓库。Windows 通过 Tailscale 内网用 SSH 拉取和推送：`ssh://root@<Linux 内网 IP>/root/projects/deskbridge`。
- Linux 仓库设置了 `receive.denyCurrentBranch=updateInstead`，所以 Windows 可以直接推送（例如编译时生成的 `go.sum`），Linux 的工作区会自动更新。前提是 Linux 工作区没有未提交的修改。
- Windows 使用专用密钥 `%USERPROFILE%\.ssh\deskbridge_sync`。Linux 上对这把密钥做了限制：
  - 只接受来自 Tailscale 内网（`100.64.0.0/10`）的连接；
  - 只能执行 Git 拉取和推送，而且只能访问这一个仓库（`/usr/local/bin/deskbridge-git-only`）；
  - 不能开终端，不能转发端口。

### 日常流程

1. AI 在 Linux 上修改代码并提交。
2. 通过远程执行让 Windows 拉取：`git -C D:/data/AI/Projects/deskbridge pull`。
3. 在 Windows 上编译和测试，读取结果。
4. 如果 Windows 上产生了需要入库的文件（如 `go.sum`），在 Windows 上提交并推送回 Linux。
5. 长时间的命令（首次下载依赖、完整编译）放到后台运行，结果写入日志文件，再读取日志。原型的单条命令最长 120 秒。
6. 需要推送到 GitHub 时，在 Windows 上执行 `git push github main`（见下一节）。

### GitHub

- 仓库：`Richard2091/deskbridge`（目前为私有，有可运行代码后再公开）。
- **只从 Windows 推送。** Windows 上已用 GitHub CLI 登录，Linux 上不保存 GitHub 凭据。
- Windows 仓库的两个远程：`origin` 指向 Linux（日常拉取和推送），`github` 指向 GitHub（只推送）。`main` 分支的上游是 `origin/main`。
- 推送前确认 Windows 已拉取 Linux 的最新提交，并征得用户同意。

## 远程执行通道

- **现在：** 使用原型 AgentDesk（`agentdesk exec ...`），经 Tailscale 连接 Windows。
- **以后：** DeskBridge 里程碑一完成后，改用 DeskBridge 自己连接和开发 DeskBridge。开发过程中遇到的不便，记录为 DeskBridge 的需求。

## 已知风险

- AgentDesk 的执行器是手动启动的。Windows 重启、睡眠或执行器崩溃后，远程通道会断开，需要用户在 Windows 上重新启动。
- 两台机器之间走香港中继，每条命令有约 230ms 的往返延迟。
- 推送到公开的远程仓库（如 GitHub）前，需要用户同意。
