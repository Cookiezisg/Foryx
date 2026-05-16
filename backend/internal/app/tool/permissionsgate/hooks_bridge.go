// hooks_bridge.go — registers MatchesRule with app/hooks at init time
// so hooks pkg's per-hook "if" filter uses the real glob matcher without
// pulling in permissionsgate as a direct dep (avoids import cycle).
//
// hooks_bridge.go ——init 时把 MatchesRule 注册给 app/hooks，让 hooks
// 包的 per-hook "if" filter 用真实 glob 匹配器，又不让 hooks 直接依赖
// permissionsgate（避循环 import）。
package permissionsgate

import (
	hooksapp "github.com/sunweilin/forgify/backend/internal/app/hooks"
)

func init() {
	hooksapp.SetMatchesRule(MatchesRule)
}
