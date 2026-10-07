//! 采集和编码测量程序（任务 24.2）。
//!
//! capenc list
//! capenc run [--output A:O] [--width W] [--fps F] [--kbps K] [--encoder hw|sw]
//!            [--seconds S] [--warmup S] [--out 文件.h264] [--stats 文件.jsonl] [--no-stamp]
//!            [--pattern static|text|motion]   同时启动测试画面，结束时关闭
//!            [--eager]   每次画面更新都取帧（默认只在发送时刻前取最新一帧）
//! capenc verify --in 文件.h264 --stats 文件.jsonl [--png 文件.png]
//!
//! 结果以 JSON 打印到标准输出。

use serde_json::{json, Value};
use std::collections::HashMap;
use std::fs::File;
use std::io::{BufRead, BufReader, Read, Write};
use stream_rs::capture::Capturer;
use stream_rs::clock::{self, qpc, to_ms};
use stream_rs::convert::{Converter, StampReader};
use stream_rs::encoder::{self, Encoder, Kind};
use stream_rs::stats::{mean, round2, summary};
use stream_rs::{d3d, decoder, metrics, mf, stamp};
use windows::Win32::Graphics::Dxgi::DXGI_ERROR_ACCESS_LOST;
use windows::Win32::UI::HiDpi::{SetProcessDpiAwarenessContext, DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2};

type Res<T> = Result<T, Box<dyn std::error::Error>>;

fn main() {
    let args: Vec<String> = std::env::args().skip(1).collect();
    let r = match args.first().map(String::as_str) {
        Some("list") => list(),
        Some("run") => run(&args[1..]),
        Some("verify") => verify(&args[1..]),
        _ => Err("用法：capenc list | run ... | verify ...".into()),
    };
    match r {
        Ok(v) => println!("{}", serde_json::to_string_pretty(&v).unwrap()),
        Err(e) => {
            println!("{}", json!({ "error": e.to_string() }));
            std::process::exit(1);
        }
    }
}

fn opt<'a>(args: &'a [String], name: &str) -> Option<&'a str> {
    args.iter().position(|a| a == name).and_then(|i| args.get(i + 1)).map(String::as_str)
}

fn opt_num<T: std::str::FromStr>(args: &[String], name: &str, default: T) -> Res<T> {
    match opt(args, name) {
        Some(s) => s.parse().map_err(|_| format!("{name} 的值无效：{s}").into()),
        None => Ok(default),
    }
}

fn init() -> Res<()> {
    unsafe {
        let _ = SetProcessDpiAwarenessContext(DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2);
    }
    mf::startup()?;
    Ok(())
}

fn list() -> Res<Value> {
    init()?;
    let mut adapters = Vec::new();
    for a in d3d::adapters()? {
        let outs: Vec<Value> = d3d::outputs(&a)?
            .iter()
            .map(|o| {
                json!({
                    "id": format!("{}:{}", o.adapter_index, o.output_index),
                    "device": o.device_name,
                    "rect": [o.rect.left, o.rect.top, o.rect.right, o.rect.bottom],
                    "attached": o.attached,
                })
            })
            .collect();
        adapters.push(json!({
            "index": a.index,
            "name": a.name,
            "vendor": format!("0x{:04X}", a.vendor_id),
            "software": a.software,
            "outputs": outs,
        }));
    }
    let encoders: Vec<Value> = encoder::list()?
        .iter()
        .map(|e| json!({ "name": e.name, "hardware": e.hardware }))
        .collect();
    Ok(json!({ "adapters": adapters, "h264_encoders": encoders, "cpus": metrics::cpu_count() }))
}

/// 选择显示输出：给出 A:O 时按编号，否则取包含坐标 (0,0) 的主屏。
fn pick_output(spec: Option<&str>) -> Res<(d3d::Adapter, d3d::Output)> {
    for a in d3d::adapters()? {
        for o in d3d::outputs(&a)? {
            let id = format!("{}:{}", o.adapter_index, o.output_index);
            let hit = match spec {
                Some(s) => s == id,
                None => o.attached && o.rect.left == 0 && o.rect.top == 0,
            };
            if hit {
                return Ok((a, o));
            }
        }
    }
    Err(format!("找不到显示输出 {spec:?}").into())
}

struct KillOnDrop(std::process::Child);

impl Drop for KillOnDrop {
    fn drop(&mut self) {
        let _ = self.0.kill();
        let _ = self.0.wait();
    }
}

struct Pending {
    t1: i64,
    present: i64,
    accumulated: u32,
}

