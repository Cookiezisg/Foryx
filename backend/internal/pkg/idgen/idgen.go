// Package idgen mints business entity IDs in the project's standard
// "<prefix>_<16hex>" form (8 bytes / 64 bits of entropy from crypto/rand).
//
// Per §S15: rand.Read failure means a broken entropy source — continuing
// would produce colliding IDs, so we panic to fail loudly.
//
// Package idgen 按项目标准 "<prefix>_<16hex>" 形式（crypto/rand 取 8 字节 /
// 64 位熵）生成业务实体 ID。
//
// 按 §S15：rand.Read 失败说明熵源损坏——继续会产生碰撞 ID，因此 panic 立刻失败。
package idgen

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// New returns "<prefix>_<16hex>". prefix should be the domain's stable short
// tag (e.g. "aki" for apikey, "f" for forge, "msg" for message). Empty prefix
// is allowed but discouraged — typed IDs aid grep/debug.
//
// New 返回 "<prefix>_<16hex>"。prefix 取 domain 稳定短标签（如 apikey 用 "aki"、
// forge 用 "f"、message 用 "msg"）。允许空 prefix 但不建议——带类型的 ID 利于 grep/调试。
func New(prefix string) string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		// Entropy source is broken; continuing risks ID collisions.
		// 熵源损坏；继续会发生 ID 碰撞。
		panic(fmt.Sprintf("idgen: crypto/rand failed: %v", err))
	}
	return prefix + "_" + hex.EncodeToString(b[:])
}
