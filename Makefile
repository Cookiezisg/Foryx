# ──────────────────────────────────────────────────────────────────
# Forgify — make commands cheat sheet (22 single-word targets)
# ──────────────────────────────────────────────────────────────────
#
#   Once     setup     install all dependencies (Go tools + npm)
#            mise      download mise binaries (sandbox runtime)
#
#   Daily    dev       run desktop app (backend + frontend, browser opens)
#            stop      kill anything we started
#            unit      Go unit tests
#            web       vitest frontend tests
#            test      unit + web aggregate
#            lint      frontend eslint + tsc + steiger
#            mock      pipeline fake-LLM tests (~60s, no tokens) ◀ daily driver
#            clean     wipe dev data (light, daily-safe)
#            reset     factory reset — prod data + build artifacts + node_modules
#
#   Pipeline sandbox   mock + real-sandbox lifecycle (FORGIFY_DEV_RESOURCES)
#            live      ONLY real-LLM tests (BURNS TOKENS — DEEPSEEK_API_KEY)
#            e2e       mock → sandbox → live aggregate (release gate)
#            cover     HTML coverage report → coverage/pipeline.html
#
#   Matrix   matrix    regenerate README coverage matrix section
#            audit     strict matrix check (used by verify; Phase 4+ enables it)
#
#   Ship     build     package the macOS .app bundle
#            verify    pre-push gate (5-platform vet+build + lintprompts + mock)
#
#   QA       smoke     playwright frontend walk + screenshots
#                      (needs 'make dev' running in another terminal)
#
#   Misc     testend   legacy testend debug console (pre-frontend era)
#
# ──────────────────────────────────────────────────────────────────

BACKEND_DATA_DIR ?= /tmp/forgify-dev
BACKEND_PORT     ?= 8742
FRONTEND_PORT    ?= 5173

SHELL := /bin/bash
LOAD_ENV := set -a; [ -f .env ] && source .env; set +a;
DEVBOX_LAUNCHER := $(HOME)/.local/bin/devbox

# AUTO_DEVBOX — first recipe line of any target needing devbox tools.
# If we're not inside `devbox shell`, re-invoke ourselves via `devbox run`.
# 不在 devbox shell 里就经 devbox run 重新跑同一 target。
define AUTO_DEVBOX
@if [ -z "$$DEVBOX_SHELL_ENABLED" ]; then \
	exec $(DEVBOX_LAUNCHER) run -- $(MAKE) $@; \
fi
endef

.DEFAULT_GOAL := help

# ──────────────────────────────────────────────────────────────────
# Menu
# ──────────────────────────────────────────────────────────────────

help:
	@echo "Forgify"
	@echo ""
	@echo "  Once:     make setup     install all dependencies"
	@echo "            make mise      download mise binaries (one-time)"
	@echo ""
	@echo "  Daily:    make dev       run the desktop app"
	@echo "            make stop      kill anything we started"
	@echo "            make unit      Go unit tests"
	@echo "            make web       vitest frontend tests"
	@echo "            make test      unit + web aggregate"
	@echo "            make lint      frontend lint (eslint + tsc + steiger)"
	@echo "            make mock      pipeline tests with fake LLM (~60s, no tokens)"
	@echo "            make clean     wipe dev data ($(BACKEND_DATA_DIR), light)"
	@echo "            make reset     factory reset"
	@echo ""
	@echo "  Pipeline: make sandbox   mock + real sandbox (FORGIFY_DEV_RESOURCES)"
	@echo "            make live      real-LLM tests only (BURNS TOKENS)"
	@echo "            make e2e       full pipeline mock+sandbox+live (release gate)"
	@echo "            make cover     HTML coverage report"
	@echo ""
	@echo "  Matrix:   make matrix    regenerate README coverage matrix section"
	@echo "            make audit     strict matrix check (used by verify)"
	@echo ""
	@echo "  Ship:     make build     package macOS .app"
	@echo "            make verify    pre-push gate (vet+build+lintprompts; Phase 5 adds audit+mock)"
	@echo ""
	@echo "  QA:       make smoke     playwright frontend walk"
	@echo ""
	@echo "  Misc:     make testend   legacy debug console"

