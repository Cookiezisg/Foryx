# Forgify

本地优先的 Agentic Workflow Platform — **Flutter 桌面 app**（macOS/Linux/Windows）+ Go 后端作 sidecar，单进程、单用户、SQLite 落盘，**不做 SaaS**。

核心心智:**Quadrinity 四项全能**（Function / Handler / Agent / Workflow）+ **Durable Execution**（节点结果记忆化 + 解释器幂等重走）。

## 快速开始

```bash
make setup             # 首次:引导 devbox（装 pin 的 go + flutter）
make server            # 起后端（FORGIFY_DEV，:8742）
# 另开一个终端跑前端（dev 挂到已跑后端）:
make fe-gen            # 首次/改注解后:codegen（freezed/json/slang）
cd frontend && FORGIFY_BACKEND_URL=http://127.0.0.1:8742 flutter run -d macos
```

需要 devbox 工具的 `make` 目标会自动进 devbox 环境跑，不用手动 `devbox shell`。
> macOS 桌面真跑需完整 Xcode + CocoaPods（Apple 工具链，devbox/nix 给不了）。

## 命令

```bash
# 后端
make server      # 起后端服务（:8742）
make stop        # SIGTERM 优雅关停
make unit        # Go 单测
make testend     # 全功能黑盒验收（真二进制 + llmmock，分钟级）
make docs        # 文档规范门禁（GOVERNANCE §11）
make build       # 后端二进制 → backend/bin/forgify-server
make verify      # 后端 pre-push（gofmt+vet+build+unit+docs）

# 前端（Flutter）
make fe-gen      # codegen（freezed/json_serializable/slang）
make fe-analyze  # flutter analyze
make fe-test     # flutter 单测
make fe-verify   # 前端 pre-push（gen + analyze + test）

make clean       # 清 dev 数据目录
```

## 环境一致性

| 文件 | 钉的内容 |
|---|---|
| `devbox.lock` | Nix 包精确 commit（go 1.25 / flutter / gnumake）|
| `backend/go.mod` | Go 依赖 |
| `frontend/pubspec.lock` | Flutter/Dart 依赖 |

升级:改对应文件 → `devbox install`（或 `flutter pub upgrade`）重生成 lock → 提交。

## 文档

- 文档入口:[`docs/INDEX.md`](docs/INDEX.md)
- 愿景 / 架构 / 实体 / 引擎 / 路线:[`docs/concepts/architecture.md`](docs/concepts/architecture.md)
- 后端总览(第 0 篇):[`docs/references/backend/overview.md`](docs/references/backend/overview.md)
- 架构决策(ADR):[`docs/decisions/`](docs/decisions/)（含 [0004 前端 Flutter 架构](docs/decisions/0004-frontend-flutter-architecture.md)）
- 工程纪律:[`CLAUDE.md`](CLAUDE.md)
