package node

// 内网协议入口：在一个连接方式的监听上提供节点协议服务。
// 每个请求先用该连接方式的 WhoIs 识别来源，只接受同一账号的设备。

import (
	"encoding/json"
	"net/http"

	"github.com/Richard2091/DeskBridge/internal/mesh"
	"github.com/Richard2091/DeskBridge/internal/protocol"
)

// channelHandler 返回一个连接方式上的节点协议服务。
func (n *Node) channelHandler(ch mesh.Channel) http.Handler {
	return n.apiHandler(ch.WhoIs)
}

// writeJSON 输出 JSON 响应。
func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

// errorResponse 是统一的错误响应格式 {"ok":false,"error":{...}}。
type errorResponse struct {
	OK    bool            `json:"ok"`
	Error *protocol.Error `json:"error"`
}

// writeProtoError 按协议错误码写出错误响应，HTTP 状态由错误码决定。
func writeProtoError(w http.ResponseWriter, perr *protocol.Error) {
	writeJSON(w, protocol.HTTPStatus(perr.Code), errorResponse{OK: false, Error: perr})
}

// writeLocalError 写出本机接口的错误响应。本机接口有一些协议错误码
// 表之外的自有错误码（如 unknown_channel），状态码由调用处指定。
func writeLocalError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorResponse{OK: false, Error: &protocol.Error{Code: code, Message: message}})
}
