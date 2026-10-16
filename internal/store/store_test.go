package store

import (
	"path/filepath"
	"testing"
	"time"
)

func open(t *testing.T) (*DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "db.sqlite")
	d, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	return d, path
}

func begin(t *testing.T, d *DB, r Record) int64 {
	t.Helper()
	id, err := d.Begin(r)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestBeginFinishQuery(t *testing.T) {
	d, _ := open(t)
	now := time.Now()
	id := begin(t, d, Record{RequestID: "r1", Role: RoleTarget, OperatorType: "ai", Via: "mcp", Session: "s1",
		Tool: "proc.exec", Args: `{"command":"dir"}`, StartedAt: now.UnixMilli()})

	got, err := d.Query(Filter{})
	if err != nil || len(got) != 1 {
		t.Fatalf("查询 = %v, %v", got, err)
	}
	if got[0].Status != StatusRunning || got[0].EndedAt != 0 {
		t.Fatalf("新记录应为 running：%+v", got[0])
	}
	if err := d.Finish(id, "ok", now.Add(time.Second), 1000); err != nil {
		t.Fatal(err)
	}
	got, _ = d.Query(Filter{RequestID: "r1"})
	if got[0].Status != "ok" || got[0].DurationMS != 1000 || got[0].EndedAt != now.Add(time.Second).UnixMilli() {
		t.Fatalf("结束后：%+v", got[0])
	}
	if got[0].Via != "mcp" || got[0].Session != "s1" || got[0].Args != `{"command":"dir"}` {
		t.Fatalf("字段丢失：%+v", got[0])
	}
}

func TestFilters(t *testing.T) {
	d, _ := open(t)
	base := time.Now().Add(-time.Hour).UnixMilli()
	mk := func(req, role, op, tool, status string, off int64) {
		id := begin(t, d, Record{RequestID: req, Role: role, OperatorType: op, Tool: tool, StartedAt: base + off})
		if status != StatusRunning {
			d.Finish(id, status, time.UnixMilli(base+off+5), 5)
		}
	}
	mk("a", RoleTarget, "ai", "proc.exec", "ok", 0)
	mk("b", RoleTarget, "human", "fs.read", "ok", 10)
	mk("c", RoleTarget, "ai", "fs.write", "permission_denied", 20)
	mk("d", RoleSource, "ai", "proc.exec", "cancelled", 30)
	mk("e", RoleTarget, "ai", "proc.exec", StatusRunning, 40)

	count := func(f Filter) int {
		t.Helper()
		got, err := d.Query(f)
		if err != nil {
			t.Fatal(err)
		}
		return len(got)
	}
	cases := []struct {
		name string
		f    Filter
		want int
	}{
		{"全部", Filter{}, 5},
		{"AI", Filter{OperatorType: "ai"}, 4},
		{"人", Filter{OperatorType: "human"}, 1},
		{"精确工具", Filter{Tool: "proc.exec"}, 3},
		{"前缀工具", Filter{Tool: "fs.*"}, 2},
		{"进行中", Filter{Status: "running"}, 1},
		{"成功", Filter{Status: "ok"}, 2},
		{"失败", Filter{Status: "failed"}, 1},
		{"错误码", Filter{Status: "cancelled"}, 1},
		{"被控端", Filter{Role: RoleTarget}, 4},
		{"时间下限", Filter{Since: base + 20}, 3},
		{"时间上限", Filter{Until: base + 20}, 2},
		{"限制条数", Filter{Limit: 2}, 2},
	}
	for _, c := range cases {
		if got := count(c.f); got != c.want {
			t.Errorf("%s: 得到 %d 条，应为 %d", c.name, got, c.want)
		}
	}

	// 翻页：每页 2 条，应按从新到旧不重不漏地取完 5 条。
	var seen []string
	var before int64
	for {
		page, err := d.Query(Filter{Limit: 2, BeforeID: before})
		if err != nil {
			t.Fatal(err)
		}
		if len(page) == 0 {
			break
		}
		for _, r := range page {
			seen = append(seen, r.RequestID)
		}
		before = page[len(page)-1].ID
	}
	if got := join(seen); got != "e,d,c,b,a" {
		t.Fatalf("翻页顺序 = %s", got)
	}
}

func join(s []string) string {
	out := ""
	for i, v := range s {
		if i > 0 {
			out += ","
		}
		out += v
	}
	return out
}

func TestMarkInterruptedAndReopen(t *testing.T) {
	d, path := open(t)
	begin(t, d, Record{RequestID: "x", Role: RoleTarget, OperatorType: "human", Tool: "proc.exec"})
	d.Close()

	d2, err := Open(path) // 重新打开：数据还在，迁移不会重复执行
	if err != nil {
		t.Fatal(err)
	}
	defer d2.Close()
	n, err := d2.MarkInterrupted()
	if err != nil || n != 1 {
		t.Fatalf("MarkInterrupted = %d, %v", n, err)
	}
	got, _ := d2.Query(Filter{})
	if len(got) != 1 || got[0].Status != StatusInterrupted || got[0].EndedAt == 0 {
		t.Fatalf("重启后的记录 = %+v", got)
	}
}

func TestRetention(t *testing.T) {
	d, path := open(t)
	old := time.Now().Add(-retention - time.Hour).UnixMilli()
	begin(t, d, Record{RequestID: "old", Role: RoleTarget, OperatorType: "ai", Tool: "t", StartedAt: old})
	begin(t, d, Record{RequestID: "new", Role: RoleTarget, OperatorType: "ai", Tool: "t"})
	d.Close()
	d2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer d2.Close()
	got, _ := d2.Query(Filter{})
	if len(got) != 1 || got[0].RequestID != "new" {
		t.Fatalf("过期记录应被清理：%+v", got)
	}
}

func TestNewerDatabaseRejected(t *testing.T) {
	d, path := open(t)
	if _, err := d.db.Exec(`PRAGMA user_version = 99`); err != nil {
		t.Fatal(err)
	}
	d.Close()
	if _, err := Open(path); err == nil {
		t.Fatal("版本更高的数据库应被拒绝")
	}
}
