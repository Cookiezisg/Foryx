// Package tokencount provides cheap heuristic token estimation with optional calibration.
//
// Package tokencount 提供便宜启发式 token 估算，可选 calibration 收敛。
package tokencount

import "unicode"

// Estimate returns a rough token count for s: CJK rune = 1 token, others = runes/4.
//
// Estimate 返 s 的粗估 token 数：CJK rune 每字 1 token，其余按 runes/4。
func Estimate(s string) int {
	if s == "" {
		return 0
	}
	var cjk, ascii int
	for _, r := range s {
		if isCJK(r) {
			cjk++
		} else {
			ascii++
		}
	}
	out := cjk + ascii/4
	if out == 0 {
		out = 1
	}
	return out
}

// EstimateBytes is the []byte form of Estimate.
//
// EstimateBytes 是 Estimate 的 []byte 版本。
func EstimateBytes(b []byte) int {
	return Estimate(bytesToString(b))
}

// Calibrate returns the (actual / estimated) ratio clamped to [0.5, 3.0].
//
// Calibrate 返 (actual / estimated) 比例，clamp 到 [0.5, 3.0]。
func Calibrate(actual, estimated int) float64 {
	if actual <= 0 || estimated <= 0 {
		return 1.0
	}
	r := float64(actual) / float64(estimated)
	if r < 0.5 {
		r = 0.5
	}
	if r > 3.0 {
		r = 3.0
	}
	return r
}

// MergeCalibration combines prev and fresh via exponential smoothing (α=0.3, stability-biased).
//
// MergeCalibration 用指数平滑（α=0.3，偏稳定）合并旧/新校准比例。
func MergeCalibration(prev, fresh float64) float64 {
	if prev <= 0 {
		return fresh
	}
	const alpha = 0.3
	return prev*(1-alpha) + fresh*alpha
}

func isCJK(r rune) bool {
	if !unicode.IsPrint(r) {
		return false
	}
	switch {
	case r >= 0x4E00 && r <= 0x9FFF:
		return true
	case r >= 0x3040 && r <= 0x30FF:
		return true
	case r >= 0xAC00 && r <= 0xD7AF:
		return true
	case r >= 0x3400 && r <= 0x4DBF:
		return true
	}
	return false
}

func bytesToString(b []byte) string {
	return string(b)
}