struct FrameRec {
    t0_ms: Option<f64>,
    stamp: Option<u32>,
    present_ms: Option<f64>,
    t1_ms: f64,
    t_in_ms: f64,
    accumulated: u32,
}

/// 唤醒并保持显示器常亮。显示器因空闲关闭后，桌面采集拿不到新帧。
fn keep_display_on() {
    use windows::Win32::System::Power::{SetThreadExecutionState, ES_CONTINUOUS, ES_DISPLAY_REQUIRED, ES_SYSTEM_REQUIRED};
    use windows::Win32::System::SystemInformation::GetTickCount;
    use windows::Win32::UI::Input::KeyboardAndMouse::{
        GetLastInputInfo, SendInput, INPUT, INPUT_0, INPUT_MOUSE, LASTINPUTINFO, MOUSEEVENTF_MOVE, MOUSEINPUT,
    };
    unsafe {
        let mut last = LASTINPUTINFO { cbSize: std::mem::size_of::<LASTINPUTINFO>() as u32, dwTime: 0 };
        let idle_ms = if GetLastInputInfo(&mut last).as_bool() { GetTickCount().wrapping_sub(last.dwTime) } else { 0 };
        SetThreadExecutionState(ES_CONTINUOUS | ES_DISPLAY_REQUIRED | ES_SYSTEM_REQUIRED);
        // 位移为 0 的鼠标移动，只用来唤醒已经关闭的显示器
        let input = INPUT {
            r#type: INPUT_MOUSE,
            Anonymous: INPUT_0 { mi: MOUSEINPUT { dwFlags: MOUSEEVENTF_MOVE, ..Default::default() } },
        };
        SendInput(&[input], std::mem::size_of::<INPUT>() as i32);
        // 显示器可能已关闭，刚唤醒时会反复切换模式，采集不断失效，等它稳定
        if idle_ms > 60_000 {
            std::thread::sleep(std::time::Duration::from_secs(8));
        }
    }
}

