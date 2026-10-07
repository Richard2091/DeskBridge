#![windows_subsystem = "windows"]

//! 测试画面：全屏显示在主屏上，左上角是时间戳方块和毫秒时钟。
//!
//! pattern [--scene static|text|motion] [--fps 60] [--seconds 60]
//! 按 Esc 或到时间后退出。

use stream_rs::{clock, stamp};
use windows::core::{w, PCWSTR};
use windows::Win32::Foundation::*;
use windows::Win32::Graphics::Dwm::DwmFlush;
use windows::Win32::Graphics::Gdi::*;
use windows::Win32::System::LibraryLoader::GetModuleHandleW;
use windows::Win32::UI::HiDpi::{SetProcessDpiAwarenessContext, DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2};
use windows::Win32::UI::Input::KeyboardAndMouse::VK_ESCAPE;
use windows::Win32::UI::WindowsAndMessaging::*;

#[derive(PartialEq, Clone, Copy)]
enum Scene {
    Static,
    Text,
    Motion,
}

extern "system" fn wndproc(hwnd: HWND, msg: u32, wp: WPARAM, lp: LPARAM) -> LRESULT {
    unsafe {
        match msg {
            WM_KEYDOWN if wp.0 == VK_ESCAPE.0 as usize => {
                PostQuitMessage(0);
                LRESULT(0)
            }
            WM_DESTROY => {
                PostQuitMessage(0);
                LRESULT(0)
            }
            WM_SETCURSOR => {
                SetCursor(None);
                LRESULT(1)
            }
            _ => DefWindowProcW(hwnd, msg, wp, lp),
        }
    }
}

fn opt<'a>(args: &'a [String], name: &str) -> Option<&'a str> {
    args.iter().position(|a| a == name).and_then(|i| args.get(i + 1)).map(String::as_str)
}

fn wide(s: &str) -> Vec<u16> {
    s.encode_utf16().collect()
}

fn rgb(r: u8, g: u8, b: u8) -> COLORREF {
    COLORREF(r as u32 | (g as u32) << 8 | (b as u32) << 16)
}

fn fill(dc: HDC, l: i32, t: i32, r: i32, b: i32, color: COLORREF) {
    unsafe {
        let br = CreateSolidBrush(color);
        FillRect(dc, &RECT { left: l, top: t, right: r, bottom: b }, br);
        let _ = DeleteObject(br.into());
    }
}

/// 建一块自上而下的 32 位 DIB，返回 DC、位图和像素指针。
fn dib(w: i32, h: i32) -> (HDC, HBITMAP, *mut u32) {
    unsafe {
        let dc = CreateCompatibleDC(None);
        let bi = BITMAPINFO {
            bmiHeader: BITMAPINFOHEADER {
                biSize: std::mem::size_of::<BITMAPINFOHEADER>() as u32,
                biWidth: w,
                biHeight: -h,
                biPlanes: 1,
                biBitCount: 32,
                biCompression: BI_RGB.0,
                ..Default::default()
            },
            ..Default::default()
        };
        let mut bits = std::ptr::null_mut();
        let bmp = CreateDIBSection(Some(dc), &bi, DIB_RGB_COLORS, &mut bits, None, 0).unwrap();
        SelectObject(dc, bmp.into());
        (dc, bmp, bits as *mut u32)
    }
}

