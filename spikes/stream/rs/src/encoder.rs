//! Media Foundation H.264 编码。硬件编码器（NVENC、Quick Sync 等）直接读显卡纹理；
//! 软件编码器（微软自带）从显卡读回内存后编码。

use crate::d3d::texture;
use crate::mf::{self, Out};
use std::sync::mpsc::{channel, Receiver, RecvTimeoutError};
use std::time::Duration;
use windows::core::{Error, Interface, Result};
use windows::Win32::Foundation::{E_FAIL, LUID};
use windows::Win32::Graphics::Direct3D11::*;
use windows::Win32::Graphics::Dxgi::Common::DXGI_FORMAT_NV12;
use windows::Win32::Media::MediaFoundation::*;
use windows::Win32::System::Com::{CoInitializeEx, COINIT_MULTITHREADED};

#[derive(Clone, Copy, PartialEq)]
pub enum Kind {
    Hardware,
    Software,
}

pub struct Config {
    pub width: u32,
    pub height: u32,
    pub fps: u32,
    pub bitrate: u32,
    pub kind: Kind,
    pub luid: LUID,
}

pub struct Encoded {
    pub index: u64,
    pub data: Vec<u8>,
    pub key: bool,
    /// 编码器交出这一帧的 QPC 时刻。
    pub t_out: i64,
}

enum Ev {
    NeedInput,
    Output(Encoded),
    DrainComplete,
    Error(String),
}

/// 异步编码器的事件线程：收到“有输出”就立即取出并记录时刻，主循环不用等。
struct EventThread {
    mft: IMFTransform,
    events: IMFMediaEventGenerator,
    provides: bool,
    alloc: u32,
    dur: i64,
}
unsafe impl Send for EventThread {}

impl EventThread {
    fn run(mut self, tx: std::sync::mpsc::Sender<Ev>) {
        unsafe {
            let _ = CoInitializeEx(None, COINIT_MULTITHREADED);
        }
        loop {
            let ev = match unsafe { self.events.GetEvent(MEDIA_EVENT_GENERATOR_GET_EVENT_FLAGS(0)) } {
                Ok(ev) => ev,
                Err(e) => {
                    let _ = tx.send(Ev::Error(e.to_string()));
                    return;
                }
            };
            let m = match unsafe { ev.GetType() }.unwrap_or(0) {
                601 => Ev::NeedInput,
                602 => match self.take() {
                    Ok(Some(e)) => Ev::Output(e),
                    Ok(None) => continue,
                    Err(e) => Ev::Error(e.to_string()),
                },
                603 => Ev::DrainComplete,
                1 => Ev::Error(format!("MEError {:?}", unsafe { ev.GetStatus() })),
                _ => continue,
            };
            let stop = matches!(m, Ev::DrainComplete | Ev::Error(_));
            if tx.send(m).is_err() || stop {
                return;
            }
        }
    }

    fn take(&mut self) -> Result<Option<Encoded>> {
        loop {
            match mf::process_output(&self.mft, self.provides, self.alloc)? {
                Out::Sample(s) => return Ok(Some(to_encoded(&s, self.dur)?)),
                Out::StreamChange => {
                    unsafe {
                        let t = self.mft.GetOutputAvailableType(0, 0)?;
                        self.mft.SetOutputType(0, &t, 0)?;
                    }
                    let (p, cb) = mf::output_info(&self.mft)?;
                    self.provides = p;
                    self.alloc = cb.max(self.alloc);
                }
                Out::NeedMore => return Ok(None),
            }
        }
    }
}

fn to_encoded(s: &IMFSample, dur: i64) -> Result<Encoded> {
    let t_out = crate::clock::qpc();
    unsafe {
        let time = s.GetSampleTime().unwrap_or(0);
        Ok(Encoded {
            index: ((time + dur / 2) / dur) as u64,
            key: s.GetUINT32(&MFSampleExtension_CleanPoint).unwrap_or(0) != 0,
            data: mf::sample_bytes(s)?,
            t_out,
        })
    }
}

