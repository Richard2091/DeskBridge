package builtin

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Richard2091/DeskBridge/internal/protocol"
	"github.com/Richard2091/DeskBridge/internal/tools"
)

// ---------- fs.stat ----------

// fsStatArgs 是 fs.stat 的参数。
type fsStatArgs struct {
	Path string `json:"path"`
}

// fsStatResult 是 fs.stat 的返回值。
type fsStatResult struct {
	Path       string `json:"path"`
	Type       string `json:"type"` // file、dir、symlink、other
	Size       int64  `json:"size"`
	Mtime      string `json:"mtime"`
	Mode       string `json:"mode"` // 如 -rw-r--r--
	LinkTarget string `json:"link_target,omitempty"`
}

// fsStatSchema 是 fs.stat 的参数描述。
const fsStatSchema = `{
  "type": "object",
  "additionalProperties": false,
  "required": ["path"],
  "properties": {
    "path": {"type": "string", "description": "文件或目录的路径，/ 和 \\ 都可以"}
  }
}`

func newFsStatTool() *tools.Tool {
	return &tools.Tool{
		Spec: protocol.ToolSpec{
			Name:        "fs.stat",
			Description: "查看一个文件或目录的元信息：类型、大小、修改时间、权限。不跟随符号链接。",
			Source:      protocol.SourceBuiltin,
			InputSchema: json.RawMessage(fsStatSchema),
		},
		Run: runFsStat,
	}
}

func runFsStat(ctx context.Context, call *tools.Call) (any, error) {
	if err := checkCtx(ctx); err != nil {
		return nil, err
	}
	var a fsStatArgs
	if err := tools.DecodeArgs(call.Args, &a); err != nil {
		return nil, err
	}
	if a.Path == "" {
		return nil, protocol.Errorf(protocol.CodeInvalidArgs, "path 不能为空")
	}
	fi, err := os.Lstat(a.Path)
	if err != nil {
		return nil, mapFSError(err)
	}
	abs, err := filepath.Abs(a.Path)
	if err != nil {
		return nil, protocol.Errorf(protocol.CodeInvalidArgs, "路径不合法：%v", err)
	}
	res := &fsStatResult{
		Path:  abs,
		Type:  fileTypeOf(fi.Mode()),
		Size:  fi.Size(),
		Mtime: fi.ModTime().Format(time.RFC3339),
		Mode:  fi.Mode().String(),
	}
	if res.Type == "symlink" {
		if target, lerr := os.Readlink(a.Path); lerr == nil {
			res.LinkTarget = target
		}
	}
	return res, nil
}

// fileTypeOf 把 FileMode 归类为 fs.stat 的 type 字段取值。
func fileTypeOf(m os.FileMode) string {
	switch {
	case m&os.ModeSymlink != 0:
		return "symlink"
	case m.IsDir():
		return "dir"
	case m.IsRegular():
		return "file"
	default:
		return "other"
	}
}

// ---------- fs.list ----------

// fsListArgs 是 fs.list 的参数。
type fsListArgs struct {
	Path       string `json:"path"`
	Recursive  bool   `json:"recursive"`
	MaxEntries int    `json:"max_entries"`
}

// fsListEntry 是 fs.list 返回里的一个条目。
type fsListEntry struct {
	Name  string `json:"name"` // 递归时为相对于 path 的路径，用 / 分隔
	Type  string `json:"type"`
	Size  int64  `json:"size"`
	Mtime string `json:"mtime"`
}

// fsListResult 是 fs.list 的返回值。
type fsListResult struct {
	Path      string        `json:"path"`
	Entries   []fsListEntry `json:"entries"`
	Truncated bool          `json:"truncated"`
}

// fsListSchema 是 fs.list 的参数描述。
const fsListSchema = `{
  "type": "object",
  "additionalProperties": false,
  "required": ["path"],
  "properties": {
    "path": {"type": "string", "description": "目录的路径"},
    "recursive": {"type": "boolean", "default": false, "description": "递归列出子目录；不进入符号链接指向的目录"},
    "max_entries": {"type": "integer", "default": 1000, "maximum": 10000, "description": "条目数量上限"}
  }
}`

func newFsListTool() *tools.Tool {
	return &tools.Tool{
		Spec: protocol.ToolSpec{
			Name:        "fs.list",
			Description: "列出一个目录里的文件和子目录，可以递归。按名称排序，条目多时截断并标记 truncated。",
			Source:      protocol.SourceBuiltin,
			InputSchema: json.RawMessage(fsListSchema),
		},
		Run: runFsList,
	}
}

