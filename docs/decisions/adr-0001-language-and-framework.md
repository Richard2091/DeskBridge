# ADR-0001：语言和框架

- 状态：**待验证**。调研已完成（2026-10-06），需要做技术验证后决定
- 日期：2026-10-06

## 需求

内置组网（Tailscale，后续 WireGuard）、一个程序同时做控制端和被控端、Windows 和 Linux（macOS 预留）、截图和输入、读取界面元素（UI Automation）、后期远程画面（视频编码 + WebRTC）、桌面客户端界面、给 AI 用的 MCP 接口。

## 调研结论

### 1. 内置组网是最硬的约束

- **能用于正式产品的内置 Tailscale 只有 Go 的 `tsnet`。**
- 其他语言的选择都是包装：`libtailscale` 是把 Go 代码编译成 C 库，Rust（`tsnet`、`rsnet` crate）、C#、Python 都通过它调用，**构建时仍然需要 Go 工具链**。
- Tailscale 官方在 2026 年 4 月发布了 Rust 版 `tailscale-rs` 预览，带 Python、Elixir、C 绑定。但官方明确说它是实验性的（pre-alpha），**完成第三方安全审计前不要用于正式产品**，而且当时只支持 Linux 和 macOS，不支持 Windows。8 月的官方文章仍然称它为实验性。
- 结论：选任何非 Go 语言，组网部分都要么依赖 C 包装，要么依赖实验性代码。

### 2. 桌面操作（Windows UI Automation）是核心价值

| 语言 | 可用的库 | 评价 |
|---|---|---|
| C# / .NET | 系统自带 UI Automation，`FlaUI`（MIT） | **最好**，微软亲生，资料最多 |
| Rust | `uiautomation` crate（基于 `windows-rs`） | 好，类型完整，有遍历元素树的示例 |
| Python | `pywinauto`、`uiautomation` | 好（微软 UFO、Windows-MCP 都用 Python），但打包和分发差 |
| Go | `go-element`、`w32uiautomation`、手写 `go-ole` | **最弱**，库小众，很多接口要手写 |

### 3. 其他方面

| 方面 | 结论 |
|---|---|
| WireGuard | Go 有官方 `wireguard-go`（Tailscale 也基于它）；Rust 有 `boringtun`。两者都成熟 |
| WebRTC | Go 的 `pion` 最成熟；Rust 的 `webrtc-rs` 次之；C# 有 `SIPSorcery` |
| 桌面界面 | Wails（Go）：v2 稳定，v3 仍是 beta；Tauri（Rust）：成熟；Avalonia（C#，MIT）：成熟，跨平台；Electron：体积大 |
| 杀毒误报 | Go 程序被误报是公认问题（Syncthing、rclone 等都遇到过）。所有语言都需要代码签名来缓解，Go 更需要 |
| 编译资源 | 开发在 Windows 上进行（12 线程、32GB），不是限制因素 |
| AI 生成代码 | Go 和 C# 一次成功率高；Rust 常需要多轮修正 |

### 4. 被排除的选项

- **Python：** 原型用过。打包体积大、启动慢、易被误报；组网要依赖 C 包装。
- **TypeScript / Electron：** 体积大，没有内置组网，桌面操作要依赖原生插件。
- **C++（Qt）：** 能做好，但开发慢，内存安全问题多，不适合一个人加 AI 的开发方式。

## 候选方案

| 方案 | 组网 | 桌面操作 | 界面 | 主要代价 |
|---|---|---|---|---|
| **A. 全部 Go** | 最好 | 最弱 | Wails | 桌面操作要自己补库 |
| **B. Go 主程序 + 桌面辅助进程（C# 或 Rust）** | 最好 | 最好 | Wails 或网页控制台 | 维护两种语言；Windows 上多一个进程 |
| **C. 全部 Rust** | 依赖 `libtailscale` 包装或实验性 `tailscale-rs` | 好 | Tauri | 组网是二手的；编译慢 |
| **D. 全部 C#** | 依赖 `libtailscale` 包装 | 最好 | Avalonia | 组网是二手的；Linux 上的桌面操作较弱 |

方案 B 的拆分点是现成的：架构里本来就计划“服务进程 + 用户会话中的桌面辅助进程”（见 architecture.md），两者用标准输入输出或命名管道传 JSON。辅助进程只负责一个平台的桌面操作，可以用该平台最合适的语言。

## 技术验证（决定前完成）

在 Windows 上，用 Go、Rust、C# 三种语言分别实现同一组桌面操作，比较代码量、稳定性、程序体积、杀毒误报和 AI 一次写对的比例：

1. 读取记事本窗口的界面元素树，按名称点击“文件”菜单。
2. 截取主显示器画面，150% 缩放下坐标正确。
3. 输入一段中文。

另外用 Go 验证 `tsnet`：在 Windows 上加入内网，提供一个 HTTP 接口，从 Linux 访问。

**判断规则：** 如果 Go 的桌面操作差距可以接受，选方案 A（一种语言最简单）；如果差距明显，选方案 B，辅助进程用验证中表现最好的语言。

## 参考来源

- Tailscale：tailscale-rs 预览（2026-04-15）https://tailscale.com/blog/tailscale-rs-rust-tsnet-library-preview
- Tailscale：Easier building with Tailscale（2026-08-31）https://tailscale.com/blog/easier-building-with-tailscale
- Wails 发布记录 https://github.com/wailsapp/wails/releases
- Rust `uiautomation` crate https://lib.rs/crates/uiautomation
- Go `go-element` https://github.com/auuunya/go-element
- Go 程序被杀毒误报的讨论 https://forum.golangbridge.org/t/my-compiled-exe-file-is-declared-as-a-virus/34038
