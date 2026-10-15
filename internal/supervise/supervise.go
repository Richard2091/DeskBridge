// Package supervise 在子进程失败后重启它。计划任务自带的“失败后重启”
// 对退出码非零的程序不生效（2026-10-15 在 Windows 上实测：退出码 1 后
// 任务不再重启），而且最短间隔 1 分钟，所以由 DeskBridge 自己守护节点。
package supervise

import (
	"context"
	"time"
)

// Options 是重启策略。零值使用默认值。
type Options struct {
	MinBackoff  time.Duration // 首次重启前等待，默认 1 秒
	MaxBackoff  time.Duration // 连续失败时等待的上限，默认 1 分钟
	StableAfter time.Duration // 运行超过这个时间才算稳定，下次失败重新从 MinBackoff 起算，默认 1 分钟
	Logf        func(format string, args ...any)
}

func (o *Options) defaults() {
	if o.MinBackoff <= 0 {
		o.MinBackoff = time.Second
	}
	if o.MaxBackoff < o.MinBackoff {
		o.MaxBackoff = max(time.Minute, o.MinBackoff)
	}
	if o.StableAfter <= 0 {
		o.StableAfter = time.Minute
	}
	if o.Logf == nil {
		o.Logf = func(string, ...any) {}
	}
}

// Run 反复调用 run，直到 ctx 取消，或 run 返回退出码 0（正常退出）。
// run 返回非零退出码或错误（含启动失败）都会等待一段退避时间后重新运行：
// 连续失败时等待加倍，直到 MaxBackoff。ctx 取消时返回 nil。
func Run(ctx context.Context, run func(ctx context.Context) (int, error), opt Options) error {
	opt.defaults()
	backoff := opt.MinBackoff
	for {
		start := time.Now()
		code, err := run(ctx)
		if ctx.Err() != nil {
			return nil
		}
		if err == nil && code == 0 {
			opt.Logf("子进程正常退出，不再重启")
			return nil
		}
		if time.Since(start) >= opt.StableAfter {
			backoff = opt.MinBackoff
		}
		if err != nil {
			opt.Logf("子进程失败：%v；%v 后重启", err, backoff)
		} else {
			opt.Logf("子进程退出，退出码 %d；%v 后重启", code, backoff)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, opt.MaxBackoff)
	}
}
