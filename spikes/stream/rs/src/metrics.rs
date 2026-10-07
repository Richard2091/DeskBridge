//! 每秒采样本进程的 CPU、内存和显卡各引擎占用。
//!
//! 显卡占用来自性能计数器 `GPU Engine`，用英文名添加，中文系统上也能用。

use std::collections::BTreeMap;
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::Arc;
use std::thread::JoinHandle;
use std::time::{Duration, Instant};
use windows::core::{w, PCWSTR};
use windows::Win32::Foundation::FILETIME;
use windows::Win32::System::Performance::*;
use windows::Win32::System::ProcessStatus::{GetProcessMemoryInfo, PROCESS_MEMORY_COUNTERS, PROCESS_MEMORY_COUNTERS_EX};
use windows::Win32::System::Threading::{GetCurrentProcess, GetProcessTimes};

#[derive(Default)]
pub struct Samples {
    pub cpu_pct: Vec<f64>,
    pub private_mb: Vec<f64>,
    /// 本进程，按引擎类型（VideoEncode、3D 等）。
    pub gpu_self: BTreeMap<String, Vec<f64>>,
    /// 整机，按引擎类型，取同类引擎中最忙的一个。
    pub gpu_all: BTreeMap<String, Vec<f64>>,
    pub error: Option<String>,
}

pub struct Sampler {
    stop: Arc<AtomicBool>,
    handle: JoinHandle<Samples>,
}

impl Sampler {
    pub fn start() -> Self {
        let stop = Arc::new(AtomicBool::new(false));
        let s = stop.clone();
        Self { stop, handle: std::thread::spawn(move || run(s)) }
    }

    pub fn stop(self) -> Samples {
        self.stop.store(true, Ordering::SeqCst);
        self.handle.join().unwrap_or_default()
    }
}

fn ft(f: FILETIME) -> f64 {
    (((f.dwHighDateTime as u64) << 32) | f.dwLowDateTime as u64) as f64 / 1e7
}

/// 本进程累计的 CPU 时间（秒）。
pub fn cpu_seconds() -> f64 {
    let (mut c, mut e, mut k, mut u) = Default::default();
    unsafe {
        let _ = GetProcessTimes(GetCurrentProcess(), &mut c, &mut e, &mut k, &mut u);
    }
    ft(k) + ft(u)
}

/// 本进程当前和峰值私有内存（MB）。
pub fn private_mb() -> (f64, f64) {
    let mut m = PROCESS_MEMORY_COUNTERS_EX::default();
    unsafe {
        let _ = GetProcessMemoryInfo(
            GetCurrentProcess(),
            &mut m as *mut _ as *mut PROCESS_MEMORY_COUNTERS,
            std::mem::size_of::<PROCESS_MEMORY_COUNTERS_EX>() as u32,
        );
    }
    (m.PrivateUsage as f64 / 1048576.0, m.PeakPagefileUsage as f64 / 1048576.0)
}

pub fn cpu_count() -> f64 {
    std::thread::available_parallelism().map(|n| n.get()).unwrap_or(1) as f64
}

fn run(stop: Arc<AtomicBool>) -> Samples {
    let pid = std::process::id();
    let ncpu = cpu_count();
    let mut s = Samples::default();
    let mut q = PDH_HQUERY::default();
    let mut c = PDH_HCOUNTER::default();
    let ok = unsafe {
        PdhOpenQueryW(PCWSTR::null(), 0, &mut q) == 0
            && PdhAddEnglishCounterW(q, w!("\\GPU Engine(*)\\Utilization Percentage"), 0, &mut c) == 0
    };
    if ok {
        unsafe { PdhCollectQueryData(q) };
    } else {
        s.error = Some("无法添加 GPU Engine 计数器".into());
    }
    let mut last_cpu = cpu_seconds();
    let mut last_t = Instant::now();
    'outer: loop {
        let until = last_t + Duration::from_secs(1);
        while Instant::now() < until {
            if stop.load(Ordering::SeqCst) {
                break 'outer;
            }
            std::thread::sleep(Duration::from_millis(20));
        }
        let now = Instant::now();
        let cpu = cpu_seconds();
        s.cpu_pct.push((cpu - last_cpu) / (now - last_t).as_secs_f64() / ncpu * 100.0);
        s.private_mb.push(private_mb().0);
        (last_cpu, last_t) = (cpu, now);
        if ok {
            if let Some((mine, all)) = gpu_sample(q, c, pid) {
                for (k, v) in mine {
                    s.gpu_self.entry(k).or_default().push(v);
                }
                for (k, v) in all {
                    s.gpu_all.entry(k).or_default().push(v);
                }
            }
        }
    }
    if ok {
        unsafe { PdhCloseQuery(q) };
    }
    s
}

type EngineMap = BTreeMap<String, f64>;

fn gpu_sample(q: PDH_HQUERY, c: PDH_HCOUNTER, pid: u32) -> Option<(EngineMap, EngineMap)> {
    unsafe {
        if PdhCollectQueryData(q) != 0 {
            return None;
        }
        let mut size = 0u32;
        let mut count = 0u32;
        PdhGetFormattedCounterArrayW(c, PDH_FMT_DOUBLE, &mut size, &mut count, None);
        if size == 0 {
            return None;
        }
        let mut buf = vec![0u64; size as usize / 8 + 1];
        let items = buf.as_mut_ptr() as *mut PDH_FMT_COUNTERVALUE_ITEM_W;
        if PdhGetFormattedCounterArrayW(c, PDH_FMT_DOUBLE, &mut size, &mut count, Some(items)) != 0 {
            return None;
        }
        let prefix = format!("pid_{pid}_");
        // 实例名形如 pid_1234_luid_0x0_0xD1B5_phys_0_eng_3_engtype_VideoEncode
        let mut mine: BTreeMap<String, f64> = BTreeMap::new();
        let mut per_engine: BTreeMap<(String, String), f64> = BTreeMap::new();
        for i in 0..count as usize {
            let it = &*items.add(i);
            let name = it.szName.to_string().unwrap_or_default();
            let v = it.FmtValue.Anonymous.doubleValue;
            let (Some(l), Some(t)) = (name.find("_luid_"), name.find("_engtype_")) else { continue };
            let engtype = name[t + 9..].to_string();
            let engine = name[l..t].to_string();
            *per_engine.entry((engtype.clone(), engine)).or_default() += v;
            if name.starts_with(&prefix) {
                *mine.entry(engtype).or_default() += v;
            }
        }
        let mut all: BTreeMap<String, f64> = BTreeMap::new();
        for ((engtype, _), v) in per_engine {
            let e = all.entry(engtype).or_default();
            *e = e.max(v);
        }
        Some((mine, all))
    }
}
