// 独立运行的 Go 核心：给 Tauri（sidecar）和 Flutter（子进程）用。
// Wails 不用它，而是在同一进程里直接挂载 core.Handler()。
package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"

	"deskbridge-spikes/ui-core/core"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:47800", "监听地址（只监听本机）")
	flag.Parse()
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatal(err)
	}
	// 父进程读取这一行确认已启动
	fmt.Printf("LISTENING %s\n", ln.Addr())
	log.Fatal(http.Serve(ln, core.Handler()))
}
