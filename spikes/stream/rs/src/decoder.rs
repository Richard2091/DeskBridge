//! 用微软自带的 H.264 软件解码器检查编码结果（`capenc verify`）。

use crate::mf::{self, Out};
use windows::core::{Error, Result};
use windows::Win32::Foundation::E_FAIL;
use windows::Win32::Media::MediaFoundation::*;
use windows::Win32::System::Com::{CoCreateInstance, CLSCTX_INPROC_SERVER};

pub struct Decoder {
    mft: IMFTransform,
    provides: bool,
    alloc: u32,
    pub width: u32,
    pub height: u32,
}

pub struct Decoded {
    pub index: u64,
    /// NV12，行宽等于 `width`。
    pub data: Vec<u8>,
    pub t_out: i64,
}

impl Decoder {
    pub fn new() -> Result<Self> {
        let mft: IMFTransform = unsafe { CoCreateInstance(&CLSID_MSH264DecoderMFT, None, CLSCTX_INPROC_SERVER)? };
        unsafe {
            let t = MFCreateMediaType()?;
            t.SetGUID(&MF_MT_MAJOR_TYPE, &MFMediaType_Video)?;
            t.SetGUID(&MF_MT_SUBTYPE, &MFVideoFormat_H264)?;
            mft.SetInputType(0, &t, 0)?;
            if let Ok(c) = windows::core::Interface::cast::<ICodecAPI>(&mft) {
                let _ = c.SetValue(&CODECAPI_AVLowLatencyMode, &mf::var_bool(true));
            }
        }
        let mut d = Self { mft, provides: false, alloc: 0, width: 0, height: 0 };
        d.set_output()?;
        unsafe {
            d.mft.ProcessMessage(MFT_MESSAGE_NOTIFY_BEGIN_STREAMING, 0)?;
            d.mft.ProcessMessage(MFT_MESSAGE_NOTIFY_START_OF_STREAM, 0)?;
        }
        Ok(d)
    }

    fn set_output(&mut self) -> Result<()> {
        let mut i = 0;
        loop {
            let t = unsafe { self.mft.GetOutputAvailableType(0, i)? };
            if unsafe { t.GetGUID(&MF_MT_SUBTYPE)? } == MFVideoFormat_NV12 {
                unsafe { self.mft.SetOutputType(0, &t, 0)? };
                (self.width, self.height) = mf::frame_size(&t).unwrap_or((0, 0));
                let (p, cb) = mf::output_info(&self.mft)?;
                self.provides = p;
                self.alloc = cb.max(self.width * self.height * 3 / 2);
                return Ok(());
            }
            i += 1;
            if i > 64 {
                return Err(Error::new(E_FAIL, "解码器不支持 NV12 输出"));
            }
        }
    }

    pub fn decode(&mut self, data: &[u8], index: u64) -> Result<Vec<Decoded>> {
        let s = mf::memory_sample(data)?;
        unsafe {
            s.SetSampleTime(index as i64 * 10_000)?;
            s.SetSampleDuration(10_000)?;
            self.mft.ProcessInput(0, &s, 0)?;
        }
        let mut outs = Vec::new();
        loop {
            match mf::process_output(&self.mft, self.provides, self.alloc)? {
                Out::Sample(s) => {
                    let t_out = crate::clock::qpc();
                    let time = unsafe { s.GetSampleTime() }.unwrap_or(0);
                    outs.push(Decoded { index: ((time + 5_000) / 10_000) as u64, data: mf::sample_bytes(&s)?, t_out });
                }
                Out::StreamChange => self.set_output()?,
                Out::NeedMore => return Ok(outs),
            }
        }
    }

    pub fn finish(&mut self) -> Result<Vec<Decoded>> {
        unsafe { self.mft.ProcessMessage(MFT_MESSAGE_COMMAND_DRAIN, 0)? };
        let mut outs = Vec::new();
        loop {
            match mf::process_output(&self.mft, self.provides, self.alloc)? {
                Out::Sample(s) => {
                    let t_out = crate::clock::qpc();
                    let time = unsafe { s.GetSampleTime() }.unwrap_or(0);
                    outs.push(Decoded { index: ((time + 5_000) / 10_000) as u64, data: mf::sample_bytes(&s)?, t_out });
                }
                Out::StreamChange => self.set_output()?,
                Out::NeedMore => return Ok(outs),
            }
        }
    }
}
