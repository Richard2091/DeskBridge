Stop-Process -Name uicore-server -ErrorAction SilentlyContinue
Start-Sleep -Seconds 1
$p = Start-Process -FilePath 'D:\data\AI\Projects\deskbridge\spikes\ui-tauri2\src-tauri\target\release\ui-tauri2.exe' -PassThru
Start-Sleep -Seconds 5
"uicore before: " + @(Get-Process uicore-server -ErrorAction SilentlyContinue).Count
$null = $p.CloseMainWindow()
Start-Sleep -Seconds 4
"app after close: " + @(Get-Process ui-tauri2 -ErrorAction SilentlyContinue).Count
"uicore after close: " + @(Get-Process uicore-server -ErrorAction SilentlyContinue).Count
