package mcp

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"

	mcpdomain "github.com/sunweilin/forgify/backend/internal/domain/mcp"
	sandboxdomain "github.com/sunweilin/forgify/backend/internal/domain/sandbox"
	mcpinfra "github.com/sunweilin/forgify/backend/internal/infra/mcp"
	installprogresspkg "github.com/sunweilin/forgify/backend/internal/pkg/installprogress"
)

// InstallFromRegistry installs a curated entry; returns ErrAlreadyInstalled on name collision.
//
// InstallFromRegistry 安装 curated 条目；同名已存在返 ErrAlreadyInstalled。
func (s *Service) InstallFromRegistry(ctx context.Context, name string, env, args map[string]string) (*mcpdomain.ServerStatus, error) {
	entry, err := s.GetRegistryEntry(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("mcpapp.InstallFromRegistry %s: %w", name, err)
	}

	s.mu.RLock()
	_, collided := s.configs[name]
	s.mu.RUnlock()
	if collided {
		return nil, fmt.Errorf("mcpapp.InstallFromRegistry %s: %w",
			name, mcpdomain.ErrAlreadyInstalled)
	}

	if missing := missingEnvKeys(entry.RequiredEnv, env); len(missing) > 0 {
		return nil, fmt.Errorf("mcpapp.InstallFromRegistry %s: %w: %s",
			name, mcpdomain.ErrRequiredEnvMissing, strings.Join(missing, ", "))
	}
	if missing := missingArgKeys(entry.RequiredArgs, args); len(missing) > 0 {
		return nil, fmt.Errorf("mcpapp.InstallFromRegistry %s: %w: %s",
			name, mcpdomain.ErrRequiredArgsMissing, strings.Join(missing, ", "))
	}

	if s.sandbox != nil && entry.Runtime != "" && entry.Runtime != "binary" {
		owner := sandboxdomain.Owner{
			Kind: sandboxdomain.OwnerKindMCP,
			ID:   entry.Name,
			Name: entry.Name,
		}
		spec := sandboxdomain.EnvSpec{
			Runtime: sandboxdomain.RuntimeSpec{Kind: entry.Runtime},
		}

		_, ensureErr := installprogresspkg.Run(ctx,
			map[string]any{
				"stage":   "installing",
				"runtime": entry.Runtime,
				"server":  entry.Name,
			},
			func(progress sandboxdomain.ProgressFunc) (*sandboxdomain.Env, error) {
				return s.sandbox.EnsureEnv(ctx, owner, spec, progress)
			})
		if ensureErr != nil {
			return nil, fmt.Errorf("mcpapp.InstallFromRegistry %s: %w: %w",
				name, mcpdomain.ErrInstallFailed, ensureErr)
		}
	}

	resolvedArgs := substituteArgs(entry.InstallCmd.Args, args)

	cfg := mcpdomain.ServerConfig{
		Name:    name,
		Command: entry.InstallCmd.Command,
		Args:    resolvedArgs,
		Env:     env,
	}

	if err := s.AddServer(ctx, cfg); err != nil {
		return nil, fmt.Errorf("mcpapp.InstallFromRegistry %s: %w", name, err)
	}

	st, err := s.GetServer(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("mcpapp.InstallFromRegistry %s: %w", name, err)
	}
	return st, nil
}

// Import merges an external mcp.json fragment; overwrite=false keeps existing.
//
// Import 合并外部 mcp.json 片段；overwrite=false 时保留已有项目。
func (s *Service) Import(ctx context.Context, incoming map[string]mcpdomain.ServerConfig, overwrite bool) (mcpinfra.MergeResult, error) {
	s.mu.Lock()
	merged, res := mcpinfra.Merge(s.cloneConfigsLocked(), incoming, overwrite)
	for _, name := range res.Imported {
		s.configs[name] = merged[name]
		s.states[name] = &mcpdomain.ServerStatus{
			Name:   name,
			Status: mcpdomain.StatusDisconnected,
			Tools:  []mcpdomain.ToolDef{},
		}
		if c, ok := s.clients[name]; ok {
			if err := c.Close(); err != nil {
				s.log.Warn("Import: overwrite-existing close failed; orphan subprocess may persist until parent exit",
					zap.String("server", name), zap.Error(err))
			}
			delete(s.clients, name)
		}
	}
	configsCopy := s.cloneConfigsLocked()
	s.mu.Unlock()

	if err := mcpinfra.Save(s.configPath, configsCopy); err != nil {
		return res, fmt.Errorf("mcpapp.Import: save mcp.json: %w", err)
	}

	for _, name := range res.Imported {
		go func(n string) {
			cctx, cancel := context.WithTimeout(ctx, initializeTimeout)
			defer cancel()
			if err := s.connectOne(cctx, n); err != nil {
				s.log.Warn("mcp imported server connect failed",
					zap.String("server", n), zap.Error(err))
			}
		}(name)
	}
	return res, nil
}

func missingEnvKeys(required []mcpdomain.EnvRequirement, supplied map[string]string) []string {
	var missing []string
	for _, req := range required {
		if v, ok := supplied[req.Name]; !ok || strings.TrimSpace(v) == "" {
			missing = append(missing, req.Name)
		}
	}
	return missing
}

func missingArgKeys(required []mcpdomain.ArgRequirement, supplied map[string]string) []string {
	var missing []string
	for _, req := range required {
		if v, ok := supplied[req.Name]; ok && strings.TrimSpace(v) != "" {
			continue
		}
		if strings.TrimSpace(req.Default) != "" {
			continue
		}
		missing = append(missing, req.Name)
	}
	return missing
}

func substituteArgs(args []string, vars map[string]string) []string {
	out := make([]string, len(args))
	for i, a := range args {
		out[i] = expandVars(a, vars)
	}
	return out
}


func expandVars(s string, vars map[string]string) string {
	var b strings.Builder
	i := 0
	for i < len(s) {
		if i+1 < len(s) && s[i] == '$' && s[i+1] == '{' {
			end := strings.IndexByte(s[i+2:], '}')
			if end >= 0 {
				key := s[i+2 : i+2+end]
				if v, ok := vars[key]; ok {
					b.WriteString(v)
					i += 2 + end + 1
					continue
				}
			}
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}
