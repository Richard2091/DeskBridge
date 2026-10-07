//! 用 D3D11 视频处理器把 BGRA 画面缩放并转成 NV12，全程在显卡上完成。

use crate::d3d::texture;
use crate::stamp;
use windows::core::{Interface, Result};
use windows::Win32::Graphics::Direct3D11::*;
use windows::Win32::Graphics::Dxgi::Common::*;

pub struct Converter {
    vctx: ID3D11VideoContext,
    vp: ID3D11VideoProcessor,
    /// 采集到的画面先复制到这里，再做转换。
    pub src: ID3D11Texture2D,
    src_view: ID3D11VideoProcessorInputView,
    outs: Vec<(ID3D11Texture2D, ID3D11VideoProcessorOutputView)>,
    next: usize,
    pub out_w: u32,
    pub out_h: u32,
}

impl Converter {
    /// `ring` 个输出纹理轮流使用，避免编码器还在读时被覆盖。
    pub fn new(
        device: &ID3D11Device,
        ctx: &ID3D11DeviceContext,
        src_w: u32,
        src_h: u32,
        out_w: u32,
        out_h: u32,
        ring: usize,
    ) -> Result<Self> {
        let vdev: ID3D11VideoDevice = device.cast()?;
        let vctx: ID3D11VideoContext = ctx.cast()?;
        let rate = DXGI_RATIONAL { Numerator: 60, Denominator: 1 };
        let cdesc = D3D11_VIDEO_PROCESSOR_CONTENT_DESC {
            InputFrameFormat: D3D11_VIDEO_FRAME_FORMAT_PROGRESSIVE,
            InputFrameRate: rate,
            InputWidth: src_w,
            InputHeight: src_h,
            OutputFrameRate: rate,
            OutputWidth: out_w,
            OutputHeight: out_h,
            Usage: D3D11_VIDEO_USAGE_OPTIMAL_SPEED,
        };
        let venum = unsafe { vdev.CreateVideoProcessorEnumerator(&cdesc)? };
        let vp = unsafe { vdev.CreateVideoProcessor(&venum, 0)? };

        let src = texture(
            device,
            src_w,
            src_h,
            DXGI_FORMAT_B8G8R8A8_UNORM,
            (D3D11_BIND_RENDER_TARGET.0 | D3D11_BIND_SHADER_RESOURCE.0) as u32,
            D3D11_USAGE_DEFAULT,
            0,
        )?;
        let ivd = D3D11_VIDEO_PROCESSOR_INPUT_VIEW_DESC {
            FourCC: 0,
            ViewDimension: D3D11_VPIV_DIMENSION_TEXTURE2D,
            Anonymous: D3D11_VIDEO_PROCESSOR_INPUT_VIEW_DESC_0 {
                Texture2D: D3D11_TEX2D_VPIV { MipSlice: 0, ArraySlice: 0 },
            },
        };
        let mut src_view = None;
        unsafe { vdev.CreateVideoProcessorInputView(&src, &venum, &ivd, Some(&mut src_view))? };

        let ovd = D3D11_VIDEO_PROCESSOR_OUTPUT_VIEW_DESC {
            ViewDimension: D3D11_VPOV_DIMENSION_TEXTURE2D,
            Anonymous: D3D11_VIDEO_PROCESSOR_OUTPUT_VIEW_DESC_0 {
                Texture2D: D3D11_TEX2D_VPOV { MipSlice: 0 },
            },
        };
        let mut outs = Vec::new();
        for _ in 0..ring {
            // 部分驱动要求编码器输入带 VIDEO_ENCODER 绑定标志，不支持时退回只用渲染目标
            let t = texture(
                device,
                out_w,
                out_h,
                DXGI_FORMAT_NV12,
                (D3D11_BIND_RENDER_TARGET.0 | D3D11_BIND_VIDEO_ENCODER.0) as u32,
                D3D11_USAGE_DEFAULT,
                0,
            )
            .or_else(|_| {
                texture(
                    device,
                    out_w,
                    out_h,
                    DXGI_FORMAT_NV12,
                    D3D11_BIND_RENDER_TARGET.0 as u32,
                    D3D11_USAGE_DEFAULT,
                    0,
                )
            })?;
            let mut v = None;
            unsafe { vdev.CreateVideoProcessorOutputView(&t, &venum, &ovd, Some(&mut v))? };
            outs.push((t, v.unwrap()));
        }

        unsafe {
            // 输入：RGB 全范围；输出：BT.709、16–235
            vctx.VideoProcessorSetStreamColorSpace(&vp, 0, &D3D11_VIDEO_PROCESSOR_COLOR_SPACE { _bitfield: 0 });
            vctx.VideoProcessorSetOutputColorSpace(
                &vp,
                &D3D11_VIDEO_PROCESSOR_COLOR_SPACE { _bitfield: 4 | (1 << 4) },
            );
            vctx.VideoProcessorSetStreamFrameFormat(&vp, 0, D3D11_VIDEO_FRAME_FORMAT_PROGRESSIVE);
            vctx.VideoProcessorSetStreamAutoProcessingMode(&vp, 0, false);
        }

        Ok(Self {
            vctx,
            vp,
            src,
            src_view: src_view.unwrap(),
            outs,
            next: 0,
            out_w,
            out_h,
        })
    }

