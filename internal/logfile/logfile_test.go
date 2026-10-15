package logfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRotate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "node.log")
	w, err := Open(path, 10, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	for _, s := range []string{"aaaaaa\n", "bbbbbb\n", "cccccc\n", "dddddd\n"} {
		if _, err := w.Write([]byte(s)); err != nil {
			t.Fatal(err)
		}
	}
	want := map[string]string{
		path:        "dddddd\n",
		path + ".1": "cccccc\n",
		path + ".2": "bbbbbb\n",
	}
	for p, c := range want {
		b, err := os.ReadFile(p)
		if err != nil || string(b) != c {
			t.Errorf("%s = %q, %v；想要 %q", filepath.Base(p), b, err, c)
		}
	}
	if _, err := os.Stat(path + ".3"); err == nil {
		t.Error("不应保留第 3 个旧文件")
	}
}

func TestAppendsToExisting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "node.log")
	os.WriteFile(path, []byte("old\n"), 0o600)
	w, err := Open(path, 100, 1)
	if err != nil {
		t.Fatal(err)
	}
	w.Write([]byte("new\n"))
	w.Close()
	b, _ := os.ReadFile(path)
	if !strings.HasPrefix(string(b), "old\n") || !strings.HasSuffix(string(b), "new\n") {
		t.Errorf("内容 %q", b)
	}
	if _, err := w.Write([]byte("x")); err == nil {
		t.Error("关闭后写入应报错")
	}
}