# ──────────────────────────────────────────────────────────────────
# Once
# ──────────────────────────────────────────────────────────────────

# setup — install everything a fresh clone needs. Must run from outside
# `devbox shell` because it bootstraps devbox itself.
# setup —— 装齐全部依赖；必须在 devbox shell 外跑（要装 devbox 自己）。
setup:
	@[ -z "$$DEVBOX_SHELL_ENABLED" ] || { \
		echo "✗ run 'make setup' from your normal shell, not inside devbox (Ctrl+D first)"; \
		exit 1; \
	}
	@if [ ! -x "$(DEVBOX_LAUNCHER)" ] && ! command -v devbox >/dev/null 2>&1; then \
		echo "→ installing devbox launcher..."; \
		mkdir -p $(HOME)/.local/bin; \
		curl -fsSL "https://releases.jetify.com/devbox?os=$$(uname -s | tr A-Z a-z)&arch=$$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')" -o $(DEVBOX_LAUNCHER); \
		chmod +x $(DEVBOX_LAUNCHER); \
	fi
	@DEVBOX=$$(command -v devbox || echo $(DEVBOX_LAUNCHER)); \
		echo "→ devbox install (may prompt for sudo on first run)..."; \
		$$DEVBOX install; \
		echo "→ devbox run bootstrap (Go tools + sandbox resources)..."; \
		$$DEVBOX run bootstrap
	@echo "→ npm install (frontend)..."
	@cd frontend && npm install --silent
	@echo "→ npm install (testend, legacy)..."
	@cd testend && (test -d node_modules || npm install --silent) || true
	@echo ""
	@echo "✓ setup done. now run:  make dev"

# mise — download mise binaries into backend/internal/infra/sandbox/mise/
# for go:embed. Default: current platform. Pass ALL=1 to fetch all
# 5 supported platforms (release builds).
#
# mise —— 下 mise 二进制给 go:embed；默认当前平台；ALL=1 拉全 5 平台。
mise:
	$(AUTO_DEVBOX)
	@cd backend && go run ./cmd/resources $(if $(ALL),--all-platforms,)

# ──────────────────────────────────────────────────────────────────
# Daily
# ──────────────────────────────────────────────────────────────────

