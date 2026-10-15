package supervise

import (
	"context"
	"errors"
	"testing"
	"time"
)

func fast() Options {
	return Options{MinBackoff: time.Millisecond, MaxBackoff: 4 * time.Millisecond, StableAfter: time.Hour}
}

func TestRestartsUntilCleanExit(t *testing.T) {
	var runs int
	err := Run(context.Background(), func(context.Context) (int, error) {
		runs++
		switch runs {
		case 1:
			return 1, nil
		case 2:
			return 0, errors.New("启动失败")
		}
		return 0, nil
	}, fast())
	if err != nil || runs != 3 {
		t.Fatalf("err=%v runs=%d", err, runs)
	}
}

func TestStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var runs int
	err := Run(ctx, func(context.Context) (int, error) {
		runs++
		if runs == 3 {
			cancel()
		}
		return 1, nil
	}, fast())
	if err != nil || runs != 3 {
		t.Fatalf("err=%v runs=%d", err, runs)
	}
}

func TestBackoffGrowsAndResets(t *testing.T) {
	var waits []time.Duration
	opt := fast()
	opt.StableAfter = 20 * time.Millisecond
	opt.Logf = func(_ string, a ...any) {
		if len(a) > 0 { // 没有参数的是“正常退出”那条
			waits = append(waits, a[len(a)-1].(time.Duration))
		}
	}
	var runs int
	Run(context.Background(), func(context.Context) (int, error) {
		runs++
		if runs == 4 {
			time.Sleep(30 * time.Millisecond) // 稳定运行过，退避应重置
		}
		if runs == 6 {
			return 0, nil
		}
		return 1, nil
	}, opt)
	ms := time.Millisecond
	want := []time.Duration{ms, 2 * ms, 4 * ms, ms, 2 * ms}
	if len(waits) < len(want) {
		t.Fatalf("waits=%v", waits)
	}
	for i, w := range want {
		if waits[i] != w {
			t.Errorf("第 %d 次等待 %v，想要 %v", i+1, waits[i], w)
		}
	}
}
