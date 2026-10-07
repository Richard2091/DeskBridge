package platform

import (
	"errors"
	"runtime"
	"testing"
)

func TestDesktopHelperName(t *testing.T) {
	name, err := DesktopHelperName()
	if runtime.GOOS == "windows" {
		if err != nil || name != "deskbridge-desktop-helper.exe" {
			t.Fatalf("DesktopHelperName() = %q, %v；期望可执行文件名", name, err)
		}
		return
	}
	if !errors.Is(err, ErrUnsupportedPlatform) {
		t.Fatalf("DesktopHelperName() 的错误 = %v，期望 ErrUnsupportedPlatform", err)
	}
}
