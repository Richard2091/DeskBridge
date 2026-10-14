package cli

// read、write、list、stat 子命令：目标设备上的文件操作（fs.*）。

import (
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"text/tabwriter"
	"time"
)

// fileChunk 是 read 和 write 每次传输的字节数（4 MiB，低于协议单次上限）。
const fileChunk = 4 << 20

// fsReadResult 是 fs.read 的返回值。
type fsReadResult struct {
	Path      string `json:"path"`
	Content   string `json:"content"`
	Encoding  string `json:"encoding"`
	Size      int64  `json:"size"`
	Offset    int64  `json:"offset"`
	BytesRead int64  `json:"bytes_read"`
	EOF       bool   `json:"eof"`
}

// fsWriteResult 是 fs.write 的返回值。
type fsWriteResult struct {
	Path         string `json:"path"`
	BytesWritten int64  `json:"bytes_written"`
	Created      bool   `json:"created"`
}

// fsEntry 是 fs.list 的一个条目，也是 fs.stat 返回值的子集。
type fsEntry struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Size  int64  `json:"size"`
	Mtime string `json:"mtime"`
}

// fsListResult 是 fs.list 的返回值。
type fsListResult struct {
	Path      string    `json:"path"`
	Entries   []fsEntry `json:"entries"`
	Truncated bool      `json:"truncated"`
}

// fsStatResult 是 fs.stat 的返回值。
type fsStatResult struct {
	Path       string `json:"path"`
	Type       string `json:"type"`
	Size       int64  `json:"size"`
	Mtime      string `json:"mtime"`
	Mode       string `json:"mode"`
	LinkTarget string `json:"link_target"`
}

const readUsage = "read [-d 设备] [--out 本地文件] [--json] <路径>"

const readHelp = `读取目标设备上的文件。选项必须写在路径之前。不加 --json 时按 4 MiB 一块
循环读取直到文件结束，把原始字节写到 --out 指定的本地文件或标准输出
（不加换行，二进制文件也可以）；加 --json 时只读第一块，输出 fs.read 的结果。`

func runRead(args []string, stdout, stderr io.Writer) int {
	t := newToolCmd("read", readUsage, readHelp, true, stdout, stderr)
	out := t.fs.String("out", "", "写到本地文件，默认写到标准输出")
	if code, ok := t.parse(args); !ok {
		return code
	}
	if t.fs.NArg() != 1 {
		return t.usageError("需要且只需要一个路径")
	}
	path := t.fs.Arg(0)
	s, code, ok := t.open()
	if !ok {
		return code
	}
	if *s.jsonOut {
		return s.finish("fs.read", map[string]any{"path": path, "max_bytes": fileChunk, "encoding": "auto"}, nil, nil)
	}

	var dst io.Writer = stdout
	var file *os.File
	if *out != "" {
		// 第一块读成功后才创建本地文件，读取失败时不留下空文件。
		defer func() {
			if file != nil {
				file.Close()
			}
		}()
	}
	var offset int64
	for {
		var r fsReadResult
		_, err := s.callResult("fs.read", map[string]any{
			"path": path, "offset": offset, "max_bytes": fileChunk, "encoding": "base64",
		}, &r)
		if err != nil {
			return s.fail(err)
		}
		data, err := base64.StdEncoding.DecodeString(r.Content)
		if err != nil {
			fmt.Fprintf(stderr, "解码文件内容失败：%v\n", err)
			return exitError
		}
		if *out != "" && file == nil {
			file, err = os.Create(*out)
			if err != nil {
				fmt.Fprintf(stderr, "创建本地文件失败：%v\n", err)
				return exitError
			}
			dst = file
		}
		if _, err := dst.Write(data); err != nil {
			fmt.Fprintf(stderr, "写出失败：%v\n", err)
			return exitError
		}
		// 到达文件末尾或没有读到数据都必须停止，避免死循环。
		if r.EOF || r.BytesRead <= 0 {
			break
		}
		offset += r.BytesRead
	}
	if file != nil {
		if err := file.Close(); err != nil {
			fmt.Fprintf(stderr, "关闭本地文件失败：%v\n", err)
			file = nil
			return exitError
		}
		file = nil
	}
	return exitOK
}

const writeUsage = "write [-d 设备] [--from 本地文件|-] [--append] [--json] <路径> [内容]"

const writeHelp = `写入目标设备上的文件。选项必须写在路径之前。内容来自位置参数（UTF-8 文本），
或 --from 指定的本地文件（- 表示标准输入，按 4 MiB 一块传输，可以是二进制）；
两者只能给一个。默认覆盖原文件，--append 追加到末尾。`

