// framenet 按视频码率和帧率经 TCP 发送假帧，对端逐帧回执，用发送端的同一个时钟测量传输情况。
//
//	framenet echo -listen 地址:端口
//	framenet send -to 地址:端口 [-fps 30] [-kbps 4000] [-seconds 20] [-key 150000] [-inflight 4] [-idle 5]
//
// 发送端先空闲测往返（每 200 ms 一个小包），再按帧率发帧：第一帧为关键帧，其余帧大小在平均值上下 30% 内随机。
// 已发出未回执的帧达到 -inflight 时跳过当前帧（只保留最新帧的做法）。推流期间每秒插入一个小包，测排队后的往返。
package main

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net"
	"os"
	"sort"
	"sync"
	"time"
)

const (
	typeFrame = 0
	typePing  = 1
	headerLen = 9 // 类型 1 字节、序号 4 字节、长度 4 字节
	ackLen    = 5 // 类型 1 字节、序号 4 字节
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "用法：framenet echo|send ...")
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "echo":
		err = echo(os.Args[2:])
	case "send":
		err = send(os.Args[2:])
	default:
		err = fmt.Errorf("未知子命令：%s", os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func echo(args []string) error {
	fs := flag.NewFlagSet("echo", flag.ExitOnError)
	listen := fs.String("listen", ":18766", "监听地址")
	fs.Parse(args)
	ln, err := net.Listen("tcp", *listen)
	if err != nil {
		return err
	}
	for {
		c, err := ln.Accept()
		if err != nil {
			return err
		}
		go serve(c)
	}
}

func serve(c net.Conn) {
	defer c.Close()
	r := bufio.NewReaderSize(c, 1<<20)
	var h [headerLen]byte
	for {
		if _, err := io.ReadFull(r, h[:]); err != nil {
			return
		}
		n := binary.BigEndian.Uint32(h[5:9])
		if _, err := io.CopyN(io.Discard, r, int64(n)); err != nil {
			return
		}
		if _, err := c.Write(h[:ackLen]); err != nil {
			return
		}
	}
}

type msg struct {
	typ  byte
	seq  uint32
	size int
}

type result struct {
	Config        map[string]any `json:"config"`
	IdleRTT       map[string]any `json:"idle_rtt_ms"`
	LoadRTT       map[string]any `json:"load_rtt_ms"`
	Delivery      map[string]any `json:"frame_delivery_ms"`
	DeliveryFirst map[string]any `json:"frame_delivery_first_half_ms"`
	DeliveryLast  map[string]any `json:"frame_delivery_last_half_ms"`
	Frames        map[string]any `json:"frames"`
	GoodputKbps   float64        `json:"goodput_kbps"`
	Error         string         `json:"error,omitempty"`
}

func send(args []string) error {
	fs := flag.NewFlagSet("send", flag.ExitOnError)
	to := fs.String("to", "", "对端地址:端口")
	fps := fs.Int("fps", 30, "帧率")
	kbps := fs.Int("kbps", 4000, "平均码率")
	seconds := fs.Float64("seconds", 20, "推流时长")
	key := fs.Int("key", 150000, "关键帧字节数")
	inflight := fs.Int("inflight", 4, "最多几帧未回执")
	idle := fs.Float64("idle", 5, "推流前空闲测往返的秒数")
	fs.Parse(args)
	if *to == "" {
		return fmt.Errorf("需要 -to")
	}
	c, err := net.DialTimeout("tcp", *to, 10*time.Second)
	if err != nil {
		return err
	}
	defer c.Close()

	var mu sync.Mutex
	sent := map[[2]uint32]time.Time{} // (类型, 序号) → 放入发送队列的时刻
	var idleRTT, loadRTT, delivery []float64
	frameSent := map[uint32]int{}
	loading := false
	var acked, ackedBytes int
	var lastAck time.Time

	out := make(chan msg, 1024)
	buf := make([]byte, headerLen+*key+*kbps*1000/8)
	go func() {
		w := bufio.NewWriterSize(c, 1<<16)
		for m := range out {
			buf[0] = m.typ
			binary.BigEndian.PutUint32(buf[1:5], m.seq)
			binary.BigEndian.PutUint32(buf[5:9], uint32(m.size))
			if _, err := w.Write(buf[:headerLen+m.size]); err != nil {
				return
			}
			if len(out) == 0 {
				w.Flush()
			}
		}
	}()
	done := make(chan struct{})
	go func() {
		defer close(done)
		r := bufio.NewReader(c)
		var a [ackLen]byte
		for {
			if _, err := io.ReadFull(r, a[:]); err != nil {
				return
			}
			now := time.Now()
			k := [2]uint32{uint32(a[0]), binary.BigEndian.Uint32(a[1:5])}
			mu.Lock()
			if t, ok := sent[k]; ok {
				ms := float64(now.Sub(t).Microseconds()) / 1000
				switch {
				case a[0] == typeFrame:
					delivery = append(delivery, ms)
					acked++
					ackedBytes += frameSent[k[1]]
					lastAck = now
				case loading:
					loadRTT = append(loadRTT, ms)
				default:
					idleRTT = append(idleRTT, ms)
				}
				delete(sent, k)
			}
			mu.Unlock()
		}
	}()
	enqueue := func(m msg) {
		mu.Lock()
		sent[[2]uint32{uint32(m.typ), m.seq}] = time.Now()
		if m.typ == typeFrame {
			frameSent[m.seq] = m.size
		}
		mu.Unlock()
		out <- m
	}

	var pingSeq uint32
	idleEnd := time.Now().Add(time.Duration(*idle * float64(time.Second)))
	for time.Now().Before(idleEnd) {
		pingSeq++
		enqueue(msg{typePing, pingSeq, 0})
		time.Sleep(200 * time.Millisecond)
	}

	mu.Lock()
	loading = true
	mu.Unlock()
	avg := *kbps * 1000 / 8 / *fps
	interval := time.Second / time.Duration(*fps)
	total := int(*seconds * float64(*fps))
	start := time.Now()
	nextPing := start
	skipped, late := 0, 0
	var seq uint32
	for i := 0; i < total; i++ {
		due := start.Add(time.Duration(i) * interval)
		if d := time.Until(due); d > 0 {
			time.Sleep(d)
		} else if -d > interval {
			late++
		}
		if !time.Now().Before(nextPing) {
			pingSeq++
			enqueue(msg{typePing, pingSeq, 0})
			nextPing = nextPing.Add(time.Second)
		}
		mu.Lock()
		pending := 0
		for k := range sent {
			if k[0] == typeFrame {
				pending++
			}
		}
		mu.Unlock()
		if pending >= *inflight {
			skipped++
			continue
		}
		size := avg + rand.Intn(avg*6/10+1) - avg*3/10
		if seq == 0 {
			size = *key
		}
		seq++
		enqueue(msg{typeFrame, seq, size})
	}
	elapsed := time.Since(start)
	// 等最后的帧回执，最多 10 秒
	for t := time.Now(); time.Since(t) < 10*time.Second; time.Sleep(50 * time.Millisecond) {
		mu.Lock()
		n := len(sent)
		mu.Unlock()
		if n == 0 {
			break
		}
	}
	close(out)
	c.Close()
	<-done

	mu.Lock()
	defer mu.Unlock()
	half := len(delivery) / 2
	span := elapsed
	if !lastAck.IsZero() && lastAck.Sub(start) > span {
		span = lastAck.Sub(start)
	}
	res := result{
		Config: map[string]any{
			"fps": *fps, "kbps": *kbps, "seconds": *seconds, "key_bytes": *key, "inflight": *inflight,
		},
		IdleRTT:       summary(idleRTT),
		LoadRTT:       summary(loadRTT),
		Delivery:      summary(delivery),
		DeliveryFirst: summary(delivery[:half]),
		DeliveryLast:  summary(delivery[half:]),
		Frames: map[string]any{
			"planned": total, "sent": int(seq), "acked": acked, "skipped": skipped, "pacing_late": late,
			"sent_fps": round(float64(seq) / elapsed.Seconds()), "acked_fps": round(float64(acked) / span.Seconds()),
		},
		GoodputKbps: round(float64(ackedBytes) * 8 / 1000 / span.Seconds()),
	}
	return json.NewEncoder(os.Stdout).Encode(res)
}

func summary(v []float64) map[string]any {
	if len(v) == 0 {
		return nil
	}
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	p := func(q float64) float64 { return round(s[int(q*float64(len(s)-1))]) }
	return map[string]any{"n": len(s), "p50": p(0.5), "p95": p(0.95), "max": round(s[len(s)-1])}
}

func round(x float64) float64 { return float64(int64(x*100+0.5)) / 100 }
