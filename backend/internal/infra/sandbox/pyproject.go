// pyproject.go renders the pyproject.toml that uv consumes when materializing
// a venv. Sandbox writes one of these per EnvID directory before invoking
// `uv sync --project <envDir>`. The schema is the minimum uv needs:
//   - [project].name / version are required by PEP 621 but not used by uv
//     beyond logging — we set name to "forge-<id>" for human-readable cache
//     paths and version to "0.0.0" as a placeholder
//   - [project].requires-python is the Python version constraint
//   - [project].dependencies is the PEP 508 specifier list
//
// We deliberately omit [tool.uv] — uv treats any pyproject.toml as managed by
// default, and we don't need the optional knobs (lock policy, sources, etc.)
// for forge venvs.
//
// pyproject.go 渲染 uv 物化 venv 用的 pyproject.toml。Sandbox 每个 EnvID
// 目录写一份，然后调 `uv sync --project <envDir>`。schema 取 uv 最低需要：
//   - [project].name / version PEP 621 必填，uv 仅用于 log——name 设
//     "forge-<id>" 便于调试，version 占位 "0.0.0"
//   - [project].requires-python 是 Python 版本约束
//   - [project].dependencies 是 PEP 508 specifier 列表
//
// 故意不写 [tool.uv]——uv 默认就 managed，forge venv 用不到 lock 策略 /
// sources 等可选项。

package sandbox

import (
	"fmt"
	"strconv"
	"strings"
)

// renderPyproject builds the pyproject.toml content for one EnvID directory.
//
// pythonVersion is the per-version constraint (ForgeVersion.PythonVersion);
// when empty, defaultPython is used (typically Sandbox.cfg.DefaultPython).
// Both are TOML-quoted via strconv.Quote so a malformed or attacker-controlled
// value can't break out of the field.
//
// Each dep specifier is also strconv.Quote'd. Blank dep entries are dropped
// silently (matches normalizeSpecifier behavior in EnvID computation).
//
// renderPyproject 构建一个 EnvID 目录下的 pyproject.toml 内容。
// pythonVersion 是 per-version 约束（ForgeVersion.PythonVersion）；
// 为空时回退到 defaultPython（通常 Sandbox.cfg.DefaultPython）。
// 两者都用 strconv.Quote 做 TOML 转义，畸形 / 恶意输入也无法突破字段边界。
// dep specifier 同样 strconv.Quote。空 dep 项静默丢弃（与 EnvID 算法的
// normalizeSpecifier 一致）。
func renderPyproject(forgeID string, deps []string, pythonVersion, defaultPython string) string {
	pyVer := strings.TrimSpace(pythonVersion)
	if pyVer == "" {
		pyVer = strings.TrimSpace(defaultPython)
	}
	if pyVer == "" {
		pyVer = ">=3.12"
	}

	var depBlock strings.Builder
	for _, d := range deps {
		d = strings.TrimSpace(d)
		if d == "" {
			continue
		}
		depBlock.WriteString("    ")
		depBlock.WriteString(strconv.Quote(d))
		depBlock.WriteString(",\n")
	}

	return fmt.Sprintf(`[project]
name = %q
version = "0.0.0"
requires-python = %s
dependencies = [
%s]
`, "forge-"+forgeID, strconv.Quote(pyVer), depBlock.String())
}
