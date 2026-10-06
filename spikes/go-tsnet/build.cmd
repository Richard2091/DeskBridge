@echo off
rem 获取依赖并编译，输出写入 build.log
cd /d %~dp0
(
  go get tailscale.com@latest
  go mod tidy
  go build -ldflags="-s -w" -o go-tsnet.exe .
  echo BUILD_EXIT=%ERRORLEVEL%
) > build.log 2>&1
