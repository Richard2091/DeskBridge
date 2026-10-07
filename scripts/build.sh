#!/bin/sh
# 交叉编译 deskbridge，输出到 dist/<系统>-<架构>/。
# 用法：scripts/build.sh [版本号]，不给版本号时为 dev。
set -eu
cd "$(dirname "$0")/.."

version=${1:-dev}
ldflags="-s -w -X github.com/Richard2091/DeskBridge/internal/version.Version=$version"

for target in windows/amd64 windows/arm64 linux/amd64 linux/arm64 darwin/amd64 darwin/arm64; do
	goos=${target%/*}
	goarch=${target#*/}
	ext=""
	[ "$goos" = windows ] && ext=".exe"
	out="dist/$goos-$goarch/deskbridge$ext"
	echo "编译 $out"
	CGO_ENABLED=0 GOOS=$goos GOARCH=$goarch go build -trimpath -ldflags "$ldflags" -o "$out" ./cmd/deskbridge
done
