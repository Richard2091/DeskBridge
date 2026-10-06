@echo off
rem build go core + flutter release, copy core into release dir
set PUB_HOSTED_URL=https://pub.flutter-io.cn
set FLUTTER_STORAGE_BASE_URL=https://storage.flutter-io.cn
set ROOT=D:\data\AI\Projects\deskbridge\spikes
cd /d %ROOT%\ui-core
go build -o %ROOT%\ui-flutter\uicore-server.exe ./cmd/uicore-server
if errorlevel 1 (echo GO_BUILD_FAIL & exit /b 1)
cd /d %ROOT%\ui-flutter
call D:\data\AI\Projects\DeskBridgeTools\flutter\bin\flutter.bat build windows --release
set RC=%ERRORLEVEL%
if "%RC%"=="0" copy /y %ROOT%\ui-flutter\uicore-server.exe %ROOT%\ui-flutter\build\windows\x64\runner\Release\ >nul
echo BUILD_EXIT %RC%