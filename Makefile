# ──────────────────────────────────────────────────────────────────
# Forgify — make commands cheat sheet
# ──────────────────────────────────────────────────────────────────
#
#   Once    setup     install all dependencies (Go tools + npm)
#
#   Daily   dev       run desktop app (backend + frontend, browser opens)
#           stop      kill anything we started
#           test      run unit tests
#           clean     wipe local dev data (/tmp/forgify-dev)
#           reset     factory reset — dev + prod data + build + node_modules
#
#   Ship    build     package the macOS .app bundle
#           verify    cross-platform build + lint (release gate)
#           e2e       end-to-end pipeline tests (needs .env keys)
#
#   QA      smoke     playwright frontend walk + screenshots
#                     (needs 'make dev' running in another terminal)
#
#   Misc    testend   legacy testend debug console (pre-frontend era)
#           mise      download mise binaries (one-time, for sandbox)
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
	@echo "  Once:    make setup     install all dependencies (Go tools + npm)"
	@echo ""
	@echo "  Daily:   make dev       run the desktop app (backend + frontend, browser opens)"
	@echo "           make stop      kill anything we started"
	@echo "           make test      run unit tests"
	@echo "           make clean     wipe local dev data ($(BACKEND_DATA_DIR))"
	@echo "           make reset     factory reset — dev + prod data + build + node_modules"
	@echo ""
	@echo "  Ship:    make build     package the macOS .app bundle"
	@echo "           make verify    cross-platform build + lint (release gate)"
	@echo "           make e2e       end-to-end pipeline tests (needs .env keys)"
	@echo ""
	@echo "  QA:      make smoke     playwright frontend walk (needs 'make dev' running)"
	@echo ""
	@echo "  Misc:    make testend   legacy testend debug console (pre-frontend era)"
	@echo "           make mise      download mise binaries (one-time, for sandbox)"

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
	    lsof -ti :$(FRONTEND_PORT) 2>/dev/null | xargs kill 2>/dev/null || true; \
	    exit 0' INT TERM EXIT; \
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

# test — unit suite (no external deps; in-memory SQLite).
# test —— 单测；纯函数 + in-memory SQLite，不需要外部依赖。
test:
	$(AUTO_DEVBOX)
	@cd backend && go test -count=1 ./... -skip TestIntegration_

# clean — stop everything + wipe the entire dev data dir.
# In --dev mode the backend roots forgify-home under $(BACKEND_DATA_DIR)/.forgify
# so one rm wipes: SQLite + attachments + sandbox + MCP + skills + catalog cache.
# Real ~/.forgify/ (prod / Wails app data) is never touched.
#
# clean —— 停服务 + 清整个 dev 目录；不动 ~/.forgify/。
clean: stop
	@rm -rf $(BACKEND_DATA_DIR)
	@echo "✓ cleared $(BACKEND_DATA_DIR)"

# reset — factory reset. Wipes EVERYTHING the app may have written:
# dev data + prod ~/.forgify (real DB, skills, mcp.json, memory, docs,
# sandbox installs) + frontend build artifacts + node_modules.
# Asks for explicit "yes" because ~/.forgify holds your real user data.
# After reset, run `make setup` to reinstall deps before `make dev`.
#
# reset —— 出厂重置。dev 数据 + ~/.forgify 真实用户数据 + 前端构建产物 +
# node_modules 一并清掉。要求显式输入 "yes"。完事后跑 `make setup` 再 `make dev`。
reset: stop
	@echo "WILL WIPE:"
	@echo "  $(BACKEND_DATA_DIR)/                   dev runtime (db / attachments / sandbox)"
	@echo "  $$HOME/.forgify/                       prod user data (db / skills / mcp / memory / docs)"
	@echo "  frontend/dist/                          vite build output"
	@echo "  frontend/node_modules/                  npm deps (reinstall via 'make setup')"
	@echo "  frontend/wailsjs/                       auto-generated Wails bindings"
	@echo "  backend/cmd/desktop/embed/              embedded frontend (build copies it back)"
	@echo "  backend/cmd/desktop/build/              wails build output (.app bundles)"
	@echo ""
	@printf "type 'yes' to confirm: "; \
	 read ans; \
	 if [ "$$ans" != "yes" ]; then echo "✗ aborted, nothing changed"; exit 1; fi; \
	 rm -rf $(BACKEND_DATA_DIR); \
	 rm -rf $$HOME/.forgify; \
	 rm -rf frontend/dist; \
	 rm -rf frontend/node_modules; \
	 rm -rf frontend/wailsjs; \
	 find backend/cmd/desktop/embed -mindepth 1 ! -name .gitignore ! -name .gitkeep -delete 2>/dev/null || true; \
	 rm -rf backend/cmd/desktop/build; \
	 rm -f backend/desktop backend/forgify-server backend/forgify-desktop; \
	 echo ""; \
	 echo "✓ reset done. run 'make setup' before next 'make dev'."

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

# verify — release gate. Cross-platform compile (5 OS×arch combos) +
# vet + prompt lint. Catches more than a simple `go build` alone.
# CGO_ENABLED=0 used for non-darwin to avoid linker errors (we use
# modernc.org/sqlite pure-Go precisely for this).
#
# verify —— 上线 gate；5 平台 vet+build + prompt lint。
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
	@echo ""
	@echo "✓ verify clean: 5 platforms × (vet+build) + prompt lint"

# e2e — pipeline tests. Sources .env so Live_ tests + sandbox tests
# pick up real keys / mise resources when present; tests skip otherwise.
# Serial (-p 1): mise installs share nothing and would race otherwise.
#
# e2e —— pipeline 测试；source .env 自动带 keys；缺时优雅 skip。串行
# 跑（mise 装机并行会撞磁盘 / 触限流）。
e2e:
	$(AUTO_DEVBOX)
	@$(LOAD_ENV) cd backend && go test -count=1 -tags=pipeline -p 1 ./test/...

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
	@$(LOAD_ENV) cd backend && go run ./cmd/server --dev --port $(BACKEND_PORT) --data-dir $(BACKEND_DATA_DIR) --collections-dir ../testend/collections --integration-dir ../testend/dist

# mise — download mise binaries into backend/internal/infra/sandbox/mise/
# for go:embed. Default: current platform. Pass ALL=1 to fetch all
# 5 supported platforms (release builds).
#
# mise —— 下 mise 二进制给 go:embed；默认当前平台；ALL=1 拉全 5 平台。
mise:
	$(AUTO_DEVBOX)
	@cd backend && go run ./cmd/resources $(if $(ALL),--all-platforms,)

.PHONY: help setup dev stop test clean reset build verify e2e smoke testend mise
