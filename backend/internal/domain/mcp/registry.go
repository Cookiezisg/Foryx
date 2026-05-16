package mcp

import (
	"context"
	"errors"
)

// RegistryEntry is one marketplace listing installable via install_mcp_server.
//
// RegistryEntry 是 marketplace 的一个可装条目（install_mcp_server / UI 共用）。
type RegistryEntry struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Homepage    string `json:"homepage,omitempty"`
	Runtime     string `json:"runtime"`

	InstallCmd   InstallCmd       `json:"installCmd"`
	RequiredEnv  []EnvRequirement `json:"requiredEnv,omitempty"`
	RequiredArgs []ArgRequirement `json:"requiredArgs,omitempty"`

	Category string `json:"category,omitempty"`

	// Tier marks setup friction: 0 zero-config, 1 single key, 2 OAuth, 3 DB/cloud creds.
	// Tier 标上手难度：0 零配置，1 一个 key，2 OAuth，3 DB/云凭证。
	Tier int `json:"tier"`

	Notes string `json:"notes,omitempty"`
}

// InstallCmd is the command the install flow runs; Args may contain "${name}" tokens.
//
// InstallCmd 是 install 流程跑的命令；Args 可含 "${name}" token，安装时替换。
type InstallCmd struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

// EnvRequirement is one env var the user must provide before install.
//
// EnvRequirement 是用户安装前必填的一个 env 变量。
type EnvRequirement struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	SetupURL    string `json:"setupUrl,omitempty"`
	Secret      bool   `json:"secret"`
}

// ArgRequirement is a value the user supplies at install time for "${name}" substitution.
//
// ArgRequirement 是用户安装时填、用于 "${name}" 模板替换的值。
type ArgRequirement struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Type        string `json:"type"`
	Default     string `json:"default,omitempty"`
}

// RegistrySource is the marketplace data source (curated registry or test fixture).
//
// RegistrySource 是 marketplace 数据源（curated registry 或测试 fixture）。
type RegistrySource interface {
	// List returns all entries sorted tier-asc then name-asc (easiest first); stable order.
	//
	// List 返所有条目，按 tier asc + name asc 稳排；同进程内多次调用顺序一致。
	List(ctx context.Context) ([]RegistryEntry, error)

	Get(ctx context.Context, name string) (*RegistryEntry, error)
}

var (
	ErrAlreadyInstalled = errors.New("mcp: server already installed")
)
