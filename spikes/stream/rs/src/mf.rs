//! Media Foundation 的公共辅助函数。

use windows::core::{Interface, Result, GUID, PWSTR};
use windows::Win32::Foundation::LUID;
use windows::Win32::Media::MediaFoundation::*;
use windows::Win32::System::Com::{CoInitializeEx, CoTaskMemFree, COINIT_MULTITHREADED};
use windows::Win32::System::Variant::{VARIANT, VT_BOOL, VT_UI4};

pub fn startup() -> Result<()> {
    unsafe {
        CoInitializeEx(None, COINIT_MULTITHREADED).ok()?;
        MFStartup(MF_VERSION, MFSTARTUP_FULL)
    }
}

pub struct MftEntry {
    pub activate: IMFActivate,
    pub name: String,
    pub hardware: bool,
}

/// 按类别和输入输出格式枚举 MFT。给出 `luid` 时只返回该显卡上的硬件 MFT。
pub fn enum_mfts(
    category: GUID,
    flags: MFT_ENUM_FLAG,
    input: Option<GUID>,
    output: Option<GUID>,
    luid: Option<LUID>,
) -> Result<Vec<MftEntry>> {
    let info = |g: GUID| MFT_REGISTER_TYPE_INFO { guidMajorType: MFMediaType_Video, guidSubtype: g };
    let input = input.map(info);
    let output = output.map(info);
    let mut attrs = None;
    unsafe { MFCreateAttributes(&mut attrs, 1)? };
    let attrs = attrs.unwrap();
    if let Some(l) = luid {
        let bytes = unsafe {
            std::slice::from_raw_parts(&l as *const LUID as *const u8, std::mem::size_of::<LUID>())
        };
        unsafe { attrs.SetBlob(&MFT_ENUM_ADAPTER_LUID, bytes)? };
    }
    let mut ptr: *mut Option<IMFActivate> = std::ptr::null_mut();
    let mut count = 0u32;
    unsafe {
        MFTEnum2(
            category,
            flags,
            input.as_ref().map(|x| x as *const _),
            output.as_ref().map(|x| x as *const _),
            &attrs,
            &mut ptr,
            &mut count,
        )?;
    }
    let mut list = Vec::new();
    if !ptr.is_null() {
        let slice = unsafe { std::slice::from_raw_parts_mut(ptr, count as usize) };
        for a in slice.iter_mut() {
            if let Some(activate) = a.take() {
                let name = friendly_name(&activate).unwrap_or_default();
                let f = unsafe { activate.GetUINT32(&MF_TRANSFORM_FLAGS_Attribute) }.unwrap_or(0);
                list.push(MftEntry { activate, name, hardware: f & MFT_ENUM_FLAG_HARDWARE.0 as u32 != 0 });
            }
        }
        unsafe { CoTaskMemFree(Some(ptr as _)) };
    }
    Ok(list)
}

fn friendly_name(a: &IMFActivate) -> Result<String> {
    let mut p = PWSTR::null();
    let mut len = 0;
    unsafe {
        a.GetAllocatedString(&MFT_FRIENDLY_NAME_Attribute, &mut p, &mut len)?;
        let s = p.to_string().unwrap_or_default();
        CoTaskMemFree(Some(p.0 as _));
        Ok(s)
    }
}

pub fn var_u32(v: u32) -> VARIANT {
    let mut var = VARIANT::default();
    unsafe {
        let inner = &mut *var.Anonymous.Anonymous;
        inner.vt = VT_UI4;
        inner.Anonymous.ulVal = v;
    }
    var
}

pub fn var_bool(b: bool) -> VARIANT {
    let mut var = VARIANT::default();
    unsafe {
        let inner = &mut *var.Anonymous.Anonymous;
        inner.vt = VT_BOOL;
        inner.Anonymous.boolVal = windows::Win32::Foundation::VARIANT_BOOL(if b { -1 } else { 0 });
    }
    var
}

/// 设置编码参数，失败只记录不中断（不同编码器支持的参数不同）。
pub fn set_codec(codec: &ICodecAPI, name: &str, api: &GUID, v: VARIANT, log: &mut Vec<String>) {
    if let Err(e) = unsafe { codec.SetValue(api, &v) } {
        log.push(format!("{name}: {e}"));
    }
}

