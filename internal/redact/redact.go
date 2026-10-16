// Package redact 生成操作记录里的参数摘要：去掉密码、令牌等敏感内容，
// 大字段只记长度，整体限制长度。摘要只用来给人看，不能还原调用参数。
package redact

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

// 摘要的长度限制。
const (
	maxSummaryBytes = 1024 // 整个摘要
	maxStringRunes  = 200  // 单个字符串
	maxItems        = 20   // 数组元素和对象字段
	maxDepth        = 6
	mask            = "***"
)

// sensitiveKeys 是键名里含有就整个隐藏的片段（小写比较）。
var sensitiveKeys = []string{
	"password", "passwd", "pwd", "secret", "token", "apikey", "api_key", "api-key",
	"credential", "authorization", "cookie", "private_key", "privatekey", "authkey", "auth_key",
}

// sizeOnlyKeys 是只记长度的大字段（文件内容、标准输入）。
var sizeOnlyKeys = map[string]bool{"content": true, "stdin": true}

// 字符串内容里的敏感写法：password=xxx、-Password xxx、Bearer xxx。
var (
	assignRe = regexp.MustCompile(`(?i)(password|passwd|pwd|secret|token|api[_-]?key|authorization)(\s*[=:]\s*)("[^"]*"|'[^']*'|\S+)`)
	flagRe   = regexp.MustCompile(`(?i)(--?(?:password|passwd|pwd|secret|token|api[_-]?key))(\s+|=)("[^"]*"|'[^']*'|\S+)`)
	bearerRe = regexp.MustCompile(`(?i)(bearer\s+)[A-Za-z0-9._~+/=-]{8,}`)
)

// Summarize 返回调用参数的摘要文本。args 为空或 null 时返回空串；
// 不是合法 JSON 时只给出长度。
func Summarize(args json.RawMessage) string {
	if len(args) == 0 || string(args) == "null" {
		return ""
	}
	var v any
	dec := json.NewDecoder(bytes.NewReader(args))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		return fmt.Sprintf("<无法解析的参数，%d 字节>", len(args))
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false) // 摘要给人看，<、> 不转义
	if err := enc.Encode(walk("", v, 0)); err != nil {
		return fmt.Sprintf("<无法编码的参数，%d 字节>", len(args))
	}
	return cut(strings.TrimSpace(buf.String()), maxSummaryBytes)
}

// Text 对一段自由文本做同样的内容脱敏（不截断），供日志类用途。
func Text(s string) string { return scrub(s) }

func walk(key string, v any, depth int) any {
	lk := strings.ToLower(key)
	if key != "" {
		for _, s := range sensitiveKeys {
			if strings.Contains(lk, s) {
				return mask
			}
		}
		if sizeOnlyKeys[lk] {
			if str, ok := v.(string); ok {
				return fmt.Sprintf("<%d 字节>", len(str))
			}
		}
	}
	if depth >= maxDepth {
		return "…"
	}
	switch x := v.(type) {
	case string:
		return cutRunes(scrub(x), maxStringRunes)
	case []any:
		n := min(len(x), maxItems)
		out := make([]any, 0, n+1)
		for _, e := range x[:n] {
			out = append(out, walk("", e, depth+1))
		}
		if len(x) > n {
			out = append(out, fmt.Sprintf("…共 %d 项", len(x)))
		}
		return out
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out := make(map[string]any, len(keys))
		for i, k := range keys {
			if i >= maxItems {
				out["…"] = fmt.Sprintf("共 %d 个字段", len(x))
				break
			}
			if lk == "env" {
				out[k] = mask // 环境变量的值常是密钥，只留名字
				continue
			}
			out[k] = walk(k, x[k], depth+1)
		}
		return out
	default:
		return v
	}
}

func scrub(s string) string {
	// Bearer 先处理：Authorization: Bearer xxx 里 xxx 才是令牌。
	s = bearerRe.ReplaceAllString(s, "${1}"+mask)
	s = assignRe.ReplaceAllString(s, "${1}${2}"+mask)
	s = flagRe.ReplaceAllString(s, "${1}${2}"+mask)
	return s
}

// cut 把 s 截到不超过 max 字节（按字符边界），超出时加省略号。
func cut(s string, max int) string {
	if len(s) <= max {
		return s
	}
	end := max
	for end > 0 && !utf8.RuneStart(s[end]) {
		end--
	}
	return s[:end] + "…"
}

// cutRunes 把 s 截到不超过 max 个字符，超出时加省略号。
func cutRunes(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	return string([]rune(s)[:max]) + "…"
}
