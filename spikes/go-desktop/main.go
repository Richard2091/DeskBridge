// 技术验证：Go + go-element 操作记事本。步骤见 spikes/README.md。
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unsafe"

	uia "github.com/auuunya/go-element"
	"github.com/kbinani/screenshot"
	"golang.org/x/sys/windows"
)

const (
	lang     = "go"
	testText = "你好，DeskBridge 测试 123"
)

var (
	user32                        = windows.NewLazySystemDLL("user32.dll")
	procSetProcessDpiAwarenessCtx = user32.NewProc("SetProcessDpiAwarenessContext")
	procGetSystemMetrics          = user32.NewProc("GetSystemMetrics")
	procSendInput                 = user32.NewProc("SendInput")
	procSetCursorPos              = user32.NewProc("SetCursorPos")
	procGetForegroundWindow       = user32.NewProc("GetForegroundWindow")
	procSetForegroundWindow       = user32.NewProc("SetForegroundWindow")
	procWindowFromPoint           = user32.NewProc("WindowFromPoint")
	procGetAncestor               = user32.NewProc("GetAncestor")

	foregroundAttempts int
)

func main() {
	runtime.LockOSThread() // COM 要求同一线程
	start := time.Now()
	res := map[string]any{}

	// DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 = -4
	ok, _, _ := procSetProcessDpiAwarenessCtx.Call(^uintptr(3))
	res["dpi_aware"] = ok != 0
	res["screen_physical"] = []int{metric(0), metric(1)}

	tmp := os.TempDir()
	txt := filepath.Join(tmp, "deskbridge-spike-"+lang+".txt")
	pngPath := filepath.Join(tmp, "deskbridge-spike-"+lang+".png")
	_ = os.WriteFile(txt, nil, 0o644)

	cmd := exec.Command("notepad.exe", txt)
	if err := cmd.Start(); err != nil {
		res["error"] = err.Error()
		output(res, start)
		return
	}
	if err := run(res, cmd.Process.Pid, txt, pngPath); err != nil {
		res["error"] = err.Error()
	}
	_ = cmd.Process.Kill()
	res["foreground_switch_attempts"] = foregroundAttempts
	output(res, start)
}

