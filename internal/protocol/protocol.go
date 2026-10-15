// Package protocol 定义节点协议的线上格式：能力报告、工具调用请求和
// 响应、统一错误。节点、客户端和 MCP 共用这些类型，字段名即线上的
// JSON 键，改动需要考虑兼容性（架构文档"节点协议"一节）。
package protocol

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
	"unicode"
)

// Version 是协议版本号，接口不兼容时递增。
const Version = 1

// DefaultPort 是节点协议在内网地址上使用的端口。
const DefaultPort = 7465

// Capability 是 GET /v1/capability 的响应：本节点对外提供的能力报告。
type Capability struct {
	Protocol  int        `json:"protocol"`
	DeviceID  string     `json:"device_id"`
	PublicKey string     `json:"public_key"` // ed25519 公钥 base64，供以后的配对和验证
	Name      string     `json:"name"`
	Version   string     `json:"version"`
	Platform  Platform   `json:"platform"`
	Tools     []ToolSpec `json:"tools"`
}

// Platform 是节点所在平台的描述。
type Platform struct {
	OS        string `json:"os"`                   // 如 windows、linux
	Arch      string `json:"arch"`                 // 如 amd64、arm64
	OSVersion string `json:"os_version,omitempty"` // 系统版本，如 10.0.19045
}

// ToolSpec 描述一个工具，同名工具在各平台语义一致，名字不带平台前缀。
type ToolSpec struct {
	Name        string          `json:"name"`         // 如 proc.exec
	Description string          `json:"description"`  // 中文，给人和 AI 看
	Source      string          `json:"source"`       // 工具来源，见 Source* 常量
	InputSchema json.RawMessage `json:"input_schema"` // 参数的 JSON Schema
}

// 工具来源。内置工具之外，以后有 MCP 转发工具和 Agent 运行器，来源在
// 运行时可能变化，能力报告每次都重新生成。
const (
	SourceBuiltin = "builtin"
	SourceMCP     = "mcp"
	SourceAgent   = "agent"
)

// Initiator 描述调用发起者，由调用方声明；调用者身份由连接层查出。
// 两者一起写进操作记录，供以后的权限判断（任务 17）。
type Initiator struct {
	Type    string `json:"type"`              // ai 或 human
	Via     string `json:"via"`               // cli、mcp、ui
	Session string `json:"session,omitempty"` // 如 MCP 会话 ID
}

// 发起者类型的取值。
const (
	InitiatorAI    = "ai"
	InitiatorHuman = "human"
)

// 发起者途径（Via）的取值。
const (
	ViaCLI = "cli"
	ViaMCP = "mcp"
	ViaUI  = "ui"
)

// MaxInitiatorSession 是 Initiator.Session 的长度上限（字节）。
const MaxInitiatorSession = 128

// Validate 检查调用方声明的发起者：Type 只能为空或 ai/human，Via 只能为空或
// cli/mcp/ui，Session 不超过 MaxInitiatorSession 字节且不含控制字符。
// 这些字段由调用方声明，节点不能原样信任，尤其不能让它们污染审计日志。
func (i Initiator) Validate() *Error {
	switch i.Type {
	case "", InitiatorAI, InitiatorHuman:
	default:
		return Errorf(CodeInvalidArgs, "initiator.type 不合法：%q", i.Type)
	}
	switch i.Via {
	case "", ViaCLI, ViaMCP, ViaUI:
	default:
		return Errorf(CodeInvalidArgs, "initiator.via 不合法：%q", i.Via)
	}
	if len(i.Session) > MaxInitiatorSession {
		return Errorf(CodeInvalidArgs, "initiator.session 超过 %d 字节", MaxInitiatorSession)
	}
	for _, r := range i.Session {
		if unicode.IsControl(r) {
			return Errorf(CodeInvalidArgs, "initiator.session 含控制字符")
		}
	}
	return nil
}

