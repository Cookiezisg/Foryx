# ──────────────────────────────────────────────────────────────────
# Forgify — make 命令（后端 Go 单体 + 前端 Flutter 桌面端）
# ──────────────────────────────────────────────────────────────────
#
#   环境   setup    创建开发环境（devbox 装 pin 的 go + flutter）
#   运行   server   起后端服务（FORGIFY_DEV，端口 $(BACKEND_PORT)）
#          stop     优雅关停后端（SIGTERM → App.Serve 有序关停）
#   测试   unit     Go 单测（in-memory SQLite）
#          testend  全功能黑盒验收（testend/ 真起后端二进制 + llmmock；分钟级，不进 verify）
#          evals    金标 LLM 旅程（testend/golden，真模型烧钱；手动触发）
#   文档   docs     文档规范门禁（cmd/docs，GOVERNANCE §11 全套）
#   出包   build    后端二进制 → bin/forgify-server
#   门禁   verify   后端 pre-push：gofmt + vet + build + unit + docs（host 平台）
#   前端   fe-gen   codegen（freezed/json/slang，build_runner）
#          fe-analyze / fe-test  Flutter 静态分析 / 单测
#          fe-verify Flutter pre-push：gen + analyze + test
#   清理   clean    清 dev 数据目录
#
# ──────────────────────────────────────────────────────────────────

BACKEND_DATA_DIR ?= /tmp/forgify-dev
BACKEND_PORT     ?= 8742

SHELL    := /bin/bash
LOAD_ENV := set -a; [ -f .env ] && source .env; set +a;
DEVBOX   := $(HOME)/.local/bin/devbox

# AUTO_DEVBOX — 任何需要 devbox 工具（pin 的 go）的 target 首行；不在 devbox shell 里就经 devbox run 重跑自己。
define AUTO_DEVBOX
@if [ -z "$$DEVBOX_SHELL_ENABLED" ]; then exec $(DEVBOX) run -- $(MAKE) $@; fi
endef

.DEFAULT_GOAL := help

help:
	@echo "Forgify（后端单体；前端待重建）"
	@echo ""
	@echo "  环境:   make setup    创建开发环境（devbox）"
	@echo "  运行:   make server   起后端服务（:$(BACKEND_PORT)）"
	@echo "          make stop     优雅关停后端"
	@echo "  测试:   make unit     Go 单测"
	@echo "          make testend  全功能黑盒验收（真二进制 + llmmock，分钟级）"
	@echo "          make evals    金标 LLM 旅程（真模型，烧钱，手动跑）"
	@echo "  文档:   make docs     文档规范门禁（GOVERNANCE §11）"
	@echo "  出包:   make build    后端二进制 → bin/forgify-server"
	@echo "  门禁:   make verify   后端 pre-push（gofmt+vet+build+unit+docs）"
	@echo "  前端:   make fe-gen   codegen（freezed/json/slang）"
	@echo "          make fe-verify Flutter pre-push（gen+analyze+test）"
	@echo "  清理:   make clean    清 dev 数据（$(BACKEND_DATA_DIR)）"

# ── 环境 ────────────────────────────────────────────────────────────

# setup — 引导 devbox（它装 pin 的 go + flutter）。运行时（python/node/uv/dotnet）首次使用时直接从上游按需下，无需预装。
# 装好后:后端 make server;前端 make fe-gen 再 cd frontend && flutter run（dev 需 FORGIFY_BACKEND_URL 指向已跑后端）。
setup:
	@[ -z "$$DEVBOX_SHELL_ENABLED" ] || { echo "✗ 在普通 shell 跑 make setup（先 Ctrl+D 退出 devbox）"; exit 1; }
	@if [ ! -x "$(DEVBOX)" ] && ! command -v devbox >/dev/null 2>&1; then \
		echo "→ 安装 devbox launcher…"; mkdir -p $(HOME)/.local/bin; \
		curl -fsSL "https://releases.jetify.com/devbox?os=$$(uname -s | tr A-Z a-z)&arch=$$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')" -o $(DEVBOX); \
		chmod +x $(DEVBOX); \
	fi
	@DBX=$$(command -v devbox || echo $(DEVBOX)); \
		echo "→ devbox install（首次可能要 sudo）…"; $$DBX install
	@echo ""
	@echo "✓ setup 完成。现在：make server"

# ── 运行 ────────────────────────────────────────────────────────────

# server — 起后端。main 读环境变量（FORGIFY_DEV/ADDR/DATA_DIR），非 flag。
server:
	$(AUTO_DEVBOX)
	@$(LOAD_ENV) cd backend && FORGIFY_DEV=1 FORGIFY_ADDR=:$(BACKEND_PORT) FORGIFY_DATA_DIR=$(BACKEND_DATA_DIR) go run ./cmd/server

# stop — 给监听进程发 SIGTERM → App.Serve 跑有序优雅关停（SSE 流 → HTTP 排空 → 后台 → DB）。非 -9。
stop:
	@PIDS=$$(lsof -ti :$(BACKEND_PORT) 2>/dev/null || true); \
	if [ -n "$$PIDS" ]; then \
		echo "→ SIGTERM :$(BACKEND_PORT)（pid $$(echo $$PIDS | tr '\n' ' ')），等优雅关停…"; \
		echo "$$PIDS" | xargs kill -TERM 2>/dev/null || true; \
		for i in $$(seq 1 20); do lsof -ti :$(BACKEND_PORT) >/dev/null 2>&1 || break; sleep 0.5; done; \
		echo "✓ 已停"; \
	else echo "✓ 没在跑"; fi

