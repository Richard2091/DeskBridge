# DeskBridge {{VERSION}}（预览版）

这是 DeskBridge 的预览版，功能还不完整，仅供试用。

## 能做什么

- 桌面客户端自带节点，打开就能把本机加入你的 Tailscale 网络。
- 设备列表：查看网络里的 DeskBridge 设备。
- 终端：在远程设备上运行命令。
- 文件：浏览远程设备上的文件，预览文本文件（还不能上传和下载）。
- AI 接入：设置里提供 MCP 配置，让 Claude Code 等 AI 客户端通过 DeskBridge 操作设备。
- 操作记录：查看对本机和远程设备做过的操作。

## 已知限制

- 没有代码签名：首次运行会弹出 Windows SmartScreen，点“更多信息”，再点“仍要运行”。
- 没有自动更新：新版本需要手动下载安装。
- 还没有截图和远程画面。
- Tailscale 需要在客户端里登录。

## 安装方式

- Windows 安装程序：运行 `DeskBridge-{{VERSION}}-windows-amd64-setup.exe`，按当前用户安装，不需要管理员权限。可选“登录后在后台运行节点”。
- Windows 免安装压缩包：解压 `DeskBridge-{{VERSION}}-windows-amd64.zip`，运行 `deskbridge_client.exe`。
- Linux：`DeskBridge-{{VERSION}}-linux-amd64.tar.gz` 或 `-arm64.tar.gz`，里面是命令行程序 `deskbridge`（没有图形客户端）。

## 校验

所有附件的 SHA256 见 `SHA256SUMS.txt`。