// MaxRequestID 是调用方指定的 request_id 的长度上限（字节）。
const MaxRequestID = 128

// ValidateRequestID 检查调用方指定的 request_id：不超过 MaxRequestID 字节
// 且不含控制字符。空串合法（节点会生成）。request_id 由调用方提供，会进入
// 审计日志和取消路径，不能原样信任。
func ValidateRequestID(id string) *Error {
	if len(id) > MaxRequestID {
		return Errorf(CodeInvalidArgs, "request_id 超过 %d 字节", MaxRequestID)
	}
	for _, r := range id {
		if unicode.IsControl(r) {
			return Errorf(CodeInvalidArgs, "request_id 含控制字符")
		}
	}
	return nil
}

// CallRequest 是 POST /v1/tools/{name} 的请求体。
type CallRequest struct {
	RequestID string          `json:"request_id,omitempty"` // 为空时节点生成
	Args      json.RawMessage `json:"args"`
	TimeoutMS int             `json:"timeout_ms,omitempty"` // 0 用工具默认值
	Initiator *Initiator      `json:"initiator,omitempty"`
}

// CallResponse 是工具调用的响应，成功和失败都用这个结构。
type CallResponse struct {
	OK         bool            `json:"ok"`
	RequestID  string          `json:"request_id,omitempty"`
	Result     json.RawMessage `json:"result,omitempty"`
	Error      *Error          `json:"error,omitempty"`
	DurationMS int64           `json:"duration_ms"`
}

// Error 是统一的业务错误，也作为 error 类型在客户端库里返回。
type Error struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`           // 中文，给人和 AI 看
	Details map[string]any `json:"details,omitempty"` // 如 permission_denied 时 {"missing":"same_owner"}
}

// Error 实现 error 接口，格式为 "code: message"。
func (e *Error) Error() string {
	if e == nil {
		return "<nil>"
	}
	return e.Code + ": " + e.Message
}

// 错误码。HTTP 状态见 HTTPStatus。
const (
	CodeInvalidArgs         = "invalid_args"
	CodeUnknownTool         = "unknown_tool"
	CodeNotFound            = "not_found"
	CodeDeviceNotFound      = "device_not_found"
	CodePermissionDenied    = "permission_denied"
	CodeDuplicateRequest    = "duplicate_request"
	CodePayloadTooLarge     = "payload_too_large"
	CodeCancelled           = "cancelled"
	CodeInternal            = "internal"
	CodeUnsupportedPlatform = "unsupported_platform"
	CodeUnavailable         = "unavailable"
	CodeTimeout             = "timeout"
)

// Errorf 创建一个带格式化消息的协议错误。
func Errorf(code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

// HTTPStatus 返回错误码对应的 HTTP 状态；未知错误码返回 500。
func HTTPStatus(code string) int {
	switch code {
	case CodeInvalidArgs:
		return http.StatusBadRequest
	case CodeUnknownTool, CodeNotFound, CodeDeviceNotFound:
		return http.StatusNotFound
	case CodePermissionDenied:
		return http.StatusForbidden
	case CodeDuplicateRequest:
		return http.StatusConflict
	case CodePayloadTooLarge:
		return http.StatusRequestEntityTooLarge
	case CodeCancelled:
		return 499 // 客户端关闭连接，非标准状态码
	case CodeInternal:
		return http.StatusInternalServerError
	case CodeUnsupportedPlatform:
		return http.StatusNotImplemented
	case CodeUnavailable:
		return http.StatusBadGateway
	case CodeTimeout:
		return http.StatusGatewayTimeout
	default:
		return http.StatusInternalServerError
	}
}

// NewRequestID 生成请求 ID：16 字节随机数的十六进制（32 个字符）。
func NewRequestID() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		// crypto/rand 失败极罕见；退化成时间戳，保证请求 ID 不为空。
		return fmt.Sprintf("t%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf[:])
}
