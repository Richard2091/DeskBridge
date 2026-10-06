@echo off
rem 添加依赖（取最新版本）并编译，输出写入 build.log
cd /d %~dp0
(
  cargo add uiautomation xcap serde_json
  cargo add windows --features Win32_Foundation,Win32_UI_HiDpi,Win32_UI_WindowsAndMessaging,Win32_UI_Input_KeyboardAndMouse
  cargo build --release
  echo BUILD_EXIT=%ERRORLEVEL%
) > build.log 2>&1
