package node

// 一次工具调用的审计记录。任务 11 会把同样的字段写进 SQLite，所以
// 集中在这一个结构里生成；日志这行不含调用参数（参数里可能有文件
// 内容等大字段，也不应进日志）。

import (
	"fmt"
	"strings"
	"time"

	"github.com/Richard2091/DeskBridge/internal/mesh"
	"github.com/Richard2091/DeskBridge/internal/protocol"
)

// auditRecord 描述一次工具调用：谁（Caller*）、以什么名义（Initiator）、
// 调了什么（Tool）、结果如何（Code）和耗时。
type auditRecord struct {
	Time         time.Time
	RequestID    string
	CallerOwner  string
	CallerDevice string
	CallerKind   mesh.Kind
	Initiator    protocol.Initiator
	Tool         string
	Code         string // "ok" 或错误码
	DurationMS   int64
}

// String 输出一行日志文本，字段用 key=value，便于以后机器解析。
func (r auditRecord) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "调用 request_id=%s tool=%s code=%s duration_ms=%d",
		r.RequestID, r.Tool, r.Code, r.DurationMS)
	fmt.Fprintf(&b, " caller=%q/%q(%s) initiator=%s",
		r.CallerOwner, r.CallerDevice, r.CallerKind, r.initiator())
	return b.String()
}

// initiator 输出发起者描述；没有声明时输出 -。
func (r auditRecord) initiator() string {
	var parts []string
	if r.Initiator.Type != "" {
		parts = append(parts, fmt.Sprintf("type=%q", r.Initiator.Type))
	}
	if r.Initiator.Via != "" {
		parts = append(parts, fmt.Sprintf("via=%q", r.Initiator.Via))
	}
	if r.Initiator.Session != "" {
		parts = append(parts, fmt.Sprintf("session=%q", r.Initiator.Session))
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, ",")
}
