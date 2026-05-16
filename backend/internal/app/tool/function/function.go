// Package function provides system tools for the LLM to interact with the user's function library.
//
// Package function 提供操作用户 function 库的 system tool。
package function

import (
	"go.uber.org/zap"

	functionapp "github.com/sunweilin/forgify/backend/internal/app/function"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	apikeydomain "github.com/sunweilin/forgify/backend/internal/domain/apikey"
	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
	forgepkg "github.com/sunweilin/forgify/backend/internal/pkg/forge"
)

// FunctionTools constructs the function system tools; pass a noop forge Publisher to disable double-write in tests.
//
// FunctionTools 构造 function system tool；测试 / 未接线时传 noop forge Publisher 关闭双写。
func FunctionTools(
	svc *functionapp.Service,
	picker modeldomain.ModelPicker,
	keys apikeydomain.KeyProvider,
	factory *llminfra.Factory,
	forge forgepkg.Publisher,
	log *zap.Logger,
) []toolapp.Tool {
	return []toolapp.Tool{
		&SearchFunction{svc: svc, picker: picker, keys: keys, factory: factory, log: log},
		&GetFunction{svc: svc},
		&CreateFunction{svc: svc, picker: picker, keys: keys, factory: factory, forge: forge},
		&EditFunction{svc: svc, picker: picker, keys: keys, factory: factory, forge: forge},
		&RevertFunction{svc: svc},
		&DeleteFunction{svc: svc},
		&RunFunction{svc: svc},
		&SearchFunctionExecutions{svc: svc},
		&GetFunctionExecution{svc: svc},
	}
}
