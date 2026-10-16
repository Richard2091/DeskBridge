package node

// 操作记录：被控端在 handleCall 里记录收到的每次调用，控制端在转发
// 时记录自己发出的调用；GET /v1/records 供同账号设备查询本机的记录。

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/Richard2091/DeskBridge/internal/protocol"
	"github.com/Richard2091/DeskBridge/internal/redact"
	"github.com/Richard2091/DeskBridge/internal/store"
)

// recordBegin 写入一条 running 记录，返回行号；没有数据库或写入失败时返回 0。
// 记录失败只记日志，不影响调用本身。
func (n *Node) recordBegin(role, device, tool string, requestID string, who protocol.Initiator, caller callerInfo, args json.RawMessage, started time.Time) int64 {
	if n.cfg.Records == nil {
		return 0
	}
	id, err := n.cfg.Records.Begin(store.Record{
		RequestID: requestID, Role: role, OperatorType: who.Type, Via: who.Via, Session: who.Session,
		CallerOwner: caller.owner, CallerDevice: caller.device, CallerKind: caller.kind,
		Device: device, Tool: tool, Args: redact.Summarize(args), StartedAt: started.UnixMilli(),
	})
	if err != nil {
		n.logf("写入操作记录失败: %v", err)
		return 0
	}
	return id
}

// recordEnd 结束 recordBegin 写入的记录。
func (n *Node) recordEnd(id int64, code string, started time.Time, durationMS int64) {
	if id == 0 || n.cfg.Records == nil {
		return
	}
	if err := n.cfg.Records.Finish(id, code, started.Add(time.Duration(durationMS)*time.Millisecond), durationMS); err != nil {
		n.logf("更新操作记录失败: %v", err)
	}
}

// callerInfo 是记录里的调用者身份（连接层查出的）。
type callerInfo struct{ owner, device, kind string }

// handleRecords 处理 GET /v1/records。查询参数：since、until（Unix 毫秒）、
// operator_type、tool（以 * 结尾为前缀）、status、role、request_id、
// before（翻页游标）、limit。
func (a *api) handleRecords(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.identify(w, r); !ok {
		return
	}
	q := r.URL.Query()
	f := store.Filter{
		OperatorType: q.Get("operator_type"), Tool: q.Get("tool"), Status: q.Get("status"),
		Role: q.Get("role"), RequestID: q.Get("request_id"),
	}
	for _, p := range []struct {
		key string
		dst *int64
	}{{"since", &f.Since}, {"until", &f.Until}, {"before", &f.BeforeID}} {
		if v := q.Get(p.key); v != "" {
			n, err := strconv.ParseInt(v, 10, 64)
			if err != nil || n < 0 {
				writeProtoError(w, protocol.Errorf(protocol.CodeInvalidArgs, "参数 %s 不是非负整数：%q", p.key, v))
				return
			}
			*p.dst = n
		}
	}
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			writeProtoError(w, protocol.Errorf(protocol.CodeInvalidArgs, "参数 limit 不是正整数：%q", v))
			return
		}
		f.Limit = n
	}
	if f.Limit <= 0 {
		f.Limit = store.DefaultLimit
	}
	f.Limit = min(f.Limit, store.MaxLimit)

	resp := protocol.RecordsResponse{Records: []protocol.Record{}}
	if db := a.n.cfg.Records; db != nil {
		recs, err := db.Query(f)
		if err != nil {
			writeProtoError(w, protocol.Errorf(protocol.CodeInternal, "%v", err))
			return
		}
		resp.Records = recs
		if len(recs) == f.Limit {
			resp.NextBefore = recs[len(recs)-1].ID
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

// forwardTool 转发一次工具调用（POST /v1/tools/{name}）并在本机记录控制端
// 记录。请求体里没有 request_id 时先补上，使两端记录能用同一个 ID 关联。
// serve 是真正的转发或本机处理。无法解析的请求原样转交，由被控端报错。
func (n *Node) forwardTool(w http.ResponseWriter, r *http.Request, device, tool string, serve func(http.ResponseWriter, *http.Request)) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxPayloadBytes))
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeProtoError(w, protocol.Errorf(protocol.CodePayloadTooLarge, "请求体超过上限 %d 字节", maxPayloadBytes))
		} else {
			writeProtoError(w, protocol.Errorf(protocol.CodeInvalidArgs, "读取请求体失败：%v", err))
		}
		return
	}
	setBody := func(b []byte) {
		r.Body = io.NopCloser(bytes.NewReader(b))
		r.ContentLength = int64(len(b))
	}
	var creq protocol.CallRequest
	valid := len(bytes.TrimSpace(body)) > 0 &&
		json.Unmarshal(body, &creq) == nil &&
		protocol.ValidateRequestID(creq.RequestID) == nil &&
		(creq.Initiator == nil || creq.Initiator.Validate() == nil)
	if !valid {
		setBody(body)
		serve(w, r)
		return
	}
	if creq.RequestID == "" {
		creq.RequestID = protocol.NewRequestID()
		if nb, merr := json.Marshal(creq); merr == nil {
			body = nb
		}
	}
	setBody(body)

	started := time.Now()
	who := initiatorOf(&creq)
	id := n.recordBegin(store.RoleSource, device, tool, creq.RequestID, who, callerInfo{}, creq.Args, started)
	cw := &captureWriter{ResponseWriter: w}
	serve(cw, r)
	n.recordEnd(id, cw.code(), started, time.Since(started).Milliseconds())
}

// captureWriter 记录响应状态码和开头一段内容，用来得出调用的结果码。
type captureWriter struct {
	http.ResponseWriter
	status int
	head   bytes.Buffer
}

const captureHeadMax = 4096

func (c *captureWriter) WriteHeader(code int) {
	if c.status == 0 {
		c.status = code
	}
	c.ResponseWriter.WriteHeader(code)
}

func (c *captureWriter) Write(p []byte) (int, error) {
	if c.status == 0 {
		c.status = http.StatusOK
	}
	if room := captureHeadMax - c.head.Len(); room > 0 {
		c.head.Write(p[:min(room, len(p))])
	}
	return c.ResponseWriter.Write(p)
}

func (c *captureWriter) Flush() {
	if f, ok := c.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (c *captureWriter) Unwrap() http.ResponseWriter { return c.ResponseWriter }

// code 返回结果码：200 为 ok；否则取响应里的错误码，取不到时用 http_<状态码>。
func (c *captureWriter) code() string {
	if c.status == 0 || c.status == http.StatusOK {
		return "ok"
	}
	var resp struct {
		Error *protocol.Error `json:"error"`
	}
	if json.Unmarshal(c.head.Bytes(), &resp) == nil && resp.Error != nil && resp.Error.Code != "" {
		return resp.Error.Code
	}
	return "http_" + strconv.Itoa(c.status)
}
