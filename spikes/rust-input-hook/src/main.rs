// 技术验证：普通权限的底层输入钩子，在不同前台窗口下能看到什么。
//
// 回答的问题（ADR-0005）：
// 1. 前台是管理员权限窗口时，普通权限的 WH_KEYBOARD_LL / WH_MOUSE_LL 还能收到物理输入吗？
// 2. 此时普通权限进程的 SendInput 会被拦截吗？钩子能看到注入的输入吗？
// 3. GetLastInputInfo（系统最后输入时间）能否作为补充？它会被注入的输入更新吗？
// 4. UAC 确认框等安全桌面出现时，以上各项的表现。
// 5. 第二轮：全局热键（RegisterHotKey，Ctrl+Alt+F12）和 Raw Input（RIDEV_INPUTSINK）在管理员窗口前台时能否收到。
//
// 隐私：只统计次数，不记录按了哪个键；前台窗口只记录程序文件名，不记录标题。

use std::{
    fs::File,
    io::Write,
    sync::atomic::{AtomicU32, Ordering::Relaxed},
    time::Instant,
};

use windows::{
    core::{w, PWSTR},
    Win32::{
        Foundation::{CloseHandle, GetLastError, HANDLE, LPARAM, LRESULT, WPARAM},
        Security::{
            GetSidSubAuthority, GetSidSubAuthorityCount, GetTokenInformation, TokenIntegrityLevel,
            TOKEN_MANDATORY_LABEL, TOKEN_QUERY,
        },
        System::{
            StationsAndDesktops::{CloseDesktop, GetUserObjectInformationW, OpenInputDesktop, DESKTOP_CONTROL_FLAGS, DESKTOP_READOBJECTS, UOI_NAME},
            SystemInformation::GetTickCount,
            Threading::{OpenProcess, OpenProcessToken, QueryFullProcessImageNameW, PROCESS_NAME_WIN32, PROCESS_QUERY_LIMITED_INFORMATION},
        },
        UI::{
            Input::{
                GetRawInputData, RegisterRawInputDevices, HRAWINPUT, RAWINPUTDEVICE, RAWINPUTHEADER, RIDEV_INPUTSINK,
                RID_HEADER, RIM_TYPEKEYBOARD, RIM_TYPEMOUSE,
                KeyboardAndMouse::{
                    GetLastInputInfo, RegisterHotKey, SendInput, INPUT, INPUT_0, INPUT_MOUSE, LASTINPUTINFO, MOD_ALT,
                    MOD_CONTROL, MOD_NOREPEAT, MOUSEEVENTF_MOVE, MOUSEINPUT, VK_F12,
                },
            },
            WindowsAndMessaging::{
                CallNextHookEx, CreateWindowExW, DispatchMessageW, GetForegroundWindow, GetMessageW,
                GetWindowThreadProcessId, KillTimer, SetTimer, SetWindowsHookExW, UnhookWindowsHookEx, KBDLLHOOKSTRUCT,
                LLKHF_INJECTED, LLMHF_INJECTED, MSG, MSLLHOOKSTRUCT, WH_KEYBOARD_LL, WH_MOUSE_LL, WINDOW_EX_STYLE,
                WINDOW_STYLE, WM_HOTKEY, WM_INPUT, WM_TIMER,
            },
        },
    },
};

static KBD_PHYS: AtomicU32 = AtomicU32::new(0);
static KBD_INJ: AtomicU32 = AtomicU32::new(0);
static MOUSE_PHYS: AtomicU32 = AtomicU32::new(0);
static MOUSE_INJ: AtomicU32 = AtomicU32::new(0);
// Raw Input 没有“注入”标记；SendInput 产生的事件 hDevice 为空，据此区分
static RAW_KBD_PHYS: AtomicU32 = AtomicU32::new(0);
static RAW_KBD_INJ: AtomicU32 = AtomicU32::new(0);
static RAW_MOUSE_PHYS: AtomicU32 = AtomicU32::new(0);
static RAW_MOUSE_INJ: AtomicU32 = AtomicU32::new(0);
static HOTKEY: AtomicU32 = AtomicU32::new(0);

unsafe extern "system" fn kbd_proc(code: i32, wparam: WPARAM, lparam: LPARAM) -> LRESULT {
    if code >= 0 {
        let k = &*(lparam.0 as *const KBDLLHOOKSTRUCT);
        if (k.flags & LLKHF_INJECTED).0 != 0 { &KBD_INJ } else { &KBD_PHYS }.fetch_add(1, Relaxed);
    }
    CallNextHookEx(None, code, wparam, lparam)
}

unsafe extern "system" fn mouse_proc(code: i32, wparam: WPARAM, lparam: LPARAM) -> LRESULT {
    if code >= 0 {
        let m = &*(lparam.0 as *const MSLLHOOKSTRUCT);
        if m.flags & LLMHF_INJECTED != 0 { &MOUSE_INJ } else { &MOUSE_PHYS }.fetch_add(1, Relaxed);
    }
    CallNextHookEx(None, code, wparam, lparam)
}