fn main() {
    let args: Vec<String> = std::env::args().skip(1).collect();
    let scene = match opt(&args, "--scene").unwrap_or("motion") {
        "static" => Scene::Static,
        "text" => Scene::Text,
        _ => Scene::Motion,
    };
    let fps: f64 = opt(&args, "--fps").and_then(|s| s.parse().ok()).unwrap_or(60.0);
    let seconds: f64 = opt(&args, "--seconds").and_then(|s| s.parse().ok()).unwrap_or(60.0);

    unsafe {
        let _ = SetProcessDpiAwarenessContext(DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2);
        let mon = MonitorFromPoint(POINT { x: 0, y: 0 }, MONITOR_DEFAULTTOPRIMARY);
        let mut mi = MONITORINFO { cbSize: std::mem::size_of::<MONITORINFO>() as u32, ..Default::default() };
        let _ = GetMonitorInfoW(mon, &mut mi);
        let r = mi.rcMonitor;
        let (sw, sh) = (r.right - r.left, r.bottom - r.top);

        let inst = GetModuleHandleW(None).unwrap();
        let class = w!("DeskBridgePattern");
        let wc = WNDCLASSW {
            lpfnWndProc: Some(wndproc),
            hInstance: inst.into(),
            lpszClassName: class,
            hbrBackground: HBRUSH(GetStockObject(BLACK_BRUSH).0),
            ..Default::default()
        };
        RegisterClassW(&wc);
        let hwnd = CreateWindowExW(
            WS_EX_TOPMOST | WS_EX_TOOLWINDOW,
            class,
            w!("DeskBridge 测试画面"),
            WS_POPUP | WS_VISIBLE,
            r.left,
            r.top,
            sw,
            sh,
            None,
            None,
            Some(inst.into()),
            None,
        )
        .unwrap();
        let _ = SetForegroundWindow(hwnd);

        if scene == Scene::Motion {
            if let Err(e) = motion::run(hwnd, sw, sh, seconds) {
                let msg = wide(&format!("高运动场景出错：{e}\0"));
                MessageBoxW(None, PCWSTR(msg.as_ptr()), w!("DeskBridge 测试画面"), MB_OK);
            }
            let _ = DestroyWindow(hwnd);
            return;
        }
        let wdc = GetDC(Some(hwnd));
        // 背缓冲
        let (bdc, _bb, _) = dib(sw, sh);
        let bg = if scene == Scene::Text { rgb(255, 255, 255) } else { rgb(90, 110, 130) };
        fill(bdc, 0, 0, sw, sh, bg);

        let font_big = CreateFontW(110, 0, 0, 0, FW_BOLD.0 as i32, 0, 0, 0, DEFAULT_CHARSET, OUT_DEFAULT_PRECIS, CLIP_DEFAULT_PRECIS, ANTIALIASED_QUALITY, (FF_MODERN.0 | FIXED_PITCH.0) as u32, w!("Consolas"));
        let font_text = CreateFontW(30, 0, 0, 0, FW_NORMAL.0 as i32, 0, 0, 0, DEFAULT_CHARSET, OUT_DEFAULT_PRECIS, CLIP_DEFAULT_PRECIS, CLEARTYPE_QUALITY, 0, w!("Microsoft YaHei"));
        SetBkMode(bdc, TRANSPARENT);

        let header_h = stamp::BLOCK as i32 + 130;
        let clock_w = 760.max(stamp::width() as i32);
        let interval_ms = 1000.0 / fps;
        let start = clock::now_ms();
        let mut last_draw = f64::MIN;
        let mut frame: i64 = 0;
        let mut msg = MSG::default();
        'main: loop {
            while PeekMessageW(&mut msg, None, 0, 0, PM_REMOVE).as_bool() {
                if msg.message == WM_QUIT {
                    break 'main;
                }
                let _ = TranslateMessage(&msg);
                DispatchMessageW(&msg);
            }
            let now = clock::now_ms();
            if now - start > seconds * 1000.0 {
                break;
            }
            if now - last_draw < interval_ms * 0.9 {
                let _ = DwmFlush();
                continue;
            }
            last_draw = now;
            frame += 1;

            // 场景内容
            let full = match scene {
                Scene::Static => false,
                Scene::Text => {
                    fill(bdc, 0, header_h, sw, sh, rgb(255, 255, 255));
                    SelectObject(bdc, font_text.into());
                    SetTextColor(bdc, rgb(20, 20, 20));
                    let line_h = 40;
                    let offset = (frame * 2) as i32;
                    let first = offset / line_h;
                    let mut y = header_h + 10 - offset % line_h;
                    let mut n = first;
                    while y < sh {
                        if y > header_h - line_h {
                            let s = wide(&format!(
                                "第 {n} 行：DeskBridge 远程画面测试，滚动文字用于观察清晰度和码率。The quick brown fox jumps over the lazy dog 0123456789"
                            ));
                            let _ = TextOutW(bdc, 40, y, &s);
                        }
                        y += line_h;
                        n += 1;
                    }
                    // 文字可能画进页眉，下面会重画页眉
                    true
                }
                Scene::Motion => unreachable!(),
            };

            // 页眉：时间戳方块和时钟
            fill(bdc, 0, 0, clock_w, header_h, rgb(0, 0, 0));
            let tick = clock::now_tick();
            let bits = stamp::encode(tick as u32);
            for (i, b) in bits.iter().enumerate() {
                if *b {
                    let x = i as i32 * stamp::BLOCK as i32;
                    fill(bdc, x, 0, x + stamp::BLOCK as i32, stamp::BLOCK as i32, rgb(255, 255, 255));
                }
            }
            SelectObject(bdc, font_big.into());
            SetTextColor(bdc, rgb(255, 255, 255));
            let secs = (tick as f64 / 10_000.0) % 1000.0;
            let _ = TextOutW(bdc, 10, stamp::BLOCK as i32 + 10, &wide(&format!("{secs:07.3}")));

            if full || frame == 1 {
                let _ = BitBlt(wdc, 0, 0, sw, sh, Some(bdc), 0, 0, SRCCOPY);
            } else {
                let _ = BitBlt(wdc, 0, 0, clock_w, header_h, Some(bdc), 0, 0, SRCCOPY);
            }
            let _ = GdiFlush();
            let _ = DwmFlush();
        }
        ReleaseDC(Some(hwnd), wdc);
        let _ = DestroyWindow(hwnd);
    }
}

