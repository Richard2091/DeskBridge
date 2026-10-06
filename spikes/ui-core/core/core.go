// Package core 是客户端界面实测（任务 24）共用的“Go 核心”，不是正式代码。
//
// 接口：
//
//	GET  /api/devices                     假设备列表（含连接方式和延迟）
//	POST /api/exec   {"command": "..."}   在本机用 PowerShell 执行命令，输出 UTF-8
//	GET  /api/frames?fps=10&width=1280&q=70
//	     连续推送主显示器截图：每帧 = 4 字节大端长度 + JPEG
//	GET  /api/stats                       推流统计
package core

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"image"
	"image/jpeg"
	"net/http"
	"os/exec"
	"strconv"
	"sync"
	"time"

	"github.com/kbinani/screenshot"
)

type Device struct {
	Name      string `json:"name"`
	OS        string `json:"os"`
	Online    bool   `json:"online"`
	Conn      string `json:"conn"` // lan / direct / relay / offline
	Relay     string `json:"relay,omitempty"`
	LatencyMS int    `json:"latency_ms"`
}

// 示例数据，名称和地址都是虚构的
var devices = []Device{
	{Name: "win-pc", OS: "windows", Online: true, Conn: "lan", LatencyMS: 2},
	{Name: "win-laptop", OS: "windows", Online: true, Conn: "direct", LatencyMS: 38},
	{Name: "linux-server", OS: "linux", Online: true, Conn: "relay", Relay: "hkg", LatencyMS: 230},
	{Name: "office-pc", OS: "windows", Online: false, Conn: "offline"},
}

type stats struct {
	mu        sync.Mutex
	Frames    int64   `json:"frames"`
	Bytes     int64   `json:"bytes"`
	CaptureMS float64 `json:"avg_capture_ms"`
	EncodeMS  float64 `json:"avg_encode_ms"`
}

var st stats

func Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/devices", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, devices)
	})
	mux.HandleFunc("POST /api/exec", handleExec)
	mux.HandleFunc("GET /api/frames", handleFrames)
	mux.HandleFunc("GET /api/stats", func(w http.ResponseWriter, r *http.Request) {
		st.mu.Lock()
		defer st.mu.Unlock()
		writeJSON(w, &st)
	})
	return cors(mux)
}

// Tauri 和 Flutter 的页面与核心不同源，需要放开本机跨域
func cors(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		h.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(v)
}

func handleExec(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Command string `json:"command"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Command == "" {
		http.Error(w, "需要 command", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	start := time.Now()
	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command",
		"[Console]::OutputEncoding=[Text.Encoding]::UTF8; "+req.Command)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	code := 0
	if err != nil {
		code = -1
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		}
	}
	writeJSON(w, map[string]any{
		"exit_code": code, "stdout": out.String(), "stderr": errb.String(),
		"ms": time.Since(start).Milliseconds(),
	})
}

func handleFrames(w http.ResponseWriter, r *http.Request) {
	fps := queryInt(r, "fps", 10, 1, 30)
	width := queryInt(r, "width", 1280, 320, 3840)
	quality := queryInt(r, "q", 70, 10, 95)
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "不支持流式响应", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Cache-Control", "no-store")
	tick := time.NewTicker(time.Second / time.Duration(fps))
	defer tick.Stop()
	var buf bytes.Buffer
	var lenb [4]byte
	for {
		select {
		case <-r.Context().Done():
			return
		case <-tick.C:
		}
		t0 := time.Now()
		img, err := screenshot.CaptureDisplay(0)
		if err != nil {
			return
		}
		t1 := time.Now()
		buf.Reset()
		if err := jpeg.Encode(&buf, downscale(img, width), &jpeg.Options{Quality: quality}); err != nil {
			return
		}
		t2 := time.Now()
		binary.BigEndian.PutUint32(lenb[:], uint32(buf.Len()))
		if _, err := w.Write(lenb[:]); err != nil {
			return
		}
		if _, err := w.Write(buf.Bytes()); err != nil {
			return
		}
		flusher.Flush()
		record(t1.Sub(t0), t2.Sub(t1), buf.Len())
	}
}

func record(capture, encode time.Duration, n int) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.Frames++
	st.Bytes += int64(n)
	// 指数平均
	a := 0.1
	st.CaptureMS = st.CaptureMS*(1-a) + float64(capture.Microseconds())/1000*a
	st.EncodeMS = st.EncodeMS*(1-a) + float64(encode.Microseconds())/1000*a
}

// 最近邻缩放到指定宽度，足够实测用
func downscale(src *image.RGBA, width int) image.Image {
	b := src.Bounds()
	if b.Dx() <= width {
		return src
	}
	height := b.Dy() * width / b.Dx()
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		sy := b.Min.Y + y*b.Dy()/height
		srow := src.Pix[(sy-b.Min.Y)*src.Stride:]
		drow := dst.Pix[y*dst.Stride:]
		for x := 0; x < width; x++ {
			sx := x * b.Dx() / width
			copy(drow[x*4:x*4+4], srow[sx*4:sx*4+4])
		}
	}
	return dst
}

func queryInt(r *http.Request, key string, def, min, max int) int {
	v, err := strconv.Atoi(r.URL.Query().Get(key))
	if err != nil {
		return def
	}
	return clamp(v, min, max)
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
