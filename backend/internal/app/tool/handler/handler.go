// Package handler provides system tools for the LLM to interact with the user's handler library.
//
// Package handler 提供操作用户 handler 库的 system tool。
package handler

import (
	"go.uber.org/zap"

	handlerapp "github.com/sunweilin/forgify/backend/internal/app/handler"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	apikeydomain "github.com/sunweilin/forgify/backend/internal/domain/apikey"
	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
	forgepkg "github.com/sunweilin/forgify/backend/internal/pkg/forge"
)

// HandlerTools constructs handler system tools; pass a noop forge Publisher to disable double-write in tests.
//
// HandlerTools 构造 handler system tool；测试 / 未接线时传 noop forge Publisher 关闭双写。
func HandlerTools(
	svc *handlerapp.Service,
	picker modeldomain.ModelPicker,
	keys apikeydomain.KeyProvider,
	factory *llminfra.Factory,
	forge forgepkg.Publisher,
	log *zap.Logger,
) []toolapp.Tool {
	return []toolapp.Tool{
		&SearchHandler{svc: svc, picker: picker, keys: keys, factory: factory, log: log},
		&GetHandler{svc: svc},
		&CreateHandler{svc: svc, picker: picker, keys: keys, factory: factory, forge: forge},
		&EditHandler{svc: svc, picker: picker, keys: keys, factory: factory, forge: forge},
		&RevertHandler{svc: svc},
		&DeleteHandler{svc: svc},
		&CallHandler{svc: svc},
		&UpdateHandlerConfig{svc: svc},
		&SearchHandlerCalls{svc: svc},
		&GetHandlerCall{svc: svc},
	}
}
