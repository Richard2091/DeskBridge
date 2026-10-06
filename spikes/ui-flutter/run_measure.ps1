# ASCII only. Launch release app with autostream, measure, close gracefully, check child exit.
$rel = 'D:\data\AI\Projects\deskbridge\spikes\ui-flutter\build\windows\x64\runner\Release'
$env:DESKBRIDGE_AUTOSTREAM = '1'
$p = Start-Process -FilePath "$rel\deskbridge_ui.exe" -PassThru
"pid: $($p.Id)"
Start-Sleep -Seconds 5
"title@5s: " + (Get-Process -Id $p.Id).MainWindowTitle
powershell -NoProfile -ExecutionPolicy Bypass -File D:\data\AI\Projects\deskbridge\spikes\ui-core\measure.ps1 -RootPid $p.Id -Seconds 20
"title@end: " + (Get-Process -Id $p.Id).MainWindowTitle
$null = $p.CloseMainWindow()
Start-Sleep -Seconds 3
"app alive after close: " + [bool](Get-Process -Id $p.Id -ErrorAction SilentlyContinue)
"uicore alive after close: " + [bool](Get-Process -Name uicore-server -ErrorAction SilentlyContinue)
if (Get-Process -Id $p.Id -ErrorAction SilentlyContinue) { Stop-Process -Id $p.Id -Force }
"DONE"