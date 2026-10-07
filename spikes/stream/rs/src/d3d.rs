//! 显卡、显示输出的枚举和 D3D11 设备创建。

use windows::core::{Interface, Result};
use windows::Win32::Foundation::{HMODULE, LUID, RECT};
use windows::Win32::Graphics::Direct3D::*;
use windows::Win32::Graphics::Direct3D11::*;
use windows::Win32::Graphics::Dxgi::Common::*;
use windows::Win32::Graphics::Dxgi::*;

pub struct Adapter {
    pub index: u32,
    pub adapter: IDXGIAdapter1,
    pub name: String,
    pub vendor_id: u32,
    pub luid: LUID,
    pub software: bool,
}

pub struct Output {
    pub adapter_index: u32,
    pub output_index: u32,
    pub output: IDXGIOutput,
    pub device_name: String,
    pub rect: RECT,
    pub attached: bool,
}

pub fn wstr(s: &[u16]) -> String {
    let n = s.iter().position(|&c| c == 0).unwrap_or(s.len());
    String::from_utf16_lossy(&s[..n])
}

pub fn adapters() -> Result<Vec<Adapter>> {
    let factory: IDXGIFactory1 = unsafe { CreateDXGIFactory1()? };
    let mut list = Vec::new();
    let mut i = 0;
    while let Ok(adapter) = unsafe { factory.EnumAdapters1(i) } {
        let desc = unsafe { adapter.GetDesc1()? };
        list.push(Adapter {
            index: i,
            adapter,
            name: wstr(&desc.Description),
            vendor_id: desc.VendorId,
            luid: desc.AdapterLuid,
            software: desc.Flags & DXGI_ADAPTER_FLAG_SOFTWARE.0 as u32 != 0,
        });
        i += 1;
    }
    Ok(list)
}

pub fn outputs(adapter: &Adapter) -> Result<Vec<Output>> {
    let mut list = Vec::new();
    let mut i = 0;
    while let Ok(output) = unsafe { adapter.adapter.EnumOutputs(i) } {
        let d = unsafe { output.GetDesc()? };
        list.push(Output {
            adapter_index: adapter.index,
            output_index: i,
            output,
            device_name: wstr(&d.DeviceName),
            rect: d.DesktopCoordinates,
            attached: d.AttachedToDesktop.as_bool(),
        });
        i += 1;
    }
    Ok(list)
}

pub fn create_device(adapter: &IDXGIAdapter1) -> Result<(ID3D11Device, ID3D11DeviceContext)> {
    let mut device = None;
    let mut ctx = None;
    unsafe {
        D3D11CreateDevice(
            &adapter.cast::<IDXGIAdapter>()?,
            D3D_DRIVER_TYPE_UNKNOWN,
            HMODULE::default(),
            D3D11_CREATE_DEVICE_BGRA_SUPPORT | D3D11_CREATE_DEVICE_VIDEO_SUPPORT,
            Some(&[D3D_FEATURE_LEVEL_11_1, D3D_FEATURE_LEVEL_11_0]),
            D3D11_SDK_VERSION,
            Some(&mut device),
            None,
            Some(&mut ctx),
        )?;
    }
    let device = device.unwrap();
    // 编码器在别的线程使用同一设备
    let mt: ID3D11Multithread = device.cast()?;
    unsafe {
        let _ = mt.SetMultithreadProtected(true);
    }
    Ok((device, ctx.unwrap()))
}

pub fn texture(
    device: &ID3D11Device,
    w: u32,
    h: u32,
    format: DXGI_FORMAT,
    bind: u32,
    usage: D3D11_USAGE,
    cpu: u32,
) -> Result<ID3D11Texture2D> {
    let desc = D3D11_TEXTURE2D_DESC {
        Width: w,
        Height: h,
        MipLevels: 1,
        ArraySize: 1,
        Format: format,
        SampleDesc: DXGI_SAMPLE_DESC { Count: 1, Quality: 0 },
        Usage: usage,
        BindFlags: bind,
        CPUAccessFlags: cpu,
        MiscFlags: 0,
    };
    let mut t = None;
    unsafe { device.CreateTexture2D(&desc, None, Some(&mut t))? };
    Ok(t.unwrap())
}
