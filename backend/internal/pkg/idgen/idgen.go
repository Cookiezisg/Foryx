// Package idgen mints business entity IDs in the project standard
// "<prefix>_<16hex>" form. See §S15.
//
// Package idgen 按项目标准 "<prefix>_<16hex>" 形式生成业务实体 ID。见 §S15。
package idgen

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// New returns "<prefix>_<16hex>". prefix should be the domain's stable short
// tag (e.g. "aki", "f", "msg"). Panics on rand.Read failure: a broken entropy
// source would silently produce colliding IDs.
//
// New 返回 "<prefix>_<16hex>"。prefix 取 domain 稳定短标签（如 "aki" / "f" / "msg"）。
// rand.Read 失败时 panic——熵源损坏会静默产生碰撞 ID。
func New(prefix string) string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Sprintf("idgen: crypto/rand failed: %v", err))
	}
	return prefix + "_" + hex.EncodeToString(b[:])
}
