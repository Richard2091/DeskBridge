package main

import (
	"embed"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"deskbridge-spikes/ui-core/core"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/dist
var assets embed.FS

// apiMiddleware 把 /api/* 交给共用 Go 核心，其余请求交给前端静态资源。
func apiMiddleware(api http.Handler) application.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/api/") {
				start := time.Now()
				sr := &statusRecorder{ResponseWriter: w, code: 200}
				api.ServeHTTP(sr, r)
				logf("asset-server %s %s -> %d, %d bytes, %v", r.Method, r.URL.Path, sr.code, sr.n, time.Since(start))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// statusRecorder 只记录状态码和字节数，故意不实现 Flusher，保持与资源服务器一致的行为。
type statusRecorder struct {
	http.ResponseWriter
	code int
	n    int
}

func (s *statusRecorder) WriteHeader(c int) { s.code = c; s.ResponseWriter.WriteHeader(c) }
func (s *statusRecorder) Write(b []byte) (int, error) {
	s.n += len(b)
	return s.ResponseWriter.Write(b)
}

func logf(format string, args ...any) {
	path := os.Getenv("DESKBRIDGE_LOG")
	if path == "" {
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, time.Now().Format("15:04:05.000 ")+format+"\n", args...)
}

func main() {
	cfg := AppConfig{
		AutoStream: os.Getenv("DESKBRIDGE_AUTOSTREAM") == "1",
	}
	api := core.Handler()

	// 实测发现 Windows 上资源服务器会把响应整个缓冲后才交给 WebView2（没有 Flusher），
	// 流式的 /api/frames 无法逐帧到达。因此默认 hybrid：
	//   asset  ：全部走资源服务器（用于复现问题，/api/frames 不可用）
	//   hybrid ：devices/exec 走资源服务器，只有 frames 走同进程 127.0.0.1:47801（默认）
	//   tcp    ：全部走 127.0.0.1:47801
	mode := os.Getenv("DESKBRIDGE_API_MODE")
	if mode == "" {
		mode = "hybrid"
	}
	cfg.Mode = mode
	if mode != "asset" {
		ln, err := net.Listen("tcp", "127.0.0.1:47801")
		if err != nil {
			log.Fatal(err)
		}
		go func() { log.Fatal(http.Serve(ln, api)) }()
		cfg.FramesBase = "http://127.0.0.1:47801"
		if mode == "tcp" {
			cfg.APIBase = cfg.FramesBase
		}
	}

	app := application.New(application.Options{
		Name:        "DeskBridge 实测",
		Description: "DeskBridge 客户端界面实测（Wails v3）",
		Services: []application.Service{
			application.NewService(&DeskService{cfg: cfg}),
		},
		Assets: application.AssetOptions{
			Handler:    application.AssetFileServerFS(assets),
			Middleware: apiMiddleware(api),
		},
	})

	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:  "DeskBridge 实测",
		Width:  1100,
		Height: 800,
		URL:    "/",
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}