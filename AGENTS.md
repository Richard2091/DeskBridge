# DeskBridge 协作约定

适用于在本项目中工作的 AI Agent 和开发者。

## 每次会话开始时

1. 读 `README.md` 和 `docs/vision.md`，了解定位。
2. 运行 `task-master list` 和 `task-master next`，确认当前进度和下一个任务。
3. 读和当前任务相关的文档：`docs/architecture.md`、`docs/decisions/` 中的相关 ADR。

## 进度记录

- 进度只在 TaskMaster 里记录，不另建进度文档。
- 开始任务：`task-master set-status --id=<id> --status=in-progress`。
- 过程记录：用 `task-master update-subtask --id=<id> --prompt="..."` 记下做了什么、遇到什么问题、验证结果。若 AI 命令不可用，在任务完成时把要点写进提交信息。
- 完成任务：验收标准全部满足并已提交后，才能设为 `done`。

## 文档

- 文档使用中文，UTF-8。
- 改变架构、协议、组网或安全边界时，同步更新 `docs/architecture.md`；重要且难以撤销的决定写成新的 ADR（`docs/decisions/adr-NNNN-*.md`）。
- 不把猜测写成事实：没有验证过的内容标为“待验证”。

## 代码

- 语言：Go 主程序 + Rust 写的 Windows 桌面辅助进程（已确认，见 ADR-0001 和 `spikes/results.md`）。客户端界面用 Flutter（ADR-0002），启动 Go 程序作为子进程通信。
- 平台相关代码放在 `_windows.go`、`_linux.go`、`_darwin.go` 中；未实现的平台返回 `unsupported_platform`，不能静默失败。
- **只参考其他开源项目的设计，不复制代码**（见 ADR-0004）。新增依赖前检查许可证，并更新 `docs/references.md`。
- 所有返回给 AI 的文本必须是 UTF-8。

## 密钥和隐私

- Git 仓库中只放脱敏信息和示例配置，真实凭据放在用户目录或系统密钥库中。
- 不在日志、测试数据、文档和提交中写入真实的密钥、令牌、IP 地址、设备名或账号。项目会开源，用示例值代替（如 `100.64.0.1`、`win-pc`）。

## 提交

- 每完成一个可验证的步骤就提交一次，只包含相关文件。
- 提交信息使用 Conventional Commits：类型用英文，描述用中文。例如 `feat: 增加 tsnet 节点登录流程`。
- 日常推送到 GitHub 私有仓库 `Richard2091/DeskBridge` 不需要逐次确认；把仓库改为公开、推送到其他远程仓库前，必须先征得用户同意。流程见 `docs/development.md`。
