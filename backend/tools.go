//go:build tools

// Package tools pins development tool versions as build dependencies.
// The `tools` build tag keeps this file out of normal builds + tests; it only
// exists so `go mod tidy` records these tools' versions in go.mod / go.sum.
//
// Install (uses pinned version from go.mod when run from this module):
//
//	go install honnef.co/go/tools/cmd/staticcheck
//	go install golang.org/x/tools/cmd/deadcode
//	go install github.com/air-verse/air
//
// Why: `@latest` drift bites you 6 months later when a new lint rule reds 100
// places that used to be green. Pinning makes "fresh install on new machine"
// produce identical behavior to "what worked yesterday".
//
// Package tools 把开发工具的版本作为 build 依赖固定下来。
// `tools` build tag 让本文件不进正常 build / 测试；仅用于让 `go mod tidy`
// 把工具版本写进 go.mod / go.sum。
//
// 安装方式（在本模块目录下跑，用 go.mod 里钉死的版本）：
//
//	go install honnef.co/go/tools/cmd/staticcheck
//	go install golang.org/x/tools/cmd/deadcode
//	go install github.com/air-verse/air
//
// 为什么：`@latest` 会漂移——半年后新的 lint 规则把以前绿的 100 处标红，
// 你以为代码漂了其实是工具漂了。锁版本让"换机重装"和"昨天能跑"行为一致。
package tools

import (
	_ "github.com/air-verse/air"
	_ "golang.org/x/tools/cmd/deadcode"
	_ "honnef.co/go/tools/cmd/staticcheck"
)
