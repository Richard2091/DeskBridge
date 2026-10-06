$lock = 'D:\data\AI\Projects\deskbridge\spikes\measure.lock'
$tries = 0
while ($true) {
  try { New-Item -ItemType Directory -Path $lock -ErrorAction Stop | Out-Null; break } catch { $tries++; if ($tries -gt 40) { "LOCK_FAIL"; exit 1 }; Start-Sleep -Seconds 10 }
}
"LOCK_ACQUIRED"
try {
  $env:DESKBRIDGE_AUTOSTREAM = '1'
  $env:WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS = '--remote-debugging-port=9222'
  $p = Start-Process -FilePath 'D:\data\AI\Projects\deskbridge\spikes\ui-tauri2\src-tauri\target\release\ui-tauri2.exe' -PassThru
  "STARTED"
  Start-Sleep -Seconds 8
  node D:\data\AI\Projects\deskbridge\spikes\ui-tauri2\cdp.mjs
  $null = $p.CloseMainWindow(); Start-Sleep -Seconds 3
  if (!$p.HasExited) { Stop-Process -Id $p.Id }
  "uicore left: " + @(Get-Process uicore-server -ErrorAction SilentlyContinue).Count
} finally { Remove-Item -Recurse -Force $lock; "LOCK_RELEASED" }
