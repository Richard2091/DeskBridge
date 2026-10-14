package builtin

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/Richard2091/DeskBridge/internal/protocol"
)

func TestFsStatFile(t *testing.T) {
	r := newTestRegistry(t)
	dir := t.TempDir()
	file := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(file, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	v, err := callByName(t, r, "fs.stat", `{"path":`+jsonString(file)+`}`)
	if err != nil {
		t.Fatalf("fs.stat 失败: %v", err)
	}
	res := v.(*fsStatResult)
	if res.Type != "file" {
		t.Errorf("type = %s，期望 file", res.Type)
	}
	if res.Size != 5 {
		t.Errorf("size = %d，期望 5", res.Size)
	}
	if res.Mode == "" || res.Mtime == "" {
		t.Errorf("mode/mtime 为空：%q %q", res.Mode, res.Mtime)
	}
	if !filepath.IsAbs(res.Path) {
		t.Errorf("path 不是绝对路径：%s", res.Path)
	}
}

func TestFsStatDirAndMissing(t *testing.T) {
	r := newTestRegistry(t)
	dir := t.TempDir()
	v, err := callByName(t, r, "fs.stat", `{"path":`+jsonString(dir)+`}`)
	if err != nil {
		t.Fatalf("fs.stat 失败: %v", err)
	}
	if res := v.(*fsStatResult); res.Type != "dir" {
		t.Errorf("type = %s，期望 dir", res.Type)
	}

	_, err = callByName(t, r, "fs.stat", `{"path":`+jsonString(filepath.Join(dir, "不存在"))+`}`)
	expectProtoError(t, err, protocol.CodeNotFound)

	_, err = callByName(t, r, "fs.stat", `{"path":`+jsonString(dir)+`,"extra":1}`)
	expectProtoError(t, err, protocol.CodeInvalidArgs)
}

func TestFsList(t *testing.T) {
	r := newTestRegistry(t)
	dir := t.TempDir()
	write := func(rel, content string) {
		t.Helper()
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write("x.txt", "xx")
	write("a.txt", "a")
	write("sub/inner.txt", "iii")

	v, err := callByName(t, r, "fs.list", `{"path":`+jsonString(dir)+`}`)
	if err != nil {
		t.Fatalf("fs.list 失败: %v", err)
	}
	res := v.(*fsListResult)
	// 非递归：a.txt、sub、x.txt 三个条目，按 name 排序。
	if len(res.Entries) != 3 {
		t.Fatalf("条目数 = %d，期望 3（%v）", len(res.Entries), res.Entries)
	}
	wantNames := []string{"a.txt", "sub", "x.txt"}
	for i, e := range res.Entries {
		if e.Name != wantNames[i] {
			t.Errorf("第 %d 项 name = %s，期望 %s", i, e.Name, wantNames[i])
		}
	}
	if res.Entries[1].Type != "dir" || res.Entries[0].Type != "file" {
		t.Errorf("type 不对：%s %s", res.Entries[0].Type, res.Entries[1].Type)
	}
	if res.Truncated {
		t.Error("truncated 应为 false")
	}

	// 递归：name 是相对路径，用 / 分隔。
	v, err = callByName(t, r, "fs.list", `{"path":`+jsonString(dir)+`,"recursive":true}`)
	if err != nil {
		t.Fatalf("fs.list 递归失败: %v", err)
	}
	res = v.(*fsListResult)
	var found bool
	for _, e := range res.Entries {
		if e.Name == "sub/inner.txt" {
			found = true
		}
	}
	if !found {
		t.Errorf("递归结果里没有 sub/inner.txt：%v", res.Entries)
	}

	// max_entries 截断。
	v, err = callByName(t, r, "fs.list", `{"path":`+jsonString(dir)+`,"recursive":true,"max_entries":2}`)
	if err != nil {
		t.Fatalf("fs.list 失败: %v", err)
	}
	res = v.(*fsListResult)
	if len(res.Entries) != 2 || !res.Truncated {
		t.Errorf("截断结果不对：n=%d truncated=%v", len(res.Entries), res.Truncated)
	}

	// path 是文件 → invalid_args。
	_, err = callByName(t, r, "fs.list", `{"path":`+jsonString(filepath.Join(dir, "a.txt"))+`}`)
	expectProtoError(t, err, protocol.CodeInvalidArgs)
}

func TestFsReadChunks(t *testing.T) {
	r := newTestRegistry(t)
	dir := t.TempDir()
	file := filepath.Join(dir, "data.txt")
	content := "0123456789abcdefghij"
	if err := os.WriteFile(file, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	// 用 4 字节的块把整个文件读完，拼起来应该等于原文。
	var got string
	for offset := 0; ; {
		v, err := callByName(t, r, "fs.read",
			`{"path":`+jsonString(file)+`,"offset":`+itoa(offset)+`,"max_bytes":4}`)
		if err != nil {
			t.Fatalf("fs.read 失败: %v", err)
		}
		res := v.(*fsReadResult)
		if res.Encoding != "utf-8" {
			t.Fatalf("encoding = %s，期望 utf-8", res.Encoding)
		}
		got += res.Content
		if res.EOF {
			break
		}
		offset += int(res.BytesRead)
	}
	if got != content {
		t.Errorf("分块读取拼接 = %q，期望 %q", got, content)
	}
}

func TestFsReadUTF8Boundary(t *testing.T) {
	r := newTestRegistry(t)
	dir := t.TempDir()
	file := filepath.Join(dir, "中文.txt")
	// 每个汉字 3 字节，"中文测试" 共 12 字节。
	if err := os.WriteFile(file, []byte("中文测试"), 0644); err != nil {
		t.Fatal(err)
	}

	// max_bytes=7 会截在第三个字符中间，应回退到 6 字节（两个字符）。
	v, err := callByName(t, r, "fs.read", `{"path":`+jsonString(file)+`,"max_bytes":7}`)
	if err != nil {
		t.Fatalf("fs.read 失败: %v", err)
	}
	res := v.(*fsReadResult)
	if res.Content != "中文" {
		t.Errorf("content = %q，期望 中文", res.Content)
	}
	if res.BytesRead != 6 {
		t.Errorf("bytes_read = %d，期望 6（回退到字符边界）", res.BytesRead)
	}

	// 从回退处继续，应读到后半部分。
	v, err = callByName(t, r, "fs.read", `{"path":`+jsonString(file)+`,"offset":6}`)
	if err != nil {
		t.Fatalf("fs.read 失败: %v", err)
	}
	if res := v.(*fsReadResult); res.Content != "测试" || !res.EOF {
		t.Errorf("续读 = %q eof=%v，期望 测试 true", res.Content, res.EOF)
	}
}

func TestFsReadBOM(t *testing.T) {
	r := newTestRegistry(t)
	dir := t.TempDir()
	file := filepath.Join(dir, "bom.txt")
	if err := os.WriteFile(file, []byte("\xEF\xBB\xBFabc"), 0644); err != nil {
		t.Fatal(err)
	}
	v, err := callByName(t, r, "fs.read", `{"path":`+jsonString(file)+`}`)
	if err != nil {
		t.Fatalf("fs.read 失败: %v", err)
	}
	res := v.(*fsReadResult)
	if res.Content != "abc" {
		t.Errorf("content = %q，期望 abc（BOM 应去掉）", res.Content)
	}
	// bytes_read 包含 BOM 的 3 个字节（docs/tools.md）。
	if res.BytesRead != 6 {
		t.Errorf("bytes_read = %d，期望 6（含 BOM）", res.BytesRead)
	}
}

func TestFsReadEncodings(t *testing.T) {
	r := newTestRegistry(t)
	dir := t.TempDir()

	// utf-8：非法字节替换为 U+FFFD。
	file := filepath.Join(dir, "bad.txt")
	if err := os.WriteFile(file, []byte("a\xffb"), 0644); err != nil {
		t.Fatal(err)
	}
	v, err := callByName(t, r, "fs.read", `{"path":`+jsonString(file)+`,"encoding":"utf-8"}`)
	if err != nil {
		t.Fatalf("fs.read 失败: %v", err)
	}
	if res := v.(*fsReadResult); res.Content != "a\uFFFDb" {
		t.Errorf("content = %q，期望 a\uFFFDb", res.Content)
	}

	// auto：GBK 等非法 UTF-8 的内容按 base64 返回，不去 BOM。
	gbk := filepath.Join(dir, "gbk.txt")
	if err := os.WriteFile(gbk, []byte{0xD6, 0xD0, 0xCE, 0xC4}, 0644); err != nil {
		t.Fatal(err)
	}
	v, err = callByName(t, r, "fs.read", `{"path":`+jsonString(gbk)+`}`)
	if err != nil {
		t.Fatalf("fs.read 失败: %v", err)
	}
	res := v.(*fsReadResult)
	if res.Encoding != "base64" {
		t.Fatalf("encoding = %s，期望 base64", res.Encoding)
	}
	want := base64.StdEncoding.EncodeToString([]byte{0xD6, 0xD0, 0xCE, 0xC4})
	if res.Content != want {
		t.Errorf("content = %q，期望 %q", res.Content, want)
	}

	// base64：总是 base64。
	v, err = callByName(t, r, "fs.read", `{"path":`+jsonString(gbk)+`,"encoding":"base64"}`)
	if err != nil {
		t.Fatalf("fs.read 失败: %v", err)
	}
	if res := v.(*fsReadResult); res.Content != want {
		t.Errorf("base64 content = %q，期望 %q", res.Content, want)
	}

	// 非法 encoding 参数。
	_, err = callByName(t, r, "fs.read", `{"path":`+jsonString(gbk)+`,"encoding":"gbk"}`)
	expectProtoError(t, err, protocol.CodeInvalidArgs)
}

func TestFsWrite(t *testing.T) {
	r := newTestRegistry(t)
	dir := t.TempDir()
	file := filepath.Join(dir, "w.txt")

	// 覆盖写入：文件原来不存在 → created=true。
	v, err := callByName(t, r, "fs.write", `{"path":`+jsonString(file)+`,"content":"hello"}`)
	if err != nil {
		t.Fatalf("fs.write 失败: %v", err)
	}
	res := v.(*fsWriteResult)
	if !res.Created || res.BytesWritten != 5 {
		t.Errorf("created=%v bytes_written=%d，期望 true 5", res.Created, res.BytesWritten)
	}
	if b, _ := os.ReadFile(file); string(b) != "hello" {
		t.Errorf("文件内容 = %q，期望 hello", b)
	}

	// 再次覆盖：created=false。
	v, err = callByName(t, r, "fs.write", `{"path":`+jsonString(file)+`,"content":"hi"}`)
	if err != nil {
		t.Fatalf("fs.write 失败: %v", err)
	}
	if res = v.(*fsWriteResult); res.Created {
		t.Error("第二次写入 created 应为 false")
	}
	if b, _ := os.ReadFile(file); string(b) != "hi" {
		t.Errorf("覆盖后内容 = %q，期望 hi", b)
	}

	// 追加。
	v, err = callByName(t, r, "fs.write", `{"path":`+jsonString(file)+`,"content":"world","append":true}`)
	if err != nil {
		t.Fatalf("fs.write 追加失败: %v", err)
	}
	if res = v.(*fsWriteResult); res.BytesWritten != 5 {
		t.Errorf("追加 bytes_written = %d，期望 5", res.BytesWritten)
	}
	if b, _ := os.ReadFile(file); string(b) != "hiworld" {
		t.Errorf("追加后内容 = %q，期望 hiworld", b)
	}

	// base64 内容。
	v, err = callByName(t, r, "fs.write",
		`{"path":`+jsonString(file)+`,"content":`+jsonString(base64.StdEncoding.EncodeToString([]byte("OK")))+`,"encoding":"base64"}`)
	if err != nil {
		t.Fatalf("fs.write 失败: %v", err)
	}
	if res = v.(*fsWriteResult); res.BytesWritten != 2 {
		t.Errorf("base64 bytes_written = %d，期望 2", res.BytesWritten)
	}
	if b, _ := os.ReadFile(file); string(b) != "OK" {
		t.Errorf("base64 写入后内容 = %q，期望 OK", b)
	}

	// 覆盖后权限位保留（Windows 的权限位只有只读，跳过）。
	if runtime.GOOS != "windows" {
		if err := os.Chmod(file, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := callByName(t, r, "fs.write", `{"path":`+jsonString(file)+`,"content":"x"}`); err != nil {
			t.Fatalf("fs.write 失败: %v", err)
		}
		if fi, err := os.Stat(file); err != nil || fi.Mode().Perm() != 0600 {
			t.Errorf("覆盖后权限 = %v，期望 0600", fi.Mode().Perm())
		}
	}

	// 自动创建父目录（默认 create_dirs=true）。
	nested := filepath.Join(dir, "p", "q", "n.txt")
	if _, err := callByName(t, r, "fs.write", `{"path":`+jsonString(nested)+`,"content":"n"}`); err != nil {
		t.Fatalf("fs.write 建父目录失败: %v", err)
	}

	// create_dirs=false 且父目录不存在 → 报错。
	_, err = callByName(t, r, "fs.write",
		`{"path":`+jsonString(filepath.Join(dir, "nope", "x.txt"))+`,"content":"x","create_dirs":false}`)
	if err == nil {
		t.Error("create_dirs=false 时应报错")
	}

	// 非法 base64。
	_, err = callByName(t, r, "fs.write", `{"path":`+jsonString(file)+`,"content":"!!!","encoding":"base64"}`)
	expectProtoError(t, err, protocol.CodeInvalidArgs)
}

func TestFsReadErrors(t *testing.T) {
	r := newTestRegistry(t)
	dir := t.TempDir()
	// 目录不能按文件读。
	_, err := callByName(t, r, "fs.read", `{"path":`+jsonString(dir)+`}`)
	expectProtoError(t, err, protocol.CodeInvalidArgs)
	// 不存在。
	_, err = callByName(t, r, "fs.read", `{"path":`+jsonString(filepath.Join(dir, "无"))+`}`)
	expectProtoError(t, err, protocol.CodeNotFound)
	// offset 为负。
	_, err = callByName(t, r, "fs.read", `{"path":`+jsonString(dir)+`,"offset":-1}`)
	expectProtoError(t, err, protocol.CodeInvalidArgs)
}

func TestFsListExactLimitNotTruncated(t *testing.T) {
	r := newTestRegistry(t)
	dir := t.TempDir()
	for _, n := range []string{"a", "b", "c"} {
		if err := os.WriteFile(filepath.Join(dir, n), nil, 0644); err != nil {
			t.Fatal(err)
		}
	}
	v, err := callByName(t, r, "fs.list", `{"path":`+jsonString(dir)+`,"max_entries":3}`)
	if err != nil {
		t.Fatal(err)
	}
	if res := v.(*fsListResult); len(res.Entries) != 3 || res.Truncated {
		t.Errorf("条目数恰好等于上限：n=%d truncated=%v，期望 3/false", len(res.Entries), res.Truncated)
	}
	v, err = callByName(t, r, "fs.list", `{"path":`+jsonString(dir)+`,"max_entries":2}`)
	if err != nil {
		t.Fatal(err)
	}
	if res := v.(*fsListResult); len(res.Entries) != 2 || !res.Truncated {
		t.Errorf("超过上限：n=%d truncated=%v，期望 2/true", len(res.Entries), res.Truncated)
	}
}

func TestFsWriteThroughSymlink(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real.txt")
	link := filepath.Join(dir, "link.txt")
	if err := os.WriteFile(real, []byte("old"), 0640); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("无法创建符号链接（Windows 需要权限）：%v", err)
	}
	r := newTestRegistry(t)
	if _, err := callByName(t, r, "fs.write", `{"path":`+jsonString(link)+`,"content":"new"}`); err != nil {
		t.Fatalf("fs.write 失败: %v", err)
	}
	if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf("符号链接被替换掉了：%v %v", fi, err)
	}
	if b, _ := os.ReadFile(real); string(b) != "new" {
		t.Errorf("真实文件内容 = %q，期望 new", b)
	}
	if runtime.GOOS != "windows" {
		if fi, _ := os.Stat(real); fi.Mode().Perm() != 0640 {
			t.Errorf("真实文件权限 = %v，期望 0640", fi.Mode().Perm())
		}
	}
}