fn run(args: &[String]) -> Res<Value> {
    init()?;
    keep_display_on();
    let fps: u32 = opt_num(args, "--fps", 30)?;
    let seconds: f64 = opt_num(args, "--seconds", 20.0)?;
    let warmup: f64 = opt_num(args, "--warmup", 5.0)?;
    let kbps: u32 = opt_num(args, "--kbps", 4000)?;
    let kind = match opt(args, "--encoder").unwrap_or("hw") {
        "hw" => Kind::Hardware,
        "sw" => Kind::Software,
        s => return Err(format!("--encoder 只能是 hw 或 sw：{s}").into()),
    };
    let use_stamp = !args.iter().any(|a| a == "--no-stamp");
    let eager = args.iter().any(|a| a == "--eager");

    // 测试画面和本程序在同一目录
    let pattern = match opt(args, "--pattern") {
        Some(scene) => {
            let exe = std::env::current_exe()?.with_file_name("pattern.exe");
            let total = opt_num(args, "--seconds", 20.0)? + opt_num(args, "--warmup", 5.0)? + 10.0;
            let child = std::process::Command::new(exe)
                .args(["--scene", scene, "--seconds", &total.to_string()])
                .spawn()?;
            std::thread::sleep(std::time::Duration::from_millis(1500));
            Some(KillOnDrop(child))
        }
        None => None,
    };

    let (adapter, output) = pick_output(opt(args, "--output"))?;
    let (device, ctx) = d3d::create_device(&adapter.adapter)?;
    let mut cap = Capturer::new(&device, &output.output)?;
    let (src_w, src_h) = (cap.width, cap.height);
    let out_w: u32 = opt_num(args, "--width", src_w)? & !1;
    let out_h = ((src_h as u64 * out_w as u64 / src_w as u64) as u32) & !1;

    let mut conv = Converter::new(&device, &ctx, src_w, src_h, out_w, out_h, 4)?;
    let stamp_reader = if use_stamp { StampReader::new(&device, src_w)? } else { None };
    let cfg = encoder::Config { width: out_w, height: out_h, fps, bitrate: kbps * 1000, kind, luid: adapter.luid };
    let t_enc_init = qpc();
    let mut enc = Encoder::new(&device, &ctx, &cfg)?;
    let enc_init_ms = to_ms(qpc() - t_enc_init);

    let mut out_file = opt(args, "--out").map(File::create).transpose()?;
    let mut stats_file = opt(args, "--stats").map(File::create).transpose()?;
    let config = json!({
        "adapter": adapter.name,
        "output": format!("{}:{}", output.adapter_index, output.output_index),
        "src": [src_w, src_h],
        "refresh_hz": round2(cap.refresh_hz),
        "capture_format": cap.format.0,
        "out": [out_w, out_h],
        "fps": fps,
        "kbps": kbps,
        "encoder": enc.name,
        "hardware": enc.hardware,
        "async": enc.is_async,
        "seconds": seconds,
        "warmup": warmup,
        "eager": eager,
    });
    if let Some(f) = stats_file.as_mut() {
        writeln!(f, "{}", json!({ "config": config }))?;
    }

    let f = clock::freq();
    let interval = f / fps as i64;
    let start = qpc();
    let warm_end = start + (warmup * f as f64) as i64;
    let end = warm_end + (seconds * f as f64) as i64;

    let mut sampler = None;
    let mut cpu_at_warm = 0.0;
    let mut pending: Option<Pending> = None;
    // 按目标帧率排的发送时刻；新画面最多可以提前四分之一个间隔发送
    let mut next_due: Option<i64> = None;
    let mut index: u64 = 0;
    let mut recs: HashMap<u64, FrameRec> = HashMap::new();
    let mut c_new = 0u64; // 采到的新画面
    let mut c_replaced = 0u64; // 来不及编码就被下一帧覆盖
    let mut c_access_lost = 0u64;
    let mut c_stamp_fail = 0u64;
    // 正式记录阶段的统计
    let (mut lat_cap, mut lat_present, mut lat_wait, mut lat_enc, mut lat_total) = (vec![], vec![], vec![], vec![], vec![]);
    let (mut sizes, mut key_sizes) = (vec![], vec![]);
    let mut per_sec: HashMap<i64, u64> = HashMap::new();
    let (mut n_out, mut n_in_measured, mut acc_sum) = (0u64, 0u64, 0u64);

    let mut handle = |outs: Vec<encoder::Encoded>,
                      recs: &mut HashMap<u64, FrameRec>,
                      out_file: &mut Option<File>,
                      stats_file: &mut Option<File>|
     -> Res<()> {
        for o in outs {
            if let Some(f) = out_file.as_mut() {
                f.write_all(&o.data)?;
            }
            let t2 = to_ms(o.t_out);
            let r = recs.remove(&o.index);
            if let Some(f) = stats_file.as_mut() {
                let line = match &r {
                    Some(r) => json!({
                        "i": o.index, "bytes": o.data.len(), "key": o.key,
                        "stamp": r.stamp, "t0": r.t0_ms, "present": r.present_ms,
                        "t1": r.t1_ms, "tin": r.t_in_ms, "t2": t2, "acc": r.accumulated,
                    }),
                    None => json!({ "i": o.index, "bytes": o.data.len(), "key": o.key, "t2": t2 }),
                };
                writeln!(f, "{line}")?;
            }
            let Some(r) = r else { continue };
            if o.t_out < warm_end {
                continue;
            }
            n_out += 1;
            acc_sum += r.accumulated as u64;
            sizes.push(o.data.len() as f64);
            if o.key {
                key_sizes.push(o.data.len() as f64);
            }
            *per_sec.entry(((o.t_out - warm_end) / f) as i64).or_default() += o.data.len() as u64;
            lat_wait.push(r.t_in_ms - r.t1_ms);
            lat_enc.push(t2 - r.t_in_ms);
            if let Some(p) = r.present_ms {
                lat_present.push(r.t1_ms - p);
            }
            if let Some(t0) = r.t0_ms {
                lat_cap.push(r.t1_ms - t0);
                lat_total.push(t2 - t0);
            }
        }
        Ok(())
    };

    loop {
        let now = qpc();
        if now >= end {
            break;
        }
        if sampler.is_none() && now >= warm_end {
            sampler = Some(metrics::Sampler::start());
            cpu_at_warm = metrics::cpu_seconds();
        }
        let deadline = next_due.map_or(now, |d| d - interval / 4);
        // 按需取帧：离发送时刻还早就先睡，醒来后取到的是这段时间里最新的画面
        if !eager && pending.is_none() && deadline - f / 1000 > now {
            let us = ((deadline - f / 1000 - now) as i128 * 1_000_000 / f as i128) as u64;
            std::thread::sleep(std::time::Duration::from_micros(us));
            continue;
        }
        let timeout_ms = if pending.is_some() {
            ((deadline - now).max(0) * 1000 / f) as u32
        } else {
            100
        };
        match cap.acquire(timeout_ms) {
            Ok(Some(fr)) => {
                let t1 = qpc();
                if fr.present_qpc != 0 {
                    unsafe { ctx.CopyResource(&conv.src, &fr.texture) };
                    c_new += 1;
                    if pending.is_some() {
                        c_replaced += 1;
                    }
                    pending = Some(Pending { t1, present: fr.present_qpc, accumulated: fr.accumulated });
                }
                cap.release()?;
            }
            Ok(None) => {}
            Err(e) if e.code() == DXGI_ERROR_ACCESS_LOST => {
                c_access_lost += 1;
                std::thread::sleep(std::time::Duration::from_millis(100));
                cap.recreate()?;
                continue;
            }
            Err(e) => return Err(e.into()),
        }

        let now = qpc();
        if pending.is_some() && now >= deadline {
            let p = pending.take().unwrap();
            let st = match &stamp_reader {
                Some(r) => r.read(&ctx, &conv.src)?,
                None => None,
            };
            if use_stamp && st.is_none() {
                c_stamp_fail += 1;
            }
            let t1_ms = to_ms(p.t1);
            let t_in = qpc();
            let nv12 = conv.convert()?;
            recs.insert(
                index,
                FrameRec {
                    t0_ms: st.map(|s| clock::tick32_to_ms(s, t1_ms)),
                    stamp: st,
                    present_ms: Some(to_ms(p.present)),
                    t1_ms,
                    t_in_ms: to_ms(t_in),
                    accumulated: p.accumulated,
                },
            );
            if t_in >= warm_end {
                n_in_measured += 1;
            }
            let outs = enc.encode(&nv12, index, index == 0)?;
            handle(outs, &mut recs, &mut out_file, &mut stats_file)?;
            next_due = Some(match next_due {
                Some(d) if d + interval >= t_in => d + interval,
                _ => t_in + interval,
            });
            index += 1;
        }
    }
    let outs = enc.finish()?;
    handle(outs, &mut recs, &mut out_file, &mut stats_file)?;
    drop(pattern);
    let wall = to_ms(qpc() - warm_end) / 1000.0;
    let cpu_total = metrics::cpu_seconds() - cpu_at_warm;
    let samples = sampler.map(|s| s.stop()).unwrap_or_default();
    let (_, peak_mb) = metrics::private_mb();

    let total_bytes: f64 = sizes.iter().sum();
    let mut sec_kbps: Vec<f64> = per_sec.values().map(|b| *b as f64 * 8.0 / 1000.0).collect();
    sec_kbps.sort_by(|a, b| a.partial_cmp(b).unwrap());
    let gpu = |m: &std::collections::BTreeMap<String, Vec<f64>>| -> Value {
        m.iter()
            .map(|(k, v)| {
                let peak = v.iter().cloned().fold(0.0, f64::max);
                (k.clone(), json!({ "avg": round2(mean(v)), "peak": round2(peak) }))
            })
            .collect::<serde_json::Map<_, _>>()
            .into()
    };
    Ok(json!({
        "config": config,
        "encoder_init_ms": round2(enc_init_ms),
        "encoder_unsupported_settings": enc.settings_errors,
        "measured_seconds": round2(wall),
        "frames": {
            "captured_new_total": c_new,
            "replaced_before_encode_total": c_replaced,
            "encoded_in": n_in_measured,
            "encoded_out": n_out,
            "out_fps": round2(n_out as f64 / wall),
            "source_updates_per_captured": round2(acc_sum as f64 / n_out.max(1) as f64),
            "stamp_unreadable_total": c_stamp_fail,
            "access_lost_total": c_access_lost,
        },
        "latency_ms": {
            "draw_to_capture": summary(&lat_cap),
            "present_to_capture": summary(&lat_present),
            "capture_to_encoder_input": summary(&lat_wait),
            "encoder_input_to_output": summary(&lat_enc),
            "draw_to_encoded": summary(&lat_total),
        },
        "bitrate_kbps": {
            "avg": round2(total_bytes * 8.0 / 1000.0 / wall),
            "peak_1s": sec_kbps.last().map(|x| round2(*x)),
        },
        "frame_bytes": summary(&sizes),
        "keyframe_bytes": summary(&key_sizes),
        "cpu_pct_of_machine": {
            "avg": round2(cpu_total / wall / metrics::cpu_count() * 100.0),
            "samples": summary(&samples.cpu_pct),
            "cpus": metrics::cpu_count(),
        },
        "private_mb": { "avg": round2(mean(&samples.private_mb)), "peak": round2(peak_mb) },
        "gpu_self_pct": gpu(&samples.gpu_self),
        "gpu_machine_pct": gpu(&samples.gpu_all),
        "gpu_counter_error": samples.error,
    }))
}

