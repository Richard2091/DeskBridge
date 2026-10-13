package tools

// 工具注册表：节点上可用工具的运行时集合。工具来源会在运行时变化
// （如转发被控端的 MCP 工具），所以支持随时增删；能力报告每次请求
// 都从注册表重新读取。

import (
	"bytes"
	"context"
	"encoding/json"
	"sort"
	"sync"
	"time"

	"github.com/Richard2091/DeskBridge/internal/mesh"
	"github.com/Richard2091/DeskBridge/internal/protocol"
)

// 超时缺省值：工具没有指定时使用。
const (
	// DefaultTimeout 是 timeout_ms 为 0（未指定）时的调用超时。
	DefaultTimeout = 60 * time.Second
	// MaxTimeout 是任何调用超时的上限。
	MaxTimeout = 10 * time.Minute
)

// Caller 是一次调用中已知的调用者信息：连接层查出的身份和调用方
// 声明的发起者。
type Caller struct {
	Identity  mesh.Identity
	Initiator protocol.Initiator
}

// Call 是传给工具的一次调用。
type Call struct {
	RequestID string
	Args      json.RawMessage // 可能为 nil 或 "null"，表示没有参数
	Caller    Caller
}

// Tool 是注册表中的一个工具。Spec、超时和 Run 注册后不应再修改。
type Tool struct {
	Spec           protocol.ToolSpec
	DefaultTimeout time.Duration // 0 为 DefaultTimeout（60 秒）
	MaxTimeout     time.Duration // 0 为 MaxTimeout（10 分钟）
	// Run 执行工具。返回普通 error 时节点按 internal 错误处理；
	// 返回 *protocol.Error 时原样返回给调用方。返回值会被序列化为
	// result（nil 序列化为 null）。
	Run func(ctx context.Context, call *Call) (any, error)
}

// Registry 是并发安全的工具注册表。
type Registry struct {
	mu    sync.RWMutex
	tools map[string]*Tool
}

// NewRegistry 创建空注册表。
func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]*Tool)}
}

// Register 注册或覆盖同名工具。名字为空时报错。
func (r *Registry) Register(t *Tool) error {
	if t == nil {
		return errNilTool
	}
	if t.Spec.Name == "" {
		return errNoName
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.tools == nil {
		r.tools = make(map[string]*Tool)
	}
	r.tools[t.Spec.Name] = t
	return nil
}

// Unregister 移除一个工具；不存在时不报错。
func (r *Registry) Unregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.tools, name)
}

// Get 按名字取工具。找到时返回 true；返回的 *Tool 注册后视为只读。
func (r *Registry) Get(name string) (*Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	return t, ok
}

// Specs 返回所有工具的描述，按名称排序。没有工具时返回空切片而非 nil，
// 便于直接作为能力报告的 tools 字段输出为 []。
func (r *Registry) Specs() []protocol.ToolSpec {
	r.mu.RLock()
	defer r.mu.RUnlock()
	specs := make([]protocol.ToolSpec, 0, len(r.tools))
	for _, t := range r.tools {
		specs = append(specs, t.Spec)
	}
	sort.Slice(specs, func(i, j int) bool { return specs[i].Name < specs[j].Name })
	return specs
}

// 注册表的错误。分开定义便于调用方区分。
var (
	errNilTool = &staticError{"tools: 不能注册空的工具"}
	errNoName  = &staticError{"tools: 工具的名字不能为空"}
)

type staticError struct{ msg string }

func (e *staticError) Error() string { return e.msg }

// DecodeArgs 把调用参数解码到 v。raw 为空或 null 时不动 v（保持零值），
// 返回 nil。未知字段会被拒绝；失败时返回 invalid_args 错误，消息里带
// 出错的字段名，供调用方直接展示。
func DecodeArgs(raw json.RawMessage, v any) error {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return protocol.Errorf(protocol.CodeInvalidArgs, "参数不合法：%v", err)
	}
	if dec.More() {
		return protocol.Errorf(protocol.CodeInvalidArgs, "参数不合法：JSON 后面有多余内容")
	}
	return nil
}
