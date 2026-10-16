package redact

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSummarize(t *testing.T) {
	tests := []struct {
		name    string
		args    string
		want    []string // 必须出现
		notWant []string // 不能出现
	}{
		{"空参数", ``, nil, nil},
		{"null", `null`, nil, nil},
		{"普通参数原样保留", `{"command":"dir","cwd":"D:\\x"}`, []string{`"command":"dir"`, `D:\\x`}, nil},
		{"敏感键整个隐藏", `{"password":"hunter2","API_Token":"abc","user":"me"}`, []string{`"password":"***"`, `"API_Token":"***"`, `"user":"me"`}, []string{"hunter2", "abc"}},
		{"嵌套对象里的敏感键", `{"a":{"client_secret":"s3"}}`, []string{`"client_secret":"***"`}, []string{"s3"}},
		{"文件内容只记长度", `{"path":"a.txt","content":"top secret data"}`, []string{`"content":"<15 字节>"`}, []string{"top secret data"}},
		{"标准输入只记长度", `{"stdin":"abc"}`, []string{`<3 字节>`}, []string{`"abc"`}},
		{"环境变量只留名字", `{"env":{"DB_URL":"postgres://u:p@h/db"}}`, []string{`"DB_URL":"***"`}, []string{"postgres"}},
		{"命令行里的赋值", `{"command":"curl -H 'x' --token=abcdef http://h"}`, []string{`--token=***`}, []string{"abcdef"}},
		{"命令行里的选项加空格", `{"command":"net use -Password hunter2 x"}`, []string{`-Password`, `***`}, []string{"hunter2"}},
		{"Bearer", `{"command":"curl -H \"Authorization: Bearer abcdefghijkl\""}`, []string{`***`}, []string{"abcdefghijkl"}},
		{"普通英文里的 token 一词不误伤", `{"command":"echo the token list"}`, []string{`the token list`}, nil},
		{"argv 数组", `{"argv":["tool","--password=p1"]}`, []string{`--password=***`}, []string{"p1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Summarize(json.RawMessage(tt.args))
			if tt.args == "" || tt.args == "null" {
				if got != "" {
					t.Fatalf("应为空，得到 %q", got)
				}
				return
			}
			for _, w := range tt.want {
				if !strings.Contains(got, w) {
					t.Errorf("摘要 %q 应包含 %q", got, w)
				}
			}
			for _, w := range tt.notWant {
				if strings.Contains(got, w) {
					t.Errorf("摘要 %q 不应包含 %q", got, w)
				}
			}
		})
	}
}

func TestSummarizeLimits(t *testing.T) {
	long := strings.Repeat("中", 5000)
	got := Summarize(json.RawMessage(`{"command":"` + long + `"}`))
	if len(got) > maxSummaryBytes+len("…") {
		t.Fatalf("摘要过长：%d 字节", len(got))
	}
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("超长摘要应以省略号结尾：%q", got[len(got)-10:])
	}
	if !strings.Contains(Summarize(json.RawMessage(`not json`)), "无法解析") {
		t.Fatal("非法 JSON 应给出提示")
	}
	many := make([]string, 50)
	for i := range many {
		many[i] = `"x"`
	}
	got = Summarize(json.RawMessage(`{"argv":[` + strings.Join(many, ",") + `]}`))
	if !strings.Contains(got, "共 50 项") {
		t.Fatalf("长数组应标明总数：%s", got)
	}
}
