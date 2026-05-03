BACKEND_DATA_DIR ?= /tmp/forgify-dev
PORT             ?= 8742

# `set -a; source .env` exports every var so child processes (go test,
# go run) inherit them. Targets that don't need secrets can skip this.
# `set -a; source .env` 让 .env 里所有变量成为环境变量被子进程继承。
SHELL := /bin/bash
LOAD_ENV := set -a; [ -f .env ] && source .env; set +a;

# Sandbox resource paths used only by environment / bootstrap-time helpers.
# 沙箱资源路径——仅 environment / bootstrap 用。
RESOURCES_DIR ?= $(HOME)/.forgify-dev-resources
PLATFORM      := $(shell uname -s | tr A-Z a-z)-$(shell uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')

DEVBOX_LAUNCHER := $(HOME)/.local/bin/devbox

# Auto-devbox dispatcher — every daily-use target's first recipe line.
# If not already inside `devbox shell`, re-invoke the same target via
# `devbox run` so the recipe body runs in the devbox env. Inside devbox,
# fall through to the rest of the recipe.
#
# 自动进 devbox 派发块：日常 target 第一行调它。不在 devbox shell 里就用
# devbox run 重新执行同一 target；在则继续往下跑 recipe。
define AUTO_DEVBOX
@if [ -z "$$DEVBOX_SHELL_ENABLED" ]; then \
	exec $(DEVBOX_LAUNCHER) run -- $(MAKE) $@; \
fi
endef

.DEFAULT_GOAL := help

# ── Primary commands ─────────────────────────────────────────────────────────

help:
	@echo "Forgify — make targets"
	@echo ""
	@echo "  Setup (run once on a new machine):"
	@echo "    make environment    Install devbox + Nix + Go tools + sandbox resources."
	@echo ""
	@echo "  Daily (from any shell — auto-enters devbox):"
	@echo "    make test-console   Live-reload backend + open testend in browser. Ctrl+C to stop."
	@echo "    make test-unit      Unit suite."
	@echo "    make test-pipeline  Real-dep / e2e suite (needs DEEPSEEK_API_KEY in .env)."
	@echo "    make stop           Kill anything bound to port $(PORT)."
	@echo ""
	@echo "  Optional:"
	@echo "    make doctor         Pre-commit health check."
	@echo "    make clear          Reset dev data dir."

# environment — first-time / new-machine setup. Must run from outer shell.
# environment——首次环境装配，必须在外层 shell。
environment: _refuse-inside-devbox
	@if [ ! -x "$(DEVBOX_LAUNCHER)" ] && ! command -v devbox >/dev/null 2>&1; then \
		echo "→ installing devbox launcher to $(DEVBOX_LAUNCHER)..."; \
		mkdir -p $(HOME)/.local/bin; \
		curl -fsSL "https://releases.jetify.com/devbox?os=$$(uname -s | tr A-Z a-z)&arch=$$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')" \
			-o $(DEVBOX_LAUNCHER); \
		chmod +x $(DEVBOX_LAUNCHER); \
	else \
		echo "✓ devbox launcher present"; \
	fi
	@DEVBOX=$$(command -v devbox || echo $(DEVBOX_LAUNCHER)); \
	echo "→ devbox install (may prompt for sudo on first run for Nix)..."; \
	$$DEVBOX install; \
	echo "→ devbox run bootstrap (Go tools + sandbox resources)..."; \
	$$DEVBOX run bootstrap
	@echo ""
	@echo "✓ environment ready. now run:"
	@echo "    make test-console"

# test-console — live-reload backend + open testend. Foreground; Ctrl+C stops.
# Auto-wraps in devbox shell if invoked from outer shell.
# test-console——前台 air live reload + 自动开浏览器。外层 shell 跑也行，自动进 devbox。
test-console:
	$(AUTO_DEVBOX)
	@lsof -ti :$(PORT) 2>/dev/null | xargs kill 2>/dev/null || true
	@sleep 0.3
	@( while ! curl -sf http://localhost:$(PORT)/api/v1/health >/dev/null 2>&1; do sleep 0.5; done; \
	   echo ""; echo "✓ http://localhost:$(PORT)/dev/ ready"; \
	   open http://localhost:$(PORT)/dev/ 2>/dev/null || true ) &
	@$(LOAD_ENV) cd backend && air -c ../.air.toml

# test-unit — pure-function / in-memory SQLite suite.
# test-unit——纯函数 / 内存 SQLite 套件。
test-unit:
	$(AUTO_DEVBOX)
	@cd backend && go test -count=1 ./... -skip TestIntegration_

# test-pipeline — real-dep / e2e suite.
# test-pipeline——真依赖 / e2e 套件。
test-pipeline:
	$(AUTO_DEVBOX)
	@$(LOAD_ENV) cd backend && go test -count=1 -v -run TestIntegration ./internal/infra/llm/...
	@$(LOAD_ENV) cd backend && go test -count=1 -v -tags=pipeline ./test/...

# stop — kill the dev backend AND its air supervisor. Without killing
# air too, it would notice its child died and respawn the backend, so
# the user appears to have stopped nothing.
#
# stop——杀 dev backend + air 监控进程。不一起杀的话 air 会以为子进程崩了
# 自动重启 backend，看起来 stop 没起作用。
stop:
	@PORT_PIDS=$$(lsof -ti :$(PORT) 2>/dev/null || true); \
	AIR_PIDS=$$(pgrep -f '\.air\.toml' 2>/dev/null || true); \
	ALL=$$(echo "$$PORT_PIDS $$AIR_PIDS" | tr ' ' '\n' | sort -u | grep -v '^$$' || true); \
	if [ -n "$$ALL" ]; then \
		echo "→ stopping PID(s): $$(echo $$ALL | tr '\n' ' ')"; \
		echo "$$ALL" | xargs kill 2>/dev/null || true; \
		echo "✓ stopped (backend + air)"; \
	else \
		echo "✓ nothing running"; \
	fi

# ── Devbox guards ────────────────────────────────────────────────────────────

# _refuse-inside-devbox — only `make environment` uses this. Setup needs
# the outer shell so it can invoke `devbox install` / `devbox run`.
# _refuse-inside-devbox——仅 environment 用；setup 必须在外层 shell。
_refuse-inside-devbox:
	@[ -z "$$DEVBOX_SHELL_ENABLED" ] || { \
		echo "✗ 'make environment' must run from your normal shell, not inside devbox."; \
		echo "  Exit devbox shell first (Ctrl+D), then re-run."; \
		exit 1; \
	}

# ── Internal helper (called by devbox bootstrap script) ─────────────────────

# download-resources — idempotent. If the platform's uv binary + PBS
# tarball already exist at $RESOURCES_DIR, skip; otherwise fetch via
# scripts/download-sandbox-resources.sh (versions pinned in that script).
# Force re-download by `rm -rf ~/.forgify-dev-resources/` first.
#
# download-resources——幂等。资源已在则跳过；不在则跑下载脚本（脚本里钉死版本）。
# 强制重下：先 `rm -rf ~/.forgify-dev-resources/`。
download-resources:
	@if [ -f "$(RESOURCES_DIR)/uv-$(PLATFORM)" ] && [ -f "$(RESOURCES_DIR)/python-$(PLATFORM).tar.gz" ]; then \
		echo "✓ sandbox resources present at $(RESOURCES_DIR) ($(PLATFORM))"; \
	else \
		echo "→ sandbox resources missing for $(PLATFORM), downloading..."; \
		FORGIFY_DEV_RESOURCES="$(RESOURCES_DIR)" bash scripts/download-sandbox-resources.sh; \
	fi

# ── Optional helpers ─────────────────────────────────────────────────────────

# doctor — pre-commit health check. Stops on first failure.
# doctor——commit / PR 前一键体检。
doctor:
	$(AUTO_DEVBOX)
	@echo "→ build"        && cd backend && go build ./...
	@echo "→ go vet"       && cd backend && go vet ./...
	@echo "→ gofmt"        && test -z "$$(cd backend && gofmt -l .)" || { echo "  gofmt issues; run: cd backend && gofmt -w ."; exit 1; }
	@echo "→ staticcheck"  && cd backend && staticcheck ./...
	@echo "→ deadcode"     && cd backend && deadcode -test ./cmd/server
	@echo "→ test-unit"    && $(MAKE) test-unit
	@echo "✓ all checks pass"

# clear — stop dev backend + reset data dir. Reuses stop so air gets
# killed too (without it, air would respawn into the freshly-cleared
# data dir and recreate the DB).
#
# clear——停 dev backend + 清数据目录。复用 stop 把 air 一起杀，否则 air
# 会朝刚清完的数据目录重启 backend，重新建表。
clear: stop
	@rm -rf $(BACKEND_DATA_DIR)
	@echo "✓ cleared (db + attachments)"

.PHONY: help environment test-console test-unit test-pipeline stop \
        _refuse-inside-devbox download-resources doctor clear
