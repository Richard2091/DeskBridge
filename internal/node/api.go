package node

// 节点协议：内网里的 HTTP + JSON 服务（能力报告、工具调用、取消），
// 在各连接方式的监听上提供，本机接口的 self 目标也复用同一个处理器。
// HTTP 处理器和监听分开，测试可以直接用 httptest 构造 api。

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"runtime"
	"time"

	"github.com/Richard2091/DeskBridge/internal/mesh"
	"github.com/Richard2091/DeskBridge/internal/protocol"
	"github.com/Richard2091/DeskBridge/internal/tools"
	"github.com/Richard2091/DeskBridge/internal/version"
)

// maxPayloadBytes 是工具调用请求体的上限（16 MiB）。
const maxPayloadBytes int64 = 16 << 20

// whoIsFunc 从连接的远端地址查出对方身份。本机接口的 self 目标
// 传入固定的本机身份，不经过连接层。
type whoIsFunc func(ctx context.Context, remoteAddr string) (mesh.Identity, error)

// api 是节点协议的 HTTP 处理器集合。
type api struct {
	n          *Node
	whoIs      whoIsFunc
	maxPayload int64 // 请求体上限，测试可以调小
}

// apiHandler 构造一个使用给定 WhoIs 的节点协议服务。
func (n *Node) apiHandler(whoIs whoIsFunc) http.Handler {
	return (&api{n: n, whoIs: whoIs, maxPayload: maxPayloadBytes}).routes()
}

// routes 注册节点协议的接口。
func (a *api) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/capability", a.handleCapability)
	mux.HandleFunc("POST /v1/tools/{name}", a.handleCall)
	mux.HandleFunc("DELETE /v1/requests/{id}", a.handleCancel)
	return mux
}

// identify 用连接层查出请求来源，只接受同一账号的设备。
// 不通过时已写好错误响应，返回 false。
func (a *api) identify(w http.ResponseWriter, r *http.Request) (mesh.Identity, bool) {
	id, err := a.whoIs(r.Context(), r.RemoteAddr)
	if err != nil {
		a.n.logf("识别请求来源失败（%s）: %v", r.RemoteAddr, err)
		writeProtoError(w, sameOwnerRequired("无法识别请求来源"))
		return mesh.Identity{}, false
	}
	if !id.Mine {
		writeProtoError(w, sameOwnerRequired("只接受同一账号的设备"))
		return mesh.Identity{}, false
	}
	return id, true
}

// sameOwnerRequired 是初版的权限模型：只接受同一账号（ADR-0009）。
func sameOwnerRequired(message string) *protocol.Error {
	return &protocol.Error{
		Code:    protocol.CodePermissionDenied,
		Message: message,
		Details: map[string]any{"missing": "same_owner"},
	}
}

// handleCapability 返回本节点的能力报告。工具列表每次从注册表重新
// 读取，工具来源在运行时变化（如 MCP 转发）也能反映出来。
func (a *api) handleCapability(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.identify(w, r); !ok {
		return
	}
	writeJSON(w, http.StatusOK, a.n.capability())
}

// capability 生成本节点的能力报告。
func (n *Node) capability() protocol.Capability {
	return protocol.Capability{
		Protocol:  protocol.Version,
		DeviceID:  n.cfg.Device.ID(),
		PublicKey: base64.StdEncoding.EncodeToString(n.cfg.Device.PublicKey()),
		Name:      n.cfg.Name,
		Version:   version.Version,
		Platform:  protocol.Platform{OS: runtime.GOOS, Arch: runtime.GOARCH},
		Tools:     n.reg.Specs(),
	}
}

