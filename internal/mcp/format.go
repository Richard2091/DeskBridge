package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// prettyJSON 把 v 格式化为两空格缩进的 JSON：不转义 HTML 字符，中文原样。
func prettyJSON(v any) (string, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return "", err
	}
	return strings.TrimRight(buf.String(), "\n"), nil
}

// prettyRaw 把节点返回的 JSON 重新格式化。节点序列化时转义了 <、>、&
// （\u003c 等），所以不能只缩进，要先解码再用不转义的编码器输出。
// 数字用 json.Number 保持原样，不经过浮点数。
func prettyRaw(raw json.RawMessage) (string, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return "", err
	}
	return prettyJSON(v)
}

// execResult 是 proc.exec 的返回值（字段见 docs/tools.md）。
type execResult struct {
	ExitCode        int    `json:"exit_code"`
	Stdout          string `json:"stdout"`
	Stderr          string `json:"stderr"`
	StdoutTruncated bool   `json:"stdout_truncated"`
	StderrTruncated bool   `json:"stderr_truncated"`
	TimedOut        bool   `json:"timed_out"`
}

// formatExec 把 proc.exec 的结果排成给人读的文本。
func formatExec(r execResult) string {
	var b strings.Builder
	if r.TimedOut {
		b.WriteString("退出码：超时，已终止\n")
	} else {
		fmt.Fprintf(&b, "退出码：%d\n", r.ExitCode)
	}
	b.WriteString("--- 标准输出 ---\n")
	b.WriteString(r.Stdout)
	if r.Stderr != "" {
		if !strings.HasSuffix(r.Stdout, "\n") && r.Stdout != "" {
			b.WriteString("\n")
		}
		b.WriteString("--- 标准错误 ---\n")
		b.WriteString(r.Stderr)
	}
	if r.StdoutTruncated || r.StderrTruncated {
		if !strings.HasSuffix(b.String(), "\n") {
			b.WriteString("\n")
		}
		b.WriteString("（输出超过上限，已截断）")
	}
	return b.String()
}

// readResult 是 fs.read 的返回值。
type readResult struct {
	Content   string `json:"content"`
	Encoding  string `json:"encoding"`
	Size      int64  `json:"size"`
	Offset    int64  `json:"offset"`
	BytesRead int64  `json:"bytes_read"`
	EOF       bool   `json:"eof"`
}

// formatRead 把 fs.read 的结果排成文本：内容在中间，base64 在前面注明，
// 没读完在后面给出继续读的 offset。
func formatRead(r readResult) string {
	var b strings.Builder
	if r.Encoding == "base64" {
		b.WriteString("（二进制文件，内容为 base64）\n")
	}
	if r.Size == 0 && r.Content == "" {
		return "（文件为空）"
	}
	b.WriteString(r.Content)
	if !r.EOF {
		next := r.Offset + r.BytesRead
		fmt.Fprintf(&b, "\n（未读完：文件共 %d 字节，已读到 %d；用 offset=%d 继续读）", r.Size, next, next)
	}
	return b.String()
}