func run(res map[string]any, pid int, txt, pngPath string) error {
	uia.CoInitialize()
	defer uia.CoUninitialize()

	// 等待记事本窗口出现，并确认属于我们启动的进程
	var hwnd uintptr
	for i := 0; i < 50 && hwnd == 0; i++ {
		time.Sleep(200 * time.Millisecond)
		if h, err := uia.GetWindowForString("Notepad", ""); err == nil && h != 0 {
			var p uint32
			windows.GetWindowThreadProcessId(windows.HWND(h), &p)
			if int(p) == pid {
				hwnd = h
			}
		}
	}
	if hwnd == 0 {
		return errors.New("找不到记事本窗口")
	}

	instance, err := uia.CreateInstance(uia.CLSID_CUIAutomation, uia.IID_IUIAutomation, uia.CLSCTX_INPROC_SERVER)
	if err != nil {
		return fmt.Errorf("创建 UIAutomation 失败: %w", err)
	}
	ppv := uia.NewIUIAutomation(uia.NewIUnKnown(instance))
	defer ppv.Release()
	rootElem, err := uia.ElementFromHandle(ppv, hwnd)
	if err != nil {
		return fmt.Errorf("获取窗口元素失败: %w", err)
	}

	// 3. 元素树
	tree := uia.TraverseUIElementTree(ppv, rootElem)
	res["window_title"] = tree.CurrentName
	res["window_rect"] = rectOf(tree)
	var sample []string
	res["tree_nodes"] = walk(tree, 0, &sample)
	res["tree_sample"] = sample

	// 4. 按名称展开“文件”菜单
	fileMenu := uia.SearchElem(tree, func(e *uia.Element) bool {
		return e.CurrentControlType == uia.UIA_MenuItemControlTypeId &&
			(strings.HasPrefix(e.CurrentName, "文件") || strings.HasPrefix(e.CurrentName, "File"))
	})
	if fileMenu != nil {
		res["menu_item"] = fileMenu.CurrentName
		ec, err := fileMenu.GetExpandCollapsePattern()
		if err != nil {
			return fmt.Errorf("菜单项不支持展开: %w", err)
		}
		_ = ec.Expand()
		time.Sleep(500 * time.Millisecond)
		res["menu_by_pattern"] = menuEntries(ppv)
		if err := ensureForeground(hwnd); err != nil {
			return err
		}
		pressKey(0x1B) // Esc
		pressKey(0x1B)
		time.Sleep(300 * time.Millisecond)

		// 5. 按坐标点击
		r := rectOf(fileMenu)
		res["menu_item_rect"] = r
		x, y := r[0]+r[2]/2, r[1]+r[3]/2
		if err := ensureForeground(hwnd); err != nil {
			return err
		}
		if !pointBelongsTo(x, y, hwnd) {
			return errors.New("点击位置不属于记事本，停止")
		}
		click(x, y)
		time.Sleep(500 * time.Millisecond)
		res["menu_by_coordinates"] = menuEntries(ppv)
		if err := ensureForeground(hwnd); err != nil {
			return err
		}
		pressKey(0x1B)
		pressKey(0x1B)
		time.Sleep(300 * time.Millisecond)
	}

	// 6. 输入中文并保存（记事本窗口在前台时，焦点默认在编辑区）
	editor := uia.SearchElem(tree, func(e *uia.Element) bool {
		return e.CurrentControlType == uia.UIA_DocumentControlTypeId || e.CurrentControlType == uia.UIA_EditControlTypeId
	})
	if editor == nil {
		return errors.New("找不到编辑区")
	}
	if err := ensureForeground(hwnd); err != nil {
		return err
	}
	time.Sleep(300 * time.Millisecond)
	res["sendinput_count"] = sendUnicode(testText)
	time.Sleep(300 * time.Millisecond)
	if vp, err := editor.GetValuePattern(); err == nil {
		v, _ := vp.Get_CurrentValue()
		res["edit_value_via_uia"] = v
		vp.Release()
	}
	if err := ensureForeground(hwnd); err != nil {
		return err
	}
	keyDown(0x11) // Ctrl
	pressKey(0x53) // S
	keyUp(0x11)
	time.Sleep(800 * time.Millisecond)
	data, err := os.ReadFile(txt)
	if err != nil {
		return err
	}
	saved := strings.TrimPrefix(string(data), "﻿")
	res["saved_text"] = saved
	res["typed_ok"] = saved == testText

	// 7. 截图
	img, err := screenshot.CaptureDisplay(0)
	if err != nil {
		return fmt.Errorf("截图失败: %w", err)
	}
	f, err := os.Create(pngPath)
	if err != nil {
		return err
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		return err
	}
	f.Close()
	st, _ := os.Stat(pngPath)
	res["screenshot"] = map[string]any{"path": pngPath, "width": img.Bounds().Dx(), "height": img.Bounds().Dy(), "bytes": st.Size()}
	return nil
}

func walk(e *uia.Element, depth int, sample *[]string) int {
	if len(*sample) < 15 {
		*sample = append(*sample, fmt.Sprintf("%s%d '%s'", strings.Repeat("  ", depth), e.CurrentControlType, e.CurrentName))
	}
	n := 1
	if depth >= 6 {
		return n
	}
	for _, c := range e.Child {
		n += walk(c, depth+1, sample)
	}
	return n
}

// 经典 Win32 弹出菜单是类名为 #32768 的顶层窗口；读取前几项名称作为“菜单已打开”的证据
func menuEntries(ppv *uia.IUIAutomation) []string {
	h, err := uia.GetWindowForString("#32768", "")
	if err != nil || h == 0 {
		return nil
	}
	el, err := uia.ElementFromHandle(ppv, h)
	if err != nil {
		return nil
	}
	tree := uia.TraverseUIElementTree(ppv, el)
	var names []string
	for _, c := range tree.Child {
		if len(names) >= 5 {
			break
		}
		names = append(names, c.CurrentName)
	}
	return names
}

