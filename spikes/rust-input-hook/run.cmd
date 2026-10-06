@echo off
rem 用户双击运行（普通权限）。运行 120 秒，结果写入 hook.log
cd /d %~dp0
target\release\rust-input-hook.exe 120 hook.log
pause
