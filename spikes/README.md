# 技术验证（任务 #21）

一次性的验证代码，**不是正式代码**，不会被正式代码引用。目的和判断规则见 [ADR-0001](../docs/decisions/adr-0001-language-and-framework.md)。

## 桌面操作验证（Go、Rust、C# 各一份）

每个程序做同样的事，最后输出一段 JSON：

1. 设置为“每显示器 DPI 感知”，记录屏幕的物理分辨率。
2. 在临时目录建一个空文本文件，用记事本打开它（只操作这个文件，不碰用户的其他文件）。
3. **界面元素：** 读取记事本窗口的元素树（深度不超过 6），统计节点数。
4. **按名称点击：** 找到名称以“文件”或“File”开头的菜单项，用 UI Automation 的展开操作打开，确认弹出了菜单，然后关闭。
5. **按坐标点击：** 用元素报告的位置，把鼠标移到“文件”菜单中心点击，确认菜单弹出。这一步检查 150% 缩放下坐标是否一致。
6. **输入中文：** 把焦点放到编辑区，输入 `你好，DeskBridge 测试 123`，按 Ctrl+S 保存，再从磁盘读回文件内容比较。
7. **截图：** 截取主显示器，保存为 PNG，记录图片尺寸。
8. 关闭记事本。

运行时记事本会在桌面上打开并被自动操作，持续几秒钟。

| 目录 | 语言 | 主要依赖 |
|---|---|---|
| `cs-desktop/` | C# (.NET 10) | FlaUI.UIA3（MIT） |
| `rust-desktop/` | Rust | `uiautomation`、`xcap`、`windows` |
| `go-desktop/` | Go | `go-element`（MIT）、`kbinani/screenshot`（MIT）、`golang.org/x/sys` |

## 组网验证

| 目录 | 内容 |
|---|---|
| `go-tsnet/` | Windows 上用 `tsnet` 加入内网并提供 HTTP 接口，从 Linux 访问 |

## 远程画面补充调研（任务 24）

| 目录 | 内容 |
|---|---|
| `stream/` | 采集、编码、经 Tailscale 传输和各框架解码显示的测量；测试目标和方法见 [stream/README.md](stream/README.md) |

## 结果

见 [results.md](results.md)（验证完成后填写）。