/// 高运动场景：用 D3D11 交换链在显卡上绘制，按显示器刷新率呈现。
/// 时钟用七段数码管样式的矩形拼出，不依赖字体。
mod motion {
    use super::stamp;
    use stream_rs::clock;
    use windows::core::{Interface, Result};
    use windows::Win32::Foundation::{HMODULE, HWND, RECT};
    use windows::Win32::Graphics::Direct3D::*;
    use windows::Win32::Graphics::Direct3D11::*;
    use windows::Win32::Graphics::Dxgi::Common::*;
    use windows::Win32::Graphics::Dxgi::*;
    use windows::Win32::UI::WindowsAndMessaging::*;

    /// 32 像素随机色块叠加渐变，最难压缩的情况。
    fn noise(w: usize, h: usize) -> Vec<u32> {
        let mut seed = 12345u32;
        let cx = w.div_ceil(32);
        let cy = h.div_ceil(32);
        let cells: Vec<u32> = (0..cx * cy)
            .map(|_| {
                seed = seed.wrapping_mul(1664525).wrapping_add(1013904223);
                seed >> 8
            })
            .collect();
        let mut px = vec![0u32; w * h];
        for y in 0..h {
            for x in 0..w {
                let c = cells[(y / 32) * cx + x / 32];
                let g = (x * 255 / w) as u32;
                let r = ((c >> 16) & 0xFF) / 2 + g / 2;
                let gg = ((c >> 8) & 0xFF) / 2 + (255 - g) / 2;
                let b = ((c & 0xFF) / 2 + (y * 127 / h) as u32).min(255);
                px[y * w + x] = 0xFF00_0000 | (r << 16) | (gg << 8) | b;
            }
        }
        px
    }

    fn rect(l: i32, t: i32, r: i32, b: i32) -> RECT {
        RECT { left: l, top: t, right: r, bottom: b }
    }

    /// 七段数码管：每个字符的白色矩形。
    fn digits(text: &str, x0: i32, y0: i32, out: &mut Vec<RECT>) {
        const W: i32 = 50;
        const H: i32 = 90;
        const T: i32 = 10;
        // a b c d e f g
        const SEG: [u8; 10] = [0x7E, 0x30, 0x6D, 0x79, 0x33, 0x5B, 0x5F, 0x70, 0x7F, 0x7B];
        let mut x = x0;
        for ch in text.chars() {
            if ch == '.' {
                out.push(rect(x, y0 + H - T, x + T, y0 + H));
                x += 2 * T;
                continue;
            }
            let Some(d) = ch.to_digit(10) else { continue };
            let m = SEG[d as usize];
            let segs = [
                rect(x, y0, x + W, y0 + T),
                rect(x + W - T, y0, x + W, y0 + H / 2),
                rect(x + W - T, y0 + H / 2, x + W, y0 + H),
                rect(x, y0 + H - T, x + W, y0 + H),
                rect(x, y0 + H / 2, x + T, y0 + H),
                rect(x, y0, x + T, y0 + H / 2),
                rect(x, y0 + H / 2 - T / 2, x + W, y0 + H / 2 + T / 2),
            ];
            for (i, r) in segs.iter().enumerate() {
                if m & (0x40 >> i) != 0 {
                    out.push(*r);
                }
            }
            x += W + 20;
        }
    }

