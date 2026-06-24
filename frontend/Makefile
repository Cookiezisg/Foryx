# ──────────────────────────────────────────────────────────────────
# Anselm 前端（Flutter 桌面端，ADR 0004）— 单词目标，在 frontend/ 下跑。
# 后端 + 仓库级目标在仓库根 Makefile。flutter 由 mise 提供（真·可写官方 SDK）。
#
# 三种启动:  make demo     真 app 形态 + fixture 数据、零后端（看效果）
#            make gallery  组件画廊（UI 套件落地后启用）
#            make app      真 app + 后端 sidecar
# 开发门禁:  setup / gen / analyze / test / verify
# ──────────────────────────────────────────────────────────────────

RUN := mise exec --

.DEFAULT_GOAL := help

help:
	@echo "Anselm 前端（Flutter）— 在 frontend/ 下:"
	@echo "  启动:  make demo     真 app 形态 + fixture、零后端"
	@echo "         make gallery  组件画廊（看每个 An* 全态）"
	@echo "         make app      真 app + 后端"
	@echo "  开发:  make setup    拉依赖（flutter pub get）"
	@echo "         make gen      codegen（dart run slang → i18n strings.g.dart）"
	@echo "         make analyze  flutter analyze（须净）"
	@echo "         make test     flutter test"
	@echo "         make verify   pre-push 门禁（gen + analyze + test）"
	@echo "  截图:  make shots    无头渲染 shell + 画廊全类 → test/dev/out/*.png"

# setup — 拉前端依赖（首次或改 pubspec 后）。
setup:
	@$(RUN) flutter pub get

# gen — codegen。当前只有 slang i18n（dart run slang，非 build_runner）。产物入库（deterministic）。
# 改 lib/i18n/*.i18n.json 后跑。freezed/json 待契约层落地再加进来。
gen:
	@$(RUN) dart run slang

# analyze — 静态分析（须净）。
analyze:
	@$(RUN) flutter analyze

# test — 单测。
test:
	@$(RUN) flutter test

# verify — pre-push 门禁:codegen + 分析净 + 单测绿。
verify:
	@echo "→ gen…"     && $(RUN) dart run slang
	@echo "→ analyze…" && $(RUN) flutter analyze
	@echo "→ test…"    && $(RUN) flutter test
	@echo "" && echo "✓ verify 全绿（gen + analyze + test）"

# demo — 真 app 形态（骨架阶段:三岛空壳;后续接 fixture 数据）。
demo:
	@LANG=en_US.UTF-8 $(RUN) flutter run -d macos

# gallery — 组件画廊（dev/gallery_main.dart）：双栏目录看每个 An* 组件全态。
gallery:
	@LANG=en_US.UTF-8 $(RUN) flutter run -d macos -t lib/dev/gallery_main.dart

# app — 真 app + 后端 sidecar（后续接 backend）。
app:
	@LANG=en_US.UTF-8 $(RUN) flutter run -d macos

# shots — 无头渲染截图(Skia toImage,免 Xcode)→ test/dev/out/*.png 供肉眼/AI 对照 demo 审。
# capture_*.dart 把 toImage/toByteData 包进 tester.runAsync:它们是引擎线程真异步,在 testWidgets 的
# fake-async zone 里 Future 永不解析→hang(flutter#49317/#50783)。一次截一类(同隔离多 toImage 卡 teardown)。
# 类目数变了就同步下面的列表(同 S22「状态即重述」)。
shots:
	@$(RUN) flutter test test/dev/capture_shell.dart
	@for i in 0 1 2 3; do $(RUN) flutter test test/dev/capture_gallery.dart --dart-define=CAT=$$i; done
	@echo "" && echo "✓ 截图落 test/dev/out/(shell.png + gallery_0..3.png)"

# shot-app — 真跑终检截图:构建+启动 gallery .app、截运行中窗口 → test/dev/out/app.png(Impeller 真渲染
# + 红绿灯,headless 还原不了)。BUILD=0 复用已构建 app 跳过重构。详见 test/dev/shot_app.sh。
shot-app:
	@bash test/dev/shot_app.sh

.PHONY: help setup gen analyze test verify demo gallery app shots shot-app