pub struct Encoder {
    mft: IMFTransform,
    codec: Option<ICodecAPI>,
    pub name: String,
    pub hardware: bool,
    pub is_async: bool,
    /// 编码器不支持的参数。
    pub settings_errors: Vec<String>,
    rx: Option<Receiver<Ev>>,
    need: u32,
    drained: bool,
    provides: bool,
    alloc: u32,
    dur: i64,
    ctx: ID3D11DeviceContext,
    staging: Option<ID3D11Texture2D>,
    w: u32,
    h: u32,
    _manager: Option<IMFDXGIDeviceManager>,
}

pub fn list() -> Result<Vec<mf::MftEntry>> {
    mf::enum_mfts(
        MFT_CATEGORY_VIDEO_ENCODER,
        MFT_ENUM_FLAG_ALL,
        Some(MFVideoFormat_NV12),
        Some(MFVideoFormat_H264),
        None,
    )
}

impl Encoder {
    pub fn new(device: &ID3D11Device, ctx: &ID3D11DeviceContext, cfg: &Config) -> Result<Self> {
        let (flags, luid) = match cfg.kind {
            Kind::Hardware => (MFT_ENUM_FLAG_HARDWARE | MFT_ENUM_FLAG_SORTANDFILTER, Some(cfg.luid)),
            Kind::Software => (MFT_ENUM_FLAG_SYNCMFT | MFT_ENUM_FLAG_SORTANDFILTER, None),
        };
        let list = mf::enum_mfts(
            MFT_CATEGORY_VIDEO_ENCODER,
            flags,
            Some(MFVideoFormat_NV12),
            Some(MFVideoFormat_H264),
            luid,
        )?;
        let mut errs = Vec::new();
        for e in list {
            let name = e.name.clone();
            match Self::init(e, device, ctx, cfg) {
                Ok(enc) => return Ok(enc),
                Err(er) => errs.push(format!("{name}: {er}")),
            }
        }
        Err(Error::new(E_FAIL, format!("没有可用的编码器：{errs:?}")))
    }

