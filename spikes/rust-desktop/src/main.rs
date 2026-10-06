// 技术验证：Rust + uiautomation 操作记事本。步骤见 spikes/README.md。
use std::{collections::BTreeMap, path::PathBuf, process::Command, thread::sleep, time::{Duration, Instant}};

use serde_json::{json, Value};
use uiautomation::{
    inputs::{Keyboard, Mouse},
    patterns::{UIExpandCollapsePattern, UIValuePattern},
    types::{ControlType, Point},
    UIAutomation, UIElement, UITreeWalker,
};
use windows::Win32::UI::{
    HiDpi::{SetProcessDpiAwarenessContext, DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2},
    Input::KeyboardAndMouse::{
        SendInput, INPUT, INPUT_0, INPUT_KEYBOARD, KEYBDINPUT, KEYEVENTF_KEYUP, KEYEVENTF_UNICODE, VIRTUAL_KEY,
    },
    WindowsAndMessaging::{
        GetAncestor, GetForegroundWindow, GetSystemMetrics, SetForegroundWindow, WindowFromPoint, GA_ROOT, SM_CXSCREEN,
        SM_CYSCREEN,
    },
};
use windows::Win32::{Foundation::{HWND, POINT}};

static mut FOREGROUND_ATTEMPTS: u32 = 0;

// 确认目标窗口在前台；不在则尝试切换，最多 3 次
fn ensure_foreground(hwnd: HWND) -> Result<(), Box<dyn std::error::Error>> {
    for _ in 0..3 {
        if unsafe { GetForegroundWindow() } == hwnd {
            return Ok(());
        }
        unsafe {
            FOREGROUND_ATTEMPTS += 1;
            let _ = SetForegroundWindow(hwnd);
        }
        sleep(Duration::from_millis(200));
    }
    if unsafe { GetForegroundWindow() } == hwnd { Ok(()) } else { Err("记事本不在前台，停止，避免误操作其他窗口".into()) }
}

// 点击位置的顶层窗口必须是目标窗口
fn point_belongs_to(x: i32, y: i32, hwnd: HWND) -> bool {
    unsafe { GetAncestor(WindowFromPoint(POINT { x, y }), GA_ROOT) == hwnd }
}

const LANG: &str = "rust";
const TEST_TEXT: &str = "你好，DeskBridge 测试 123";

fn main() {
    let start = Instant::now();
    let mut res: BTreeMap<&str, Value> = BTreeMap::new();

    let dpi = unsafe { SetProcessDpiAwarenessContext(DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2) };
    res.insert("dpi_aware", json!(dpi.is_ok()));
    let (sw, sh) = unsafe { (GetSystemMetrics(SM_CXSCREEN), GetSystemMetrics(SM_CYSCREEN)) };
    res.insert("screen_physical", json!([sw, sh]));

    let tmp = std::env::temp_dir();
    let txt: PathBuf = tmp.join(format!("deskbridge-spike-{LANG}.txt"));
    let png: PathBuf = tmp.join(format!("deskbridge-spike-{LANG}.png"));
    std::fs::write(&txt, "").unwrap();

    let mut child = Command::new("notepad.exe").arg(&txt).spawn().expect("启动记事本失败");
    if let Err(e) = run(&mut res, &txt, &png) {
        res.insert("error", json!(e.to_string()));
    }
    let _ = child.kill();

    res.insert("foreground_switch_attempts", json!(unsafe { FOREGROUND_ATTEMPTS }));
    res.insert("elapsed_ms", json!(start.elapsed().as_millis()));
    println!("{}", serde_json::to_string_pretty(&res).unwrap());
}

