# Claude Code Instructions

项目协作约定见 @./AGENTS.md

## Task Master AI Instructions
**Import Task Master's development workflow commands and guidelines, treat as if import is in the main CLAUDE.md file.**
@./.taskmaster/CLAUDE.md

## 执行环境例外

- 本项目允许在 Linux 上进行编辑、审查、提交和推送。
- 其余操作（编译、测试、下载依赖等）仍在 Windows 上执行，通道见下一节。

## 远程执行：DeskBridge

里程碑一完成后（2026-10-14），开发本项目用 DeskBridge 自己连接 Windows（详见 `docs/development.md` 的“远程执行通道”）。只允许访问 D 盘下的项目。

- 台式机的设备名是 `desktop`（项目在 `D:/data/AI/Projects/`），笔记本是 `work-laptop`（项目在 `D:/Program/AIProject/`）。命令行用 `-d desktop` 或环境变量 `DESKBRIDGE_DEVICE=desktop`。
- `deskbridge devices`：列出设备
- `deskbridge exec -d desktop -- <命令>`：默认用 PowerShell，`--shell cmd` 改用 cmd
- `read` / `write` / `list` / `stat` / `ps` / `kill` / `info`：文件、进程和系统信息
- `deskbridge call -d desktop --tool <名称> --args '<json>'`：调用任意工具
- 超过 10 分钟的命令放到后台写日志（`--shell cmd` 加 `start /b cmd /c "... > 日志 2>&1"`），再用 `read` 轮询。
- 选项写在位置参数前面（Go flag 包的限制）。

**备用通道：** AgentDesk 暂时保留。两台 Windows 的节点都由计划任务手动启动（台式机 `DeskBridgeNodeDev`，笔记本 `DeskBridgeNode`；自启在任务 8 实现），节点起不来或更新失败时用 `agentdesk`（台式机 `win-dev`，笔记本加 `AGENTDESK_MACHINE=laptop`）处理。更新已部署节点的步骤见 `docs/development.md`。
