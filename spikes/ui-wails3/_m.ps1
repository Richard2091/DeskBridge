param([string]$Mode = "asset")
$env:DESKBRIDGE_AUTOSTREAM = "1"
if ($Mode -eq "tcp") { $env:DESKBRIDGE_API_MODE = "tcp" }
$env:DESKBRIDGE_LOG = "D:\data\AI\Projects\deskbridge\spikes\ui-wails3\_app.log"
Remove-Item $env:DESKBRIDGE_LOG -ErrorAction SilentlyContinue
$exe = "D:\data\AI\Projects\deskbridge\spikes\ui-wails3\bin\uiwails3.exe"
$p = Start-Process $exe -PassThru
Write-Output ("pid " + $p.Id + " mode " + $Mode)
Start-Sleep -Seconds 5
Write-Output ("title_at_5s: " + (Get-Process -Id $p.Id).MainWindowTitle)
& powershell -NoProfile -ExecutionPolicy Bypass -File D:\data\AI\Projects\deskbridge\spikes\ui-core\measure.ps1 -RootPid $p.Id -Seconds 20
Write-Output ("title_after: " + (Get-Process -Id $p.Id).MainWindowTitle)
Get-CimInstance Win32_Process | Where-Object { $_.ProcessId -eq $p.Id -or $_.ParentProcessId -eq $p.Id } | Out-Null
Stop-Process -Id $p.Id -Force
Write-Output "--- app log"
Get-Content $env:DESKBRIDGE_LOG -Encoding UTF8
Start-Sleep -Seconds 2
Get-Process msedgewebview2 -ErrorAction SilentlyContinue | Out-Null