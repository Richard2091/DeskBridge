# 开发方式

## 两份代码，Git 同步，重活在 Windows

| | Linux 服务器 | Windows 开发机 |
|---|---|---|
| 位置 | `/root/projects/deskbridge` | `D:/data/AI/Projects/deskbridge` |
| 用途 | AI 在这里编辑代码和文档、提交 | 下载依赖、编译、测试、运行桌面功能 |
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

## 远程执行通道

- **现在：** 使用原型 AgentDesk（`agentdesk exec ...`），经 Tailscale 连接 Windows。
- **以后：** DeskBridge 里程碑一完成后，改用 DeskBridge 自己连接和开发 DeskBridge。开发过程中遇到的不便，记录为 DeskBridge 的需求。

## 已知风险

- AgentDesk 的执行器是手动启动的。Windows 重启、睡眠或执行器崩溃后，远程通道会断开，需要用户在 Windows 上重新启动。
- 两台机器之间走香港中继，每条命令有约 230ms 的往返延迟。
- 推送到公开的远程仓库（如 GitHub）前，需要用户同意。
