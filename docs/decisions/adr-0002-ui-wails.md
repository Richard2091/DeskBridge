# ADR-0002：桌面客户端界面框架

- 状态：**已接受（2026-10-06）：Wails v3 + TypeScript**。用户要求继续并完成 Claude session 中的剩余任务，实测结果支持这个推荐。整体架构据此定型，可以进入项目骨架开发；v3 beta 风险用锁定版本和 CI 冒烟测试控制
- 日期：2026-10-06

## 背景

需要一个桌面客户端：登录、设备列表（显示连接方式和延迟）、执行命令、看截图、看操作记录，后期显示远程画面（实时视频 + 鼠标键盘）。希望只维护一套界面代码，和 Go 主程序集成方便，前端最好能复用为网页控制台。

## 当前范围（用户 2026-10-06 决定）

只考虑两种场景：

- **Linux 控制 Windows：** Linux 服务器上的 AI 通过命令行和 MCP 操作 Windows。Linux 上不需要图形界面。
- **Windows 控制 Windows：** 在 Windows 上用桌面客户端操作另一台 Windows。

**Linux 的图形界面（Linux 上的桌面客户端、操作 Linux 桌面）暂时搁置。** 所以客户端框架现在只按 Windows 评估；Linux 上 WebKitGTK 的问题（见下）记录在案，以后恢复 Linux 桌面时再处理。

## 网上调研结果（2026-10-06）

以下来自子代理整理的网上资料（GitHub 发布页、官方文档等），**未经实测**。

### 候选

| | Wails v3 | Tauri v2 | Flutter |
|---|---|---|---|
| 版本 | v3.0.0-beta.28（2026-10-05），**尚未正式发布**；v2.14.0 稳定 | 2.12.1（2026-09-30），稳定；v3 是 alpha | 3.47.6（2026-10-01），稳定 |
| 和 Go 主程序 | **同一进程**，直接调用 | Go 只能作为子进程（sidecar）运行 | FFI（需 cgo 共享库）或子进程 |
| 和 Rust 辅助进程 | Go 启动子进程 | 主进程就是 Rust | 可用 `flutter_rust_bridge` |
| Windows 界面 | WebView2（Chromium） | WebView2 | 自绘，不依赖 WebView |
| 托盘、全局快捷键、多窗口、自动更新、签名 | 都内置 | 有（更新强制签名） | 需第三方插件（未详细查） |
| 网页控制台 | `-tags server` 可把同一份前端编成网页服务 | 前端是网页，可复用 | 网页版兼容性一般 |
| 远程画面 | WebView2 支持 WebRTC 和 WebCodecs | 同左 | RustDesk 已验证：Rust 解码 + Flutter 纹理显示 |
| 体积 | 官方称约 15 MB（未实测） | 未查到官方数字 | RustDesk 安装包 25.9 MB（参考） |
| 许可证 | MIT | Apache-2.0 / MIT | BSD-3 |
| 主要代价 | 测试版的风险 | 多一个进程和通信层 | 多一门 Dart 语言 |

### 排除

| 框架 | 原因 |
|---|---|
| Electron | 体积大（自带 Chromium，压缩包 120 MB 以上）。保留为远程画面的最后退路 |
| Avalonia | 多一门 C# |
| Qt | C++ 维护成本高；LGPL/GPL 义务影响商业授权 |
| Slint | 官方不建议做网页；与“AGPL + 商业授权”的关系需向官方确认 |
| Fyne、Gio | 没有视频方案；Gio 未到 1.0 |
| egui、iced | 接口仍在变化，没有视频控件 |

### 远程画面

- Windows 的 WebView2 是 Chromium 内核，WebRTC 和 WebCodecs 都可用，Wails、Tauri 在 Windows 上没有障碍。
- Linux 的 WebKitGTK 默认不启用 WebRTC（预计 2027 年才默认开启），Wails、Tauri 在 Linux 上做远程画面有风险。当前范围不涉及 Linux 图形界面，暂不处理。
- 建议的传输方式：在 `tsnet` 连接上直接传 H.264 帧，界面用 WebCodecs（或 Canvas 显示 JPEG）解码，不依赖 WebRTC。
- 参考：RustDesk 先降码率再降帧率（最低 5 帧），以 150ms 延迟为阈值。

## 实测结果

实测已完成，数据和环境见 [`spikes/results.md`](../../spikes/results.md) 的“客户端框架实测”。三个候选都完成登录、设备列表、执行命令和远程画面流。Wails v3 达到 10 fps、exe 11.7 MB、MSIX 4.7 MB；Tauri v2 达到 9.3–9.4 fps、exe 5.96 MB；Flutter 达到 9.0–10.0 fps、release 目录 35.7 MB。

选择 Wails v3 的决定性原因是：Go 核心可以留在同一进程，没有 sidecar 通信层；同一份 TypeScript 前端可复用为网页控制台；体积和帧率满足早期客户端目标。Flutter 内存最低，但引入 Dart 且网页复用较弱。Tauri 更稳定，但 Go 必须作为 sidecar，集成边界更复杂。Wails v3 仍是 beta，这是主要风险。

## 实测记录

在 Windows 上为每个候选做同样的小程序：

1. 登录页 + 设备列表（假数据，显示连接方式和延迟）+ 调用一次 Go 核心（执行一条命令）。
2. 远程画面：Go 每秒推送 5–10 帧截图，界面显示；记录实际帧率、CPU 和内存。
3. 记录安装包或程序体积、编译时间、AI 第一次就写对的比例。

结果已写入 `spikes/results.md`。正式客户端应锁定 Wails v3 版本；升级或切换框架前必须重跑远程画面和高延迟场景。

## 风险

- 客户端界面是可选的：核心功能（节点、命令行、MCP）不依赖界面框架，无界面环境只构建命令行版本。
