package tools

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Richard2091/DeskBridge/internal/protocol"
)

func testTool(name string) *Tool {
	return &Tool{Spec: protocol.ToolSpec{Name: name, Description: "测试工具", Source: protocol.SourceBuiltin}}
}

func TestRegistryBasics(t *testing.T) {
	r := NewRegistry()
	if _, ok := r.Get("a"); ok {
		t.Error("空注册表不应有工具")
	}
	if specs := r.Specs(); specs == nil || len(specs) != 0 {
		t.Errorf("空注册表 Specs() = %v, 期望空切片", specs)
	}
	for _, name := range []string{"c", "a", "b"} {
		if err := r.Register(testTool(name)); err != nil {
			t.Fatalf("Register(%s): %v", name, err)
		}
	}
	if err := r.Register(&Tool{}); err == nil {
		t.Error("无名工具应报错")
	}
	if err := r.Register(nil); err == nil {
		t.Error("注册 nil 应报错")
	}
	// Specs 按名称排序。
	specs := r.Specs()
	if len(specs) != 3 || specs[0].Name != "a" || specs[1].Name != "b" || specs[2].Name != "c" {
		t.Errorf("Specs() = %v, 期望按名称排序的 a b c", specs)
	}
	// 覆盖同名。
	replaced := testTool("a")
	replaced.Spec.Description = "替换后"
	if err := r.Register(replaced); err != nil {
		t.Fatalf("覆盖注册: %v", err)
	}
	got, ok := r.Get("a")
	if !ok || got.Spec.Description != "替换后" {
		t.Errorf("覆盖注册失败: %+v", got)
	}
	// 移除。
	r.Unregister("b")
	if _, ok := r.Get("b"); ok {
		t.Error("Unregister 后不应再有该工具")
	}
	r.Unregister("不存在") // 不应 panic
	if len(r.Specs()) != 2 {
		t.Errorf("移除后剩 %d 个工具", len(r.Specs()))
	}
}

func TestRegistryConcurrent(t *testing.T) {
	// 并发读写不应出现数据竞争（配合 -race 跑）。
	r := NewRegistry()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			name := string(rune('a' + i))
			_ = r.Register(testTool(name))
			r.Get(name)
			r.Specs()
			r.Unregister(name)
		}(i)
	}
	wg.Wait()
}

func TestDecodeArgs(t *testing.T) {
	type args struct {
		Text string `json:"text"`
		N    int    `json:"n"`
	}
	t.Run("正常", func(t *testing.T) {
		var a args
		if err := DecodeArgs(json.RawMessage(`{"text":"你好","n":3}`), &a); err != nil {
			t.Fatalf("DecodeArgs: %v", err)
		}
		if a.Text != "你好" || a.N != 3 {
			t.Errorf("解码结果 = %+v", a)
		}
	})
	t.Run("空和null", func(t *testing.T) {
		var a args
		if err := DecodeArgs(nil, &a); err != nil {
			t.Errorf("nil: %v", err)
		}
		if err := DecodeArgs(json.RawMessage("null"), &a); err != nil {
			t.Errorf("null: %v", err)
		}
	})
	t.Run("未知字段", func(t *testing.T) {
		var a args
		err := DecodeArgs(json.RawMessage(`{"nope":1}`), &a)
		var perr *protocol.Error
		if err == nil {
			t.Fatal("应报错")
		}
		if perrOK := asProtocolError(err, &perr); !perrOK || perr.Code != protocol.CodeInvalidArgs {
			t.Fatalf("错误 = %v, 期望 invalid_args", err)
		}
		if !strings.Contains(perr.Message, "nope") {
			t.Errorf("错误消息应包含字段名: %q", perr.Message)
		}
	})
	t.Run("类型错误", func(t *testing.T) {
		var a args
		err := DecodeArgs(json.RawMessage(`{"text":123}`), &a)
		var perr *protocol.Error
		if err == nil {
			t.Fatal("应报错")
		}
		if perrOK := asProtocolError(err, &perr); !perrOK || perr.Code != protocol.CodeInvalidArgs {
			t.Fatalf("错误 = %v, 期望 invalid_args", err)
		}
		if !strings.Contains(perr.Message, "text") {
			t.Errorf("错误消息应包含字段名: %q", perr.Message)
		}
	})
	t.Run("多余内容", func(t *testing.T) {
		var a args
		if err := DecodeArgs(json.RawMessage(`{} {}`), &a); err == nil {
			t.Error("应报错")
		}
	})
}

func asProtocolError(err error, target **protocol.Error) bool {
	if perr, ok := err.(*protocol.Error); ok {
		*target = perr
		return true
	}
	return false
}

// 确认 DefaultTimeout 和 MaxTimeout 是说明里的值。
func TestTimeoutConstants(t *testing.T) {
	if DefaultTimeout != 60*time.Second {
		t.Errorf("DefaultTimeout = %v", DefaultTimeout)
	}
	if MaxTimeout != 10*time.Minute {
		t.Errorf("MaxTimeout = %v", MaxTimeout)
	}
}
