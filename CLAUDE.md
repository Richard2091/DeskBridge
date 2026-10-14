# Claude Code Instructions

项目协作约定见 @./AGENTS.md

## Task Master AI Instructions
**Import Task Master's development workflow commands and guidelines, treat as if import is in the main CLAUDE.md file.**
@./.taskmaster/CLAUDE.md

## 执行环境例外

- 本项目允许在 Linux 上进行编辑、审查、提交和推送。
- 其余操作（编译、测试、下载依赖等）仍通过 `agentdesk` 在 Windows 执行。

## AgentDesk 远程执行（临时）

DeskBridge 里程碑一完成、改用 DeskBridge 自身后，删除本节。详见 `docs/development.md` 的"远程执行通道"。与 `/root/CloudCLI/CLAUDE.md` 中的同名一节保持一致，删除时一并删除。

用 `agentdesk` 命令操作 Windows，只允许访问 D 盘下的项目（computer 即 `win-dev` 在 `D:/data/AI/Projects/`，laptop 用 `AGENTDESK_MACHINE=laptop`，在 `D:/Program/AIProject/`）：
- `agentdesk capability`：查询执行器能力
- `agentdesk exec <命令>` / `read <路径>` / `write <路径> <内容>` / `list <目录>` / `ps`
- `agentdesk call --tool <名称> --args '<json>'`：调用任意工具
