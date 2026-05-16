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

// ProductionChecker wires the four backing services into a CapabilityChecker.
//
// ProductionChecker 把四个 service 装配为 CapabilityChecker。
type ProductionChecker struct {
	Function *functionapp.Service
	Handler  *handlerapp.Service
	Skill    *skillapp.Service
	MCP      *mcpapp.Service
}

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