    pub unsafe fn run(hwnd: HWND, sw: i32, sh: i32, seconds: f64) -> Result<()> {
        let (w, h) = (sw as u32, sh as u32);
        let mut device = None;
        let mut ctx = None;
        D3D11CreateDevice(
            None,
            D3D_DRIVER_TYPE_HARDWARE,
            HMODULE::default(),
            D3D11_CREATE_DEVICE_BGRA_SUPPORT,
            Some(&[D3D_FEATURE_LEVEL_11_1, D3D_FEATURE_LEVEL_11_0]),
            D3D11_SDK_VERSION,
            Some(&mut device),
            None,
            Some(&mut ctx),
        )?;
        let device: ID3D11Device = device.unwrap();
        // 最多排队 1 帧，让绘制时刻尽量接近上屏时刻
        device.cast::<IDXGIDevice1>()?.SetMaximumFrameLatency(1)?;
        let ctx: ID3D11DeviceContext1 = ctx.unwrap().cast()?;
        let factory: IDXGIFactory2 = CreateDXGIFactory1()?;
        let desc = DXGI_SWAP_CHAIN_DESC1 {
            Width: w,
            Height: h,
            Format: DXGI_FORMAT_B8G8R8A8_UNORM,
            SampleDesc: DXGI_SAMPLE_DESC { Count: 1, Quality: 0 },
            BufferUsage: DXGI_USAGE_RENDER_TARGET_OUTPUT,
            BufferCount: 2,
            Scaling: DXGI_SCALING_STRETCH,
            SwapEffect: DXGI_SWAP_EFFECT_FLIP_DISCARD,
            AlphaMode: DXGI_ALPHA_MODE_UNSPECIFIED,
            ..Default::default()
        };
        let sc = factory.CreateSwapChainForHwnd(&device, hwnd, &desc, None, None)?;

        let px = noise(w as usize, h as usize);
        let tdesc = D3D11_TEXTURE2D_DESC {
            Width: w,
            Height: h,
            MipLevels: 1,
            ArraySize: 1,
            Format: DXGI_FORMAT_B8G8R8A8_UNORM,
            SampleDesc: DXGI_SAMPLE_DESC { Count: 1, Quality: 0 },
            Usage: D3D11_USAGE_DEFAULT,
            BindFlags: 0,
            CPUAccessFlags: 0,
            MiscFlags: 0,
        };
        let init = D3D11_SUBRESOURCE_DATA { pSysMem: px.as_ptr() as _, SysMemPitch: w * 4, SysMemSlicePitch: 0 };
        let mut noise_tex = None;
        device.CreateTexture2D(&tdesc, Some(&init), Some(&mut noise_tex))?;
        let noise_tex = noise_tex.unwrap();
        drop(px);

        let header_h = stamp::BLOCK as i32 + 130;
        let clock_w = 760.max(stamp::width() as i32);
        let colors: [[f32; 4]; 6] = [
            [1.0, 0.25, 0.25, 1.0],
            [0.25, 1.0, 0.25, 1.0],
            [0.25, 0.25, 1.0, 1.0],
            [1.0, 1.0, 0.25, 1.0],
            [1.0, 0.25, 1.0, 1.0],
            [0.25, 1.0, 1.0, 1.0],
        ];
        let white = [1.0f32, 1.0, 1.0, 1.0];
        let black = [0.0f32, 0.0, 0.0, 1.0];
        let start = clock::now_ms();
        let mut frame: i64 = 0;
        let mut msg = MSG::default();
        let mut rects = Vec::new();
        loop {
            while PeekMessageW(&mut msg, None, 0, 0, PM_REMOVE).as_bool() {
                if msg.message == WM_QUIT {
                    return Ok(());
                }
                let _ = TranslateMessage(&msg);
                DispatchMessageW(&msg);
            }
            if clock::now_ms() - start > seconds * 1000.0 {
                return Ok(());
            }
            frame += 1;
            let back: ID3D11Texture2D = sc.GetBuffer(0)?;
            let mut rtv = None;
            device.CreateRenderTargetView(&back, None, Some(&mut rtv))?;
            let rtv = rtv.unwrap();

            // 底图横向滚动
            let o = ((frame * 8) % w as i64) as u32;
            let b1 = D3D11_BOX { left: o, top: 0, front: 0, right: w, bottom: h, back: 1 };
            ctx.CopySubresourceRegion(&back, 0, 0, 0, 0, &noise_tex, 0, Some(&b1));
            if o > 0 {
                let b2 = D3D11_BOX { left: 0, top: 0, front: 0, right: o, bottom: h, back: 1 };
                ctx.CopySubresourceRegion(&back, 0, w - o, 0, 0, &noise_tex, 0, Some(&b2));
            }
            // 反弹的色块
            for (k, c) in colors.iter().enumerate() {
                let k = k as i64;
                let size = 160 + 40 * k as i32;
                let px = ((frame * (7 + 3 * k)) % (2 * (sw - size) as i64)) as i32;
                let py = ((frame * (5 + 2 * k)) % (2 * (sh - header_h - size) as i64)) as i32;
                let x = if px > sw - size { 2 * (sw - size) - px } else { px };
                let y = header_h + if py > sh - header_h - size { 2 * (sh - header_h - size) - py } else { py };
                ctx.ClearView(&rtv, c, Some(&[rect(x, y, x + size, y + size)]));
            }
            // 页眉：时间戳方块和时钟
            ctx.ClearView(&rtv, &black, Some(&[rect(0, 0, clock_w, header_h)]));
            let tick = clock::now_tick();
            rects.clear();
            for (i, b) in stamp::encode(tick as u32).iter().enumerate() {
                if *b {
                    let x = i as i32 * stamp::BLOCK as i32;
                    rects.push(rect(x, 0, x + stamp::BLOCK as i32, stamp::BLOCK as i32));
                }
            }
            let secs = (tick as f64 / 10_000.0) % 1000.0;
            digits(&format!("{secs:07.3}"), 10, stamp::BLOCK as i32 + 20, &mut rects);
            ctx.ClearView(&rtv, &white, Some(&rects));
            sc.Present(1, DXGI_PRESENT(0)).ok()?;
        }
    }
}
