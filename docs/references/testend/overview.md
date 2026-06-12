---
id: DOC-038
type: reference
status: active
owner: @weilin
created: 2026-06-12
reviewed: 2026-06-12
review-due: 2026-09-12
audience: [human, ai]
---

# testend —— 全功能黑盒验收套件

> 与 `backend/` 平级的独立 Go module。**零 backend import**：编译并拉起真实 `cmd/server` 二进制，只走纯 HTTP/SSE——验的就是用户与前端实际拿到的东西；场景在这里碰到的别扭本身就是前端开发者体验 finding。

## 入口

- `make testend` —— 全功能黑盒验收（scenarios/，llmmock 驱动 LLM 面，零 token，分钟级；**不进 `make verify`**）。
- `make evals` —— 金标 LLM 旅程（golden/，真模型；`EVALS=1` 门控 + `EVALS_BASE_URL/EVALS_MODEL/EVALS_KEY`；烧钱手动跑）。

## 布局

| 目录 | 职责 |
|---|---|
| `harness/` | 座架：`server.go`（编译+拉起真二进制、临时 dataDir、空闲端口、等 health、退出清理；sandbox 运行时经 `~/.forgify-testend-cache` 预置——首跑下载、后跑搭车）· `client.go`（N1 envelope 解包、workspace 头、`OK`/`Fail(状态,码)` 断言、`Eventually` 异步涟漪轮询）· `llmmock.go`（OpenAI 兼容假模型：剧本化回应驱动 chat/agent/utility 全链零 token；**请求抓包即 promptdump**——线缆上的请求体就是模型真实看到的全部）· `sse.go`（三流订阅与事件断言） |
| `scenarios/` | 验收场景 = 普通 go test：每个测试函数是 PLAN 的一个「feature × 情况」单元，函数名即台账行；`-run` 过滤单域 |
| `golden/` | 真模型金标旅程（12 条端到端，机器可验收终态） |

## 纪律

- 黑盒铁律：禁止 import backend 任何包——线缆事实（header 名、payload 形状）从 api.md 复述，对不上即 doc/产品 finding。
- 场景对 `Eventually` 的依赖即产品的异步语义（索引/通知涟漪）；超时值是体验断言的一部分。
- 验收程序（acceptance-review）结束后本套件转为常驻回归：改 prompt/工具/契约后跑 `make testend`，改提示词工程后跑 `make evals`。
