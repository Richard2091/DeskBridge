package protocol

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestHTTPStatus(t *testing.T) {
	tests := []struct {
		code string
		want int
	}{
		{CodeInvalidArgs, http.StatusBadRequest},
		{CodeUnknownTool, http.StatusNotFound},
		{CodeNotFound, http.StatusNotFound},
		{CodeDeviceNotFound, http.StatusNotFound},
		{CodePermissionDenied, http.StatusForbidden},
		{CodeDuplicateRequest, http.StatusConflict},
		{CodePayloadTooLarge, http.StatusRequestEntityTooLarge},
		{CodeCancelled, 499},
		{CodeInternal, http.StatusInternalServerError},
		{CodeUnsupportedPlatform, http.StatusNotImplemented},
		{CodeUnavailable, http.StatusBadGateway},
		{CodeTimeout, http.StatusGatewayTimeout},
		{"no_such_code", http.StatusInternalServerError},
	}
	for _, tt := range tests {
		if got := HTTPStatus(tt.code); got != tt.want {
			t.Errorf("HTTPStatus(%q) = %d, 期望 %d", tt.code, got, tt.want)
		}
	}
}

func TestErrorString(t *testing.T) {
	e := Errorf(CodeInvalidArgs, "参数 %s 不合法", "text")
	if e.Error() != "invalid_args: 参数 text 不合法" {
		t.Errorf("Error() = %q", e.Error())
	}
	var target error = e
	if target.Error() == "" {
		t.Error("error 接口返回空")
	}
}

func TestCallResponseJSON(t *testing.T) {
	// 错误响应的线上格式：{"ok":false,"error":{"code","message","details"}}。
	resp := CallResponse{
		OK:         false,
		RequestID:  "abc",
		DurationMS: 12,
		Error: &Error{
			Code:    CodePermissionDenied,
			Message: "只接受同一账号的设备",
			Details: map[string]any{"missing": "same_owner"},
		},
	}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}
	var back map[string]any
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if back["ok"] != false || back["request_id"] != "abc" {
		t.Errorf("字段不对: %s", data)
	}
	errObj, ok := back["error"].(map[string]any)
	if !ok || errObj["code"] != CodePermissionDenied || errObj["missing"] != nil {
		t.Errorf("error 字段不对: %s", data)
	}
	if errObj["details"].(map[string]any)["missing"] != "same_owner" {
		t.Errorf("details 不对: %s", data)
	}

	// 成功响应不包含 error 字段。
	okResp := CallResponse{OK: true, RequestID: "x", Result: json.RawMessage(`{"a":1}`)}
	data, _ = json.Marshal(okResp)
	if strings.Contains(string(data), `"error"`) {
		t.Errorf("成功响应不应有 error 字段: %s", data)
	}
}

func TestCapabilityJSON(t *testing.T) {
	capability := Capability{
		Protocol:  Version,
		DeviceID:  "abcd1234abcd1234abcd1234abcd1234",
		PublicKey: "c2FtcGxlLWtleQ==",
		Name:      "win-pc",
		Version:   "dev",
		Platform:  Platform{OS: "windows", Arch: "amd64"},
		Tools: []ToolSpec{{
			Name:        "proc.exec",
			Description: "执行命令",
			Source:      SourceBuiltin,
			InputSchema: json.RawMessage(`{"type":"object"}`),
		}},
	}
	data, err := json.Marshal(capability)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}
	for _, key := range []string{
		`"protocol":1`, `"device_id"`, `"public_key"`, `"name":"win-pc"`,
		`"platform"`, `"os":"windows"`, `"arch":"amd64"`, `"tools"`,
		`"name":"proc.exec"`, `"source":"builtin"`, `"input_schema"`,
	} {
		if !strings.Contains(string(data), key) {
			t.Errorf("JSON 缺少 %s：%s", key, data)
		}
	}
}

func TestNewRequestID(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := NewRequestID()
		if len(id) != 32 {
			t.Fatalf("request_id 长度 = %d, 期望 32（%q）", len(id), id)
		}
		if seen[id] {
			t.Fatalf("request_id 重复: %s", id)
		}
		seen[id] = true
	}
}

func TestInitiatorAndCallRequestJSON(t *testing.T) {
	req := CallRequest{
		Args:      json.RawMessage(`{"text":"hi"}`),
		TimeoutMS: 1000,
		Initiator: &Initiator{Type: InitiatorAI, Via: "mcp", Session: "s1"},
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}
	var back CallRequest
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if back.TimeoutMS != 1000 || back.Initiator == nil ||
		back.Initiator.Type != InitiatorAI || back.Initiator.Via != "mcp" || back.Initiator.Session != "s1" {
		t.Errorf("往返结果不对: %+v", back)
	}
	if string(back.Args) != `{"text":"hi"}` {
		t.Errorf("args 往返不对: %s", back.Args)
	}
}