// handleCall 处理一次工具调用。
func (a *api) handleCall(w http.ResponseWriter, r *http.Request) {
	id, ok := a.identify(w, r)
	if !ok {
		return
	}
	name := r.PathValue("name")
	tool, found := a.n.reg.Get(name)
	if !found {
		writeProtoError(w, protocol.Errorf(protocol.CodeUnknownTool, "没有这个工具：%s", name))
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, a.maxPayload))
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeProtoError(w, protocol.Errorf(protocol.CodePayloadTooLarge, "请求体超过上限 %d 字节", a.maxPayload))
		} else {
			writeProtoError(w, protocol.Errorf(protocol.CodeInvalidArgs, "读取请求体失败：%v", err))
		}
		return
	}
	if len(body) == 0 {
		// 空请求体视为没有参数。
		body = []byte("null")
	}
	var creq protocol.CallRequest
	if err := json.Unmarshal(body, &creq); err != nil {
		writeProtoError(w, protocol.Errorf(protocol.CodeInvalidArgs, "请求不是合法的 JSON：%v", err))
		return
	}

	if creq.Initiator != nil {
		if err := creq.Initiator.Validate(); err != nil {
			writeProtoError(w, err)
			return
		}
	}

	reqID := creq.RequestID
	if reqID == "" {
		reqID = protocol.NewRequestID()
	}
	timeout := callTimeout(tool, creq.TimeoutMS)
	callCtx, cancel := context.WithTimeout(r.Context(), timeout)

	// 先登记再执行，同一个 request_id 并发调用时后者得到 duplicate_request。
	rc := &runningCall{cancel: cancel, owner: id.Owner}
	if _, loaded := a.n.inflight.LoadOrStore(reqID, rc); loaded {
		cancel()
		writeProtoError(w, protocol.Errorf(protocol.CodeDuplicateRequest, "请求 %s 正在执行", reqID))
		return
	}
	defer a.n.inflight.Delete(reqID)
	defer cancel()

	started := time.Now()
	call := &tools.Call{
		RequestID: reqID,
		Args:      creq.Args,
		Caller:    tools.Caller{Identity: id, Initiator: initiatorOf(&creq)},
	}
	result, runErr := tool.Run(callCtx, call)

	resp := protocol.CallResponse{OK: true, RequestID: reqID, DurationMS: time.Since(started).Milliseconds()}
	var perr *protocol.Error
	switch {
	case runErr == nil:
		raw, merr := json.Marshal(result)
		if merr != nil {
			perr = protocol.Errorf(protocol.CodeInternal, "工具结果无法编码：%v", merr)
			break
		}
		resp.Result = raw
	case errors.As(runErr, &perr):
		// 工具返回的协议错误原样返回。
	case errors.Is(callCtx.Err(), context.DeadlineExceeded):
		perr = protocol.Errorf(protocol.CodeTimeout, "工具 %s 在 %s 后超时", name, timeout)
	case errors.Is(callCtx.Err(), context.Canceled):
		perr = protocol.Errorf(protocol.CodeCancelled, "工具 %s 的调用已取消", name)
	default:
		perr = protocol.Errorf(protocol.CodeInternal, "%v", runErr)
	}
	if perr != nil {
		resp.OK = false
		resp.Error = perr
	}

	a.n.logf("%s", auditRecord{
		Time:         started,
		RequestID:    reqID,
		Tool:         name,
		Code:         auditCode(resp.Error),
		DurationMS:   resp.DurationMS,
		CallerOwner:  id.Owner,
		CallerDevice: id.Device,
		CallerKind:   id.Kind,
		Initiator:    call.Caller.Initiator,
	})

	status := http.StatusOK
	if resp.Error != nil {
		status = protocol.HTTPStatus(resp.Error.Code)
	}
	writeJSON(w, status, resp)
}

// initiatorOf 取请求声明的发起者；没有声明时按人工经本机接口处理。
func initiatorOf(creq *protocol.CallRequest) protocol.Initiator {
	if creq.Initiator != nil {
		return *creq.Initiator
	}
	return protocol.Initiator{Type: protocol.InitiatorHuman, Via: protocol.ViaCLI}
}

// callTimeout 计算一次调用的超时：请求指定了 timeout_ms 用它（截到
// 工具的 MaxTimeout），否则用工具默认值。
func callTimeout(tool *tools.Tool, timeoutMS int) time.Duration {
	max := tool.MaxTimeout
	if max <= 0 {
		max = tools.MaxTimeout
	}
	var d time.Duration
	if timeoutMS > 0 {
		// 先和上限比较再换算，避免大值乘法溢出成负数。
		if int64(timeoutMS) >= max.Milliseconds() {
			return max
		}
		d = time.Duration(timeoutMS) * time.Millisecond
	} else {
		d = tool.DefaultTimeout
		if d <= 0 {
			d = tools.DefaultTimeout
		}
	}
	if d > max {
		d = max
	}
	return d
}

// auditCode 返回审计记录里的结果码：成功为 "ok"，失败为错误码。
func auditCode(perr *protocol.Error) string {
	if perr == nil {
		return "ok"
	}
	return perr.Code
}

// handleCancel 取消一条正在执行的调用。只允许与发起者同一 Owner 的
// 调用者取消（初版都是同一账号，但检查不能少）。
func (a *api) handleCancel(w http.ResponseWriter, r *http.Request) {
	id, ok := a.identify(w, r)
	if !ok {
		return
	}
	reqID := r.PathValue("id")
	v, found := a.n.inflight.Load(reqID)
	if !found {
		writeProtoError(w, protocol.Errorf(protocol.CodeNotFound, "没有正在执行的请求 %s", reqID))
		return
	}
	rc, ok := v.(*runningCall)
	if !ok {
		writeProtoError(w, protocol.Errorf(protocol.CodeInternal, "请求记录类型不对"))
		return
	}
	if rc.owner != id.Owner {
		writeProtoError(w, sameOwnerRequired("只能取消自己发起的调用"))
		return
	}
	rc.cancel()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
