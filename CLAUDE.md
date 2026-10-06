# Claude Code Instructions

项目协作约定见 @./AGENTS.md

## Task Master AI Instructions
**Import Task Master's development workflow commands and guidelines, treat as if import is in the main CLAUDE.md file.**
@./.taskmaster/CLAUDE.md

## AgentDesk 远程执行（临时）

DeskBridge 里程碑一完成、改用 DeskBridge 自身后，删除本节。详见 `docs/development.md` 的“远程执行通道”。与 `/root/CloudCLI/CLAUDE.md` 中的同名一节保持一致，删除时一并删除。

用 `agentdesk` 命令操作 Windows，只允许访问 `D:/data/AI/Projects/` 下的项目：
- `agentdesk capability`：查询执行器能力
- `agentdesk exec <命令>` / `read <路径>` / `write <路径> <内容>` / `list <目录>` / `ps`
- `agentdesk call --tool <名称> --args '<json>'`：调用任意工具
