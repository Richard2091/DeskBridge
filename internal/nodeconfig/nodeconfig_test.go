package nodeconfig

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissing(t *testing.T) {
	c, err := Load(filepath.Join(t.TempDir(), "node.json"))
	if err != nil || c != (Config{}) {
		t.Fatalf("文件不存在应返回零值: %+v, %v", c, err)
	}
}

func TestRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "node.json")
	want := Config{Name: "我的电脑", Hostname: "win-pc", ControlURL: "https://hs.example.com", Port: 7500}
	if err := Save(p, want); err != nil {
		t.Fatal(err)
	}
	got, err := Load(p)
	if err != nil || got != want {
		t.Fatalf("往返不一致: %+v, %v", got, err)
	}
	if _, err := os.Stat(p + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("临时文件应已改名: %v", err)
	}
}

func TestPartialFields(t *testing.T) {
	p := filepath.Join(t.TempDir(), "node.json")
	if err := os.WriteFile(p, []byte(`{"hostname":"a"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := Load(p)
	if err != nil || c != (Config{Hostname: "a"}) {
		t.Fatalf("缺省字段应为零值: %+v, %v", c, err)
	}
}

func TestLoadCorrupt(t *testing.T) {
	p := filepath.Join(t.TempDir(), "node.json")
	if err := os.WriteFile(p, []byte(`{"name":`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(p); err == nil {
		t.Fatal("损坏的 JSON 应返回错误")
	}
}
