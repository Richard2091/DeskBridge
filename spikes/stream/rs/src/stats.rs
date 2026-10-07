//! 统计辅助函数。

use serde_json::{json, Value};

/// 返回 p50、p95、最大值和样本数。
pub fn summary(values: &[f64]) -> Value {
    if values.is_empty() {
        return json!(null);
    }
    let mut v = values.to_vec();
    v.sort_by(|a, b| a.partial_cmp(b).unwrap());
    let pick = |p: f64| v[((v.len() - 1) as f64 * p).round() as usize];
    json!({
        "p50": round2(pick(0.5)),
        "p95": round2(pick(0.95)),
        "max": round2(*v.last().unwrap()),
        "n": v.len(),
    })
}

pub fn mean(values: &[f64]) -> f64 {
    if values.is_empty() {
        0.0
    } else {
        values.iter().sum::<f64>() / values.len() as f64
    }
}

pub fn round2(x: f64) -> f64 {
    (x * 100.0).round() / 100.0
}
