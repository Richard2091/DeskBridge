//go:build windows

package builtin

import (
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"golang.org/x/sys/windows"
)

// decodeOutput 把子进程的输出解码为 UTF-8（docs/tools.md 编码一节）：
// 去掉 BOM，合法 UTF-8 原样返回，否则按 OEM 代码页解码，再试 ANSI，
// 仍不行就把不合法的字节替换为 U+FFFD。截断留下的半个双字节字符由
// decodeCodePage 在解码前丢弃。
func decodeOutput(b []byte) string {
	b = trimUTF8BOM(b)
	// 输出可能在多字节字符中间被截断，先去掉末尾不完整的 UTF-8 字符，
	// 否则整段会被误判为非 UTF-8。
	if t := trimIncompleteUTF8(b); utf8.Valid(t) {
		if s, ok := decodeOEMIfPlausible(t); ok {
			return s
		}
		return string(t)
	}
	if utf8.Valid(b) {
		if s, ok := decodeOEMIfPlausible(b); ok {
			return s
		}
		return string(b)
	}
	if s, ok := decodeCodePage(b, getOEMCP()); ok {
		return s
	}
	if s, ok := decodeCodePage(b, windows.GetACP()); ok {
		return s
	}
	return strings.ToValidUTF8(string(b), "\uFFFD")
}

// decodeOEMIfPlausible 处理一种歧义：GBK 的双字节字符有少数组合（例如
// “值”的 GBK 编码是 D6 B5）正好也是合法 UTF-8，会解成希伯来文之类的
// 字符。子进程的控制台代码页是 OEM（中文系统是 936）时 cmd 输出就是 GBK，
// 所以 UTF-8 解出来只有希腊、西里尔、希伯来、阿拉伯这类同样落在两字节
// 区间的字符，而 OEM 解出来有中日韩文字时，按 OEM 解码。拉丁字母（如
// café 的 é）仍在两字节区间之外或属于拉丁字母，不受影响。
func decodeOEMIfPlausible(b []byte) (string, bool) {
	if !hasNonASCII(b) {
		return "", false
	}
	u := string(b)
	if containsCJK(u) || !hasTwoByteScript(u) {
		return "", false
	}
	s, ok := decodeCodePage(b, getOEMCP())
	if !ok || !containsCJK(s) {
		return "", false
	}
	return s, true
}

// hasNonASCII 报告字节里是否有非 ASCII 字节。
func hasNonASCII(b []byte) bool {
	for _, c := range b {
		if c >= utf8.RuneSelf {
			return true
		}
	}
	return false
}

// hasTwoByteScript 报告文本里是否有 U+0370–U+07FF 的字符。GBK 的字节被
// 误当成 UTF-8 时只会解成这个区间（两字节 UTF-8）里的字符。
func hasTwoByteScript(s string) bool {
	for _, r := range s {
		if r >= 0x0370 && r <= 0x07FF {
			return true
		}
	}
	return false
}

// containsCJK 报告文本里是否有中日韩文字或全角符号。
func containsCJK(s string) bool {
	for _, r := range s {
		switch {
		case r >= 0x2E80 && r <= 0x9FFF, // 部首、假名、中日韩统一表意文字
			r >= 0xF900 && r <= 0xFAFF,   // 兼容表意文字
			r >= 0xFF00 && r <= 0xFFEF,   // 全角符号
			r >= 0x20000 && r <= 0x2FA1F: // 扩展 B 及以后
			return true
		}
	}
	return false
}

// decodeCodePage 用 MultiByteToWideChar 把单字节编码的文本解码为
// UTF-8；字节不是这个代码页的合法文本时返回 false。
func decodeCodePage(b []byte, codePage uint32) (string, bool) {
	if codePage == 0 || codePage == 65001 || len(b) == 0 {
		return "", false // 65001 是 UTF-8，走到这里说明已经不是合法 UTF-8
	}
	b = trimIncompleteDBCS(b, codePage)
	if len(b) == 0 {
		return "", false
	}
	n, err := windows.MultiByteToWideChar(codePage, mbErrInvalidChars, &b[0], int32(len(b)), nil, 0)
	if err != nil || n <= 0 {
		return "", false
	}
	buf := make([]uint16, n)
	if _, err := windows.MultiByteToWideChar(codePage, mbErrInvalidChars, &b[0], int32(len(b)), &buf[0], int32(n)); err != nil {
		return "", false
	}
	return string(utf16.Decode(buf)), true
}

// trimIncompleteDBCS 去掉末尾落单的双字节前导字节（输出在字符中间被截断
// 所致）。必须从头扫描：前导字节后面跟的是尾字节，尾字节的取值范围和
// 前导字节重叠，只看最后一个字节会误删。
func trimIncompleteDBCS(b []byte, codePage uint32) []byte {
	i := 0
	for i < len(b) {
		if isDBCSLeadByteEx(codePage, b[i]) {
			if i+1 >= len(b) {
				return b[:i]
			}
			i += 2
		} else {
			i++
		}
	}
	return b
}
