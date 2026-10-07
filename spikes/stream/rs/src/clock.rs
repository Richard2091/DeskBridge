//! 高精度时钟。同一台机器上所有进程的 QPC 一致，可以直接相减。

use std::sync::OnceLock;
use windows::Win32::System::Performance::{QueryPerformanceCounter, QueryPerformanceFrequency};

static FREQ: OnceLock<i64> = OnceLock::new();

pub fn freq() -> i64 {
    *FREQ.get_or_init(|| {
        let mut f = 0;
        unsafe { QueryPerformanceFrequency(&mut f).unwrap() };
        f
    })
}

pub fn qpc() -> i64 {
    let mut c = 0;
    unsafe { QueryPerformanceCounter(&mut c).unwrap() };
    c
}

/// QPC 换算成毫秒。
pub fn to_ms(c: i64) -> f64 {
    c as f64 * 1000.0 / freq() as f64
}

pub fn now_ms() -> f64 {
    to_ms(qpc())
}

/// 画面时间戳的单位是 0.1 毫秒，取低 32 位（约 4.9 天回绕一次）。
pub fn now_tick() -> u64 {
    (qpc() as i128 * 10_000 / freq() as i128) as u64
}

/// 把读回的 32 位时间戳还原成毫秒，参照时刻 `ref_ms` 必须晚于它且相差不到 4.9 天。
pub fn tick32_to_ms(tick: u32, ref_ms: f64) -> f64 {
    let ref_tick = (ref_ms * 10.0) as u64;
    let delta = (ref_tick as u32).wrapping_sub(tick) as u64;
    (ref_tick - delta) as f64 / 10.0
}
