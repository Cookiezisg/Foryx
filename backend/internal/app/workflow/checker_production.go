// checker_production.go — production CapabilityChecker backed by the live
// function / handler / skill / mcp services. Used in main.go DI wiring.
//
// Resolution policy:
//   - HasFunction(id):   functionapp.Get(ctx, id) → ErrNotFound = false
//   - HasHandler(name):  handlerapp.GetByName(ctx, name) → ErrNotFound = false
//   - HasSkill(name):    skillapp.Get(ctx, name) → ErrSkillNotFound = false
//   - HasMCPServer(name): mcpapp.GetServer(ctx, name) → ErrServerNotFound = false
//
// Any other error from these services bubbles up (treated as transient by
// caller — validation will report ErrInvalidReference + the underlying message).
//
// checker_production.go — 接生产 service 的 CapabilityChecker;main.go 用。
// 各服务 ErrNotFound = 不存在;其他错误透传(调用方按 transient 报)。

package workflow

import (
	"context"
	"errors"

	functionapp "github.com/sunweilin/forgify/backend/internal/app/function"
	handlerapp "github.com/sunweilin/forgify/backend/internal/app/handler"
	mcpapp "github.com/sunweilin/forgify/backend/internal/app/mcp"
	skillapp "github.com/sunweilin/forgify/backend/internal/app/skill"
	functiondomain "github.com/sunweilin/forgify/backend/internal/domain/function"
	handlerdomain "github.com/sunweilin/forgify/backend/internal/domain/handler"
	mcpdomain "github.com/sunweilin/forgify/backend/internal/domain/mcp"
	skilldomain "github.com/sunweilin/forgify/backend/internal/domain/skill"
)

// ProductionChecker wires the four backing services into a single
// CapabilityChecker. Pass nil for any service whose checks should fall
// through to "exists" (e.g. tests that don't wire skill).
//
// ProductionChecker 把四个 service 接到一个 CapabilityChecker。任一 nil 会
// 让对应 check 返 true(测试场景)。
type ProductionChecker struct {
	Function *functionapp.Service
	Handler  *handlerapp.Service
	Skill    *skillapp.Service
	MCP      *mcpapp.Service
}

// Compile-time interface assertion.
//
// 编译期接口断言。
var _ CapabilityChecker = (*ProductionChecker)(nil)

func (c *ProductionChecker) HasFunction(ctx context.Context, id string) (bool, error) {
	if c.Function == nil {
		return true, nil
	}
	_, err := c.Function.Get(ctx, id)
	if errors.Is(err, functiondomain.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (c *ProductionChecker) HasHandler(ctx context.Context, name string) (bool, error) {
	if c.Handler == nil {
		return true, nil
	}
	_, err := c.Handler.GetByName(ctx, name)
	if errors.Is(err, handlerdomain.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (c *ProductionChecker) HasSkill(ctx context.Context, name string) (bool, error) {
	if c.Skill == nil {
		return true, nil
	}
	_, err := c.Skill.Get(ctx, name)
	if errors.Is(err, skilldomain.ErrSkillNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (c *ProductionChecker) HasMCPServer(ctx context.Context, name string) (bool, error) {
	if c.MCP == nil {
		return true, nil
	}
	_, err := c.MCP.GetServer(ctx, name)
	if errors.Is(err, mcpdomain.ErrServerNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
