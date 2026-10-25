# 内置工具

节点内置的工具，参数和返回值即线上格式（JSON 键名）。通用约定见 `docs/architecture.md` 的“节点协议”一节。

## 通用约定

- **参数严格解析：** 未知字段返回 `invalid_args`。可选字段省略即取默认值。
- **路径：** 相对路径相对于节点进程的当前目录（不建议依赖），返回值中的 `path` 一律是绝对路径。Windows 上 `/` 和 `\` 都接受。
- **时间：** RFC 3339 字符串，带时区，如 `2026-10-14T21:05:00+08:00`。
- **错误：** 文件或进程不存在返回 `not_found`；没有权限返回 `permission_denied`（`details.missing` 为 `os_permission`，表示被操作系统拒绝）；参数不合理（如路径是目录却要读文件）返回 `invalid_args`。
- **超时：** 由调用请求的 `timeout_ms` 控制（协议层；缺省时用各工具自己的默认值，上限不超过全局 10 分钟），工具参数里不再另设超时。超时后 `proc.exec` 返回已收到的输出并标记 `timed_out`，其他工具返回 `timeout` 错误。取消一律返回 `cancelled`。
- **文本一律 UTF-8：** 见下文“编码”。

## sys.info

参数：无。

返回：

| 字段 | 说明 |
|---|---|
| `name` | 设备名称（`--name`） |
| `hostname` | 系统主机名 |
| `os` / `arch` / `os_version` | 同能力报告的 `platform` |
| `version` | DeskBridge 版本 |
| `user` | 节点进程的用户名（Windows 为 `域\用户`） |
| `home` | 用户目录 |
| `num_cpu` | 逻辑 CPU 数 |
| `memory_total_bytes` / `memory_available_bytes` | 内存；取不到时省略 |
| `boot_time` | 开机时间；取不到时省略 |
| `timezone` | 时区名，如 `Asia/Shanghai`；取不到时用 UTC 偏移，如 `+08:00` |
| `codepages` | 仅 Windows：`{"ansi":936,"oem":936}` |
| `session` | 桌面会话：`{"id":1,"interactive":true}`。Windows 上 `id` 为进程所在会话，`interactive` 表示它就是当前控制台会话；Linux 上 `interactive` 表示有 `DISPLAY` 或 `WAYLAND_DISPLAY` |

## fs.stat

参数：`path`（必填）。

返回：`path`、`type`（`file`、`dir`、`symlink`、`other`）、`size`、`mtime`、`mode`（如 `-rw-r--r--`）、`link_target`（仅符号链接）。不跟随符号链接。

## fs.list

参数：

| 字段 | 默认 | 说明 |
|---|---|---|
| `path` | 必填 | 目录 |
| `recursive` | `false` | 递归列出；不进入符号链接指向的目录 |
| `max_entries` | `1000` | 上限 10000 |

返回：`path`、`entries`、`truncated`。每个条目：`name`（递归时为相对于 `path` 的路径，用 `/` 分隔）、`type`、`size`、`mtime`。按 `name` 排序。递归时遇到无权访问的子目录跳过，不报错。

## fs.read

参数：

| 字段 | 默认 | 说明 |
|---|---|---|
| `path` | 必填 | 文件 |
| `offset` | `0` | 起始字节，用于分块读取大文件 |
| `max_bytes` | `1048576`（1 MiB） | 上限 8 MiB |
| `encoding` | `auto` | `auto`：是合法 UTF-8 就按文本返回，否则 base64；`utf-8`：按文本返回，不合法的字节替换为 U+FFFD；`base64`：总是 base64 |

返回：`path`、`content`、`encoding`（实际使用的 `utf-8` 或 `base64`）、`size`（文件总大小）、`offset`、`bytes_read`、`eof`。

- 按文本返回时，如果 `max_bytes` 截断在一个多字节字符中间，回退到字符边界，`bytes_read` 按回退后计算，下一块从 `offset + bytes_read` 继续即可。
- 带 UTF-8 BOM 的文件，`offset` 为 0 时去掉 BOM（`bytes_read` 仍包含 BOM 的 3 个字节）。
- 不做 GBK 等其他编码的猜测：这类文件用 `auto` 会得到 base64，调用方自行处理。

## fs.write

参数：

| 字段 | 默认 | 说明 |
|---|---|---|
| `path` | 必填 | 文件 |
| `content` | 必填 | 内容，可为空字符串 |
| `encoding` | `utf-8` | `utf-8` 或 `base64` |
| `append` | `false` | 追加到末尾，用于分块写入大文件 |
| `create_dirs` | `true` | 自动创建父目录 |

返回：`path`、`bytes_written`、`created`（文件原来不存在）。

- 覆盖写入时先写同目录下的临时文件再改名，中途失败不会留下写了一半的文件；保留原文件的权限位。
- 单次请求体上限 16 MiB（协议层），更大的文件用 `append` 分块写入。

## proc.exec

参数：

| 字段 | 默认 | 说明 |
|---|---|---|
| `command` | — | 命令行，交给 `shell` 执行。和 `argv` 二选一 |
| `argv` | — | 程序和参数，不经过 shell 直接执行。和 `command` 二选一 |
| `shell` | Windows `powershell`，其他 `sh` | `powershell`、`pwsh`、`cmd`、`sh`、`bash` |
| `cwd` | 节点当前目录 | 工作目录 |
| `env` | — | 追加或覆盖的环境变量，`{"NAME":"value"}` |
| `stdin` | 空 | 标准输入；不提供时为空输入，等待输入的命令会立即读到结束；`visible` 模式不支持 |
| `console` | `hidden` | 控制台窗口：`hidden` 不弹窗口；`visible` 在设备桌面上打开带标题的窗口显示命令行和实时输出（仅 Windows），此时不返回 `stdout`/`stderr` |
| `max_output_bytes` | `1048576`（1 MiB） | 标准输出和标准错误各自的上限，上限 8 MiB；超出部分丢弃 |

返回：

| 字段 | 说明 |
|---|---|
| `exit_code` | 退出码；超时或被取消而终止时为 `-1` |
| `stdout` / `stderr` | 已解码为 UTF-8 的输出 |
| `stdout_truncated` / `stderr_truncated` | 是否超过 `max_output_bytes` |
| `timed_out` | 是否因超时被终止 |
| `console` | 是否在可见窗口模式下运行；为 `true` 时没有 `stdout`/`stderr` |
| `duration_ms` | 执行耗时 |

- 命令启动不了（程序不存在、目录不存在）返回 `not_found` 或 `invalid_args` 错误；启动后非零退出不是错误，结果里看 `exit_code`。
- 超时或取消时终止整个进程树：Windows 用作业对象（Job Object），Linux 用进程组。
- 各 shell 的调用方式：
  - `powershell` / `pwsh`：`-NoLogo -NoProfile -NonInteractive -ExecutionPolicy Bypass -Command <前缀 + 换行 + 命令>`，前缀是把 `[Console]::InputEncoding`（让 `stdin` 按 UTF-8 读取）、`[Console]::OutputEncoding` 和 `$OutputEncoding` 设为 UTF-8 的语句。不用 `-EncodedCommand`：输出被重定向时它会让 PowerShell 把错误流和进度流序列化成 CLIXML 写到 `stderr`；`-Command` 下 `stderr` 是和控制台一致的纯文本。引号由 Go 按标准规则转义，PowerShell 同样规则还原。警告流（`Write-Warning`）在文本模式下写到 `stdout`，和控制台一致。
  - `cmd`：`cmd /d /s /c "<command>"`，自己拼命令行（Go 的 `SysProcAttr.CmdLine`），不让 Go 再转义一次。
  - `sh` / `bash`：`-c <command>`。
- **控制台窗口（Windows）：** 默认用 `CREATE_NO_WINDOW` 启动子进程，不弹窗口（节点自己没有控制台时，控制台程序会新开一个空白窗口，标题还是程序路径）。`console` 为 `visible` 时改用 `CREATE_NEW_CONSOLE` 启动内部的 `deskbridge console` 子命令：它在设备桌面上新建一个控制台窗口，标题是命令摘要（如 `DeskBridge · git clone …`），窗口里先回显一行模拟人工输入的提示符和命令，再把控制台交给真正的 shell，输出直接进窗口；命令结束后窗口保留 5 秒再关闭。
  - `visible` 只适合下载、安装、构建这类耗时长、需要人看着的命令：输出进了窗口，所以结果里拿不到 `stdout`/`stderr`，只有 `exit_code`、`duration_ms` 等；需要输出时必须用默认的 `hidden`（或在命令里把输出写进日志文件）。
  - 其他平台把 `console` 设为 `visible` 返回 `unsupported_platform`。

## proc.list

参数：`name`（可选，进程名包含该字符串，不区分大小写）、`max_entries`（默认 500，上限 10000）。

返回：`processes`、`truncated`。每个进程：`pid`、`ppid`、`name`、`memory_bytes`（常驻内存，取不到时省略）、`user`（取不到时省略）。按 `pid` 排序。

## proc.kill

参数：`pid`（必填）、`tree`（默认 `false`，连同子进程）、`force`（默认 `false`；Linux 上 `false` 发 SIGTERM、`true` 发 SIGKILL；Windows 上总是强制结束）。

返回：`killed`（实际结束的 PID 列表）。不允许结束节点进程自己（`invalid_args`）。

`tree` 为 `true` 时的行为：

- 子进程按父子关系和创建时间逐层核对（子进程不能比父进程更早创建，Windows 上快照里的父 PID 可能已过期）；核对不过的进程及其整棵子树都不结束，宁可漏杀也不误杀。
- 根进程没有被结束时返回错误（`not_found` 或 `permission_denied` 等）；子进程结束失败不算错误，`killed` 只列实际结束的。
- 节点自己及其子进程不会被结束。
- 只按 PID 指定目标：PID 在调用方查看之后被复用的情况无法识别（已知限制）。

## 浏览器工具

节点内置一组 `browser.*` 工具，经 Chrome DevTools Protocol（CDP）操作浏览器。**默认启动系统设置的默认浏览器**（Windows 读注册表的 UserChoice，Linux 用 `xdg-settings` / `xdg-mime`），默认无头运行，用独立的配置档案，不接管用户正在使用的窗口。默认浏览器不是 Chromium 内核（如 Firefox、Safari）时无法用 CDP 自动化：`browser.open` 会在 `warnings` 里说明，并按 `allow_fallback`（默认开启）退回本机已知的 Chromium 浏览器；关闭回退时返回 `unsupported_platform`。

配置在数据目录下的 `browser.json`，字段都可缺省：

| 字段 | 默认 | 说明 |
|---|---|---|
| `enabled` | `true` | 为 `false` 时浏览器工具返回 `unavailable` |
| `headless` | `true` | 是否默认无头运行 |
| `executable` | 空 | 指定浏览器可执行文件；空表示用系统默认浏览器 |
| `use_default_browser` | `true` | 是否优先使用系统默认浏览器 |
| `allow_fallback` | `true` | 默认浏览器不可用时是否退回已知的 Chromium 浏览器 |
| `max_sessions` | `2` | 同时打开的会话上限（最大 4） |
| `idle_timeout_seconds` | `600` | 空闲多久自动关闭会话（最大 3600） |
| `allowed_schemes` | `http`、`https`、`about`、`data` | 允许导航的 URL scheme；默认禁止 `file` |
| `profile_mode` | `isolated` | `isolated`：每会话一个临时目录，关闭即删；`persistent`：保存在数据目录的 `browser/profiles/<名字>` |

**会话：** `browser.open` 返回 `session`，其余浏览器工具都要带这个参数。会话只属于创建它的调用者（连接方式 + 主人 + 设备）；默认最多同时 2 个，空闲 10 分钟自动关闭；浏览器崩溃后会话不可恢复，重新 `browser.open`。节点退出时关闭全部会话（Windows 作业对象、Linux Pdeathsig 兜底结束浏览器进程）。

**ref 快照：** `browser.snapshot` 返回当前页面的可访问性树，节点形如 `{"ref":"e12","role":"button","name":"提交"}`。`ref` 在同一个 `snapshot_id` 内有效，导航或重新快照后失效（用旧 ref 返回 `invalid_args`，`details.reason` 为 `stale_ref`）。点击和输入优先用 ref，比 CSS 选择器稳定。

| 工具 | 参数 | 返回 |
|---|---|---|
| `browser.open` | `headless`、`executable`、`profile`、`profile_name`、`viewport`（都可选） | `session`、`browser`、`executable`、`headless`、`default_browser`、`method`、`warnings` |
| `browser.close` | `session` | `closed` |
| `browser.info` | 无 | 默认浏览器、可用浏览器列表、配置、已打开的会话 |
| `browser.navigate` | `session`、`url`、`wait_until`（`load`/`domcontentloaded`/`network_idle`） | `url`、`title` |
| `browser.snapshot` | `session`、`max_nodes`（默认 400）、`max_bytes`（默认 256 KiB） | `snapshot_id`、`url`、`title`、`nodes`、`truncated` |
| `browser.click` | `session`，`ref` / `selector` / `point` 三选一；`button`、`click_count`、`modifiers`、`snapshot_id` 可选 | `clicked`、`url`、`title` |
| `browser.type` | `session`、`text`，`ref` / `selector` 二选一；`clear`、`submit` 可选 | `typed` |
| `browser.key` | `session`、`key`、`modifiers`（可选） | `pressed` |
| `browser.wait` | `session`，`selector` / `text` / `url` 至少一个；`state`（`visible`/`attached`） | `waited_ms` |
| `browser.screenshot` | `session`、`full_page`、`format`、`quality`、`clip`、`save_path`（都可选） | `path`、`width`、`height`、`bytes` |
| `browser.eval` | `session`、`expression`、`await_promise`、`max_bytes`（可选） | `value`、`truncated` |
| `browser.console` | `session`、`level`、`max_entries`、`clear`（可选） | `entries`、`count` |
| `browser.network` | `session`、`filter`（`all`/`xhr`/`document`/`failed`）、`max_entries` | `requests`、`count` |
| `browser.tabs` | `session`、`action`（`list`/`new`/`select`/`close`）、`url`、`index` | `tabs` |
| `browser.cookies` | `session`、`action`（`get`/`set`/`clear`）、`url`、`cookies` | `cookies` 或 `set` 数量 |

- 截图默认保存到数据目录 `browser/screenshots/`，`save_path` 是绝对路径或相对数据目录的路径。
- `browser.eval` 等同于在页面里执行任意 JavaScript，权限上按“执行命令”对待；表达式在操作记录里截断。
- Cookie 的值在操作记录里脱敏。
- 找不到浏览器时返回 `unavailable`（默认浏览器不支持 CDP 且不允许回退时是 `unsupported_platform`）；浏览器崩溃返回 `unavailable`；导航被 scheme 白名单拒绝返回 `permission_denied`。
- **不影响用户正在使用的窗口：** 每次会话都启动独立的浏览器实例（独立用户数据目录，默认无头），不接管、不聚焦、不关闭用户已经打开的浏览器窗口；`browser.close` 只结束这个会话自己启动的实例（Windows 用独立作业对象，Linux/macOS 用独立进程组）。`persistent` 配置档案同一时间只允许一个会话使用。

## 编码

Windows 上外部程序的输出不一定是 UTF-8：`cmd` 内部命令和很多老程序按控制台代码页（中文系统为 936，即 GBK）输出，PowerShell 的错误流也可能不受 `OutputEncoding` 影响。`proc.exec` 对标准输出和标准错误分别处理：

1. 去掉开头的 UTF-8 BOM；
2. 是合法 UTF-8 就原样返回；例外是 GBK 双字节字符碰巧也是合法 UTF-8 的情况（如“值”的 GBK 编码 `D6 B5` 会被当成希伯来文 U+05B5）：此时如果 UTF-8 结果里只有希腊、西里尔、希伯来、阿拉伯这类两字节区间的字符，而 OEM 解出来有中日韩文字，就按 OEM 解码（拉丁字母不受影响）；
3. 否则按 OEM 代码页（`GetOEMCP`）用 `MultiByteToWideChar` 解码；失败再试 ANSI 代码页（`GetACP`）；仍失败就把不合法的字节替换为 U+FFFD。

截断在解码之前按字节进行，截断处的半个字符在解码时丢弃。Linux 和 macOS 只做第 1、2 步和替换。
