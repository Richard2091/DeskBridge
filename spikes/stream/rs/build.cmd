@echo off
cd /d %~dp0
cargo build --release > build.log 2>&1
echo BUILD_EXIT=%ERRORLEVEL% >> build.log