/// 前台窗口所属程序的文件名和完整性级别（Medium / High / System ...）。
fn foreground_info() -> (String, String) {
    unsafe {
        let hwnd = GetForegroundWindow();
        if hwnd.0.is_null() {
            return ("(无前台窗口)".into(), "-".into());
        }
        let mut pid = 0u32;
        GetWindowThreadProcessId(hwnd, Some(&mut pid));
        let Ok(proc) = OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION, false, pid) else {
            return (format!("pid {pid}"), format!("无法打开进程: {:?}", GetLastError()));
        };
        let mut buf = [0u16; 512];
        let mut len = buf.len() as u32;
        let name = match QueryFullProcessImageNameW(proc, PROCESS_NAME_WIN32, PWSTR(buf.as_mut_ptr()), &mut len) {
            Ok(()) => String::from_utf16_lossy(&buf[..len as usize]).rsplit('\\').next().unwrap_or("").to_string(),
            Err(e) => format!("pid {pid} ({e})"),
        };
        let level = integrity_level(proc);
        let _ = CloseHandle(proc);
        (name, level)
    }
}

unsafe fn integrity_level(proc: HANDLE) -> String {
    let mut token = HANDLE::default();
    if let Err(e) = OpenProcessToken(proc, TOKEN_QUERY, &mut token) {
        return format!("无法读取令牌: {e}");
    }
    let mut buf = [0u8; 256];
    let mut ret = 0u32;
    let r = GetTokenInformation(token, TokenIntegrityLevel, Some(buf.as_mut_ptr().cast()), buf.len() as u32, &mut ret);
    let _ = CloseHandle(token);
    if let Err(e) = r {
        return format!("无法读取级别: {e}");
    }
    let label = &*(buf.as_ptr() as *const TOKEN_MANDATORY_LABEL);
    let sid = label.Label.Sid;
    let n = *GetSidSubAuthorityCount(sid);
    let rid = *GetSidSubAuthority(sid, (n - 1) as u32);
    match rid {
        0x0000 => "Untrusted",
        0x1000 => "Low",
        0x2000 => "Medium",
        0x2100 => "Medium+",
        0x3000 => "High（管理员）",
        0x4000 => "System",
        _ => return format!("0x{rid:x}"),
    }
    .into()
}

/// 当前接收输入的桌面名称。普通桌面是 Default；UAC 确认框、锁屏是安全桌面（通常无权打开）。
fn input_desktop() -> String {
    unsafe {
        match OpenInputDesktop(DESKTOP_CONTROL_FLAGS(0), false, DESKTOP_READOBJECTS) {
            Ok(desk) => {
                let mut buf = [0u16; 128];
                let mut needed = 0u32;
                let r = GetUserObjectInformationW(
                    HANDLE(desk.0),
                    UOI_NAME,
                    Some(buf.as_mut_ptr().cast()),
                    (buf.len() * 2) as u32,
                    Some(&mut needed),
                );
                let _ = CloseDesktop(desk);
                match r {
                    Ok(()) => String::from_utf16_lossy(&buf).trim_end_matches('\0').to_string(),
                    Err(e) => format!("无法读取名称: {e}"),
                }
            }
            Err(e) => format!("无法打开（可能是安全桌面）: {e}"),
        }
    }
}

unsafe fn count_raw_input(h: HRAWINPUT) {
    let mut header = RAWINPUTHEADER::default();
    let mut size = std::mem::size_of::<RAWINPUTHEADER>() as u32;
    let n = GetRawInputData(h, RID_HEADER, Some((&mut header as *mut RAWINPUTHEADER).cast()), &mut size, size);
    if n == u32::MAX {
        return;
    }
    let injected = header.hDevice.0.is_null();
    let counter = match (header.dwType, injected) {
        (t, false) if t == RIM_TYPEKEYBOARD.0 => &RAW_KBD_PHYS,
        (t, true) if t == RIM_TYPEKEYBOARD.0 => &RAW_KBD_INJ,
        (t, false) if t == RIM_TYPEMOUSE.0 => &RAW_MOUSE_PHYS,
        (t, true) if t == RIM_TYPEMOUSE.0 => &RAW_MOUSE_INJ,
        _ => return,
    };
    counter.fetch_add(1, Relaxed);
}

fn last_input_tick() -> u32 {
    let mut li = LASTINPUTINFO { cbSize: std::mem::size_of::<LASTINPUTINFO>() as u32, dwTime: 0 };
    unsafe {
        let _ = GetLastInputInfo(&mut li);
    }
    li.dwTime
}

/// 注入一次几乎看不出的鼠标移动（右移 1 像素再移回）。返回 SendInput 的返回值和错误码。
fn inject_probe() -> (u32, u32) {
    let mv = |dx: i32| INPUT {
        r#type: INPUT_MOUSE,
        Anonymous: INPUT_0 { mi: MOUSEINPUT { dx, dy: 0, mouseData: 0, dwFlags: MOUSEEVENTF_MOVE, time: 0, dwExtraInfo: 0 } },
    };
    let inputs = [mv(1), mv(-1)];
    unsafe {
        let n = SendInput(&inputs, std::mem::size_of::<INPUT>() as i32);
        (n, GetLastError().0)
    }
}