# dev — start backend + frontend in one terminal. Ctrl+C stops both.
# Vite runs in the foreground (so you see its logs); backend runs in
# the background. EXIT trap cleans up both whatever way we exit.
#
# dev —— 一个终端同时起后端 + 前端；Ctrl+C 停两边。vite 在前台（看得到
# 日志）；后端在后台。EXIT trap 兜底两边都关掉。
dev:
	$(AUTO_DEVBOX)
	@cd frontend && [ -d node_modules ] || npm install --silent
	@lsof -ti :$(BACKEND_PORT) 2>/dev/null | xargs kill 2>/dev/null || true
	@lsof -ti :$(FRONTEND_PORT) 2>/dev/null | xargs kill 2>/dev/null || true
	@sleep 0.3
	@trap '\
	    echo ""; \
	    echo "→ stopping..."; \
	    lsof -ti :$(BACKEND_PORT) 2>/dev/null | xargs kill 2>/dev/null || true; \
	    lsof -ti :$(FRONTEND_PORT) 2>/dev/null | xargs kill 2>/dev/null || true' EXIT; \
	 ( $(LOAD_ENV) cd backend && go run ./cmd/server --dev --port $(BACKEND_PORT) --data-dir $(BACKEND_DATA_DIR) ) & \
	 while ! curl -sf http://localhost:$(BACKEND_PORT)/api/v1/health >/dev/null 2>&1; do sleep 0.3; done; \
	 echo ""; \
	 echo "✓ backend ready on :$(BACKEND_PORT)"; \
	 echo "  opening http://localhost:$(FRONTEND_PORT)/ in 2s..."; \
	 echo ""; \
	 ( sleep 2 && (open http://localhost:$(FRONTEND_PORT)/ 2>/dev/null || true) ) & \
	 cd frontend && FORGIFY_BACKEND_PORT=$(BACKEND_PORT) npm run dev

# stop — kill anything bound to either port.
# stop —— 杀占用端口的进程。
stop:
	@FOUND=0; \
	for p in $(BACKEND_PORT) $(FRONTEND_PORT); do \
	  PIDS=$$(lsof -ti :$$p 2>/dev/null || true); \
	  if [ -n "$$PIDS" ]; then \
	    echo "→ killing :$$p (pid $$(echo $$PIDS | tr '\n' ' '))"; \
	    echo "$$PIDS" | xargs kill 2>/dev/null || true; \
	    FOUND=1; \
	  fi; \
	done; \
	[ $$FOUND = 1 ] && echo "✓ stopped" || echo "✓ nothing running"

# unit — Go unit tests. In-memory SQLite, no external deps. Skips
# TestIntegration_* (legacy real-LLM unit-style tests; covered in 'live').
#
# unit —— Go 单测；in-memory SQLite，无外部依赖；skip TestIntegration_*。
unit:
	$(AUTO_DEVBOX)
	@cd backend && go test -count=1 ./... -skip TestIntegration_

# web — vitest frontend tests.
# web —— 前端 vitest 单测。
web:
	@cd frontend && [ -d node_modules ] || npm install --silent
	@cd frontend && npm run test --silent

# test — both layers' unit suites.
# test —— 后端 + 前端单测一并跑。
test: unit web

# lint — frontend lint: typecheck (tsc --noEmit) + eslint + steiger (FSD).
# lint —— 前端 lint；tsc + eslint + steiger（FSD 结构）。
lint:
	@cd frontend && [ -d node_modules ] || npm install --silent
	@cd frontend && npm run typecheck && npm run lint && npm run fsd

# mock — pipeline tests with fake LLM. No env vars needed; no tokens burned.
# Tests gated by RequireDeepSeekKey / RequireSandboxResources skip cleanly.
# This is the daily-driver pipeline check.
#
# mock —— pipeline 测试日常 driver；fake LLM；无 env 依赖；不烧 token。
# 需要 key/resource 的测试自动 skip。
#
# Phase 0 note: the api/cross/sse/lifecycle/errcodes/live axis split happens
# in Phase 2; for now this runs the full ./test/... (lifecycle / live tests
# still gracefully skip when env unset).
mock:
	$(AUTO_DEVBOX)
	@cd backend && go test -count=1 -race -tags=pipeline -p 1 -timeout=10m ./test/...

# clean — stop everything + wipe the dev data dir (light, daily-safe).
# In --dev mode the backend roots forgify-home under $(BACKEND_DATA_DIR)/.forgify
# so one rm wipes: SQLite + attachments + sandbox + MCP + skills + catalog cache.
# Build artifacts (stray binaries / dist / coverage / node_modules / mise) and
# superpowers scratch (.superpowers / docs) are NOT touched here — they belong to
# `reset`. Real ~/.forgify/ (prod / Wails app data) is never touched.
#
# clean —— 停服务 + 清 dev 数据目录（轻量，日常安全）。
# 构建产物 / node_modules / mise / superpowers 散件（.superpowers / docs）都归 reset，
# 这里不碰；也不动 ~/.forgify/。
clean: stop
	@rm -rf $(BACKEND_DATA_DIR)
	@echo "✓ cleared $(BACKEND_DATA_DIR)"

# reset — factory reset. Wipes EVERYTHING regenerable or app-written:
# dev data + prod ~/.forgify (real DB, skills, mcp.json, memory, docs,
# sandbox installs) + all build artifacts (stray binaries / dist / coverage /
# sandbox) + every node_modules + superpowers scratch (.superpowers / docs).
# Asks for explicit "yes" because ~/.forgify holds your real user data.
# After reset, run `make setup` to reinstall deps before `make dev`.
#
# reset —— 出厂重置。dev 数据 + ~/.forgify 真实用户数据 + 全部构建产物（散落二进制 /
# dist / coverage / sandbox）+ 所有 node_modules + superpowers 散件（.superpowers / docs）
# 一并清掉。要求显式输入 "yes"。完事后跑 `make setup` 再 `make dev`。
reset: stop
	@echo "WILL WIPE:"
	@echo "  $(BACKEND_DATA_DIR)/                   dev runtime (db / attachments / sandbox)"
	@echo "  $$HOME/.forgify/                       prod user data (db / skills / mcp / memory / docs)"
	@echo "  frontend/{dist,coverage}/               vite build + test coverage"
	@echo "  {frontend,testend,/}node_modules/       npm deps (reinstall via 'make setup')"
	@echo "  testend/dist/                           testend build output"
	@echo "  frontend/wailsjs/                       auto-generated Wails bindings"
	@echo "  backend/cmd/desktop/embed/              embedded frontend (build copies it back)"
	@echo "  backend/cmd/desktop/build/              wails build output (.app bundles)"
	@echo "  backend/sandbox/ + stray go binaries    sandbox install + go build leftovers"
	@echo "  .superpowers/  docs/  coverage/         superpowers scratch + pipeline coverage report"
	@echo ""
	@printf "type 'yes' to confirm: "; \
	 read ans; \
	 if [ "$$ans" != "yes" ]; then echo "✗ aborted, nothing changed"; exit 1; fi; \
	 rm -rf $(BACKEND_DATA_DIR); \
	 rm -rf $$HOME/.forgify; \
	 rm -rf frontend/dist frontend/coverage frontend/node_modules frontend/wailsjs; \
	 rm -rf testend/dist testend/node_modules node_modules; \
	 rm -rf backend/sandbox; \
	 find backend/cmd/desktop/embed -mindepth 1 ! -name .gitignore ! -name .gitkeep -delete 2>/dev/null || true; \
	 rm -rf backend/cmd/desktop/build; \
	 rm -f backend/server backend/lintprompts backend/fakeserver backend/fetch-mise.exe \
	       backend/desktop backend/forgify-server backend/forgify-desktop backend/cmd/desktop/Forgify; \
	 rm -rf .superpowers docs coverage; \
	 echo ""; \
	 echo "✓ reset done. run 'make setup' before next 'make dev'."

# ──────────────────────────────────────────────────────────────────
# Pipeline (tiered by external dependency cost)
# ──────────────────────────────────────────────────────────────────

# sandbox — mock + real-sandbox lifecycle tests.
# Tests requiring FORGIFY_DEV_RESOURCES skip cleanly when unset.
# Doesn't burn LLM tokens. Phase 0: equivalent to mock (subdir split lands Phase 2).
#
# sandbox —— mock + 真 sandbox lifecycle 测试；FORGIFY_DEV_RESOURCES 缺则 skip。
# 不烧 token。Phase 0：等同 mock（lifecycle/ 子目录拆分在 Phase 2）。
sandbox: mock
	@echo ""
	@echo "ℹ sandbox tier currently aliased to mock (lifecycle/ subdir split lands Phase 2)."

# live — ONLY real-LLM tests (filtered by 'Live_' prefix in test name).
# Requires DEEPSEEK_API_KEY in .env. BURNS REAL TOKENS.
# Use after mock + sandbox green, before release.
#
# live —— 只跑真 LLM 测试（Live_ 前缀过滤）；需 .env DEEPSEEK_API_KEY；烧 token。
live:
	$(AUTO_DEVBOX)
	@$(LOAD_ENV) cd backend && go test -count=1 -race -tags=pipeline -p 1 -timeout=10m -run "Live_" ./test/...

# e2e — full pipeline: mock → sandbox → live in order.
# Fail-fast: if mock breaks, sandbox/live never run (no wasted tokens).
# Run before release.
#
# e2e —— 全套 pipeline：mock → sandbox → live 串行 fail-fast；release gate。
e2e: mock sandbox live

# cover — HTML coverage report (pipeline tests against ./internal/).
# Output: coverage/pipeline.html
#
# cover —— 生成 HTML coverage 报告到 coverage/pipeline.html。
cover:
	$(AUTO_DEVBOX)
	@mkdir -p coverage
	@cd backend && go test -count=1 -tags=pipeline -p 1 -timeout=10m \
		-coverprofile=../coverage/pipeline.out -covermode=atomic \
		-coverpkg=./internal/... \
		./test/...
	@go tool cover -html=coverage/pipeline.out -o coverage/pipeline.html
	@echo ""
	@echo "✓ coverage report: coverage/pipeline.html"

# ──────────────────────────────────────────────────────────────────
# Matrix (Phase 4 will implement; placeholders for now)
# ──────────────────────────────────────────────────────────────────

# matrix — regenerate README coverage matrix section + stdout summary.
# Tool: backend/cmd/coverage-matrix/. Scans handlers + errmap + SSE truth +
# seams.yaml; reconciles with `// covers:` annotations on pipeline tests.
#
# matrix —— 生成 README 矩阵段。工具:backend/cmd/coverage-matrix。
matrix:
	$(AUTO_DEVBOX)
	@cd backend && go run ./cmd/coverage-matrix --update

# audit — strict matrix check. Phase 4: warn-only (annotations not yet
# backfilled). Phase 5 elevates to --strict (fail on uncovered/orphan/unannotated).
#
# audit —— 矩阵严格检查。Phase 4 warn-only,Phase 5 切 --strict。
audit:
	$(AUTO_DEVBOX)
	@cd backend && go run ./cmd/coverage-matrix --check || \
		echo "ℹ matrix has uncovered targets (Phase 4 warn-only; Phase 5 strict)."

# ──────────────────────────────────────────────────────────────────
# Ship
# ──────────────────────────────────────────────────────────────────

# build — package the macOS .app bundle. Builds frontend → copies to
# embed dir → wails build wraps into Forgify.app.
# build —— 打包 .app；vite build → 拷到 embed → wails 包成 .app。
build:
	$(AUTO_DEVBOX)
	@echo "→ vite build..."
	@cd frontend && (test -d node_modules || npm install --silent) && npm run build
	@rm -rf backend/cmd/desktop/embed
	@mkdir -p backend/cmd/desktop/embed
	@cp -R frontend/dist/. backend/cmd/desktop/embed/
	@echo "→ wails build..."
	@cd backend/cmd/desktop && PATH="$$HOME/go/bin:$$PATH" wails build -clean
	@echo ""
	@echo "✓ packaged: backend/cmd/desktop/build/bin/Forgify.app"

# verify — pre-push gate. Cross-platform compile (5 OS×arch combos) +
# vet + prompt lint + matrix audit + pipeline mock tests. Free, offline,
# no tokens. CGO_ENABLED=0 used for non-darwin to avoid linker errors.
#
# Phase 5 outcome: mock is fully green (4 pre-existing drift fixed); audit
# runs in warn-only mode (--strict elevation deferred; ~80% of truth still
# uncovered, awaiting Phase 5+ annotation backfill).
#
# verify —— pre-push gate;5 平台 vet+build + prompt lint + matrix audit + mock。
# 离线 / 不烧 token。Phase 5 起 mock 全绿;audit 仍 warn-only。
verify:
	$(AUTO_DEVBOX)
	@echo "→ vet × 5 platforms..."
	@cd backend && GOOS=darwin  GOARCH=amd64 go vet ./...
	@cd backend && GOOS=darwin  GOARCH=arm64 go vet ./...
	@cd backend && CGO_ENABLED=0 GOOS=linux   GOARCH=amd64 go vet ./...
	@cd backend && CGO_ENABLED=0 GOOS=linux   GOARCH=arm64 go vet ./...
	@cd backend && CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go vet ./...
	@echo "→ build × 5 platforms..."
	@cd backend && GOOS=darwin  GOARCH=amd64 go build ./...
	@cd backend && GOOS=darwin  GOARCH=arm64 go build ./...
	@cd backend && CGO_ENABLED=0 GOOS=linux   GOARCH=amd64 go build ./...
	@cd backend && CGO_ENABLED=0 GOOS=linux   GOARCH=arm64 go build ./...
	@cd backend && CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build ./...
	@echo "→ lint prompts..."
	@cd backend && go run ./cmd/lintprompts
	@echo "→ matrix audit (warn-only)..."
	@cd backend && go run ./cmd/coverage-matrix --check 2>&1 || true
	@echo "→ pipeline mock (fake LLM, ~60s)..."
	@cd backend && go test -count=1 -race -tags=pipeline -p 1 -timeout=10m ./test/...
	@echo ""
	@echo "✓ verify clean: 5 platforms × (vet+build) + lintprompts + audit + mock"

# ──────────────────────────────────────────────────────────────────
# QA
# ──────────────────────────────────────────────────────────────────

# smoke — Playwright-driven frontend walk. Requires `make dev` already
# running in another terminal (or BACKEND_URL+FRONTEND_URL env vars
# pointing elsewhere). Reads DEEPSEEK_KEY from .env to enable the live
# chat tests; skips them cleanly when absent.
# Screenshots land in /tmp/forgify-tests/.
#
# smoke —— Playwright 跑前端冒烟；需另一个终端先 `make dev`。
# .env 里有 DEEPSEEK_KEY 则跑 live chat 测试，否则优雅 skip。
# 截图存到 /tmp/forgify-tests/。
smoke:
	$(AUTO_DEVBOX)
	@cd frontend && [ -d node_modules ] || npm install --silent
	@$(LOAD_ENV) cd frontend && \
	  BACKEND_URL=http://localhost:$(BACKEND_PORT) \
	  FRONTEND_URL=http://localhost:$(FRONTEND_PORT) \
	  node tests/run.mjs

# ──────────────────────────────────────────────────────────────────
# Misc
# ──────────────────────────────────────────────────────────────────

# testend — legacy Vue/Vite debug console at /dev/. Pre-frontend-era
# tool. New React desktop UI lives in frontend/ and runs via `make dev`.
#
# testend —— 老 Vue/Vite 调试控制台；前 React 时代用；现在用 `make dev`。
testend:
	$(AUTO_DEVBOX)
	@if [ ! -f testend/dist/index.html ]; then \
	  echo "→ building testend..."; \
	  cd testend && (test -d node_modules || npm install) && npm run build:nocheck; \
	fi
	@lsof -ti :$(BACKEND_PORT) 2>/dev/null | xargs kill 2>/dev/null || true
	@sleep 0.3
	@( while ! curl -sf http://localhost:$(BACKEND_PORT)/api/v1/health >/dev/null 2>&1; do sleep 0.5; done; \
	   open http://localhost:$(BACKEND_PORT)/dev/ 2>/dev/null || true ) &
	@$(LOAD_ENV) cd backend && go run ./cmd/server --dev --port $(BACKEND_PORT) --data-dir $(BACKEND_DATA_DIR) --integration-dir ../testend/dist

.PHONY: help setup mise dev stop unit web test lint mock sandbox live e2e cover matrix audit build verify clean reset smoke testend
