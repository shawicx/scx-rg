package main

// 版本信息由 goreleaser 构建时通过 -ldflags "-X main.version=..." 注入；
// 本地 go build 不注入，保持这里的 dev 默认值。
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)
