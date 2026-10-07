//! DeskBridge 的 Windows 桌面辅助进程。
//!
//! 由 Go 主程序启动，经标准输入输出传 JSON，负责截图、模拟输入和读取界面元素。
//! 协议和桌面功能在后续任务中实现，这里只有骨架。

/// 程序名和版本，例如 `deskbridge-desktop-helper 0.1.0`。
pub fn version_line() -> String {
    format!("{} {}", env!("CARGO_PKG_NAME"), env!("CARGO_PKG_VERSION"))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn version_line_has_name_and_version() {
        assert_eq!(version_line(), "deskbridge-desktop-helper 0.1.0");
    }
}
