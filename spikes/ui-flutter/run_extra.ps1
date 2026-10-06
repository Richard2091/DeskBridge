$rel = 'D:\data\AI\Projects\deskbridge\spikes\ui-flutter\build\windows\x64\runner\Release'
$env:DESKBRIDGE_AUTOSTREAM = '1'
$p = Start-Process -FilePath "$rel\deskbridge_ui.exe" -PassThru
Start-Sleep -Seconds 4
$u = Get-Process -Name uicore-server
"uicore window handle: " + $u.MainWindowHandle + " title='" + $u.MainWindowTitle + "'"
Stop-Process -Id $p.Id -Force
Start-Sleep -Seconds 2
"after force-kill of app, uicore alive: " + [bool](Get-Process -Name uicore-server -ErrorAction SilentlyContinue)
Get-Process -Name uicore-server -ErrorAction SilentlyContinue | Stop-Process -Force
Start-Sleep -Seconds 1
"--- second measurement"
$p = Start-Process -FilePath "$rel\deskbridge_ui.exe" -PassThru
Start-Sleep -Seconds 5
powershell -NoProfile -ExecutionPolicy Bypass -File D:\data\AI\Projects\deskbridge\spikes\ui-core\measure.ps1 -RootPid $p.Id -Seconds 20
$null = $p.CloseMainWindow()
Start-Sleep -Seconds 3
"app alive: " + [bool](Get-Process -Id $p.Id -ErrorAction SilentlyContinue)
"uicore alive: " + [bool](Get-Process -Name uicore-server -ErrorAction SilentlyContinue)
if (Get-Process -Id $p.Id -ErrorAction SilentlyContinue) { Stop-Process -Id $p.Id -Force }
"DONE"