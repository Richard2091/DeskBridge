# 参考的开源项目与许可证

规则见 [ADR-0004](decisions/adr-0004-open-source-and-references.md)：**参考设计，不复制代码**；依赖库只用宽松许可证。

> 下表的许可证是根据已有了解整理的，**引入依赖或参考前需要再次在项目仓库中核实**。

## 参考设计（不复制代码）

| 项目 | 许可证 | 参考什么 |
|---|---|---|
| RustDesk | AGPL-3.0 | 远程画面的编码与传输、中继服务器设计、Windows 服务 + 会话辅助进程、自建服务器的部署体验 |
| MeshCentral | Apache-2.0 | 多设备管理、设备分组、事件日志、用户权限模型 |
| 微软 UFO | MIT | Windows 上 AI 操作桌面的方法：UI Automation 与截图结合 |
| Windows-Use（Jeomon/Windows-Use） | MIT | UIA 无障碍树给 LLM 的序列化格式（扁平表 + 临时 id）、CacheRequest 批量取属性、按窗口隔离失败、Agent 循环防重复检测 |
| Windows 桌面控制类 MCP 服务器（如 Windows-MCP） | 待核实 | AI 桌面工具的设计：工具粒度、返回格式 |
| Anthropic Computer Use 参考实现 | 待核实 | 截图 + 坐标点击的工具接口 |
| Tailscale 客户端 | BSD-3 | 登录流程、设备列表的交互 |
| ToDesk、网易 UU 远程（闭源产品，只参考公开的界面） | 不适用 | 每个远程会话一个独立窗口，可以同时操作多台设备（用户提出参考，这两款产品的具体做法未核实；任务 27） |

## 计划使用的依赖库

| 库 | 用途 | 许可证 |
|---|---|---|
| `tailscale.com/tsnet` | 内置组网 | BSD-3 |
| Headscale（部署，不作为代码依赖） | 自建控制服务器 | BSD-3 |
| `derper`（Tailscale 仓库内） | 自建中继 | BSD-3 |
| Flutter（Dart） | 桌面客户端、安卓控制端 | BSD-3 |
| `github.com/pion/webrtc` | 远程画面（后期，是否使用随任务 15 决定） | MIT |
| `modernc.org/sqlite` | 本地存储 | BSD-3 |
| `github.com/zalando/go-keyring` | 系统密钥库 | MIT |
| `github.com/go-ole/go-ole` | Windows COM / UI Automation | MIT |
| `github.com/kbinani/screenshot` | 截图 | MIT |
| MCP 官方 Go SDK（`github.com/modelcontextprotocol/go-sdk`） | MCP 服务器 | 新贡献 Apache-2.0、原有代码 MIT（已核实仓库说明，2026-10-13） |
| `golang.org/x/text` | Windows 命令输出按代码页（如 GBK）解码 | BSD-3 |
| `golang.org/x/sys` | Windows 系统调用（进程、作业对象、代码页） | BSD-3 |

远程画面补充调研（`spikes/stream/`，任务 24）的验证代码另外用到下面几个库，许可证已在 crate 的 `Cargo.toml` 中核实（2026-10-07）。间接依赖未逐个核实。

| 库 | 用途 | 许可证 |
|---|---|---|
| `windows`（Rust，0.62） | DXGI 采集、D3D11 视频处理、Media Foundation 编解码 | MIT OR Apache-2.0 |
| `serde_json`（Rust） | 输出测量结果 | MIT OR Apache-2.0 |
| `png`（Rust） | 保存解码后的检查画面 | MIT OR Apache-2.0 |

开发工具（只在检查脚本和 CI 中通过 `go run` 运行，不编进程序）：

| 工具 | 用途 | 许可证 |
|---|---|---|
| staticcheck（`honnef.co/go/tools`，2026.2.1） | Go 静态检查 | MIT（已核实 LICENSE，2026-10-07） |

H.264 编码只用系统自带的 Media Foundation 编码器（显卡厂商的硬件编码器和微软软件编码器），不引入 openh264、x264 等编码库。

## 参考记录

开发中每参考一个项目的某部分设计，在这里追加一行：日期、项目、参考内容、对应的 DeskBridge 模块。

| 日期 | 项目 | 参考内容 | DeskBridge 模块 |
|---|---|---|---|
| 2026-10-07 | RustDesk | 硬件编码优先、软件编码兜底；按目标帧率采集，只发最新一帧（只参考思路，未阅读代码） | `spikes/stream/`（任务 24.2） |
