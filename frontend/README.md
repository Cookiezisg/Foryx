# Forgify 前端（Flutter 桌面端）

Forgify 的桌面客户端。架构、分层、状态/SSE/契约策略与取舍见
[`../docs/decisions/0004-frontend-flutter-architecture.md`](../docs/decisions/0004-frontend-flutter-architecture.md)
与根 [`CLAUDE.md`](../CLAUDE.md) 前端守则节。

## 开发

工具链经 devbox 提供（`flutter`）。从仓库根:

```bash
make fe-gen       # codegen（freezed / json_serializable / slang）
make fe-analyze   # 静态分析（须净）
make fe-test      # 单测
make fe-verify    # 三合一 pre-push 门禁

# dev 运行（挂到已跑后端;桌面真跑需完整 Xcode + CocoaPods）:
make server                                                    # 另开终端起后端（:8742）
cd frontend && FORGIFY_BACKEND_URL=http://127.0.0.1:8742 flutter run -d macos
```

## 结构（`lib/`）

- `app/` — 装配根（`ProviderScope` + sidecar 拉起 + baseUrl 注入）、shell、router
- `core/` — 跨切地基:`net`（契约/Dio）· `sse`（gateway + sealed 帧 + demux）· `contract` · `design` · `i18n`
- `domain/` — 纯 Dart 实体 + repository 接口（freezed DTO 镜像后端契约）
- `features/` — 各域垂直切片（data + state + ui），随 app 形态铺
