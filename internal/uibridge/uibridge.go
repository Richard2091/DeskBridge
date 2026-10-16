// Package uibridge 是桌面客户端（Flutter）启动的 Go 子进程：在 127.0.0.1 上
// 提供带令牌的 HTTP 服务，把客户端的请求转给本机节点的 unix socket，
// 并提供跨设备合并的操作记录。界面不直接连节点，所以不依赖 Dart 对
// unix socket 的支持，节点的权限模型也不变（令牌只给启动它的客户端）。
package uibridge

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httputil"
	"strconv"
	"strings"

	"github.com/Richard2091/DeskBridge/internal/client"
	"github.com/Richard2091/DeskBridge/internal/protocol"
)

// NewToken 生成随机令牌。
func NewToken() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// Handler 返回桌面客户端用的 HTTP 处理器。除令牌校验外：
//
//	GET /api/ping        {"ok":true}
//	GET /api/activity    跨设备合并的操作记录，参数 since、operator_type、tool、status、limit
//	/local/...           原样转给本机节点（设备列表、工具调用、取消、实时事件）
func Handler(socket, token string) http.Handler {
	c := client.New(socket)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/ping", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})
	mux.HandleFunc("GET /api/activity", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		aq := client.ActivityQuery{OperatorType: q.Get("operator_type"), Tool: q.Get("tool"), Status: q.Get("status")}
		if v := q.Get("since"); v != "" {
			n, err := strconv.ParseInt(v, 10, 64)
			if err != nil || n < 0 {
				writeError(w, http.StatusBadRequest, protocol.CodeInvalidArgs, "since 不是非负整数")
				return
			}
			aq.Since = n
		}
		if v := q.Get("limit"); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n <= 0 {
				writeError(w, http.StatusBadRequest, protocol.CodeInvalidArgs, "limit 不是正整数")
				return
			}
			aq.Limit = n
		}
		res, err := c.Activity(r.Context(), aq)
		if err != nil {
			status, code, msg := mapError(err)
			writeError(w, status, code, msg)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	mux.Handle("/local/", &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.Out.URL.Scheme = "http"
			pr.Out.URL.Host = "deskbridge.local"
			pr.Out.Host = "deskbridge.local"
			pr.Out.Header.Del("Authorization") // 令牌只给桥接进程，不转给节点
			pr.Out.Header.Del("Origin")        // WebSocket 握手：节点只接受同源，桥接已校验令牌
		},
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", socket)
			},
		},
		FlushInterval: -1,
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			writeError(w, http.StatusBadGateway, "node_not_running", "本机节点未运行或没有响应："+err.Error())
		},
	})
	return requireToken(token, mux)
}

// requireToken 要求请求带 Authorization: Bearer <令牌>。
func requireToken(token string, next http.Handler) http.Handler {
	want := []byte(token)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if token == "" || subtle.ConstantTimeCompare([]byte(got), want) != 1 {
			writeError(w, http.StatusUnauthorized, "unauthorized", "缺少或错误的令牌")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func mapError(err error) (status int, code, msg string) {
	if perr, ok := err.(*protocol.Error); ok {
		return protocol.HTTPStatus(perr.Code), perr.Code, perr.Message
	}
	if strings.Contains(err.Error(), client.ErrNotRunning.Error()) {
		return http.StatusBadGateway, "node_not_running", err.Error()
	}
	return http.StatusInternalServerError, protocol.CodeInternal, err.Error()
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]any{"ok": false, "error": protocol.Error{Code: code, Message: msg}})
}
