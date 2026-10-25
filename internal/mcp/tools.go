package mcp

// 七个通用工具的定义和处理函数。参数结构体既是 JSON Schema 的来源
// （json 标签决定名字和是否必填：带 omitempty 的是可选），也是解析目标，
// 所以 schema 和实际解析不会不一致。

import (
	"context"
	"encoding/json"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type listDevicesArgs struct {
	Refresh bool `json:"refresh,omitempty" jsonschema:"为 true 时忽略节点 30 秒的探测缓存，重新探测各设备是否运行 DeskBridge；默认 false"`
}

type deviceToolsArgs struct {
	Device string `json:"device" jsonschema:"目标设备：设备名称、主机名或内网 IP，也可以写成“连接方式:设备”；self 表示本机。先用 list_devices 查看有哪些设备。"`
}

type callToolArgs struct {
	Device    string         `json:"device" jsonschema:"目标设备：设备名称、主机名或内网 IP，也可以写成“连接方式:设备”；self 表示本机。先用 list_devices 查看有哪些设备。"`
	Tool      string         `json:"tool" jsonschema:"工具名，如 sys.info、proc.list；用 device_tools 查看该设备有哪些工具"`
	Args      map[string]any `json:"args,omitempty" jsonschema:"工具参数（对象），格式见 device_tools 返回的 input_schema；工具没有参数时省略"`
	TimeoutMS int            `json:"timeout_ms,omitempty" jsonschema:"调用超时，单位毫秒；省略时用该工具的默认值（一般 60000，proc.exec 为 300000），上限 600000"`
}

type execArgs struct {
	Device         string `json:"device" jsonschema:"目标设备：设备名称、主机名或内网 IP，也可以写成“连接方式:设备”；self 表示本机。先用 list_devices 查看有哪些设备。"`
	Command        string `json:"command" jsonschema:"要执行的命令行，交给 shell 执行"`
	Shell          string `json:"shell,omitempty" jsonschema:"powershell、pwsh、cmd、sh、bash 之一；省略时 Windows 用 powershell，其他系统用 sh"`
	Cwd            string `json:"cwd,omitempty" jsonschema:"工作目录；省略时用节点进程的当前目录"`
	Stdin          string `json:"stdin,omitempty" jsonschema:"标准输入的内容；省略时为空输入，等待输入的命令会立即读到结束"`
	Console        string `json:"console,omitempty" jsonschema:"控制台窗口：hidden（默认）不弹窗口；visible 在设备桌面上打开带标题的窗口显示命令行和实时输出，适合下载、安装、构建等耗时较长、需要人看着的命令，此时不返回 stdout/stderr，只返回退出码；仅 Windows 支持"`
	TimeoutMS      int    `json:"timeout_ms,omitempty" jsonschema:"超时，单位毫秒；默认 300000（5 分钟），上限 600000（10 分钟）。超时会终止整个进程树并返回已收到的输出"`
	MaxOutputBytes int    `json:"max_output_bytes,omitempty" jsonschema:"标准输出和标准错误各自保留的字节数；默认 65536，上限 8388608。超出部分被丢弃，结果末尾会注明已截断"`
}

type readFileArgs struct {
	Device   string `json:"device" jsonschema:"目标设备：设备名称、主机名或内网 IP，也可以写成“连接方式:设备”；self 表示本机。先用 list_devices 查看有哪些设备。"`
	Path     string `json:"path" jsonschema:"文件路径，用绝对路径（Windows 上 / 和 \\ 都可以）"`
	Offset   int64  `json:"offset,omitempty" jsonschema:"起始字节，默认 0；文件没读完时用结果里提示的 offset 继续读下一块"`
	MaxBytes int    `json:"max_bytes,omitempty" jsonschema:"本次最多读取的字节数；默认 262144，上限 8388608。不是合法 UTF-8 的文件会以 base64 返回"`
}

type writeFileArgs struct {
	Device  string `json:"device" jsonschema:"目标设备：设备名称、主机名或内网 IP，也可以写成“连接方式:设备”；self 表示本机。先用 list_devices 查看有哪些设备。"`
	Path    string `json:"path" jsonschema:"文件路径，用绝对路径；父目录不存在时自动创建"`
	Content string `json:"content" jsonschema:"要写入的文本（UTF-8），可以为空字符串。单次不超过约 16 MiB，更大的内容分多次、用 append=true 追加"`
	Append  bool   `json:"append,omitempty" jsonschema:"为 true 时追加到文件末尾；默认 false，覆盖整个文件"`
}

type listDirArgs struct {
	Device     string `json:"device" jsonschema:"目标设备：设备名称、主机名或内网 IP，也可以写成“连接方式:设备”；self 表示本机。先用 list_devices 查看有哪些设备。"`
	Path       string `json:"path" jsonschema:"目录路径，用绝对路径"`
	Recursive  bool   `json:"recursive,omitempty" jsonschema:"为 true 时递归列出子目录，条目名是相对 path 的路径；默认 false"`
	MaxEntries int    `json:"max_entries,omitempty" jsonschema:"最多返回的条目数；默认 500，上限 10000。结果里 truncated 为 true 表示被截断，请缩小目录范围而不是调大上限"`
}

// addTool 注册一个工具。handler 返回的结果原样交给 SDK；输出不声明
// schema（Out 为 any），内容全部是文本。
func addTool[In any](srv *sdk.Server, name, desc string, readOnly bool, h func(context.Context, *sdk.CallToolRequest, In) *sdk.CallToolResult) {
	t := &sdk.Tool{Name: name, Description: desc}
	if readOnly {
		t.Annotations = &sdk.ToolAnnotations{ReadOnlyHint: true}
	}
	sdk.AddTool(srv, t, func(ctx context.Context, req *sdk.CallToolRequest, in In) (*sdk.CallToolResult, any, error) {
		return h(ctx, req, in), nil, nil
	})
}

func (s *server) addTools(srv *sdk.Server) {
	addTool(srv, "list_devices",
		"列出内网里的设备（按连接方式分组，本机排第一），包含是否在线、系统、是否运行 DeskBridge 及工具数量。其他工具的 device 参数从这里取。",
		true, s.listDevices)
	addTool(srv, "device_tools",
		"查看一台设备的能力报告：它的系统信息，以及它提供的全部工具（名称、说明、参数 JSON Schema）。要用 call_tool 调用不常用的工具前先看这里。",
		true, s.deviceTools)
	addTool(srv, "call_tool",
		"调用设备上的任意工具并返回其结果（JSON）。常用操作有专用工具（exec、read_file、write_file、list_dir），其他工具（如 sys.info、proc.list、screen.capture）用这个，参数格式见 device_tools。",
		false, s.callToolTool)
	addTool(srv, "exec",
		"在设备上执行一条命令，返回退出码、标准输出和标准错误。超时默认 300000 毫秒，上限 600000 毫秒（10 分钟）；输出默认各保留 65536 字节，被截断时结果末尾会注明。"+
			"耗时很长的命令不要一直等：把它放到后台并把输出写进日志文件，再用 read_file 轮询日志。"+
			"Windows 上用 shell=cmd 执行 start /b cmd /c \"命令 > 日志文件 2>&1\" 启动后台进程，命令正常结束后进程会保留。"+
			"Windows 上默认不弹控制台窗口；下载、安装、构建等耗时较长、需要人在被控端桌面上看着的命令可以设 console=visible，"+
			"在那个桌面上打开带标题的窗口显示命令行和实时输出，此时不返回 stdout/stderr，只返回退出码。",
		false, s.exec)
	addTool(srv, "read_file",
		"读取设备上的文件。默认最多读 262144 字节；没读完时结果末尾会给出继续读的 offset，用 offset 分块读取。不是合法 UTF-8 的文件以 base64 返回，开头会注明。",
		true, s.readFile)
	addTool(srv, "write_file",
		"写入设备上的文件（默认覆盖，覆盖时先写临时文件再改名，不会留下写了一半的文件）。大文件分块写：第一块覆盖，之后用 append=true。",
		false, s.writeFile)
	addTool(srv, "list_dir",
		"列出设备上的目录，返回 JSON（条目的 name、type、size、mtime）。默认最多 500 条，超出时 truncated 为 true。",
		true, s.listDir)
}

// prettyResult 把节点返回的 JSON 格式化成文本结果。
func prettyResult(raw json.RawMessage) *sdk.CallToolResult {
	if len(raw) == 0 {
		return textResult("（工具没有返回内容）")
	}
	text, err := prettyRaw(raw)
	if err != nil {
		return errorResult("错误：解析工具返回值失败：" + err.Error())
	}
	return textResult(text)
}

// pretty 把值格式化成文本结果。
func pretty(v any) *sdk.CallToolResult {
	text, err := prettyJSON(v)
	if err != nil {
		return errorResult("错误：编码结果失败：" + err.Error())
	}
	return textResult(text)
}

func (s *server) listDevices(ctx context.Context, _ *sdk.CallToolRequest, in listDevicesArgs) *sdk.CallToolResult {
	ctx, cancel := context.WithTimeout(ctx, metaTimeout)
	defer cancel()
	list, err := s.c.Devices(ctx, in.Refresh)
	if err != nil {
		return failure(err)
	}
	return pretty(list)
}

func (s *server) deviceTools(ctx context.Context, _ *sdk.CallToolRequest, in deviceToolsArgs) *sdk.CallToolResult {
	if in.Device == "" {
		return errorResult("错误（invalid_args）：device 不能为空，先用 list_devices 查看设备")
	}
	ctx, cancel := context.WithTimeout(ctx, metaTimeout)
	defer cancel()
	capab, err := s.c.Capability(ctx, in.Device)
	if err != nil {
		return failure(err)
	}
	return pretty(capab)
}

func (s *server) callToolTool(ctx context.Context, req *sdk.CallToolRequest, in callToolArgs) *sdk.CallToolResult {
	if in.Tool == "" {
		return errorResult("错误（invalid_args）：tool 不能为空，用 device_tools 查看设备的工具")
	}
	var args any
	if in.Args != nil {
		args = in.Args
	}
	resp, fail := s.callTool(ctx, req, in.Device, in.Tool, args, in.TimeoutMS)
	if fail != nil {
		return fail
	}
	return prettyResult(resp.Result)
}

func (s *server) exec(ctx context.Context, req *sdk.CallToolRequest, in execArgs) *sdk.CallToolResult {
	if in.Command == "" {
		return errorResult("错误（invalid_args）：command 不能为空")
	}
	maxOut := in.MaxOutputBytes
	if maxOut <= 0 {
		maxOut = defaultExecOutputBytes
	}
	args := map[string]any{"command": in.Command, "max_output_bytes": maxOut}
	if in.Shell != "" {
		args["shell"] = in.Shell
	}
	if in.Cwd != "" {
		args["cwd"] = in.Cwd
	}
	if in.Stdin != "" {
		args["stdin"] = in.Stdin
	}
	if in.Console != "" {
		args["console"] = in.Console
	}
	resp, fail := s.callTool(ctx, req, in.Device, "proc.exec", args, in.TimeoutMS)
	if fail != nil {
		return fail
	}
	var r execResult
	if err := json.Unmarshal(resp.Result, &r); err != nil {
		return errorResult("错误：解析 proc.exec 的返回值失败：" + err.Error())
	}
	return textResult(formatExec(r))
}

func (s *server) readFile(ctx context.Context, req *sdk.CallToolRequest, in readFileArgs) *sdk.CallToolResult {
	if in.Path == "" {
		return errorResult("错误（invalid_args）：path 不能为空")
	}
	maxBytes := in.MaxBytes
	if maxBytes <= 0 {
		maxBytes = defaultReadBytes
	}
	args := map[string]any{"path": in.Path, "max_bytes": maxBytes, "encoding": "auto"}
	if in.Offset > 0 {
		args["offset"] = in.Offset
	}
	resp, fail := s.callTool(ctx, req, in.Device, "fs.read", args, 0)
	if fail != nil {
		return fail
	}
	var r readResult
	if err := json.Unmarshal(resp.Result, &r); err != nil {
		return errorResult("错误：解析 fs.read 的返回值失败：" + err.Error())
	}
	return textResult(formatRead(r))
}

func (s *server) writeFile(ctx context.Context, req *sdk.CallToolRequest, in writeFileArgs) *sdk.CallToolResult {
	if in.Path == "" {
		return errorResult("错误（invalid_args）：path 不能为空")
	}
	args := map[string]any{"path": in.Path, "content": in.Content, "encoding": "utf-8"}
	if in.Append {
		args["append"] = true
	}
	resp, fail := s.callTool(ctx, req, in.Device, "fs.write", args, 0)
	if fail != nil {
		return fail
	}
	return prettyResult(resp.Result)
}

func (s *server) listDir(ctx context.Context, req *sdk.CallToolRequest, in listDirArgs) *sdk.CallToolResult {
	if in.Path == "" {
		return errorResult("错误（invalid_args）：path 不能为空")
	}
	maxEntries := in.MaxEntries
	if maxEntries <= 0 {
		maxEntries = defaultListEntries
	}
	args := map[string]any{"path": in.Path, "max_entries": maxEntries}
	if in.Recursive {
		args["recursive"] = true
	}
	resp, fail := s.callTool(ctx, req, in.Device, "fs.list", args, 0)
	if fail != nil {
		return fail
	}
	return prettyResult(resp.Result)
}
