@echo off
rem 编译，输出写入 build.log
cd /d %~dp0
(
  cargo build --release
  echo BUILD_EXIT=%ERRORLEVEL%
) > build.log 2>&1