func runWrite(args []string, stdout, stderr io.Writer) int {
	t := newToolCmd("write", writeUsage, writeHelp, true, stdout, stderr)
	from := t.fs.String("from", "", "内容来自本地文件，- 表示标准输入")
	appendMode := t.fs.Bool("append", false, "追加到文件末尾而不是覆盖")
	if code, ok := t.parse(args); !ok {
		return code
	}
	na := t.fs.NArg()
	switch {
	case na == 0 || na > 2:
		return t.usageError("用法错误：需要路径，以及内容或 --from")
	case *from != "" && na != 1:
		return t.usageError("内容和 --from 只能给一个")
	case *from == "" && na != 2:
		return t.usageError("缺少内容：给出内容参数或 --from")
	}
	path := t.fs.Arg(0)
	s, code, ok := t.open()
	if !ok {
		return code
	}

	var total fsWriteResult
	if *from == "" {
		var r fsWriteResult
		if _, err := s.callResult("fs.write", map[string]any{
			"path": path, "content": t.fs.Arg(1), "append": *appendMode,
		}, &r); err != nil {
			return s.fail(err)
		}
		total = r
	} else {
		var src io.Reader = stdinReader
		if *from != "-" {
			f, err := os.Open(*from)
			if err != nil {
				fmt.Fprintf(stderr, "打开本地文件失败：%v\n", err)
				return exitError
			}
			defer f.Close()
			src = f
		}
		buf := make([]byte, fileChunk)
		for first := true; ; first = false {
			n, rerr := io.ReadFull(src, buf)
			atEnd := rerr == io.EOF || rerr == io.ErrUnexpectedEOF
			if rerr != nil && !atEnd {
				fmt.Fprintf(stderr, "读取本地内容失败：%v\n", rerr)
				return exitError
			}
			// 空来源也要写一次，才能创建或清空目标文件。
			if n == 0 && !first {
				break
			}
			var r fsWriteResult
			// 第一块按 --append 决定覆盖还是追加，之后的块都追加。
			if _, err := s.callResult("fs.write", map[string]any{
				"path": path, "content": base64.StdEncoding.EncodeToString(buf[:n]),
				"encoding": "base64", "append": *appendMode || !first,
			}, &r); err != nil {
				return s.fail(err)
			}
			if first {
				total = r
			} else {
				total.BytesWritten += r.BytesWritten
			}
			if atEnd {
				break
			}
		}
	}
	if *s.jsonOut {
		printJSON(stdout, total)
		return exitOK
	}
	fmt.Fprintf(stdout, "已写入 %d 字节到 %s\n", total.BytesWritten, total.Path)
	return exitOK
}

const listUsage = "list [-d 设备] [-r] [--max N] [--json] <路径>"

const listHelp = "列出目标设备上的目录（别名 ls）。选项必须写在路径之前。每行依次是类型（d 目录、- 文件、l 符号链接、? 其他）、\n大小、修改时间（本地时区）、名字（目录后加 /）。"

func runList(args []string, stdout, stderr io.Writer) int {
	t := newToolCmd("list", listUsage, listHelp, true, stdout, stderr)
	recursive := t.fs.Bool("r", false, "递归列出")
	maxEntries := t.fs.Int("max", 0, "最多列出的条目数，0 用对端默认值（1000，上限 10000）")
	if code, ok := t.parse(args); !ok {
		return code
	}
	if t.fs.NArg() != 1 {
		return t.usageError("需要且只需要一个路径")
	}
	s, code, ok := t.open()
	if !ok {
		return code
	}
	callArgs := map[string]any{"path": t.fs.Arg(0), "recursive": *recursive}
	if *maxEntries > 0 {
		callArgs["max_entries"] = *maxEntries
	}
	var r fsListResult
	return s.finish("fs.list", callArgs, &r, func() {
		tw := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
		for _, e := range r.Entries {
			name := e.Name
			if e.Type == "dir" {
				name += "/"
			}
			fmt.Fprintf(tw, "%s\t%d\t%s\t%s\n", typeMark(e.Type), e.Size, localTime(e.Mtime), name)
		}
		tw.Flush()
		if r.Truncated {
			fmt.Fprintln(stderr, "提示：条目过多，列表已被截断，可用 --max 调大上限")
		}
	})
}

// typeMark 返回文件类型的单字符标记。
func typeMark(typ string) string {
	switch typ {
	case "dir":
		return "d"
	case "file":
		return "-"
	case "symlink":
		return "l"
	default:
		return "?"
	}
}

// localTime 把 RFC 3339 时间转成本地时区的 2006-01-02 15:04；解析失败时原样返回。
func localTime(s string) string {
	ts, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return s
	}
	return ts.Local().Format("2006-01-02 15:04")
}

const statUsage = "stat [-d 设备] [--json] <路径>"

func runStat(args []string, stdout, stderr io.Writer) int {
	t := newToolCmd("stat", statUsage, "查看目标设备上文件或目录的信息。选项必须写在路径之前。", true, stdout, stderr)
	if code, ok := t.parse(args); !ok {
		return code
	}
	if t.fs.NArg() != 1 {
		return t.usageError("需要且只需要一个路径")
	}
	s, code, ok := t.open()
	if !ok {
		return code
	}
	var r fsStatResult
	return s.finish("fs.stat", map[string]any{"path": t.fs.Arg(0)}, &r, func() {
		fmt.Fprintf(stdout, "路径：%s\n类型：%s\n大小：%d\n修改时间：%s\n权限：%s\n", r.Path, r.Type, r.Size, localTime(r.Mtime), r.Mode)
		if r.LinkTarget != "" {
			fmt.Fprintf(stdout, "链接目标：%s\n", r.LinkTarget)
		}
	})
}
