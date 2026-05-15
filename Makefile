BACKEND_DATA_DIR ?= /tmp/forgify-dev
PORT             ?= 8742

# `set -a; source .env` exports every var so child processes (go test,
# go run) inherit them. Targets that don't need secrets can skip this.
# `set -a; source .env` 让 .env 里所有变量成为环境变量被子进程继承。
SHELL := /bin/bash
LOAD_ENV := set -a; [ -f .env ] && source .env; set +a;

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
	@echo "    make test-unit      Unit suite (no external deps)."
	@echo "    make test-pipeline  E2e pipeline suite. Sources .env — Live_/sandbox tests run"
	@echo "                        when keys/resources are present, skip gracefully when not."
	@echo "    make stop           Kill anything bound to port $(PORT)."
	@echo ""
	@echo "  Optional:"
	@echo "    make clear          Reset dev data dir."

# environment — first-time / new-machine setup. Must run from outer shell:
# the recipe needs to invoke `devbox install` / `devbox run`, which can't
# happen from inside a devbox shell.
#
# environment——首次环境装配，必须在外层 shell（recipe 要调 devbox install /
# devbox run，devbox shell 内无法跑）。
environment:
	@[ -z "$$DEVBOX_SHELL_ENABLED" ] || { \
		echo "✗ 'make environment' must run from your normal shell, not inside devbox."; \
		echo "  Exit devbox shell first (Ctrl+D), then re-run."; \
		exit 1; \
	}
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

