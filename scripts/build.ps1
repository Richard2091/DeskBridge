# 交叉编译 deskbridge，输出到 dist/<系统>-<架构>/。
# 用法：powershell -ExecutionPolicy Bypass -File scripts\build.ps1 [-Version v0.1.0]
# 本文件必须保存为带 BOM 的 UTF-8，否则 Windows PowerShell 5.1 会按 GBK 读取中文。
param([string]$Version = 'dev')
$ErrorActionPreference = 'Stop'
Set-Location (Join-Path $PSScriptRoot '..')

$ldflags = "-s -w -X github.com/Richard2091/DeskBridge/internal/version.Version=$Version"
$targets = 'windows/amd64', 'windows/arm64', 'linux/amd64', 'linux/arm64', 'darwin/amd64', 'darwin/arm64'

$env:CGO_ENABLED = '0'
try {
    foreach ($target in $targets) {
        $goos, $goarch = $target -split '/'
        $ext = if ($goos -eq 'windows') { '.exe' } else { '' }
        $out = "dist/$goos-$goarch/deskbridge$ext"
        Write-Host "编译 $out"
        $env:GOOS = $goos
        $env:GOARCH = $goarch
        go build -trimpath -ldflags $ldflags -o $out ./cmd/deskbridge
        if ($LASTEXITCODE -ne 0) { throw "编译 $target 失败，退出码 $LASTEXITCODE" }
    }
} finally {
    Remove-Item Env:GOOS, Env:GOARCH, Env:CGO_ENABLED -ErrorAction SilentlyContinue
}
Write-Host '全部编译完成'
