# Windows 上的格式检查、静态检查、单元测试和本机构建。
# 用法：powershell -ExecutionPolicy Bypass -File scripts\check.ps1
# 本文件必须保存为带 BOM 的 UTF-8，否则 Windows PowerShell 5.1 会按 GBK 读取中文。
$ErrorActionPreference = 'Stop'
Set-Location (Join-Path $PSScriptRoot '..')

$StaticcheckVersion = '2026.2.1'
$Helper = 'desktop-helper/Cargo.toml'

function Step([string]$Name, [scriptblock]$Block) {
    Write-Host "== $Name"
    & $Block
    if ($LASTEXITCODE -ne 0) { throw "$Name 失败，退出码 $LASTEXITCODE" }
}

Step 'gofmt' {
    $unformatted = gofmt -l cmd internal
    if ($unformatted) { Write-Host "以下文件需要 gofmt：`n$unformatted"; $global:LASTEXITCODE = 1 }
}
Step 'go vet' { go vet ./... }
Step 'staticcheck' { go run "honnef.co/go/tools/cmd/staticcheck@$StaticcheckVersion" ./... }
Step 'go test' { go test ./... }
Step 'go build' { go build -trimpath -o dist/windows-amd64/deskbridge.exe ./cmd/deskbridge }

Step 'cargo fmt' { cargo fmt --manifest-path $Helper --check }
Step 'cargo clippy' { cargo clippy --manifest-path $Helper --all-targets --locked -- -D warnings }
Step 'cargo test' { cargo test --manifest-path $Helper --locked }
Step 'cargo build' { cargo build --manifest-path $Helper --release --locked }

Write-Host '全部通过'
