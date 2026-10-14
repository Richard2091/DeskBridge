//go:build windows

package builtin

import (
	"strings"
	"testing"
	"unicode/utf8"

	"golang.org/x/sys/windows"
)

// TestDecodeCodePageGBK 验证 GBK 字节在代码页 936 下解码正确。
func TestDecodeCodePageGBK(t *testing.T) {
	// "中文" 的 GBK 编码。
	got, ok := decodeCodePage([]byte{0xD6, 0xD0, 0xCE, 0xC4}, 936)
	if !ok {
		t.Fatal("用代码页 936 解码 GBK 字节失败")
	}
	if got != "中文" {
		t.Errorf("解码结果 = %q，期望 中文", got)
	}

	// 非法字节应失败，走下一层兜底。
	if _, ok := decodeCodePage([]byte{0xFF, 0xFE, 0x81, 0x27}, 936); ok {
		t.Error("非法 GBK 字节不应解码成功")
	}
}

// TestProcExecCmdEchoChinese 验证 cmd 内部命令的 GBK 输出能正确解码。
func TestProcExecCmdEchoChinese(t *testing.T) {
	res, err := callExec(t, map[string]any{"command": "echo 中文测试", "shell": "cmd"}, 0)
	if err != nil {
		t.Fatalf("proc.exec 失败: %v", err)
	}
	if got := strings.TrimSpace(res.Stdout); got != "中文测试" {
		t.Errorf("stdout = %q，期望 中文测试", got)
	}
}

// TestProcExecCmdUnknownCommand 验证 cmd 的错误输出是合法 UTF-8，
// 中文系统上包含提示文本。
func TestProcExecCmdUnknownCommand(t *testing.T) {
	res, err := callExec(t, map[string]any{"command": "不存在的命令xyz", "shell": "cmd"}, 0)
	if err != nil {
		t.Fatalf("proc.exec 失败: %v", err)
	}
	if res.ExitCode == 0 {
		t.Errorf("不存在的命令 exit_code = %d，应为非 0", res.ExitCode)
	}
	if res.Stderr == "" {
		t.Fatal("stderr 为空")
	}
	if !utf8.ValidString(res.Stderr) {
		t.Fatalf("stderr 不是合法 UTF-8：%q", res.Stderr)
	}
	if strings.Contains(res.Stderr, "is not recognized") {
		t.Skip("英文系统，跳过中文提示断言")
	}
	if !strings.Contains(res.Stderr, "不是内部或外部命令") {
		t.Errorf("stderr = %q，应包含 不是内部或外部命令", res.Stderr)
	}
}

// TestProcExecPowerShellChinese 验证 PowerShell 的输出和错误流。
func TestProcExecPowerShellChinese(t *testing.T) {
	res, err := callExec(t, map[string]any{"command": "Write-Output '中文'", "shell": "powershell"}, 0)
	if err != nil {
		t.Fatalf("proc.exec 失败: %v", err)
	}
	if got := strings.TrimSpace(res.Stdout); got != "中文" {
		t.Errorf("stdout = %q，期望 中文", got)
	}

	res, err = callExec(t, map[string]any{"command": "Write-Error '中文错误'", "shell": "powershell"}, 0)
	if err != nil {
		t.Fatalf("proc.exec 失败: %v", err)
	}
	if !utf8.ValidString(res.Stderr) {
		t.Fatalf("stderr 不是合法 UTF-8：%q", res.Stderr)
	}
	if !strings.Contains(res.Stderr, "中文错误") {
		t.Errorf("stderr = %q，应包含 中文错误", res.Stderr)
	}
}

// TestSysInfoCodePages 验证 sys.info 带 Windows 特有的字段。
func TestSysInfoCodePages(t *testing.T) {
	r := newTestRegistry(t)
	v, err := callByName(t, r, "sys.info", "")
	if err != nil {
		t.Fatalf("sys.info 失败: %v", err)
	}
	res := v.(*sysInfoResult)
	if res.Codepages == nil {
		t.Fatal("Windows 上 codepages 不应省略")
	}
	if res.Codepages.ANSI == 0 || res.Codepages.OEM == 0 {
		t.Errorf("codepages = %+v，代码页不应为 0", res.Codepages)
	}
	if int(windows.GetACP()) != res.Codepages.ANSI {
		t.Errorf("ansi = %d，GetACP = %d", res.Codepages.ANSI, windows.GetACP())
	}
	if res.Session == nil {
		t.Fatal("Windows 上 session 不应省略")
	}
	if res.Session.ID <= 0 {
		t.Errorf("session.id = %d", res.Session.ID)
	}
}