func runFsList(ctx context.Context, call *tools.Call) (any, error) {
	if err := checkCtx(ctx); err != nil {
		return nil, err
	}
	var a fsListArgs
	if err := tools.DecodeArgs(call.Args, &a); err != nil {
		return nil, err
	}
	if a.Path == "" {
		return nil, protocol.Errorf(protocol.CodeInvalidArgs, "path 不能为空")
	}
	max := a.MaxEntries
	if max <= 0 {
		max = defaultListEntries
	}
	if max > maxListEntries {
		max = maxListEntries
	}
	abs, err := filepath.Abs(a.Path)
	if err != nil {
		return nil, protocol.Errorf(protocol.CodeInvalidArgs, "路径不合法：%v", err)
	}
	fi, err := os.Lstat(abs)
	if err != nil {
		return nil, mapFSError(err)
	}
	if !fi.IsDir() {
		return nil, protocol.Errorf(protocol.CodeInvalidArgs, "%s 不是目录", abs)
	}
	// 根目录自身的错误要报告；子目录读不了则跳过（见 listWalk）。
	if _, err := os.ReadDir(abs); err != nil {
		return nil, mapFSError(err)
	}
	res := &fsListResult{Path: abs, Entries: []fsListEntry{}}
	listWalk(abs, "", a.Recursive, max, &res.Entries)
	slices.SortFunc(res.Entries, func(x, y fsListEntry) int { return strings.Compare(x.Name, y.Name) })
	res.Truncated = len(res.Entries) >= max
	return res, nil
}

// listWalk 把 dir 下的条目追加到 out，达到 limit 停止。prefix 是当前
// 目录在结果里的相对路径前缀（递归时用 / 分隔）。
func listWalk(dir, prefix string, recursive bool, limit int, out *[]fsListEntry) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return // 无权访问或其他原因，跳过该目录，不报错
	}
	for _, e := range entries {
		name := e.Name()
		if prefix != "" {
			name = prefix + "/" + e.Name()
		}
		*out = append(*out, fsListEntry{
			Name:  name,
			Type:  entryTypeOf(e),
			Size:  entrySizeOf(e),
			Mtime: entryMtimeOf(e),
		})
		if len(*out) >= limit {
			return
		}
		// 只对真实目录递归；DirEntry 的 IsDir 对符号链接是 false，
		// 天然不会进入符号链接指向的目录。
		if recursive && e.IsDir() {
			listWalk(filepath.Join(dir, e.Name()), name, recursive, limit, out)
			if len(*out) >= limit {
				return
			}
		}
	}
}

// entryTypeOf 把 DirEntry 归类为 type 字段取值。
func entryTypeOf(d os.DirEntry) string {
	switch {
	case d.Type()&os.ModeSymlink != 0:
		return "symlink"
	case d.IsDir():
		return "dir"
	case d.Type().IsRegular():
		return "file"
	default:
		return "other"
	}
}

// entrySizeOf / entryMtimeOf 取条目信息，取不到时给零值，不中断列出。
func entrySizeOf(d os.DirEntry) int64 {
	if info, err := d.Info(); err == nil {
		return info.Size()
	}
	return 0
}

func entryMtimeOf(d os.DirEntry) string {
	if info, err := d.Info(); err == nil {
		return info.ModTime().Format(time.RFC3339)
	}
	return time.Time{}.Format(time.RFC3339)
}

// ---------- fs.read ----------

// fsReadArgs 是 fs.read 的参数。
type fsReadArgs struct {
	Path     string `json:"path"`
	Offset   int64  `json:"offset"`
	MaxBytes int64  `json:"max_bytes"`
	Encoding string `json:"encoding"` // auto、utf-8、base64
}

// fsReadResult 是 fs.read 的返回值。
type fsReadResult struct {
	Path      string `json:"path"`
	Content   string `json:"content"`
	Encoding  string `json:"encoding"` // 实际使用的 utf-8 或 base64
	Size      int64  `json:"size"`
	Offset    int64  `json:"offset"`
	BytesRead int64  `json:"bytes_read"`
	EOF       bool   `json:"eof"`
}

// fsReadSchema 是 fs.read 的参数描述。
const fsReadSchema = `{
  "type": "object",
  "additionalProperties": false,
  "required": ["path"],
  "properties": {
    "path": {"type": "string", "description": "文件的路径"},
    "offset": {"type": "integer", "default": 0, "minimum": 0, "description": "起始字节，用于分块读取大文件"},
    "max_bytes": {"type": "integer", "default": 1048576, "maximum": 8388608, "description": "单次最多读取的字节数"},
    "encoding": {"type": "string", "default": "auto", "enum": ["auto", "utf-8", "base64"], "description": "auto：合法 UTF-8 按文本返回，否则 base64；utf-8：非法字节替换为 U+FFFD；base64：总是 base64"}
  }
}`