pub fn video_type(subtype: GUID, w: u32, h: u32, fps: u32) -> Result<IMFMediaType> {
    unsafe {
        let t = MFCreateMediaType()?;
        t.SetGUID(&MF_MT_MAJOR_TYPE, &MFMediaType_Video)?;
        t.SetGUID(&MF_MT_SUBTYPE, &subtype)?;
        t.SetUINT64(&MF_MT_FRAME_SIZE, ((w as u64) << 32) | h as u64)?;
        t.SetUINT64(&MF_MT_FRAME_RATE, ((fps as u64) << 32) | 1)?;
        t.SetUINT64(&MF_MT_PIXEL_ASPECT_RATIO, (1u64 << 32) | 1)?;
        t.SetUINT32(&MF_MT_INTERLACE_MODE, MFVideoInterlace_Progressive.0 as u32)?;
        t.SetUINT32(&MF_MT_YUV_MATRIX, MFVideoTransferMatrix_BT709.0 as u32)?;
        t.SetUINT32(&MF_MT_VIDEO_NOMINAL_RANGE, MFNominalRange_16_235.0 as u32)?;
        Ok(t)
    }
}

pub fn frame_size(t: &IMFMediaType) -> Result<(u32, u32)> {
    let v = unsafe { t.GetUINT64(&MF_MT_FRAME_SIZE)? };
    Ok(((v >> 32) as u32, v as u32))
}

pub enum Out {
    Sample(IMFSample),
    NeedMore,
    StreamChange,
}

/// 调用一次 ProcessOutput。`provides` 为假时由调用方分配 `alloc` 字节的输出缓冲。
pub fn process_output(mft: &IMFTransform, provides: bool, alloc: u32) -> Result<Out> {
    let sample = if provides {
        None
    } else {
        unsafe {
            let s = MFCreateSample()?;
            s.AddBuffer(&MFCreateMemoryBuffer(alloc)?)?;
            Some(s)
        }
    };
    let mut buf = [MFT_OUTPUT_DATA_BUFFER {
        dwStreamID: 0,
        pSample: std::mem::ManuallyDrop::new(sample),
        dwStatus: 0,
        pEvents: std::mem::ManuallyDrop::new(None),
    }];
    let mut status = 0u32;
    let r = unsafe { mft.ProcessOutput(0, &mut buf, &mut status) };
    let [mut b] = buf;
    let s = unsafe { std::mem::ManuallyDrop::take(&mut b.pSample) };
    unsafe { std::mem::ManuallyDrop::drop(&mut b.pEvents) };
    match r {
        Ok(()) => Ok(Out::Sample(s.expect("ProcessOutput 成功但没有返回样本"))),
        Err(e) if e.code() == MF_E_TRANSFORM_NEED_MORE_INPUT => Ok(Out::NeedMore),
        Err(e) if e.code() == MF_E_TRANSFORM_STREAM_CHANGE => Ok(Out::StreamChange),
        Err(e) => Err(e),
    }
}

pub fn sample_bytes(s: &IMFSample) -> Result<Vec<u8>> {
    unsafe {
        let b = s.ConvertToContiguousBuffer()?;
        let mut p = std::ptr::null_mut();
        let mut len = 0u32;
        b.Lock(&mut p, None, Some(&mut len))?;
        let v = std::slice::from_raw_parts(p, len as usize).to_vec();
        b.Unlock()?;
        Ok(v)
    }
}

pub fn memory_sample(data: &[u8]) -> Result<IMFSample> {
    unsafe {
        let b = MFCreateMemoryBuffer(data.len() as u32)?;
        let mut p = std::ptr::null_mut();
        b.Lock(&mut p, None, None)?;
        std::ptr::copy_nonoverlapping(data.as_ptr(), p, data.len());
        b.Unlock()?;
        b.SetCurrentLength(data.len() as u32)?;
        let s = MFCreateSample()?;
        s.AddBuffer(&b)?;
        Ok(s)
    }
}

pub fn output_info(mft: &IMFTransform) -> Result<(bool, u32)> {
    let info = unsafe { mft.GetOutputStreamInfo(0)? };
    let provides = info.dwFlags
        & (MFT_OUTPUT_STREAM_PROVIDES_SAMPLES.0 | MFT_OUTPUT_STREAM_CAN_PROVIDE_SAMPLES.0) as u32
        != 0;
    Ok((provides, info.cbSize))
}

pub fn is_async(mft: &IMFTransform) -> bool {
    unsafe { mft.GetAttributes() }
        .and_then(|a| unsafe { a.GetUINT32(&MF_TRANSFORM_ASYNC) })
        .unwrap_or(0)
        != 0
}

/// 把 IMFMediaEventGenerator 交给另一个线程（MTA 对象，可以跨线程使用）。
pub struct SendEvents(pub IMFMediaEventGenerator);
unsafe impl Send for SendEvents {}

pub fn events_of(mft: &IMFTransform) -> Result<IMFMediaEventGenerator> {
    mft.cast()
}
