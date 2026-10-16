package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestRun(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantStdout string
		wantStderr string
	}{
		{"无参数显示用法", nil, exitUsage, "", "用法："},
		{"帮助", []string{"help"}, exitOK, "用法：", ""},
		{"帮助选项", []string{"--help"}, exitOK, "用法：", ""},
		{"版本", []string{"version"}, exitOK, "deskbridge dev", ""},
		{"版本选项", []string{"--version"}, exitOK, "deskbridge dev", ""},
		{"未知子命令", []string{"nope"}, exitUsage, "", "未知的子命令：nope"},
		{"ui-core 不接受位置参数", []string{"ui-core", "x"}, exitUsage, "", "未知参数：x"},
		{"capability 缺参数", []string{"capability"}, exitUsage, "", "用法："},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := Run(tt.args, &stdout, &stderr)
			if code != tt.wantCode {
				t.Errorf("退出码 = %d，期望 %d", code, tt.wantCode)
			}
			if tt.wantStdout != "" && !strings.Contains(stdout.String(), tt.wantStdout) {
				t.Errorf("标准输出 = %q，期望包含 %q", stdout.String(), tt.wantStdout)
			}
			if tt.wantStderr != "" && !strings.Contains(stderr.String(), tt.wantStderr) {
				t.Errorf("标准错误 = %q，期望包含 %q", stderr.String(), tt.wantStderr)
			}
		})
	}
}
