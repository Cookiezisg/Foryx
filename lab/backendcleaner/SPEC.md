# backendcleaner — SPEC

> 执行契约：阶段、闸门、禁止事项。判据见 `target/criteria.md`，顺序见 `target/order.md`，骨架见 `target/module-template.md`，每轮手册见 `PLAYBOOK.md`，当前进度见 `target/STATE.md`。

## 0. 一句话

不在原 `backend/` 上修补，在 **`backend-new/`** 按当前产品契约平行重建一套干净后端；全部完成后覆盖回去，再调前端 / testend 兼容。

## 1. "干净"的定义

- 读实现直接看出业务意图；一个函数只做一件主要事。
- 分支少，每个分支都有当前产品理由；错误路径明确，不靠 silent fallback / best-effort / 兼容 alias 掩盖。
- 命名表达领域含义，不靠注释解释含混命名；少用 Manager/Helper/Util/Adapter。
- 依赖方向清楚，跨层调用少；没有为旧测试/旧 harness/旧执行模型保留的绕路。
- 抽象只在减少真实重复、切明确边界、稳定外部契约时用——不把旧复杂度换个形式藏进新 helper。
- **第一准则：没有任何历史包袱。** 近乎重写；"一半新一半旧"的实现合并成一个干净的。

## 2. 策略：backend-new 平行重写

- 新建 `backend-new/`，`go.mod` 用**最终** module path `github.com/sunweilin/forgify/backend`（覆盖时 `mv` 即可，import 与 Makefile 零改动）。
- 现有 `backend/` 全程不动、可编译可跑（前端/testend 继续连），仅作**只读考古材料**。
- **不需要"删测试"步骤**：旧测试随旧 backend 在覆盖时一并消失；backend-new 从零按新标准写测试。需要参考旧测试断言的行为时，从现有 backend / git 历史查。
- **不需要开分支**：backend-new 是新增目录、不破坏 backend，重写期全程在 `main` 上 commit + push（符合 main-only + auto-push、投资人可见）。只有"覆盖"那一步是破坏性的，单独谨慎处理。
- 旧实现与旧测试都不是权威。权威 = 当前产品契约（`criteria.md`）+ 模块契约 + 新写测试。

## 3. 阶段

| Phase | 内容 | 位置 |
|---|---|---|
| **0 计划** | lab 定稿（本目录）。不写 backend-new 代码。 | main |
| **1 骨架** | 建 `backend-new/go.mod` + 波次 0 地基；立最小 smoke（启动 / `/api/v1/health` / 用户初始化）。 | main，新增 backend-new/ |
| **2 逐模块** | 按 `order.md` 波次，每模块走 PLAYBOOK 四步循环。每波次收尾 `backend-new` 全仓 `go build ./...` + smoke 绿。 | main |
| **3 覆盖** | `rm -rf backend && mv backend-new backend`。 | main（破坏性，单独 commit） |
| **4 兼容** | 按 `contract-changes.md` 逐条调前端 + testend。 | main |
| **5 全链路** | 产品旅程 e2e + 全量 verification（≈ `make verify`）。 | main |
| **6 闭环** | 文件清单 / 模块清单 / 产品旅程清单（`capability-ledger.md`）三表全绿。 | main |

## 4. 验收闸门

- **每模块**：`gofmt` + `backend-new` `go build` + 该模块及直接依赖包 `go test` + 无反向依赖 + 干净自证 + `rounds/NNNN/round.md` 记录 +（若契约变）`contract-changes.md` 追加。
- **每波次**：`backend-new` `go build ./...` + 最小 smoke。
- **覆盖前**：全平台 `go build` + 全部新测试绿 + `capability-ledger.md` 全勾。
- **Phase 5 后**：全链路 e2e + 全量 verification。

## 5. 禁止

- 把旧实现 / 旧测试当权威（它们是考古，不是规格）。
- 为旧测试 / 旧路径 / 旧执行模型留绕路或兼容 alias。
- **grep 式表面替换**——必须理解后重构（整模块删掉重写，不是逐行 patch）。
- 强行套层（违背 `module-template.md` 按需取层）。
- 一轮混多个高耦合核心模块（workflow / scheduler / chat / loop / wiring 各自独立轮）。
- 只看主文件、不看上下游。
- 改了对外契约却不 take note 到 `contract-changes.md`。
- 用 coverage 数字替代产品旅程闭环。