fn main() {
    let args: Vec<String> = std::env::args().collect();
    let seconds: u32 = args.get(1).and_then(|s| s.parse().ok()).unwrap_or(120);
    let log_path = args.get(2).cloned().unwrap_or_else(|| "hook.log".into());
    let mut log = File::create(&log_path).expect("无法创建日志文件");

    let (self_name, self_level) = unsafe {
        let me = windows::Win32::System::Threading::GetCurrentProcess();
        ("本程序".to_string(), integrity_level(me))
    };
    let header = format!("# {self_name} 的完整性级别: {self_level}；运行 {seconds} 秒；每 5 秒注入一次 1 像素鼠标移动");
    println!("{header}");
    writeln!(log, "{header}").ok();
    writeln!(log, "秒\t前台程序\t前台级别\t输入桌面\t物理键盘\t注入键盘\t物理鼠标\t注入鼠标\tRaw物理键盘\tRaw注入键盘\tRaw物理鼠标\tRaw注入鼠标\t热键\t系统最后输入(距今ms)\t注入探测").ok();

    unsafe {
        let kh = SetWindowsHookExW(WH_KEYBOARD_LL, Some(kbd_proc), None, 0).expect("键盘钩子安装失败");
        let mh = SetWindowsHookExW(WH_MOUSE_LL, Some(mouse_proc), None, 0).expect("鼠标钩子安装失败");
        let timer = SetTimer(None, 0, 1000, None);

        // 隐藏窗口，用来接收 Raw Input
        let hwnd = CreateWindowExW(WINDOW_EX_STYLE(0), w!("STATIC"), w!("rust-input-hook"), WINDOW_STYLE(0), 0, 0, 0, 0, None, None, None, None)
            .expect("创建窗口失败");
        let devices = [
            RAWINPUTDEVICE { usUsagePage: 1, usUsage: 6, dwFlags: RIDEV_INPUTSINK, hwndTarget: hwnd },
            RAWINPUTDEVICE { usUsagePage: 1, usUsage: 2, dwFlags: RIDEV_INPUTSINK, hwndTarget: hwnd },
        ];
        if let Err(e) = RegisterRawInputDevices(&devices, std::mem::size_of::<RAWINPUTDEVICE>() as u32) {
            writeln!(log, "# Raw Input 注册失败: {e}").ok();
        }
        // 线程热键：按下时 WM_HOTKEY 发到本线程的消息队列
        if let Err(e) = RegisterHotKey(None, 1, MOD_CONTROL | MOD_ALT | MOD_NOREPEAT, VK_F12.0 as u32) {
            writeln!(log, "# 热键 Ctrl+Alt+F12 注册失败: {e}").ok();
        }

        let start = Instant::now();
        let mut msg = MSG::default();
        let mut sec = 0u32;
        while GetMessageW(&mut msg, None, 0, 0).as_bool() {
            if msg.message == WM_HOTKEY {
                HOTKEY.fetch_add(1, Relaxed);
            } else if msg.message == WM_INPUT {
                count_raw_input(HRAWINPUT(msg.lParam.0 as _));
            } else if msg.message == WM_TIMER {
                sec = start.elapsed().as_secs() as u32;
                // 先读取计数再注入：本秒的计数反映上一秒注入的结果
                let kp = KBD_PHYS.swap(0, Relaxed);
                let ki = KBD_INJ.swap(0, Relaxed);
                let mp = MOUSE_PHYS.swap(0, Relaxed);
                let mi = MOUSE_INJ.swap(0, Relaxed);
                let rkp = RAW_KBD_PHYS.swap(0, Relaxed);
                let rki = RAW_KBD_INJ.swap(0, Relaxed);
                let rmp = RAW_MOUSE_PHYS.swap(0, Relaxed);
                let rmi = RAW_MOUSE_INJ.swap(0, Relaxed);
                let hk = HOTKEY.swap(0, Relaxed);
                let idle = GetTickCount().wrapping_sub(last_input_tick());
                let (fg, level) = foreground_info();
                let desk = input_desktop();
                let probe = if sec % 5 == 0 {
                    let (n, err) = inject_probe();
                    format!("SendInput={n} err={err}")
                } else {
                    String::new()
                };
                let line = format!("{sec}\t{fg}\t{level}\t{desk}\t{kp}\t{ki}\t{mp}\t{mi}\t{rkp}\t{rki}\t{rmp}\t{rmi}\t{hk}\t{idle}\t{probe}");
                println!("{line}");
                writeln!(log, "{line}").ok();
                log.flush().ok();
                if sec >= seconds {
                    break;
                }
            }
            DispatchMessageW(&msg);
        }
        let _ = KillTimer(None, timer);
        let _ = UnhookWindowsHookEx(kh);
        let _ = UnhookWindowsHookEx(mh);
        writeln!(log, "# 结束，共 {sec} 秒").ok();
    }
}
