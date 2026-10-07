// Package version 记录程序版本，发布构建时用 -ldflags 注入。
package version

// Version 在构建时通过
// -ldflags "-X github.com/Richard2091/DeskBridge/internal/version.Version=v0.1.0"
// 设置；本地开发构建为 "dev"。
var Version = "dev"
