package protocol

// 操作记录的查询接口和实时事件的线上格式。

// Record 是一条操作记录。同一次调用在被控端（role=target）和控制端
// （role=source）各有一条，request_id 相同。
type Record struct {
	ID           int64  `json:"id"` // 本机数据库行号，只用于分页，跨设备无意义
	RequestID    string `json:"request_id"`
	Role         string `json:"role"`
	OperatorType string `json:"operator_type"` // ai / human
	Via          string `json:"via,omitempty"` // cli / mcp / ui
	Session      string `json:"session,omitempty"`
	CallerOwner  string `json:"caller_owner,omitempty"`
	CallerDevice string `json:"caller_device,omitempty"`
	CallerKind   string `json:"caller_kind,omitempty"`
	Device       string `json:"device,omitempty"` // 被控设备
	Tool         string `json:"tool"`
	Args         string `json:"args,omitempty"` // 已脱敏的参数摘要
	Status       string `json:"status"`         // running / ok / 错误码 / interrupted
	StartedAt    int64  `json:"started_at"`     // Unix 毫秒
	EndedAt      int64  `json:"ended_at,omitempty"`
	DurationMS   int64  `json:"duration_ms,omitempty"`
}

// RecordsResponse 是 GET /v1/records 的响应。还有更多记录时 NextBefore 非 0，
// 作为下一页请求的 before 参数。
type RecordsResponse struct {
	Records    []Record `json:"records"`
	NextBefore int64    `json:"next_before,omitempty"`
}

// 事件类型。
const (
	EventStart  = "start"
	EventOutput = "output"
	EventEnd    = "end"
)

// Event 是 GET /v1/requests/{id}/events（WebSocket）推送的一条消息。
// 订阅后先收到 start，再依次收到缓冲中保留的输出和实时输出，最后收到 end。
type Event struct {
	Type      string `json:"type"`
	RequestID string `json:"request_id"`
	Time      int64  `json:"time"` // Unix 毫秒

	// start：工具名；输出缓冲已溢出时，最早被丢弃的字节数。
	Tool         string `json:"tool,omitempty"`
	DroppedBytes int64  `json:"dropped_bytes,omitempty"`

	// output：stdout 或 stderr，以及已解码为 UTF-8 的文本片段。
	Stream string `json:"stream,omitempty"`
	Data   string `json:"data,omitempty"`

	// end：结果码（ok 或错误码）和耗时。
	Code       string `json:"code,omitempty"`
	DurationMS int64  `json:"duration_ms,omitempty"`
}

// 记录的角色。
const (
	RoleTarget = "target" // 被控端：收到并执行了调用
	RoleSource = "source" // 控制端：发出了调用
)
