@echo off
set GOPROXY=https://goproxy.cn,direct
set GOFLAGS=-mod=mod
cd /d D:\data\AI\Projects\deskbridge\spikes\ui-wails3
echo tidy-start %TIME%
go mod edit -go=1.26.0 -require=deskbridge-spikes/ui-core@v0.0.0 -replace=deskbridge-spikes/ui-core=../ui-core
go mod tidy
echo tidy-exit %ERRORLEVEL% %TIME%