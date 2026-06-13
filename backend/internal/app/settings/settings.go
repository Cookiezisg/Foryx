// Package settings owns <dataDir>/settings.json — today exactly the "limits" block (the
// user-tunable operational ceilings). Load installs the file's values as the live
// limits.Current() source at boot; Patch merges a partial update, validates, persists
// atomically and hot-swaps the source — consumers see new values on their next read,
// no restart.
//
// Package settings 拥有 <dataDir>/settings.json——目前恰是 "limits" 段（用户可调运行上限）。
// Load 在 boot 时把文件值装成活动 limits.Current() 来源；Patch 合并部分更新、校验、原子
// 持久化并热换来源——消费方下一次读取即见新值，无需重启。
package settings

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	errorspkg "github.com/sunweilin/forgify/backend/internal/pkg/errors"
	limitspkg "github.com/sunweilin/forgify/backend/internal/pkg/limits"
)

// ErrLimitsInvalid rejects a PATCH whose values are out of range (negative ceilings, a
// trigger ratio outside (0,1)).
//
// ErrLimitsInvalid 拒绝取值越界的 PATCH（负上限、trigger ratio 不在 (0,1)）。
var ErrLimitsInvalid = errorspkg.New(errorspkg.KindInvalid, "SETTINGS_LIMITS_INVALID", "limits values out of range")

// fileShape is the settings.json layout (room for future non-limits blocks).
//
// fileShape 是 settings.json 布局（为未来非 limits 段留位）。
type fileShape struct {
	Limits limitspkg.Limits `json:"limits"`
}

// Service loads, serves and patches the settings file.
//
// Service 加载、提供并修补 settings 文件。
type Service struct {
	mu   sync.Mutex
	path string
	cur  limitspkg.Limits
}

// Load reads <dataDir>/settings.json (absent file = pure defaults), installs the result
// as the live limits source, and returns the service. A malformed file is an error —
// silently ignoring a user's hand-edited settings would be worse than failing boot.
//
// Load 读 <dataDir>/settings.json（无文件 = 纯默认），把结果装成活动 limits 来源并返回
// service。文件畸形是错误——静默忽略用户手编的 settings 比 boot 失败更糟。
func Load(dataDir string) (*Service, error) {
	s := &Service{path: filepath.Join(dataDir, "settings.json"), cur: limitspkg.Default()}
	raw, err := os.ReadFile(s.path)
	switch {
	case os.IsNotExist(err):
		// pure defaults. 纯默认。
	case err != nil:
		return nil, fmt.Errorf("settings: read %s: %w", s.path, err)
	default:
		var f fileShape
		if err := json.Unmarshal(raw, &f); err != nil {
			return nil, fmt.Errorf("settings: parse %s: %w", s.path, err)
		}
		s.cur = limitspkg.WithDefaults(f.Limits)
		if err := validate(s.cur); err != nil {
			return nil, fmt.Errorf("settings: %s: %w", s.path, err)
		}
	}
	s.install()
	return s, nil
}

// Limits returns the live values.
//
// Limits 返活动值。
func (s *Service) Limits() limitspkg.Limits {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cur
}

// PatchLimits merges a partial JSON object over the current limits (absent fields keep
// their value), validates, persists atomically and hot-swaps the live source.
//
// PatchLimits 把部分 JSON 对象合并到当前 limits 上（缺省字段保持），校验、原子持久化并
// 热换活动来源。
func (s *Service) PatchLimits(patch json.RawMessage) (limitspkg.Limits, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.cur
	if err := json.Unmarshal(patch, &next); err != nil {
		return limitspkg.Limits{}, fmt.Errorf("%w: %v", ErrLimitsInvalid, err)
	}
	next = limitspkg.WithDefaults(next)
	if err := validate(next); err != nil {
		return limitspkg.Limits{}, err
	}
	if err := s.persist(next); err != nil {
		return limitspkg.Limits{}, err
	}
	s.cur = next
	s.install()
	return next, nil
}

// install swaps the package-level limits source to this service's current value.
//
// install 把包级 limits 来源换成本 service 当前值。
func (s *Service) install() {
	cur := s.cur
	limitspkg.SetProvider(func() limitspkg.Limits { return cur })
}

// persist writes the file atomically (temp + rename).
//
// persist 原子写文件（临时文件 + rename）。
func (s *Service) persist(l limitspkg.Limits) error {
	b, err := json.MarshalIndent(fileShape{Limits: l}, "", "  ")
	if err != nil {
		return fmt.Errorf("settings: marshal: %w", err)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil {
		return fmt.Errorf("settings: write: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("settings: rename: %w", err)
	}
	return nil
}

// validate enforces physical sanity: every ceiling positive, ratio in (0,1).
//
// validate 守物理合法性：上限全正、ratio 在 (0,1)。
func validate(l limitspkg.Limits) error {
	ints := []int{
		l.Agent.MaxSteps, l.Agent.InvokeMaxTurns,
		l.Timeout.LLMIdleSec, l.Timeout.MCPCallSec, l.Timeout.BashDefaultTimeoutSec,
		l.Tools.ReadDefaultLines, l.Tools.BashOutputCapKB, l.Tools.ToolResultCapKB,
		l.Guards.AttachmentMaxMB, l.Guards.WebhookBodyMaxMB,
	}
	for _, v := range ints {
		if v <= 0 {
			return ErrLimitsInvalid
		}
	}
	if l.Context.TriggerRatio <= 0 || l.Context.TriggerRatio >= 1 {
		return ErrLimitsInvalid
	}
	return nil
}
