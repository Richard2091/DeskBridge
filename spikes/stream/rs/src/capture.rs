//! DXGI Desktop Duplication 采集。

use windows::core::{Interface, Result};
use windows::Win32::Graphics::Direct3D11::*;
use windows::Win32::Graphics::Dxgi::Common::DXGI_FORMAT;
use windows::Win32::Graphics::Dxgi::*;

pub struct Capturer {
    dup: IDXGIOutputDuplication,
    output: IDXGIOutput1,
    device: ID3D11Device,
    held: bool,
    pub width: u32,
    pub height: u32,
    pub format: DXGI_FORMAT,
    /// 显示器刷新率（Hz）。
    pub refresh_hz: f64,
}

pub struct Frame {
    pub texture: ID3D11Texture2D,
    /// 系统把这一帧显示到屏幕的 QPC 时刻；只有鼠标变化时为 0。
    pub present_qpc: i64,
    /// 上次取帧以来累计的画面更新次数，大于 1 说明中间的更新被合并了。
    pub accumulated: u32,
}

impl Capturer {
    pub fn new(device: &ID3D11Device, output: &IDXGIOutput) -> Result<Self> {
        let output: IDXGIOutput1 = output.cast()?;
        let dup = unsafe { output.DuplicateOutput(device)? };
        let desc = unsafe { dup.GetDesc() };
        Ok(Self {
            dup,
            output,
            device: device.clone(),
            held: false,
            width: desc.ModeDesc.Width,
            height: desc.ModeDesc.Height,
            format: desc.ModeDesc.Format,
            refresh_hz: desc.ModeDesc.RefreshRate.Numerator as f64 / desc.ModeDesc.RefreshRate.Denominator.max(1) as f64,
        })
    }

    /// 等待下一帧，超时返回 None。拿到帧后必须调用 `release`。
    pub fn acquire(&mut self, timeout_ms: u32) -> Result<Option<Frame>> {
        self.release()?;
        let mut info = DXGI_OUTDUPL_FRAME_INFO::default();
        let mut res: Option<IDXGIResource> = None;
        match unsafe { self.dup.AcquireNextFrame(timeout_ms, &mut info, &mut res) } {
            Ok(()) => {
                self.held = true;
                Ok(Some(Frame {
                    texture: res.unwrap().cast()?,
                    present_qpc: info.LastPresentTime,
                    accumulated: info.AccumulatedFrames,
                }))
            }
            Err(e) if e.code() == DXGI_ERROR_WAIT_TIMEOUT => Ok(None),
            Err(e) => Err(e),
        }
    }

    pub fn release(&mut self) -> Result<()> {
        if self.held {
            self.held = false;
            unsafe { self.dup.ReleaseFrame()? };
        }
        Ok(())
    }

    /// 出现 DXGI_ERROR_ACCESS_LOST（切换桌面、改分辨率、UAC 弹窗）后重建。
    pub fn recreate(&mut self) -> Result<()> {
        self.held = false;
        self.dup = unsafe { self.output.DuplicateOutput(&self.device)? };
        Ok(())
    }
}