func newFsReadTool() *tools.Tool {
	return &tools.Tool{
		Spec: protocol.ToolSpec{
			Name:        "fs.read",
			Description: "读取一个文件的内容，支持分块（offset 和 max_bytes）和文本/base64 两种返回方式。",
			Source:      protocol.SourceBuiltin,
			InputSchema: json.RawMessage(fsReadSchema),
		},
		Run: runFsRead,
	}
}

func runFsRead(ctx context.Context, call *tools.Call) (any, error) {
	if err := checkCtx(ctx); err != nil {
		return nil, err
	}
	var a fsReadArgs
	if err := tools.DecodeArgs(call.Args, &a); err != nil {
		return nil, err
	}
	switch a.Encoding {
	case "", "auto", "utf-8", "base64":
	default:
		return nil, protocol.Errorf(protocol.CodeInvalidArgs, "encoding 只能是 auto、utf-8 或 base64：%s", a.Encoding)
	}
	if a.Offset < 0 {
		return nil, protocol.Errorf(protocol.CodeInvalidArgs, "offset 不能为负数")
	}
	max := a.MaxBytes
	if max <= 0 {
		max = defaultReadBytes
	}
	if max > maxReadBytes {
		max = maxReadBytes
	}
	abs, err := filepath.Abs(a.Path)
	if err != nil {
		return nil, protocol.Errorf(protocol.CodeInvalidArgs, "路径不合法：%v", err)
	}
	f, err := os.Open(abs)
	if err != nil {
		return nil, mapFSError(err)
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil, mapFSError(err)
	}
	if fi.IsDir() {
		return nil, protocol.Errorf(protocol.CodeInvalidArgs, "%s 是目录，不能按文件读取", abs)
	}
	if _, err := f.Seek(a.Offset, io.SeekStart); err != nil {
		return nil, mapFSError(err)
	}
	raw := make([]byte, max)
	n, rerr := io.ReadFull(f, raw)
	eof := rerr == io.EOF || rerr == io.ErrUnexpectedEOF
	if rerr != nil && !eof {
		return nil, protocol.Errorf(protocol.CodeInternal, "读取文件失败：%v", rerr)
	}
	if rerr == nil {
		// 恰好读完 max 个字节时再看一眼，区分刚好到结尾和还有内容。
		var one [1]byte
		if _, e2 := f.Read(one[:]); e2 == io.EOF {
			eof = true
		}
	}
	raw = raw[:n]

	res := &fsReadResult{
		Path:   abs,
		Size:   fi.Size(),
		Offset: a.Offset,
		EOF:    eof,
	}
	switch a.Encoding {
	case "base64":
		res.Encoding = "base64"
		res.Content = base64.StdEncoding.EncodeToString(raw)
		res.BytesRead = int64(n)
	default:
		// 文本路径：去 BOM、回退半个字符，再判定 auto 的走向。
		// bytes_read 按实际读的字节算（BOM 的 3 个字节也算在内，
		// 回退的半个字符除外），下一块从 offset + bytes_read 继续。
		text := raw
		if a.Offset == 0 {
			text = trimUTF8BOM(text)
		}
		trimmed := 0
		if !eof {
			boundary := truncateAtRuneBoundary(text)
			trimmed = len(text) - boundary
			text = text[:boundary]
		}
		if a.Encoding == "utf-8" || utf8.Valid(text) {
			res.Encoding = "utf-8"
			res.Content = strings.ToValidUTF8(string(text), "\uFFFD")
			res.BytesRead = int64(n - trimmed)
		} else {
			// auto：不是合法 UTF-8（如 GBK 文件），按原始字节返回，
			// 不去掉 BOM（那是文本路径的规则）。
			res.Encoding = "base64"
			res.Content = base64.StdEncoding.EncodeToString(raw)
			res.BytesRead = int64(n)
		}
	}
	return res, nil
}

// ---------- fs.write ----------

// fsWriteArgs 是 fs.write 的参数。CreateDirs 用指针区分省略（默认
// true）和显式 false。
type fsWriteArgs struct {
	Path       string `json:"path"`
	Content    string `json:"content"`
	Encoding   string `json:"encoding"` // utf-8、base64
	Append     bool   `json:"append"`
	CreateDirs *bool  `json:"create_dirs"`
}

// fsWriteResult 是 fs.write 的返回值。
type fsWriteResult struct {
	Path         string `json:"path"`
	BytesWritten int64  `json:"bytes_written"`
	Created      bool   `json:"created"`
}

