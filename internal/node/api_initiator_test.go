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