fn verify(args: &[String]) -> Res<Value> {
    init()?;
    let input = opt(args, "--in").ok_or("缺少 --in")?;
    let stats = opt(args, "--stats").ok_or("缺少 --stats")?;
    let mut data = Vec::new();
    File::open(input)?.read_to_end(&mut data)?;

    let mut config = Value::Null;
    let mut frames: Vec<(u64, usize, Option<u32>)> = Vec::new();
    for line in BufReader::new(File::open(stats)?).lines() {
        let v: Value = serde_json::from_str(&line?)?;
        if let Some(c) = v.get("config") {
            config = c.clone();
            continue;
        }
        frames.push((
            v["i"].as_u64().unwrap_or(0),
            v["bytes"].as_u64().unwrap_or(0) as usize,
            v["stamp"].as_u64().map(|s| s as u32),
        ));
    }
    let src_w = config["src"][0].as_u64().ok_or("统计文件缺少 config")? as f64;

    let mut dec = decoder::Decoder::new()?;
    let mut expect: HashMap<u64, Option<u32>> = HashMap::new();
    let mut decoded = Vec::new();
    let mut times = Vec::new();
    let mut off = 0;
    for (i, len, st) in &frames {
        if off + len > data.len() {
            return Err("码流比统计文件记录的短".into());
        }
        expect.insert(*i, *st);
        let t = qpc();
        let outs = dec.decode(&data[off..off + len], *i)?;
        times.push(to_ms(qpc() - t));
        decoded.extend(outs);
        off += len;
    }
    decoded.extend(dec.finish()?);

    let (w, h) = (dec.width as usize, dec.height as usize);
    let scale = w as f64 / src_w;
    let (mut ok, mut bad, mut unreadable, mut no_expect) = (0, 0, 0, 0);
    for d in &decoded {
        let got = stamp::decode(|i| {
            let (x, y) = stamp::center(i);
            let (x, y) = ((x as f64 * scale) as usize, (y as f64 * scale) as usize);
            d.data.get(y * w + x).copied().unwrap_or(0)
        });
        match (expect.get(&d.index).copied().flatten(), got) {
            (None, _) => no_expect += 1,
            (Some(_), None) => unreadable += 1,
            (Some(a), Some(b)) if a == b => ok += 1,
            _ => bad += 1,
        }
    }

    if let Some(path) = opt(args, "--png") {
        if let Some(d) = decoded.get(decoded.len() / 2) {
            save_png(path, &d.data, w, h)?;
        }
    }
    Ok(json!({
        "config": config,
        "bytes_total": data.len(),
        "frames_in_stats": frames.len(),
        "frames_decoded": decoded.len(),
        "decoded_size": [w, h],
        "stamp": { "match": ok, "mismatch": bad, "unreadable": unreadable, "not_recorded": no_expect },
        "decode_call_ms": summary(&times),
    }))
}

