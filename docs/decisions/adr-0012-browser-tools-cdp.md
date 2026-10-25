# ADR-0012：浏览器工具走原生 CDP，默认使用系统默认浏览器

- 状态：**已接受（2026-10-25）**
- 日期：2026-10-25

## 背景

DeskBridge 已经规划了 Windows 桌面工具（任务 9、10），但 AI 在设备上做的大部分事情是网页：登录、填表、查资料、截图、验证页面。原有计划里没有浏览器能力，AI 只能靠“截图 + 坐标点击”操作浏览器窗口，既慢又不可靠。

用户要求：

1. 加入浏览器自动化能力；
2. **操作被控端浏览器时，调用系统设置的默认浏览器**，不要写死 Chrome。

外部可选方案有 Playwright（多浏览器、自动等待、trace）、chrome-devtools-mcp（性能、网络、Lighthouse）和 Windows-MCP（UIA）。它们都是 Node/Python 运行时或桌面 API 的封装，和 DeskBridge“单二进制、跨平台、可审计”的目标冲突（详见 ADR-0001 的分层原则和任务 22 的 MCP 转发计划）。

## 决定

1. **浏览器工具放进 Go 内核，直接用 Chrome DevTools Protocol（CDP）。**
   - 不引入 Playwright、Puppeteer、Node 或 Python 运行时；不新增第三方 CDP 客户端依赖，只复用已有的 `coder/websocket` 和 `golang.org/x/sys`。
   - 工具名为 `browser.*`，来源仍是内置工具（`builtin`），参数和返回值见 `docs/tools.md`。
   - 浏览器操作属于“不干扰”类操作，不需要桌面控制权（ADR-0005）；无头模式不依赖桌面会话，服务模式下同样可用。

2. **默认启动系统设置的默认浏览器。**
   - Windows：读 `HKCU\Software\Microsoft\Windows\Shell\Associations\UrlAssociations\http\UserChoice` 的 ProgId，再读 `HKCR\<ProgId>\shell\open\command` 得到可执行文件。
   - Linux：`xdg-settings get default-web-browser` / `xdg-mime query default x-scheme-handler/http`，再解析 `.desktop` 的 `Exec`。
   - macOS：读 LaunchServices 的 LSHandlers（未在真机验证，先按已知浏览器路径兜底）。
   - 只有 Chromium 内核支持 CDP。默认浏览器是 Firefox、Safari 等时无法自动化：返回明确提示，并在 `allow_fallback`（默认开启）时退回本机已知的 Chromium 浏览器；关闭回退则报 `unsupported_platform`。
   - 启动默认浏览器时**使用独立的用户数据目录**（`browser.json` 的 `profile_mode`，默认 `isolated`），不接管用户正在使用的窗口和登录态。Chrome 136 起，默认 profile 下 `--remote-debugging-port` 会被忽略，独立目录也是硬性要求。

3. **默认无头运行（`--headless=new`），屏幕看不到浏览器窗口。**
   - 需要看画面时用 `browser.screenshot`；以后接“AI 画面查看”（任务 30）时复用截图。
   - 有头模式（`headless: false`）留给以后和桌面控制权仲裁（任务 23）一起做。

4. **会话显式建模。**
   - `browser.open` 返回 `session`，其余工具都带这个参数；会话归属调用者（连接方式 + 主人 + 设备），别人不能操作。
   - 默认最多 2 个会话，空闲 10 分钟自动关闭；节点退出时关闭全部会话（Windows 作业对象、Linux Pdeathsig 兜底结束浏览器进程）。
   - 浏览器崩溃后会话不可恢复，调用返回 `unavailable`，重新 `browser.open` 即可。

5. **安全边界。**
   - 默认只允许 `http`、`https`、`about`、`data` 四种 scheme，`file://` 默认禁止（可在 `browser.json` 的 `allowed_schemes` 中显式加入）。
   - `browser.eval` 等同于在该页面执行任意 JavaScript，权限上按“执行命令”对待；调用记录会记下截断后的表达式，操作记录按任务 17 的权限模型再细分。
   - Cookie 的 `value` 在操作记录里脱敏。
   - 不接管用户默认 profile，避免把用户已登录的会话暴露给 AI。
   - **不碰用户正在使用的窗口：** 每个会话启动独立的浏览器实例（独立用户数据目录、默认无头），不关闭、不聚焦、不操作用户已有的浏览器窗口；`browser.close` 只结束本会话启动的实例（Windows 每个实例一个作业对象，Linux/macOS 一个进程组），节点退出时也只回收自己启动的进程。

6. **不做的部分：** 多浏览器引擎（Firefox/WebKit）、Lighthouse、trace viewer、视频录制、附着到用户正在使用的标签页。这些能力需要 Playwright 或 DevTools 生态，以后按任务 22（MCP 转发）作为可选来源接入，不进安装包。

## 影响

- 新增 `internal/browser/`（配置、默认浏览器、启动、CDP 客户端、会话、快照、动作、控制台/网络采集）和 `internal/tools/builtin/browser.go`（15 个工具）。
- 默认浏览器解析在 Windows 上做成注册表读取，Linux 走 xdg；macOS 待验证。
- 新增数据目录下的 `browser.json` 配置和 `browser/` 子目录（会话 profile、截图）。
- 工具数量增加 15 个，`device_tools` 能直接看到；`deskbridge mcp` 不需要改动（`call_tool` 通用）。
- 任务 9、10（Rust 桌面辅助进程）不受影响，可并行；任务 22 的 MCP 转发仍是 Playwright/DevTools/Windows-MCP 的正规入口。

## 未采用的方案

- **内置 Playwright / chrome-devtools-mcp：** 需要 Node 运行时和数百 MB 浏览器/driver，破坏单二进制和“下载就能用”；第三方工具版本漂移会让 AI 侧的 schema 不稳定。
- **只做 MCP 转发：** 需要目标机先装 Node 和对应 MCP 服务器，体验门槛高；且第三方桌面输入绕过 ADR-0005 的仲裁。
- **用 chromedp 等 Go 库：** 能省一些代码，但会新增依赖和版本约束；本项目需要的命令集很小，手写 JSON 客户端更容易控制会话、事件和错误映射。
- **附着到用户正在用的浏览器：** 需要用户以调试模式重启浏览器，且会把登录态暴露给 AI，初版不做。
