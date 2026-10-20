//go:build darwin

package autostart

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLabelFor(t *testing.T) {
	if got := labelFor(""); got != "io.github.richard2091.DeskBridgeNode" {
		t.Errorf("默认标签 = %q", got)
	}
	if got := labelFor("x"); got != "io.github.richard2091.x" {
		t.Errorf("标签 = %q", got)
	}
}

func TestLaunchdPlist(t *testing.T) {
	c := Config{
		Exe:  "/Applications/Deskbridge & Co/db",
		Args: []string{"supervise", "-log-file", `/tmp/a b/<x>.log`, `q"uote&<>`, "--", "node"},
	}
	label := labelFor("t&1")
	f := filepath.Join(t.TempDir(), "t.plist")
	if err := os.WriteFile(f, launchdPlist(label, c), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("plutil", "-lint", f).CombinedOutput(); err != nil {
		t.Fatalf("plutil -lint 失败: %v\n%s", err, out)
	}
	out, err := exec.Command("plutil", "-convert", "json", "-o", "-", f).Output()
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Label                  string
		ProgramArguments       []string
		RunAtLoad              bool
		KeepAlive              map[string]bool
		LimitLoadToSessionType string
		ProcessType            string
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	if got.Label != label {
		t.Errorf("Label = %q", got.Label)
	}
	if want := append([]string{c.Exe}, c.Args...); !reflect.DeepEqual(got.ProgramArguments, want) {
		t.Errorf("ProgramArguments = %q，期望 %q", got.ProgramArguments, want)
	}
	if !got.RunAtLoad || got.LimitLoadToSessionType != "Aqua" || got.ProcessType != "Interactive" {
		t.Errorf("基本键不对: %+v", got)
	}
	if v, ok := got.KeepAlive["SuccessfulExit"]; !ok || v {
		t.Errorf("KeepAlive = %v", got.KeepAlive)
	}
}