# test-console — run dev backend in foreground + open testend. Ctrl+C stops.
# Code changes require manual restart (Ctrl+C, re-run).
# Auto-wraps in devbox shell if invoked from outer shell.
# test-console——前台跑 dev backend + 自动开浏览器；改代码要手动重启（Ctrl+C 再跑一次）。
# 新 testend(v2 Vue+Vite SPA):build 产物在 testend/dist/,backend 经
# --integration-dir 服务静态文件;如果 dist/ 不存在,自动 build 一次。
test-console:
	$(AUTO_DEVBOX)
	@if [ ! -f testend/dist/index.html ]; then \
	  echo "==> testend/dist not found, building..."; \
	  cd testend && (test -d node_modules || npm install) && npm run build:nocheck; \
	fi
	@lsof -ti :$(PORT) 2>/dev/null | xargs kill 2>/dev/null || true
	@sleep 0.3
	@( while ! curl -sf http://localhost:$(PORT)/api/v1/health >/dev/null 2>&1; do sleep 0.5; done; \
	   echo ""; echo "✓ http://localhost:$(PORT)/dev/ ready"; \
	   open http://localhost:$(PORT)/dev/ 2>/dev/null || true ) &
	@$(LOAD_ENV) cd backend && go run ./cmd/server --dev --port $(PORT) --data-dir $(BACKEND_DATA_DIR) --collections-dir ../testend/collections --integration-dir ../testend/dist

# test-console-dev — 前端 hot-reload:启 vite dev server (5173) + backend (5174).
# 改 testend UI 时用,看着代码刷,不用每次 rebuild。
# 浏览器开 http://localhost:5173/  (它会 proxy /api + /dev 到 backend)
test-console-dev:
	$(AUTO_DEVBOX)
	@lsof -ti :$(PORT) 2>/dev/null | xargs kill 2>/dev/null || true
	@lsof -ti :5173 2>/dev/null | xargs kill 2>/dev/null || true
	@sleep 0.3
	@( while ! curl -sf http://localhost:$(PORT)/api/v1/health >/dev/null 2>&1; do sleep 0.5; done; \
	   cd testend && BACKEND_PORT=$(PORT) npm run dev ) &
	@$(LOAD_ENV) cd backend && go run ./cmd/server --dev --port $(PORT) --data-dir $(BACKEND_DATA_DIR) --collections-dir ../testend/collections --integration-dir ../testend/dist

# build-testend — explicit testend rebuild (npm install + vite build)
build-testend:
	@cd testend && (test -d node_modules || npm install) && npm run build:nocheck

# test-unit — pure-function / in-memory SQLite suite.
# test-unit——纯函数 / 内存 SQLite 套件。
test-unit:
	$(AUTO_DEVBOX)
	@cd backend && go test -count=1 ./... -skip TestIntegration_

# test-pipeline — the one e2e suite. Sources .env so Live_ tests run when
# DEEPSEEK_API_KEY is present; forge sandbox tests run when the v2 PluginSandbox
# bootstraps (i.e. mise binary is embedded — run `make resources` once after
# clone). Tests skip gracefully when prerequisites are absent.
#
# Runs serially (-p 1): each pipeline package boots a fresh harness and lazy-
# installs Python + uv via mise on first use. Parallel package execution
# triggers concurrent mise installs sharing nothing, which exhausts disk /
# trips upstream rate limits / hits race conditions in mise's plugin cache.
# Serial cost is ~4 min total — well worth the determinism.
#
# test-pipeline——唯一的 e2e 套件。自动 source .env：有 DEEPSEEK_API_KEY 则跑
# Live_ 测试；mise binary 已 embed（克隆后跑一次 `make resources`）则跑 forge
# sandbox 测试；缺时均优雅 skip。
#
# 串行（-p 1）：每个 pipeline 包起新 harness，首次用时 lazy 装 Python + uv via
# mise。并行包执行时多个 mise install 互不知情，会撞磁盘 / 触上游限流 /
# 击中 mise plugin 缓存竞态。串行约 4 分钟跑完，换确定性。
test-pipeline:
	$(AUTO_DEVBOX)
	@$(LOAD_ENV) cd backend && go test -count=1 -tags=pipeline -p 1 ./test/...

# stop — kill anything bound to the dev port.
# stop——杀占用 dev 端口的进程。
stop:
	@PIDS=$$(lsof -ti :$(PORT) 2>/dev/null || true); \
	if [ -n "$$PIDS" ]; then \
		echo "→ stopping PID(s): $$(echo $$PIDS | tr '\n' ' ')"; \
		echo "$$PIDS" | xargs kill 2>/dev/null || true; \
		echo "✓ stopped"; \
	else \
		echo "✓ nothing running"; \
	fi

# ── Optional helpers ─────────────────────────────────────────────────────────

# resources — download mise binary into backend/internal/infra/sandbox/mise/
# for go:embed (D2-2). Default: current platform only. Pass ALL=1 to fetch
# all 5 supported platforms (release pipeline use). Pin version via
# MISE_VERSION env (defaults to latest).
#
# resources——把 mise 二进制下到 backend/internal/infra/sandbox/mise/ 给
# go:embed（D2-2）用。默认仅当前平台；ALL=1 拉全 5 平台（release pipeline 用）。
# MISE_VERSION env 钉版本（默认 latest）。
resources:
	$(AUTO_DEVBOX)
	@cd backend && go run ./cmd/resources $(if $(ALL),--all-platforms,)

# clear — stop dev backend + wipe the entire dev data dir.
#
# In --dev mode the backend roots its `forgify-home` (mcp.json /
# skills/ / .catalog.json) under $(BACKEND_DATA_DIR)/.forgify, so a
# single rm -rf wipes EVERYTHING dev: SQLite db + attachments + sandbox
# v2 mise installs + dev-installed MCP servers + dev-scanned skills +
# catalog cache. Real ~/.forgify/ (prod / Wails app data) is never
# touched.
#
# clear——停 dev backend + 清整个 dev 数据目录。
# dev 模式下 forgify-home 在 $(BACKEND_DATA_DIR)/.forgify 下，所以一次
# rm -rf 把 dev 全清干净：DB + attachments + sandbox 装机 + dev 装 MCP +
# 扫到的 skill + catalog cache。真 ~/.forgify/（prod / Wails）不动。
clear: stop
	@rm -rf $(BACKEND_DATA_DIR)
	@echo "✓ cleared $(BACKEND_DATA_DIR) (db + attachments + sandbox + dev forgify-home)"

# check-cross — cross-platform compile + vet for all 3 supported targets.
# Catches more than `go build` alone: vet flags suspicious type conversions,
# struct tag typos, unreachable code, ineffective assignments, etc. Run
# this before every release tag to ensure the Windows/Linux/Darwin code
# branches all parse cleanly even when only macOS-side dev happens.
#
# Doesn't actually run tests on non-host platforms (would need real
# Windows/Linux machines for that). Code-layer audit only.
#
# check-cross——跨平台 compile + vet 三平台。比 `go build` 抓得多：vet
# 标可疑类型转换、struct tag 拼错、不可达代码、无效赋值等。release tag
# 前必跑，让 Windows/Linux/Darwin 三个码径在 mac-only 开发下也能解析干净。
# 不真在非 host 平台跑测试（需真机）；仅代码层 audit。
check-cross:
	$(AUTO_DEVBOX)
	@# CGO_ENABLED=0 for cross-targets: cgo would invoke the Mac SDK
	@# linker against Linux/Windows headers and fail. We deliberately
	@# use modernc.org/sqlite (pure Go) precisely so we can cross-vet
	@# from any host without a per-target toolchain.
	@# CGO_ENABLED=0 跨目标用：cgo 会让 Mac SDK linker 对 Linux/Windows
	@# 头文件做错事。我们故意用 modernc.org/sqlite（纯 Go）就为了让任意
	@# host 都能 cross-vet 不需 per-target 工具链。
	@echo "→ darwin/amd64 vet..."
	@cd backend && GOOS=darwin GOARCH=amd64 go vet ./...
	@echo "→ darwin/arm64 vet..."
	@cd backend && GOOS=darwin GOARCH=arm64 go vet ./...
	@echo "→ linux/amd64 vet..."
	@cd backend && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go vet ./...
	@echo "→ linux/arm64 vet..."
	@cd backend && CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go vet ./...
	@echo "→ windows/amd64 vet..."
	@cd backend && CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go vet ./...
	@echo ""
	@echo "→ darwin/amd64 build..."
	@cd backend && GOOS=darwin GOARCH=amd64 go build ./...
	@echo "→ darwin/arm64 build..."
	@cd backend && GOOS=darwin GOARCH=arm64 go build ./...
	@echo "→ linux/amd64 build..."
	@cd backend && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build ./...
	@echo "→ linux/arm64 build..."
	@cd backend && CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build ./...
	@echo "→ windows/amd64 build..."
	@cd backend && CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build ./...
	@echo ""
	@echo "✓ vet + build clean across darwin/linux/windows × amd64/arm64"

.PHONY: help environment test-console test-console-dev build-testend test-unit test-pipeline stop clear check-cross