/// 把 NV12 转成 RGB 保存，宽度超过 1280 时按整数倍缩小。
fn save_png(path: &str, nv12: &[u8], w: usize, h: usize) -> Res<()> {
    let step = w.div_ceil(1280).max(1);
    let (ow, oh) = (w / step, h / step);
    let mut rgb = Vec::with_capacity(ow * oh * 3);
    for oy in 0..oh {
        for ox in 0..ow {
            let (x, y) = (ox * step, oy * step);
            let yv = nv12[y * w + x] as f64 - 16.0;
            let uv = w * h + (y / 2) * w + (x / 2) * 2;
            let (u, v) = (nv12[uv] as f64 - 128.0, nv12[uv + 1] as f64 - 128.0);
            // BT.709，16–235
            let c = |x: f64| x.round().clamp(0.0, 255.0) as u8;
            rgb.push(c(1.164 * yv + 1.793 * v));
            rgb.push(c(1.164 * yv - 0.213 * u - 0.533 * v));
            rgb.push(c(1.164 * yv + 2.112 * u));
        }
    }
    let mut enc = png::Encoder::new(File::create(path)?, ow as u32, oh as u32);
    enc.set_color(png::ColorType::Rgb);
    enc.set_depth(png::BitDepth::Eight);
    enc.write_header()?.write_image_data(&rgb)?;
    Ok(())
}
