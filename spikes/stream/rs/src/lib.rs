//! 远程画面调研的共用代码：时钟、画面时间戳、采集、颜色转换、编码、解码和资源采样。

pub mod capture;
pub mod clock;
pub mod convert;
pub mod d3d;
pub mod decoder;
pub mod encoder;
pub mod metrics;
pub mod mf;
pub mod stamp;
pub mod stats;
