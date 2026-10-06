@echo off
cd /d D:\data\AI\Projects\deskbridge\spikes\ui-tauri2
echo START %TIME% > step1.log
if not exist src-tauri\binaries mkdir src-tauri\binaries
pushd ..\ui-core
go build -o ..\ui-tauri2\src-tauri\binaries\uicore-server-x86_64-pc-windows-msvc.exe .\cmd\uicore-server >> ..\ui-tauri2\step1.log 2>&1
popd
echo GOBUILD_DONE %TIME% >> step1.log
call pnpm install >> step1.log 2>&1
echo INSTALL_DONE %TIME% >> step1.log
call pnpm tauri build --no-bundle >> step1.log 2>&1
echo BUILD_EXIT %ERRORLEVEL% %TIME% >> step1.log
