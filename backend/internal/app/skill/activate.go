package skill

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	skilldomain "github.com/sunweilin/forgify/backend/internal/domain/skill"
	subagentapp "github.com/sunweilin/forgify/backend/internal/app/subagent"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

const bodyReadRetryDelay = 100 * time.Millisecond

// Activate loads body, substitutes placeholders, sets ActiveSkill, dispatches fork.
//
// Activate 加载 body、替换占位、设置 ActiveSkill，fork 模式时派发 subagent。
func (s *Service) Activate(ctx context.Context, name string, arguments []string) (result string, err error) {
	startedAt := time.Now().UTC()
	defer func() {
		s.recordExecution(ctx, name, arguments, result, err, startedAt, time.Now().UTC())
	}()
	return s.activateInternal(ctx, name, arguments)
}

func (s *Service) activateInternal(ctx context.Context, name string, arguments []string) (string, error) {
	s.mu.RLock()
	skill, ok := s.skills[name]
	s.mu.RUnlock()
	if !ok {
		return "", fmt.Errorf("skillapp.Activate: %w: %q", skilldomain.ErrSkillNotFound, name)
	}

	body, err := readBodyWithRetry(skill.BodyPath)
	if err != nil {
		return "", fmt.Errorf("skillapp.Activate %s: %w", name, err)
	}
	if len(body) > skilldomain.MaxBodyBytes {
		return "", fmt.Errorf("skillapp.Activate %s: %w", name, skilldomain.ErrBodyTooLarge)
	}

	convID, _ := reqctxpkg.GetConversationID(ctx)
	substituted := substitute(string(body), substituteVars{
		Arguments:    arguments,
		NamedArgs:    skill.Frontmatter.Arguments,
		SkillDir:     skill.DirPath,
		SessionID:    convID,
		Effort:       skill.Frontmatter.Effort,
	})

	if state, hasState := reqctxpkg.GetAgentState(ctx); hasState {
		state.SetActiveSkill(skill)
	}

	if skill.Frontmatter.Context == "fork" {
		if depth := reqctxpkg.GetSubagentDepth(ctx); depth >= 1 {
			s.log.Info("skill activated within subagent; ignoring fork directive",
				zap.String("skill", name), zap.Int("depth", depth))
			return substituted, nil
		}
		if s.subagent == nil {
			return "", fmt.Errorf("skillapp.Activate %s: fork requested but SubagentService is nil", name)
		}
		agentType := skill.Frontmatter.Agent
		result, err := s.subagent.Spawn(ctx, agentType, substituted, subagentapp.SpawnOpts{})
		if err != nil {
			return "", fmt.Errorf("skillapp.Activate %s: subagent spawn: %w", name, err)
		}
		return result.Result, nil
	}

	return substituted, nil
}

// readBodyWithRetry retries once on ErrNotExist to ride out editor rename races.
//
// readBodyWithRetry 在 ErrNotExist 时延迟重试一次，避开编辑器 rename 竞态。
func readBodyWithRetry(path string) ([]byte, error) {
	body, err := os.ReadFile(path)
	if err == nil {
		return body, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	time.Sleep(bodyReadRetryDelay)
	return os.ReadFile(path)
}

type substituteVars struct {
	Arguments []string
	NamedArgs []string
	SkillDir  string
	SessionID string
	Effort    string
}

// substitute applies $1/$ARGUMENTS/${CLAUDE_*}/named placeholders.
//
// substitute 应用 $1/$ARGUMENTS/${CLAUDE_*}/命名占位的替换。
func substitute(body string, v substituteVars) string {
	pairs := []string{
		"${CLAUDE_SKILL_DIR}", v.SkillDir,
		"${CLAUDE_SESSION_ID}", v.SessionID,
		"${CLAUDE_EFFORT}", v.Effort,
		"$ARGUMENTS", strings.Join(v.Arguments, " "),
	}
	for i := len(v.Arguments); i >= 1; i-- {
		pairs = append(pairs, "$"+strconv.Itoa(i), v.Arguments[i-1])
	}
	for i, name := range v.NamedArgs {
		if i >= len(v.Arguments) {
			break
		}
		if name == "" {
			continue
		}
		pairs = append(pairs, "$"+name, v.Arguments[i])
	}
	return strings.NewReplacer(pairs...).Replace(body)
}