func rectOf(e *uia.Element) []int {
	r := e.CurrentBoundingRectangle
	if r == nil {
		return nil
	}
	return []int{int(r.Left), int(r.Top), int(r.Right - r.Left), int(r.Bottom - r.Top)}
}

func metric(i int) int {
	v, _, _ := procGetSystemMetrics.Call(uintptr(i))
	return int(int32(v))
}

// 确认目标窗口在前台；不在则尝试切换，最多 3 次
func ensureForeground(hwnd uintptr) error {
	for i := 0; i < 3; i++ {
		if fg, _, _ := procGetForegroundWindow.Call(); fg == hwnd {
			return nil
		}
		foregroundAttempts++
		procSetForegroundWindow.Call(hwnd)
		time.Sleep(200 * time.Millisecond)
	}
	if fg, _, _ := procGetForegroundWindow.Call(); fg == hwnd {
		return nil
	}
	return errors.New("记事本不在前台，停止，避免误操作其他窗口")
}

// 点击位置的顶层窗口必须是目标窗口
func pointBelongsTo(x, y int, hwnd uintptr) bool {
	pt := uintptr(uint32(int32(x))) | uintptr(uint32(int32(y)))<<32 // POINT 按值传递（amd64）
	w, _, _ := procWindowFromPoint.Call(pt)
	root, _, _ := procGetAncestor.Call(w, 2) // GA_ROOT
	return root == hwnd
}

// INPUT 结构在 amd64 上为 40 字节；键盘和鼠标分别按各自布局填充
type keyboardInput struct {
	typ     uint32
	_       uint32
	wVk     uint16
	wScan   uint16
	dwFlags uint32
	time    uint32
	_       uint32
	extra   uintptr
	_       [8]byte
}

type mouseInput struct {
	typ       uint32
	_         uint32
	dx, dy    int32
	mouseData uint32
	dwFlags   uint32
	time      uint32
	_         uint32
	extra     uintptr
}

const (
	inputMouse       = 0
	inputKeyboard    = 1
	keyeventfKeyUp   = 0x2
	keyeventfUnicode = 0x4
	mouseLeftDown    = 0x2
	mouseLeftUp      = 0x4
)

func sendKeyboard(in []keyboardInput) int {
	if len(in) == 0 {
		return 0
	}
	n, _, _ := procSendInput.Call(uintptr(len(in)), uintptr(unsafe.Pointer(&in[0])), unsafe.Sizeof(in[0]))
	return int(n)
}

// 每个字符都以 Unicode 发送，绕过中文输入法
func sendUnicode(text string) int {
	var in []keyboardInput
	for _, u := range windows.StringToUTF16(text) {
		if u == 0 {
			continue
		}
		in = append(in,
			keyboardInput{typ: inputKeyboard, wScan: u, dwFlags: keyeventfUnicode},
			keyboardInput{typ: inputKeyboard, wScan: u, dwFlags: keyeventfUnicode | keyeventfKeyUp})
	}
	return sendKeyboard(in)
}

func keyDown(vk uint16) { sendKeyboard([]keyboardInput{{typ: inputKeyboard, wVk: vk}}) }
func keyUp(vk uint16)   { sendKeyboard([]keyboardInput{{typ: inputKeyboard, wVk: vk, dwFlags: keyeventfKeyUp}}) }
func pressKey(vk uint16) {
	keyDown(vk)
	keyUp(vk)
}

func click(x, y int) {
	procSetCursorPos.Call(uintptr(x), uintptr(y))
	in := []mouseInput{{typ: inputMouse, dwFlags: mouseLeftDown}, {typ: inputMouse, dwFlags: mouseLeftUp}}
	procSendInput.Call(2, uintptr(unsafe.Pointer(&in[0])), unsafe.Sizeof(in[0]))
}

func output(res map[string]any, start time.Time) {
	res["elapsed_ms"] = time.Since(start).Milliseconds()
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	_ = enc.Encode(res)
	os.Stdout.Write(buf.Bytes())
}
