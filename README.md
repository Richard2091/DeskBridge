# DeskBridge

让 AI Agent 和人都能顺畅操作多台设备的开源控制平台：命令、文件和图形桌面，跨 Windows 和 Linux（macOS 预留）。

> 状态：设计阶段，尚无可运行代码。原型 AgentDesk（Python）保留在 `/root/projects/agentdesk`，只作参考。

## 要解决的问题

AI Agent 通过 SSH 操作 Linux 服务器已经很顺畅，但 Windows 这类以图形界面为主的设备，今天基本只能由人用鼠标键盘操作：

- SSH 能跑命令，却碰不到用户正在使用的桌面，很多软件只有图形界面。
- 中文 Windows 的命令输出编码（GBK）、PowerShell 和 cmd 的差异，让 AI 很容易出错。
- 多台设备分散在不同网络里，打通连接要么付费（ToDesk 等），要么要自己折腾组网。

## DeskBridge 做什么

1. **一个客户端，自带组网。** 内置 Tailscale（`tsnet`），用 OAuth 账号登录后自动列出你的所有设备，个人用户的设备之间免费互通。可以改用自建的 Headscale 和中继。
2. **AI 和人用同一套操作。** 命令、文件、截图、点击、输入、读取界面元素，都是统一的工具。AI 通过 MCP 调用，人通过桌面客户端或命令行调用。
3. **桌面级控制。** 节点运行在用户的桌面会话里，所以 AI 能看到并操作人正在用的那个桌面。
4. **加分项（后续）：** 操作记录、权限、危险操作审批、他人设备共享、人工远程画面。

## 文档

- [项目定位](docs/vision.md)
- [总体架构](docs/architecture.md)
- [路线图](docs/roadmap.md)
- [架构决策（ADR）](docs/decisions/)
- [参考的开源项目与许可证](docs/references.md)
- [原型 AgentDesk 的经验](docs/prototype-lessons.md)
- [开发方式（Linux 编辑、Windows 编译）](docs/development.md)

## 进度跟踪

任务和进度用 TaskMaster 管理（`.taskmaster/tasks/tasks.json`）：

```bash
task-master list          # 全部任务
task-master next          # 下一个可做的任务
task-master show <id>     # 任务详情
```

## 许可证

[AGPL-3.0](LICENSE)。作者保留另行提供商业授权的权利，见 [ADR-0004](docs/decisions/adr-0004-open-source-and-references.md)。
