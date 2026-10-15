package autostart

import (
	"encoding/xml"
	"strings"
	"testing"
)

func TestQuoteArg(t *testing.T) {
	cases := map[string]string{
		"node":        "node",
		"":            `""`,
		"台式 机":        `"台式 机"`,
		`a"b`:         `"a\"b"`,
		`C:\a b\`:     `"C:\a b\\"`,
		`C:\dir\file`: `C:\dir\file`,
		`x\"y z`:      `"x\\\"y z"`,
	}
	for in, want := range cases {
		if got := quoteArg(in); got != want {
			t.Errorf("quoteArg(%q) = %s，想要 %s", in, got, want)
		}
	}
}

func TestTaskXML(t *testing.T) {
	x := taskXML(Config{
		Exe:  `D:\a & b\deskbridge.exe`,
		Args: []string{"node", "--name", "台式 <机>"},
		User: `PC\me`,
	})
	if err := xml.Unmarshal([]byte(strings.Replace(x, "UTF-16", "UTF-8", 1)), new(struct{})); err != nil {
		t.Fatalf("不是合法的 XML: %v", err)
	}
	for _, s := range []string{
		`<Command>D:\a &amp; b\deskbridge.exe</Command>`,
		`<Arguments>node --name &#34;台式 &lt;机&gt;&#34;</Arguments>`,
		`<LogonTrigger>`, `<RestartOnFailure>`, `<ExecutionTimeLimit>PT0S`,
	} {
		if !strings.Contains(x, s) {
			t.Errorf("缺少 %s", s)
		}
	}
}

func TestUTF16LE(t *testing.T) {
	b := utf16LE("a中")
	want := []byte{0xFF, 0xFE, 'a', 0, 0x2D, 0x4E}
	if string(b) != string(want) {
		t.Errorf("% x", b)
	}
}
