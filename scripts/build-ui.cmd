@echo off
rem 构建 Windows 客户端：Go 程序（deskbridge.exe）与 Flutter 客户端放在同一目录。
rem 用法：scripts\build-ui.cmd [Flutter 安装目录]
rem 输出：ui\build\windows\x64\runner\Release（目录）和 dist\deskbridge-client-windows-amd64.zip（压缩包）
setlocal
set FLUTTER_DIR=%~1
if "%FLUTTER_DIR%"=="" set FLUTTER_DIR=D:\data\AI\Projects\DeskBridgeTools\flutter
set PUB_HOSTED_URL=https://pub.flutter-io.cn
set FLUTTER_STORAGE_BASE_URL=https://storage.flutter-io.cn
set ROOT=%~dp0..
cd /d %ROOT%
set CGO_ENABLED=0
go build -trimpath -ldflags "-s -w" -o ui\deskbridge.exe .\cmd\deskbridge || (echo GO_BUILD_FAIL & exit /b 1)
cd /d %ROOT%\ui
call "%FLUTTER_DIR%\bin\flutter.bat" build windows --release
if errorlevel 1 (echo FLUTTER_BUILD_FAIL & exit /b 1)
copy /y deskbridge.exe build\windows\x64\runner\Release\ >nul
cd /d %ROOT%
if not exist dist mkdir dist
powershell -NoProfile -Command "Compress-Archive -Force -Path 'ui\build\windows\x64\runner\Release\*' -DestinationPath 'dist\deskbridge-client-windows-amd64.zip'" || (echo ZIP_FAIL & exit /b 1)
echo BUILD_UI_OK
