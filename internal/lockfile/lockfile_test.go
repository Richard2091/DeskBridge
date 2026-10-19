package lockfile

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestAcquireTwice(t *testing.T) {
	p := filepath.Join(t.TempDir(), "node.lock")
	rel, err := Acquire(p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Acquire(p); !errors.Is(err, ErrLocked) {
		t.Fatalf("第二次获取应返回 ErrLocked，实际 %v", err)
	}
	rel()
	rel2, err := Acquire(p)
	if err != nil {
		t.Fatalf("释放后应能再次获取: %v", err)
	}
	rel2()
}
