package node

import (
	"net/http"
	"strings"
	"testing"

	"github.com/Richard2091/DeskBridge/internal/protocol"
	"github.com/Richard2091/DeskBridge/internal/tools"
)

func TestAPIInitiatorValidation(t *testing.T) {
	srv, _ := fixedAPI(t, newAPINode(t))
	long := strings.Repeat("a", protocol.MaxInitiatorSession+1)
	bad := []struct{ name, init string }{
		{"类型未知", `{"type":"robot"}`},
		{"途径未知", `{"type":"ai","via":"ssh"}`},
		{"会话含换行", `{"type":"ai","via":"mcp","session":"a\nb"}`},
		{"会话含控制字符", `{"session":"a\u0000b"}`},
		{"会话过长", `{"session":"` + long + `"}`},
	}
	for _, tt := range bad {
		t.Run(tt.name, func(t *testing.T) {
			resp, out := postCall(t, srv.URL+"/v1/tools/test.echo", `{"args":{"text":"x"},"initiator":`+tt.init+`}`)
			wantProtoError(t, resp, out, protocol.CodeInvalidArgs)
		})
	}
	good := []string{
		`{}`,
		`{"type":"ai","via":"mcp","session":"s-1"}`,
		`{"type":"human","via":"ui"}`,
		`{"session":"` + strings.Repeat("a", protocol.MaxInitiatorSession) + `"}`,
	}
	for _, g := range good {
		resp, out := postCall(t, srv.URL+"/v1/tools/test.echo", `{"args":{"text":"x"},"initiator":`+g+`}`)
		if resp.StatusCode != http.StatusOK || !out.OK {
			t.Errorf("initiator %s 应通过：%d %+v", g, resp.StatusCode, out.Error)
		}
	}
}

func TestAuditInitiatorQuoted(t *testing.T) {
	r := auditRecord{Initiator: protocol.Initiator{Type: "ai", Via: "mcp", Session: "x\ncode=ok"}}
	line := r.String()
	if strings.Contains(line, "\n") {
		t.Fatalf("审计行含换行：%q", line)
	}
	if !strings.Contains(line, `session="x\ncode=ok"`) {
		t.Fatalf("session 未用 %%q 输出：%s", line)
	}
}

func TestCallTimeoutOverflow(t *testing.T) {
	tool := &tools.Tool{}
	for _, ms := range []int{1, 1000, int(tools.MaxTimeout.Milliseconds()), 1 << 40, 10_000_000_000_000, int(^uint(0) >> 1)} {
		d := callTimeout(tool, ms)
		if d <= 0 || d > tools.MaxTimeout {
			t.Errorf("callTimeout(%d) = %v", ms, d)
		}
	}
	if d := callTimeout(tool, 10_000_000_000_000); d != tools.MaxTimeout {
		t.Errorf("大值应截到上限，得到 %v", d)
	}
	if d := callTimeout(tool, -5); d != tools.DefaultTimeout {
		t.Errorf("负数应视为未指定，得到 %v", d)
	}
	// 工具自己的上限超过全局上限时也要截到全局上限，客户端才等得够久。
	big := &tools.Tool{MaxTimeout: 2 * tools.MaxTimeout, DefaultTimeout: 3 * tools.MaxTimeout}
	if d := callTimeout(big, 0); d != tools.MaxTimeout {
		t.Errorf("默认值应受全局上限约束，得到 %v", d)
	}
	if d := callTimeout(big, int(2*tools.MaxTimeout.Milliseconds())); d != tools.MaxTimeout {
		t.Errorf("工具上限应受全局上限约束，得到 %v", d)
	}
}

func TestInitiatorOfDefaults(t *testing.T) {
	tests := []struct {
		name string
		in   *protocol.Initiator
		want protocol.Initiator
	}{
		{"未声明", nil, protocol.Initiator{Type: "human", Via: "cli"}},
		{"空对象", &protocol.Initiator{}, protocol.Initiator{Type: "human", Via: "cli"}},
		{"只给会话", &protocol.Initiator{Session: "s-1"}, protocol.Initiator{Type: "human", Via: "cli", Session: "s-1"}},
		{"只给类型 ai", &protocol.Initiator{Type: "ai"}, protocol.Initiator{Type: "ai", Via: "cli"}},
		{"全部给出", &protocol.Initiator{Type: "ai", Via: "mcp", Session: "s"}, protocol.Initiator{Type: "ai", Via: "mcp", Session: "s"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := initiatorOf(&protocol.CallRequest{Initiator: tt.in})
			if got != tt.want {
				t.Errorf("initiatorOf = %+v, 期望 %+v", got, tt.want)
			}
		})
	}
}

func TestAuditRequestIDAndToolQuoted(t *testing.T) {
	r := auditRecord{RequestID: "a\nb code=ok", Tool: "t\nx"}
	line := r.String()
	if strings.Contains(line, "\n") {
		t.Fatalf("审计行含换行：%q", line)
	}
	if !strings.Contains(line, `request_id="a\nb code=ok"`) || !strings.Contains(line, `tool="t\nx"`) {
		t.Fatalf("request_id 或 tool 未用 %%q 输出：%s", line)
	}
}

func TestAPIRequestIDValidation(t *testing.T) {
	srv, _ := fixedAPI(t, newAPINode(t))
	long := strings.Repeat("a", protocol.MaxRequestID+1)
	bad := []string{`"a\nb"`, `"a\u0000b"`, `"` + long + `"`}
	for _, id := range bad {
		resp, out := postCall(t, srv.URL+"/v1/tools/test.echo", `{"args":{"text":"x"},"request_id":`+id+`}`)
		wantProtoError(t, resp, out, protocol.CodeInvalidArgs)
	}
	good := []string{`"req-1"`, `"` + strings.Repeat("a", protocol.MaxRequestID) + `"`}
	for _, id := range good {
		resp, out := postCall(t, srv.URL+"/v1/tools/test.echo", `{"args":{"text":"x"},"request_id":`+id+`}`)
		if resp.StatusCode != http.StatusOK || !out.OK {
			t.Errorf("request_id %s 应通过：%d %+v", id, resp.StatusCode, out.Error)
		}
	}
}