# ── 测试 / 文档 ──────────────────────────────────────────────────────

unit:
	$(AUTO_DEVBOX)
	@cd backend && go test -count=1 ./...

# testend — 全功能黑盒验收：编译并拉起真 backend 二进制，纯 HTTP/SSE 打全功能场景（零 backend import）。
# 首跑会下载 sandbox 运行时（之后走 ~/.forgify-testend-cache 缓存）。
testend:
	$(AUTO_DEVBOX)
	@cd testend && go test -count=1 -timeout 30m ./scenarios/...

# evals — 金标 LLM 旅程：真模型端到端（柱C）。烧钱，手动跑。自动 source 仓库根 .env（若存在）注入
# key——默认认 DEEPSEEK_API_KEY + deepseek-v4-flash；EVALS_BASE_URL/EVALS_MODEL/EVALS_KEY 可覆盖。
evals:
	$(AUTO_DEVBOX)
	@if [ -f .env ]; then set -a; . ./.env; set +a; fi; cd testend && EVALS=1 go test -count=1 -timeout 60m ./golden/...

# docs — 文档规范门禁：frontmatter / 类型 / 生命周期 / INDEX≤50 / 孤儿链接（GOVERNANCE §11）。
docs:
	$(AUTO_DEVBOX)
	@cd backend && go run ./cmd/docs --root=..

# ── 出包 ────────────────────────────────────────────────────────────

# build — 后端 host 二进制。TODO：打包时把它作为 sidecar 二进制随 Flutter app 分发（flutter build
# <platform> + 把 forgify-server 放进 bundle，客户端经 FORGIFY_ADDR 拉起，见 ADR 0004 §1）。
build:
	$(AUTO_DEVBOX)
	@cd backend && go build -o bin/forgify-server ./cmd/server
	@echo "✓ backend/bin/forgify-server"

# ── 门禁 ────────────────────────────────────────────────────────────

# verify — pre-push 门禁：gofmt 净 + vet + build + 单测 + 文档门禁。
# 跨平台 release 现在就是 `cd backend && GOOS=x GOARCH=y go build ./cmd/server`——无内嵌、无预拉；
# 运行时（python/node/uv/dotnet）在目标机首次使用时按需下，故无平台依赖、go build 可直接交叉编译。
verify:
	$(AUTO_DEVBOX)
	@echo "→ gofmt…"
	@cd backend && f=$$(gofmt -l .); [ -z "$$f" ] || { echo "✗ gofmt 未净:"; echo "$$f"; exit 1; }
	@echo "→ go vet…"
	@cd backend && go vet ./...
	@echo "→ go build…"
	@cd backend && go build ./...
	@echo "→ unit…"
	@cd backend && go test -count=1 ./...
	@echo "→ docs…"
	@cd backend && go run ./cmd/docs --root=..
	@echo ""
	@echo "✓ verify 全绿（gofmt + vet + build + unit + docs）"

# ── 前端（Flutter 桌面端，ADR 0004）────────────────────────────────────
#
# 前端 target 用单行守卫 FE_GUARD（非 AUTO_DEVBOX）:flutter 仅存在于 devbox。守卫与命令必须在
# 同一 recipe 行——exec 替换当前 shell,使 `;` 后的命令不会在外层(非 devbox)shell 重跑一次
# （两行写法对 devbox-only 工具会双跑、外层那次 127）。
FE_GUARD = if [ -z "$$DEVBOX_SHELL_ENABLED" ]; then exec $(DEVBOX) run -- $(MAKE) $@; fi

# fe-setup — 拉前端依赖（首次或改 pubspec 后）。
fe-setup:
	@$(FE_GUARD); cd frontend && flutter pub get

# fe-gen — codegen：freezed/json_serializable/slang 经 build_runner 生成 *.g.dart / *.freezed.dart。
# 注:本仓库 codegen 产物入库（源等价、deterministic），故 fresh checkout 直接 fe-analyze 即可；
# 改了带注解的源或 i18n 文案后重跑本目标。
fe-gen:
	@$(FE_GUARD); cd frontend && flutter pub run build_runner build

# fe-analyze — Flutter 静态分析（须净）。
fe-analyze:
	@$(FE_GUARD); cd frontend && flutter analyze

# fe-test — Flutter 单测。
fe-test:
	@$(FE_GUARD); cd frontend && flutter test

# fe-verify — 前端 pre-push 门禁：codegen + 分析净 + 单测绿。
# （桌面真跑 flutter run -d macos 需完整 Xcode + CocoaPods，属机器层面、不入门禁。）
fe-verify:
	@$(FE_GUARD); cd frontend \
		&& echo "→ fe codegen…"  && flutter pub run build_runner build \
		&& echo "→ fe analyze…"  && flutter analyze \
		&& echo "→ fe test…"     && flutter test \
		&& echo "" && echo "✓ fe-verify 全绿（gen + analyze + test）"

# ── 清理 ────────────────────────────────────────────────────────────

# clean — 停服务 + 清 dev 数据目录（SQLite + 附件 + sandbox 运行时 + mcp + skills 都在 $(BACKEND_DATA_DIR)）。
# 不碰 ~/.forgify（真实用户数据）、不碰 docs/。
clean: stop
	@rm -rf $(BACKEND_DATA_DIR)
	@echo "✓ 已清 $(BACKEND_DATA_DIR)"

.PHONY: help setup server stop unit docs build verify clean testend evals \
        fe-setup fe-gen fe-analyze fe-test fe-verify