    fn init(e: mf::MftEntry, device: &ID3D11Device, ctx: &ID3D11DeviceContext, cfg: &Config) -> Result<Self> {
        let (w, h, fps) = (cfg.width, cfg.height, cfg.fps);
        let hardware = cfg.kind == Kind::Hardware;
        let mft: IMFTransform = unsafe { e.activate.ActivateObject()? };
        let is_async = mf::is_async(&mft);
        if is_async {
            unsafe { mft.GetAttributes()?.SetUINT32(&MF_TRANSFORM_ASYNC_UNLOCK, 1)? };
        }
        let mut manager = None;
        if hardware {
            let mut token = 0u32;
            let mut m = None;
            unsafe {
                MFCreateDXGIDeviceManager(&mut token, &mut m)?;
                let m = m.unwrap();
                m.ResetDevice(device, token)?;
                mft.ProcessMessage(MFT_MESSAGE_SET_D3D_MANAGER, m.as_raw() as usize)?;
                manager = Some(m);
            }
        }

        let codec: Option<ICodecAPI> = mft.cast().ok();
        let mut log = Vec::new();
        if let Some(c) = &codec {
            use mf::{set_codec, var_bool, var_u32};
            set_codec(c, "RateControlMode", &CODECAPI_AVEncCommonRateControlMode, var_u32(eAVEncCommonRateControlMode_CBR.0 as u32), &mut log);
            set_codec(c, "MeanBitRate", &CODECAPI_AVEncCommonMeanBitRate, var_u32(cfg.bitrate), &mut log);
            set_codec(c, "LowLatencyMode", &CODECAPI_AVLowLatencyMode, var_bool(true), &mut log);
            set_codec(c, "BPictureCount", &CODECAPI_AVEncMPVDefaultBPictureCount, var_u32(0), &mut log);
            set_codec(c, "GOPSize", &CODECAPI_AVEncMPVGOPSize, var_u32(fps * 600), &mut log);
        } else {
            log.push("没有 ICodecAPI".into());
        }

        unsafe {
            let out = mf::video_type(MFVideoFormat_H264, w, h, fps)?;
            out.SetUINT32(&MF_MT_AVG_BITRATE, cfg.bitrate)?;
            out.SetUINT32(&MF_MT_MPEG2_PROFILE, eAVEncH264VProfile_Main.0 as u32)?;
            mft.SetOutputType(0, &out, 0)?;
            let inp = mf::video_type(MFVideoFormat_NV12, w, h, fps)?;
            mft.SetInputType(0, &inp, 0)?;
        }
        let (provides, cb) = mf::output_info(&mft)?;

        unsafe {
            // 微软软件编码器在开始前不接受清空命令，忽略错误
            let _ = mft.ProcessMessage(MFT_MESSAGE_COMMAND_FLUSH, 0);
            mft.ProcessMessage(MFT_MESSAGE_NOTIFY_BEGIN_STREAMING, 0)?;
            mft.ProcessMessage(MFT_MESSAGE_NOTIFY_START_OF_STREAM, 0)?;
        }

        let dur = 10_000_000 / fps as i64;
        let alloc = cb.max(w * h * 2);
        let rx = if is_async {
            let t = EventThread { mft: mft.clone(), events: mf::events_of(&mft)?, provides, alloc, dur };
            let (tx, rx) = channel();
            std::thread::spawn(move || t.run(tx));
            Some(rx)
        } else {
            None
        };

        let staging = if hardware {
            None
        } else {
            Some(texture(device, w, h, DXGI_FORMAT_NV12, 0, D3D11_USAGE_STAGING, D3D11_CPU_ACCESS_READ.0 as u32)?)
        };

        Ok(Self {
            mft,
            codec,
            name: e.name,
            hardware: e.hardware,
            is_async,
            settings_errors: log,
            rx,
            need: 0,
            drained: false,
            provides,
            alloc,
            dur,
            ctx: ctx.clone(),
            staging,
            w,
            h,
            _manager: manager,
        })
    }

    fn make_sample(&self, tex: &ID3D11Texture2D) -> Result<IMFSample> {
        unsafe {
            let Some(staging) = &self.staging else {
                let buf = MFCreateDXGISurfaceBuffer(&ID3D11Texture2D::IID, tex, 0, false)?;
                let len = buf.cast::<IMF2DBuffer>()?.GetContiguousLength()?;
                buf.SetCurrentLength(len)?;
                let s = MFCreateSample()?;
                s.AddBuffer(&buf)?;
                return Ok(s);
            };
            self.ctx.CopyResource(staging, tex);
            let mut m = D3D11_MAPPED_SUBRESOURCE::default();
            self.ctx.Map(staging, 0, D3D11_MAP_READ, 0, Some(&mut m))?;
            let (w, h, pitch) = (self.w as usize, self.h as usize, m.RowPitch as usize);
            let size = w * h * 3 / 2;
            let buf = MFCreateMemoryBuffer(size as u32)?;
            let mut p = std::ptr::null_mut();
            buf.Lock(&mut p, None, None)?;
            let src = m.pData as *const u8;
            // Y 平面 h 行，UV 平面 h/2 行，紧跟在 Y 平面之后
            for y in 0..h + h / 2 {
                std::ptr::copy_nonoverlapping(src.add(y * pitch), p.add(y * w), w);
            }
            buf.Unlock()?;
            self.ctx.Unmap(staging, 0);
            buf.SetCurrentLength(size as u32)?;
            let s = MFCreateSample()?;
            s.AddBuffer(&buf)?;
            Ok(s)
        }
    }

