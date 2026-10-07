#!/bin/sh
# Go 的格式检查、静态检查和单元测试。desktop-helper 只在 Windows 上检查，见 check.ps1。
set -eu
cd "$(dirname "$0")/.."

STATICCHECK_VERSION=2026.2.1

echo "== gofmt"
unformatted=$(gofmt -l cmd internal)
if [ -n "$unformatted" ]; then
	echo "以下文件需要 gofmt：" >&2
	echo "$unformatted" >&2
	exit 1
fi

echo "== go vet"
go vet ./...

echo "== staticcheck"
go run "honnef.co/go/tools/cmd/staticcheck@$STATICCHECK_VERSION" ./...

echo "== go test"
go test ./...

echo "全部通过"
