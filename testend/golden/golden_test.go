// Package golden holds the real-model LLM journeys (make evals): the same black-box
// harness, but the model is real — gated behind EVALS=1 so the suite never burns tokens
// by accident. Populated in wave W7.
//
// Package golden 放真模型 LLM 旅程（make evals）：同一套黑盒 harness，但模型是真的——
// EVALS=1 门控，绝不意外烧钱。W7 波填充。
package golden

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	if os.Getenv("EVALS") == "" {
		os.Exit(0) // gated: only runs via make evals. 门控：仅 make evals 触发。
	}
	os.Exit(m.Run())
}
