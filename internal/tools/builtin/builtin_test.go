package builtin

import (
	"context"
	"encoding/json"
	"runtime"
	"testing"

	"github.com/Richard2091/DeskBridge/internal/protocol"
	"github.com/Richard2091/DeskBridge/internal/tools"
)

// newTestRegistry 建一个注册了内置工具的注册表。
func newTestRegistry(t *testing.T) *tools.Registry {
	t.Helper()
	r := tools.NewRegistry()
	if err := Register(r, Info{Name: "测试设备", Version: "test-1.0"}); err != nil {
		t.Fatalf("Register 失败: %v", err)
	}
	return r
}

// callByName 按名字调用注册表里的工具，返回结果或协议错误。
func callByName(t *testing.T, r *tools.Registry, name, args string) (any, error) {
	t.Helper()
	tool, ok := r.Get(name)
	if !ok {
		t.Fatalf("工具 %s 没有注册", name)
	}
	var raw json.RawMessage
	if args != "" {
		raw = json.RawMessage(args)
	}
	return tool.Run(context.Background(), &tools.Call{RequestID: "test", Args: raw})
}

// expectProtoError 断言 err 是指定错误码的协议错误。
func expectProtoError(t *testing.T, err error, code string) {
	t.Helper()
	if err == nil {
		t.Fatalf("期望错误 %s，得到 nil", code)
	}
	var perr *protocol.Error
	if !asProtoError(err, &perr) {
		t.Fatalf("期望协议错误，得到 %T: %v", err, err)
	}
	if perr.Code != code {
		t.Fatalf("期望错误码 %s，得到 %s（%v）", code, perr.Code, err)
	}
}

// asProtoError 包装 errors.As，避免每个测试都导入 errors。
func asProtoError(err error, target **protocol.Error) bool {
	for err != nil {
		if e, ok := err.(*protocol.Error); ok {
			*target = e
			return true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}

// expectedTools 是各平台应该注册的工具集合。
func expectedTools() []string {
	base := []string{"sys.info", "fs.stat", "fs.list", "fs.read", "fs.write", "proc.exec"}
	browser := []string{
		"browser.open", "browser.close", "browser.info", "browser.navigate",
		"browser.snapshot", "browser.click", "browser.type", "browser.key",
		"browser.wait", "browser.screenshot", "browser.eval", "browser.console",
		"browser.network", "browser.tabs", "browser.cookies",
	}
	base = append(base, browser...)
	switch runtime.GOOS {
	case "windows", "linux":
		return append(base, "proc.list", "proc.kill")
	default:
		return base
	}
}

func TestRegisterRegistersExpectedTools(t *testing.T) {
	r := newTestRegistry(t)
	want := expectedTools()
	for _, name := range want {
		tool, ok := r.Get(name)
		if !ok {
			t.Errorf("工具 %s 没有注册", name)
			continue
		}
		if tool.Spec.Description == "" {
			t.Errorf("工具 %s 缺少描述", name)
		}
		if tool.Spec.Source != protocol.SourceBuiltin {
			t.Errorf("工具 %s 的来源是 %s，期望 builtin", name, tool.Spec.Source)
		}
		if len(tool.Spec.InputSchema) == 0 {
			t.Errorf("工具 %s 缺少 InputSchema", name)
			continue
		}
		var schema map[string]any
		if err := json.Unmarshal(tool.Spec.InputSchema, &schema); err != nil {
			t.Errorf("工具 %s 的 InputSchema 不是合法 JSON: %v", name, err)
			continue
		}
		if schema["additionalProperties"] != false {
			t.Errorf("工具 %s 的 InputSchema 缺少 additionalProperties:false", name)
		}
	}
}

func TestSysInfo(t *testing.T) {
	r := newTestRegistry(t)
	v, err := callByName(t, r, "sys.info", "")
	if err != nil {
		t.Fatalf("sys.info 失败: %v", err)
	}
	res, ok := v.(*sysInfoResult)
	if !ok {
		t.Fatalf("结果类型不对：%T", v)
	}
	if res.Name != "测试设备" {
		t.Errorf("name = %q，期望 测试设备", res.Name)
	}
	if res.Version != "test-1.0" {
		t.Errorf("version = %q，期望 test-1.0", res.Version)
	}
	if res.OS != runtime.GOOS || res.Arch != runtime.GOARCH {
		t.Errorf("os/arch = %s/%s，期望 %s/%s", res.OS, res.Arch, runtime.GOOS, runtime.GOARCH)
	}
	if res.Hostname == "" {
		t.Error("hostname 为空")
	}
	if res.NumCPU <= 0 {
		t.Errorf("num_cpu = %d，应为正数", res.NumCPU)
	}
	if res.Timezone == "" {
		t.Error("timezone 为空")
	}
}

func TestSysInfoRejectsAnyArgs(t *testing.T) {
	r := newTestRegistry(t)
	_, err := callByName(t, r, "sys.info", `{"foo":1}`)
	expectProtoError(t, err, protocol.CodeInvalidArgs)
}
