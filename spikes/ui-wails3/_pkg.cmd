@echo off
set GOPROXY=https://goproxy.cn,direct
set PATH=%PATH%;C:\Program Files (x86)\Windows Kits\10\bin\10.0.26100.0\x64
cd /d D:\data\AI\Projects\deskbridge\spikes\ui-wails3
echo pkg-start %TIME%
wails3 package FORMAT=msix
echo pkg-exit %ERRORLEVEL% %TIME%