fn run(res: &mut BTreeMap<&str, Value>, txt: &PathBuf, png: &PathBuf) -> Result<(), Box<dyn std::error::Error>> {
    let automation = UIAutomation::new()?;
    let win = automation.create_matcher().classname("Notepad").timeout(10000).find_first()?;
    res.insert("window_title", json!(win.get_name()?));
    let raw: isize = win.get_native_window_handle()?.into();
    let hwnd = HWND(raw as _);
    res.insert("window_rect", json!(rect(&win)?));

    // 3. 元素树
    let walker = automation.get_control_view_walker()?;
    let mut sample = Vec::new();
    let n = walk(&walker, &win, 0, &mut sample);
    res.insert("tree_nodes", json!(n));
    res.insert("tree_sample", json!(sample));

    // 4. 按名称展开“文件”菜单
    let items = automation.create_matcher().from(win.clone()).control_type(ControlType::MenuItem).find_all()?;
    let file_menu = items.into_iter().find(|e| {
        let name = e.get_name().unwrap_or_default();
        name.starts_with("文件") || name.starts_with("File")
    });
    if let Some(menu) = &file_menu {
        res.insert("menu_item", json!(menu.get_name()?));
        menu.get_pattern::<UIExpandCollapsePattern>()?.expand()?;
        sleep(Duration::from_millis(500));
        res.insert("menu_by_pattern", json!(menu_entries(&automation, &walker, &win)));
        ensure_foreground(hwnd)?;
        Keyboard::new().send_keys("{esc}{esc}")?;
        sleep(Duration::from_millis(300));

        // 5. 按坐标点击
        let r = rect(menu)?;
        res.insert("menu_item_rect", json!(r));
        let (x, y) = (r[0] + r[2] / 2, r[1] + r[3] / 2);
        ensure_foreground(hwnd)?;
        if !point_belongs_to(x, y, hwnd) {
            return Err("点击位置不属于记事本，停止".into());
        }
        Mouse::new().click(&Point::new(x, y))?;
        sleep(Duration::from_millis(500));
        res.insert("menu_by_coordinates", json!(menu_entries(&automation, &walker, &win)));
        // 第一次 Esc 关闭下拉菜单，第二次退出菜单栏，否则菜单栏会吞掉下一个按键
        ensure_foreground(hwnd)?;
        Keyboard::new().send_keys("{esc}{esc}")?;
        sleep(Duration::from_millis(300));
    }

    // 6. 输入中文并保存
    let edit = automation.create_matcher().from(win.clone()).control_type(ControlType::Edit).find_first()?;
    edit.set_focus()?;
    sleep(Duration::from_millis(300));
    // 每个字符都以 Unicode 发送，绕过中文输入法
    ensure_foreground(hwnd)?;
    res.insert("sendinput_count", json!(send_unicode(TEST_TEXT)));
    sleep(Duration::from_millis(300));
    if let Ok(vp) = edit.get_pattern::<UIValuePattern>() {
        res.insert("edit_value_via_uia", json!(vp.get_value()?));
    }
    ensure_foreground(hwnd)?;
    Keyboard::new().send_keys("{ctrl}(s)")?;
    sleep(Duration::from_millis(800));
    let saved = std::fs::read_to_string(txt)?.trim_start_matches('\u{feff}').to_string();
    res.insert("typed_ok", json!(saved == TEST_TEXT));
    res.insert("saved_text", json!(saved));

    // 7. 截图
    let monitors = xcap::Monitor::all()?;
    let primary = monitors.into_iter().find(|m| m.is_primary().unwrap_or(false)).ok_or("找不到主显示器")?;
    let img = primary.capture_image()?;
    img.save(png)?;
    res.insert("screenshot", json!({
        "path": png.display().to_string(),
        "width": img.width(),
        "height": img.height(),
        "bytes": std::fs::metadata(png)?.len(),
    }));
    Ok(())
}

fn rect(e: &UIElement) -> Result<[i32; 4], uiautomation::Error> {
    let r = e.get_bounding_rectangle()?;
    Ok([r.get_left(), r.get_top(), r.get_width(), r.get_height()])
}

fn walk(walker: &UITreeWalker, e: &UIElement, depth: usize, sample: &mut Vec<String>) -> usize {
    if sample.len() < 15 {
        let ct = e.get_control_type().map(|c| format!("{c:?}")).unwrap_or_else(|_| "?".into());
        sample.push(format!("{}{} '{}'", "  ".repeat(depth), ct, e.get_name().unwrap_or_default()));
    }
    let mut n = 1;
    if depth >= 6 {
        return n;
    }
    let mut child = walker.get_first_child(e).ok();
    while let Some(c) = child {
        n += walk(walker, &c, depth + 1, sample);
        child = walker.get_next_sibling(&c).ok();
    }
    n
}

// 弹出菜单可能是桌面的直接子元素，也可能挂在记事本窗口下；读取前几项名称作为“菜单已打开”的证据
fn menu_entries(automation: &UIAutomation, walker: &UITreeWalker, win: &UIElement) -> Option<Vec<String>> {
    let root = automation.get_root_element().ok()?;
    let mut menu = None;
    let mut child = walker.get_first_child(&root).ok();
    while let Some(c) = child {
        if c.get_control_type().ok() == Some(ControlType::Menu) {
            menu = Some(c);
            break;
        }
        child = walker.get_next_sibling(&c).ok();
    }
    let menu = match menu {
        Some(m) => m,
        None => automation.create_matcher().from(win.clone()).control_type(ControlType::Menu).timeout(0).find_first().ok()?,
    };
    let mut names = Vec::new();
    let mut item = walker.get_first_child(&menu).ok();
    while let Some(i) = item {
        if names.len() >= 5 {
            break;
        }
        names.push(i.get_name().unwrap_or_default());
        item = walker.get_next_sibling(&i).ok();
    }
    Some(names)
}

fn send_unicode(text: &str) -> u32 {
    let mut inputs = Vec::new();
    for unit in text.encode_utf16() {
        for flags in [KEYEVENTF_UNICODE, KEYEVENTF_UNICODE | KEYEVENTF_KEYUP] {
            inputs.push(INPUT {
                r#type: INPUT_KEYBOARD,
                Anonymous: INPUT_0 {
                    ki: KEYBDINPUT { wVk: VIRTUAL_KEY(0), wScan: unit, dwFlags: flags, time: 0, dwExtraInfo: 0 },
                },
            });
        }
    }
    unsafe { SendInput(&inputs, std::mem::size_of::<INPUT>() as i32) }
}
