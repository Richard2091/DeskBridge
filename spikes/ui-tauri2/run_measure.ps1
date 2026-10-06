$ErrorActionPreference = 'Continue'
$lock = 'D:\data\AI\Projects\deskbridge\spikes\measure.lock'
$tries = 0
while ($true) {
  try { New-Item -ItemType Directory -Path $lock -ErrorAction Stop | Out-Null; break } catch { $tries++; if ($tries -gt 40) { "LOCK_FAIL"; exit 1 }; Start-Sleep -Seconds 30 }
}
"LOCK_ACQUIRED"
try {
  $env:DESKBRIDGE_AUTOSTREAM = '1'
  $p = Start-Process -FilePath 'D:\data\AI\Projects\deskbridge\spikes\ui-tauri2\src-tauri\target\release\ui-tauri2.exe' -PassThru
  "PID " + $p.Id
  Start-Sleep -Seconds 5
  "title_before: " + (Get-Process -Id $p.Id).MainWindowTitle
  powershell -NoProfile -ExecutionPolicy Bypass -File D:\data\AI\Projects\deskbridge\spikes\ui-core\measure.ps1 -RootPid $p.Id -Seconds 20
  Stop-Process -Id $p.Id
  Start-Sleep -Seconds 3
  "after_exit uicore-server: " + @(Get-Process uicore-server -ErrorAction SilentlyContinue).Count
  "after_exit ui-tauri2: " + @(Get-Process ui-tauri2 -ErrorAction SilentlyContinue).Count
  "after_exit msedgewebview2 under app: n/a"
} finally {
  Remove-Item -Recurse -Force $lock
  "LOCK_RELEASED"
}
"DONE"
