# 原型 AgentDesk 的经验

原型位置：`/root/projects/agentdesk`（Python，GitHub `Richard2091/agentdesk`）。只作参考，不迁移代码。

## 原型做到了什么

- 0.1 版消息协议：消息信封、`request_id`、能力报告、请求状态、取消，有 JSON Schema。
- Windows 执行器的 12 个工具：系统信息、文件读写列表、执行命令、进程列表、Git 状态/差异/日志、环境变量、代码搜索、截图。
- 2026-10-05 至 10-06 完成“传输轨道”：放弃 SSH 反向隧道，改用 Tailscale。Linux 客户端支持多机器配置、连接重试、超时和分层 `ping` 诊断。

## 已验证的事实（2026-10-06）

- 一台 Linux 服务器（北美）和一台 Windows（中国大陆）通过 Tailscale 互通。
- 两边都无法直连：Windows 在运营商 NAT 后面（`MappingVariesByDestIP: true`），走香港中继，往返约 230ms。
- 通过 Tailscale 测试成功：`ping`、`capability`、`list`、`read`、`exec`、`ps`。`write` 未测试。
- Windows 上没有安装 OpenSSH Server。

## 学到的经验

1. **12 个工具里有 11 个，SSH + PowerShell 都能做。** 只有截图需要在用户的桌面会话里运行。所以新项目的价值必须放在桌面操作上，命令和文件只是基础。
2. **中文 Windows 的编码问题是真实存在的。** 原型在一次需要管理员权限的命令中返回了乱码（GBK 未解码）。新项目必须统一输出 UTF-8。
3. **共享密钥（HMAC）不好管理。** 原型的默认密钥曾被写进公开仓库。新项目用组网层的身份（`WhoIs`）取代共享密钥。
4. **工具名不应带平台前缀**（原型是 `windows.file.read`）。跨平台后应该是 `fs.read`，平台差异放在能力报告里。
5. **节点要在桌面会话里运行才能截图。** 原型能截图，是因为执行器是用户在自己的桌面里手动启动的。
6. **文档治理很有用，但原型的规则过重。** 新项目保留：中文文档、决策记录、进度跟踪、Conventional Commits；进度改用 TaskMaster 跟踪。

## 可以沿用的设计

- 能力报告 + 每个工具带 JSON Schema。
- `request_id`、超时、取消、结构化错误（错误码 + 说明）。
- 分层诊断：网络连接 → 身份认证 → 协议版本，`ping` 能指出是哪一层的问题。