// fsWriteSchema 是 fs.write 的参数描述。
const fsWriteSchema = `{
  "type": "object",
  "additionalProperties": false,
  "required": ["path", "content"],
  "properties": {
    "path": {"type": "string", "description": "文件的路径"},
    "content": {"type": "string", "description": "内容，可以为空字符串"},
    "encoding": {"type": "string", "default": "utf-8", "enum": ["utf-8", "base64"], "description": "content 的编码"},
    "append": {"type": "boolean", "default": false, "description": "追加到末尾，用于分块写入大文件"},
    "create_dirs": {"type": "boolean", "default": true, "description": "自动创建父目录"}
  }
}`

func newFsWriteTool() *tools.Tool {
	return &tools.Tool{
		Spec: protocol.ToolSpec{
			Name:        "fs.write",
			Description: "写入一个文件：覆盖（先写临时文件再改名，失败不留半个文件）或追加，内容按 utf-8 或 base64 解释。",
			Source:      protocol.SourceBuiltin,
			InputSchema: json.RawMessage(fsWriteSchema),
		},
		Run: runFsWrite,
	}
}

func runFsWrite(ctx context.Context, call *tools.Call) (any, error) {
	if err := checkCtx(ctx); err != nil {
		return nil, err
	}
	var a fsWriteArgs
	if err := tools.DecodeArgs(call.Args, &a); err != nil {
		return nil, err
	}
	if a.Path == "" {
		return nil, protocol.Errorf(protocol.CodeInvalidArgs, "path 不能为空")
	}
	createDirs := true
	if a.CreateDirs != nil {
		createDirs = *a.CreateDirs
	}
	data, err := decodeWriteContent(a.Content, a.Encoding)
	if err != nil {
		return nil, err
	}
	abs, err := filepath.Abs(a.Path)
	if err != nil {
		return nil, protocol.Errorf(protocol.CodeInvalidArgs, "路径不合法：%v", err)
	}

	// 记录原文件是否存在：created 字段，覆盖时还要保留它的权限位。
	existed := false
	perm := os.FileMode(0644)
	if fi, serr := os.Lstat(abs); serr == nil {
		existed = true
		perm = fi.Mode().Perm()
		if fi.IsDir() {
			return nil, protocol.Errorf(protocol.CodeInvalidArgs, "%s 是目录，不能按文件写入", abs)
		}
	} else if !os.IsNotExist(serr) {
		return nil, mapFSError(serr)
	}

	if createDirs {
		if derr := os.MkdirAll(filepath.Dir(abs), 0755); derr != nil {
			return nil, mapFSError(derr)
		}
	}

	if a.Append {
		f, err := os.OpenFile(abs, os.O_WRONLY|os.O_CREATE|os.O_APPEND, perm)
		if err != nil {
			return nil, mapFSError(err)
		}
		n, werr := f.Write(data)
		f.Close()
		if werr != nil {
			return nil, protocol.Errorf(protocol.CodeInternal, "写入文件失败：%v", werr)
		}
		return &fsWriteResult{Path: abs, BytesWritten: int64(n), Created: !existed}, nil
	}

	// 覆盖：先写同目录临时文件再改名，中途失败不留半个文件。
	tmp, err := os.CreateTemp(filepath.Dir(abs), ".deskbridge-*")
	if err != nil {
		return nil, mapFSError(err)
	}
	tmpName := tmp.Name()
	n, werr := tmp.Write(data)
	if cerr := tmp.Close(); werr == nil {
		werr = cerr
	}
	// CreateTemp 的权限是 0600，统一改成目标权限（新文件 0644，
	// 覆盖时保留原文件的权限位）。
	if werr == nil {
		werr = os.Chmod(tmpName, perm)
	}
	if werr == nil {
		werr = os.Rename(tmpName, abs)
	}
	if werr != nil {
		os.Remove(tmpName)
		return nil, mapFSError(werr)
	}
	return &fsWriteResult{Path: abs, BytesWritten: int64(n), Created: !existed}, nil
}

// decodeWriteContent 按 encoding 解码要写入的内容。
func decodeWriteContent(content, encoding string) ([]byte, error) {
	switch encoding {
	case "", "utf-8":
		return []byte(content), nil
	case "base64":
		data, err := base64.StdEncoding.DecodeString(content)
		if err != nil {
			return nil, protocol.Errorf(protocol.CodeInvalidArgs, "content 不是合法的 base64：%v", err)
		}
		return data, nil
	default:
		return nil, protocol.Errorf(protocol.CodeInvalidArgs, "encoding 只能是 utf-8 或 base64：%s", encoding)
	}
}