    /// 编码一帧，返回这次调用期间交出的所有帧（通常就是这一帧）。
    pub fn encode(&mut self, tex: &ID3D11Texture2D, index: u64, force_key: bool) -> Result<Vec<Encoded>> {
        if force_key {
            if let Some(c) = &self.codec {
                mf::set_codec(c, "ForceKeyFrame", &CODECAPI_AVEncVideoForceKeyFrame, mf::var_u32(1), &mut self.settings_errors);
            }
        }
        let sample = self.make_sample(tex)?;
        unsafe {
            sample.SetSampleTime(index as i64 * self.dur)?;
            sample.SetSampleDuration(self.dur)?;
        }
        let mut outs = Vec::new();
        if self.is_async {
            self.pump(&mut outs, Duration::ZERO, false)?;
            while self.need == 0 {
                self.pump(&mut outs, Duration::from_secs(2), true)?;
            }
            unsafe { self.mft.ProcessInput(0, &sample, 0)? };
            self.need -= 1;
            self.pump(&mut outs, Duration::ZERO, false)?;
        } else {
            unsafe { self.mft.ProcessInput(0, &sample, 0)? };
            self.drain_sync(&mut outs)?;
        }
        Ok(outs)
    }

    /// 结束编码，取出编码器里剩下的帧。
    pub fn finish(&mut self) -> Result<Vec<Encoded>> {
        let mut outs = Vec::new();
        unsafe {
            self.mft.ProcessMessage(MFT_MESSAGE_NOTIFY_END_OF_STREAM, 0)?;
            self.mft.ProcessMessage(MFT_MESSAGE_COMMAND_DRAIN, 0)?;
        }
        if self.is_async {
            while !self.drained {
                self.pump(&mut outs, Duration::from_secs(2), true)?;
            }
        } else {
            self.drain_sync(&mut outs)?;
        }
        Ok(outs)
    }

    /// 处理事件线程送来的消息。`timeout` 为零时只取已经到达的，`required` 时至少等到一条。
    fn pump(&mut self, outs: &mut Vec<Encoded>, timeout: Duration, required: bool) -> Result<()> {
        let rx = self.rx.as_ref().unwrap();
        let mut first = true;
        loop {
            let was_first = first;
            first = false;
            let r = if was_first { rx.recv_timeout(timeout) } else { rx.try_recv().map_err(|_| RecvTimeoutError::Timeout) };
            match r {
                Ok(Ev::NeedInput) => self.need += 1,
                Ok(Ev::Output(e)) => outs.push(e),
                Ok(Ev::DrainComplete) => self.drained = true,
                Ok(Ev::Error(s)) => return Err(Error::new(E_FAIL, format!("编码器事件错误：{s}"))),
                Err(RecvTimeoutError::Timeout) if !required || !was_first => return Ok(()),
                Err(_) => return Err(Error::new(E_FAIL, "编码器 2 秒内没有响应")),
            }
        }
    }

    fn drain_sync(&mut self, outs: &mut Vec<Encoded>) -> Result<()> {
        loop {
            if !self.take_output(outs)? {
                return Ok(());
            }
        }
    }

    /// 调用一次 ProcessOutput。返回 false 表示编码器需要更多输入。
    fn take_output(&mut self, outs: &mut Vec<Encoded>) -> Result<bool> {
        match mf::process_output(&self.mft, self.provides, self.alloc)? {
            Out::Sample(s) => {
                outs.push(to_encoded(&s, self.dur)?);
                Ok(true)
            }
            Out::StreamChange => {
                unsafe {
                    let t = self.mft.GetOutputAvailableType(0, 0)?;
                    self.mft.SetOutputType(0, &t, 0)?;
                }
                let (p, cb) = mf::output_info(&self.mft)?;
                self.provides = p;
                self.alloc = cb.max(self.alloc);
                Ok(true)
            }
            Out::NeedMore => Ok(false),
        }
    }
}

impl Drop for Encoder {
    fn drop(&mut self) {
        if let Ok(s) = self.mft.cast::<IMFShutdown>() {
            unsafe {
                let _ = s.Shutdown();
            }
        }
    }
}
