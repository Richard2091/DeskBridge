# 参考的开源项目与许可证

规则见 [ADR-0004](decisions/adr-0004-open-source-and-references.md)：**参考设计，不复制代码**；依赖库只用宽松许可证。

> 下表的许可证是根据已有了解整理的，**引入依赖或参考前需要再次在项目仓库中核实**。

## 参考设计（不复制代码）

| 项目 | 许可证 | 参考什么 |
|---|---|---|
| RustDesk | AGPL-3.0 | 远程画面的编码与传输、中继服务器设计、Windows 服务 + 会话辅助进程、自建服务器的部署体验 |
| MeshCentral | Apache-2.0 | 多设备管理、设备分组、事件日志、用户权限模型 |
| 微软 UFO | MIT | Windows 上 AI 操作桌面的方法：UI Automation 与截图结合 |
| Windows 桌面控制类 MCP 服务器（如 Windows-MCP） | 待核实 | AI 桌面工具的设计：工具粒度、返回格式 |
| Anthropic Computer Use 参考实现 | 待核实 | 截图 + 坐标点击的工具接口 |
| Tailscale 客户端 | BSD-3 | 登录流程、设备列表的交互 |

## 计划使用的依赖库

| 库 | 用途 | 许可证 |
|---|---|---|
| `tailscale.com/tsnet` | 内置组网 | BSD-3 |
| Headscale（部署，不作为代码依赖） | 自建控制服务器 | BSD-3 |
| `derper`（Tailscale 仓库内） | 自建中继 | BSD-3 |
| Wails v3（客户端候选，未决策） | 桌面客户端 | MIT |
| `github.com/pion/webrtc` | 远程画面（后期） | MIT |
| `modernc.org/sqlite` | 本地存储 | BSD-3 |
| `github.com/zalando/go-keyring` | 系统密钥库 | MIT |
| `github.com/go-ole/go-ole` | Windows COM / UI Automation | MIT |
| `github.com/kbinani/screenshot` | 截图 | MIT |
| MCP 官方 Go SDK（`github.com/modelcontextprotocol/go-sdk`） | MCP 服务器 | 待核实 |

## 参考记录

开发中每参考一个项目的某部分设计，在这里追加一行：日期、项目、参考内容、对应的 DeskBridge 模块。

| 日期 | 项目 | 参考内容 | DeskBridge 模块 |
|---|---|---|---|
| | | | |
