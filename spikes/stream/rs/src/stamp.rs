//! 画面里的机器可读时间戳：屏幕左上角一排黑白方块。
//!
//! 布局：起始标记（白、黑）+ 32 位数据（高位在前）+ 1 位偶校验，每块 `BLOCK` 像素见方。

pub const BLOCK: u32 = 24;
pub const BITS: usize = 32;
pub const TOTAL: usize = 2 + BITS + 1;

pub fn width() -> u32 {
    TOTAL as u32 * BLOCK
}

/// 每块是否为白色。
pub fn encode(v: u32) -> [bool; TOTAL] {
    let mut out = [false; TOTAL];
    out[0] = true;
    out[1] = false;
    for i in 0..BITS {
        out[2 + i] = (v >> (BITS - 1 - i)) & 1 == 1;
    }
    out[TOTAL - 1] = v.count_ones() % 2 == 1;
    out
}

/// `luma(i)` 返回第 i 块中心的亮度（0–255）。标记或校验不对时返回 None。
pub fn decode(luma: impl Fn(usize) -> u8) -> Option<u32> {
    let bit = |i: usize| luma(i) >= 128;
    if !bit(0) || bit(1) {
        return None;
    }
    let mut v = 0u32;
    for i in 0..BITS {
        v = (v << 1) | bit(2 + i) as u32;
    }
    if (v.count_ones() % 2 == 1) != bit(TOTAL - 1) {
        return None;
    }
    Some(v)
}

/// 第 i 块中心在原始分辨率下的坐标。
pub fn center(i: usize) -> (u32, u32) {
    (i as u32 * BLOCK + BLOCK / 2, BLOCK / 2)
}
