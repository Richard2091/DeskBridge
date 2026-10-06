// 技术验证：用 tsnet 让程序自己加入 Tailscale 内网，提供 HTTP 接口，并识别请求者身份。
// 运行后若需要登录，日志里会打印登录链接；默认 20 分钟后自动退出并注销节点。
package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"tailscale.com/tsnet"
)

func main() {
	hostname := flag.String("hostname", "deskbridge-spike", "内网中的主机名")
	lifetime := flag.Duration("for", 20*time.Minute, "运行多久后自动退出")
	flag.Parse()

	dir := filepath.Join(os.Getenv("LOCALAPPDATA"), "deskbridge-spike", "tsnet")
	srv := &tsnet.Server{Hostname: *hostname, Dir: dir, UserLogf: log.Printf}
	defer srv.Close()

	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	status, err := srv.Up(ctx) // 未登录时会在日志中打印登录链接，等待登录完成
	cancel()
	if err != nil {
		log.Fatalf("加入内网失败: %v", err)
	}
	log.Printf("已加入内网：%v，耗时 %v", status.TailscaleIPs, time.Since(start).Round(time.Millisecond))

	lc, err := srv.LocalClient()
	if err != nil {
		log.Fatal(err)
	}
	ln, err := srv.Listen("tcp", ":8080")
	if err != nil {
		log.Fatal(err)
	}

	http.HandleFunc("/whoami", func(w http.ResponseWriter, r *http.Request) {
		res := map[string]any{"remote_addr": r.RemoteAddr, "time": time.Now().Format(time.RFC3339)}
		if who, err := lc.WhoIs(r.Context(), r.RemoteAddr); err != nil {
			res["error"] = err.Error()
		} else {
			res["login"] = who.UserProfile.LoginName
			res["display_name"] = who.UserProfile.DisplayName
			res["node"] = who.Node.ComputedName
			res["node_os"] = who.Node.Hostinfo.OS()
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		enc := json.NewEncoder(w)
		enc.SetEscapeHTML(false)
		_ = enc.Encode(res)
	})

	go func() {
		time.Sleep(*lifetime)
		log.Printf("到达运行时限，注销节点并退出")
		lctx, c := context.WithTimeout(context.Background(), 10*time.Second)
		_ = lc.Logout(lctx)
		c()
		os.Exit(0)
	}()
	log.Printf("HTTP 服务：http://%s:8080/whoami", *hostname)
	log.Fatal(http.Serve(ln, nil))
}
