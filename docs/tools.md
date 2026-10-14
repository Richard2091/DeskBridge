# 内置工具

节点内置的工具，参数和返回值即线上格式（JSON 键名）。通用约定见 `docs/architecture.md` 的“节点协议”一节。

## 通用约定

- **参数严格解析：** 未知字段返回 `invalid_args`。可选字段省略即取默认值。
- **路径：** 相对路径相对于节点进程的当前目录（不建议依赖），返回值中的 `path` 一律是绝对路径。Windows 上 `/` 和 `\` 都接受。
- **时间：** RFC 3339 字符串，带时区，如 `2026-10-14T21:05:00+08:00`。
- **错误：** 文件或进程不存在返回 `not_found`；没有权限返回 `permission_denied`（`details.missing` 为 `os_permission`，表示被操作系统拒绝）；参数不合理（如路径是目录却要读文件）返回 `invalid_args`。
- **超时：** 由调用请求的 `timeout_ms` 控制（协议层），工具参数里不再另设超时。超时后 `proc.exec` 返回已收到的输出并标记 `timed_out`，其他工具返回 `timeout` 错误。取消一律返回 `cancelled`。
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
| `stdin` | 空 | 标准输入；不提供时为空输入，等待输入的命令会立即读到结束 |
| `max_output_bytes` | `1048576`（1 MiB） | 标准输出和标准错误各自的上限，上限 8 MiB；超出部分丢弃 |

返回：

| 字段 | 说明 |
|---|---|
| `exit_code` | 退出码；超时或被取消而终止时为 `-1` |
| `stdout` / `stderr` | 已解码为 UTF-8 的输出 |
| `stdout_truncated` / `stderr_truncated` | 是否超过 `max_output_bytes` |
| `timed_out` | 是否因超时被终止 |
| `duration_ms` | 执行耗时 |

- 命令启动不了（程序不存在、目录不存在）返回 `not_found` 或 `invalid_args` 错误；启动后非零退出不是错误，结果里看 `exit_code`。
- 超时或取消时终止整个进程树：Windows 用作业对象（Job Object），Linux 用进程组。
- 各 shell 的调用方式：
  - `powershell` / `pwsh`：`-NoLogo -NoProfile -NonInteractive -ExecutionPolicy Bypass -Command <前缀 + 换行 + 命令>`，前缀是把 `[Console]::InputEncoding`（让 `stdin` 按 UTF-8 读取）、`[Console]::OutputEncoding` 和 `$OutputEncoding` 设为 UTF-8 的语句。不用 `-EncodedCommand`：输出被重定向时它会让 PowerShell 把错误流和进度流序列化成 CLIXML 写到 `stderr`；`-Command` 下 `stderr` 是和控制台一致的纯文本。引号由 Go 按标准规则转义，PowerShell 同样规则还原。警告流（`Write-Warning`）在文本模式下写到 `stdout`，和控制台一致。
  - `cmd`：`cmd /d /s /c "<command>"`，自己拼命令行（Go 的 `SysProcAttr.CmdLine`），不让 Go 再转义一次。
  - `sh` / `bash`：`-c <command>`。

## proc.list

参数：`name`（可选，进程名包含该字符串，不区分大小写）、`max_entries`（默认 500，上限 10000）。

返回：`processes`、`truncated`。每个进程：`pid`、`ppid`、`name`、`memory_bytes`（常驻内存，取不到时省略）、`user`（取不到时省略）。按 `pid` 排序。

## proc.kill

参数：`pid`（必填）、`tree`（默认 `false`，连同子进程）、`force`（默认 `false`；Linux 上 `false` 发 SIGTERM、`true` 发 SIGKILL；Windows 上总是强制结束）。

返回：`killed`（实际结束的 PID 列表）。不允许结束节点进程自己（`invalid_args`）。

## 编码

Windows 上外部程序的输出不一定是 UTF-8：`cmd` 内部命令和很多老程序按控制台代码页（中文系统为 936，即 GBK）输出，PowerShell 的错误流也可能不受 `OutputEncoding` 影响。`proc.exec` 对标准输出和标准错误分别处理：

1. 去掉开头的 UTF-8 BOM；
2. 是合法 UTF-8 就原样返回；
3. 否则按 OEM 代码页（`GetOEMCP`）用 `MultiByteToWideChar` 解码；失败再试 ANSI 代码页（`GetACP`）；仍失败就把不合法的字节替换为 U+FFFD。

截断在解码之前按字节进行，截断处的半个字符在解码时丢弃。Linux 和 macOS 只做第 1、2 步和替换。
