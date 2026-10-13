package node

// 内网协议：在各连接方式的监听上提供的 HTTP 服务。
// 每个请求先用连接方式的 WhoIs 识别来源，只接受同一账号的设备。

import (
	"encoding/json"
	"net/http"
	"runtime"

	"github.com/Richard2091/DeskBridge/internal/mesh"
	"github.com/Richard2091/DeskBridge/internal/version"
)

// 协议版本号，接口不兼容时递增。
const protocolVersion = 1

// capability 是 /v1/capability 的响应。
type capability struct {
	Protocol int         `json:"protocol"`
	DeviceID string      `json:"device_id"`
	Name     string      `json:"name"`
	Version  string      `json:"version"`
	Platform platformLib `json:"platform"`
	Tools    []string    `json:"tools"`
}

// platformLib 是对端报告的操作系统和架构。
type platformLib struct {
	OS   string `json:"os"`
	Arch string `json:"arch"`
}

// channelHandler 返回一个连接方式上的内网协议服务。
func (n *Node) channelHandler(ch mesh.Channel) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/capability", func(w http.ResponseWriter, r *http.Request) {
		id, err := ch.WhoIs(r.Context(), r.RemoteAddr)
		if err != nil {
			n.logf("识别请求来源失败（%s）: %v", r.RemoteAddr, err)
			writeError(w, http.StatusForbidden, "permission_denied", "只接受同一账号的设备")
			return
		}
		if !id.Mine {
			writeError(w, http.StatusForbidden, "permission_denied", "只接受同一账号的设备")
			return
		}
		writeJSON(w, http.StatusOK, capability{
			Protocol: protocolVersion,
			DeviceID: n.cfg.Device.ID(),
			Name:     n.cfg.Name,
			Version:  version.Version,
			Platform: platformLib{OS: runtime.GOOS, Arch: runtime.GOARCH},
			Tools:    []string{},
		})
	})
	return mux
}

// writeJSON 输出 JSON 响应。
func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

// apiError 是统一的错误响应格式 {"ok":false,"error":{"code","message"}}。
type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type errorResponse struct {
	OK    bool     `json:"ok"`
	Error apiError `json:"error"`
}

// writeError 输出统一格式的错误响应。
func writeError(w http.ResponseWriter, code int, errCode, message string) {
	writeJSON(w, code, errorResponse{OK: false, Error: apiError{Code: errCode, Message: message}})
}