    /// 把 `src` 转换到下一个输出纹理并返回它。
    pub fn convert(&mut self) -> Result<ID3D11Texture2D> {
        let (tex, view) = &self.outs[self.next];
        self.next = (self.next + 1) % self.outs.len();
        let stream = D3D11_VIDEO_PROCESSOR_STREAM {
            Enable: true.into(),
            OutputIndex: 0,
            InputFrameOrField: 0,
            PastFrames: 0,
            FutureFrames: 0,
            ppPastSurfaces: std::ptr::null_mut(),
            pInputSurface: std::mem::ManuallyDrop::new(Some(self.src_view.clone())),
            ppFutureSurfaces: std::ptr::null_mut(),
            ppPastSurfacesRight: std::ptr::null_mut(),
            pInputSurfaceRight: std::mem::ManuallyDrop::new(None),
            ppFutureSurfacesRight: std::ptr::null_mut(),
        };
        let streams = [stream];
        let r = unsafe { self.vctx.VideoProcessorBlt(&self.vp, view, 0, &streams) };
        let [mut stream] = streams;
        unsafe { std::mem::ManuallyDrop::drop(&mut stream.pInputSurface) };
        r?;
        Ok(tex.clone())
    }
}

/// 从原始分辨率的 BGRA 画面里读回时间戳方块。
pub struct StampReader {
    staging: ID3D11Texture2D,
    w: u32,
}

impl StampReader {
    pub fn new(device: &ID3D11Device, src_w: u32) -> Result<Option<Self>> {
        let w = stamp::width();
        if src_w < w {
            return Ok(None);
        }
        let staging = texture(
            device,
            w,
            stamp::BLOCK,
            DXGI_FORMAT_B8G8R8A8_UNORM,
            0,
            D3D11_USAGE_STAGING,
            D3D11_CPU_ACCESS_READ.0 as u32,
        )?;
        Ok(Some(Self { staging, w }))
    }

    pub fn read(&self, ctx: &ID3D11DeviceContext, src: &ID3D11Texture2D) -> Result<Option<u32>> {
        let bx = D3D11_BOX { left: 0, top: 0, front: 0, right: self.w, bottom: stamp::BLOCK, back: 1 };
        unsafe {
            ctx.CopySubresourceRegion(&self.staging, 0, 0, 0, 0, src, 0, Some(&bx));
            let mut m = D3D11_MAPPED_SUBRESOURCE::default();
            ctx.Map(&self.staging, 0, D3D11_MAP_READ, 0, Some(&mut m))?;
            let base = m.pData as *const u8;
            let pitch = m.RowPitch as usize;
            let v = stamp::decode(|i| {
                let (x, y) = stamp::center(i);
                let p = base.add(y as usize * pitch + x as usize * 4);
                ((*p as u32 + *p.add(1) as u32 + *p.add(2) as u32) / 3) as u8
            });
            ctx.Unmap(&self.staging, 0);
            Ok(v)
        }
    }
}
