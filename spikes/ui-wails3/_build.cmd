@echo off
set GOPROXY=https://goproxy.cn,direct
cd /d D:\data\AI\Projects\deskbridge\spikes\ui-wails3
echo build-start %TIME%
wails3 build
echo build-exit %ERRORLEVEL% %TIME%
