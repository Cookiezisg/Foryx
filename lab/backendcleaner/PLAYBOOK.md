# backendcleaner — PLAYBOOK

你是执行后端 clean-room 的 AI。每轮开始前必须读本文件、`SPEC.md`、`target/STATE.md`、`target/CONCLUSIONS.md` 和目标模块契约。

## 1. 权威顺序

1. 当前产品意图
2. 模块契约
3. 清理后的代码结构
4. 新写测试
5. git 历史中的旧代码和旧测试事实

旧测试不能驱动实现。正式清理分支上，旧后端测试已经被删除；需要参考时只从 git 历史查。

## 1.1 “干净”的可执行定义

干净不是抽象多，也不是文件拆得多。

你清理后的代码应该满足：

- 读实现能直接理解业务意图。
- 函数短而直接，一个函数只负责一个主要动作。
- 状态变化显式，不靠隐藏副作用。
- 错误路径明确，不靠 silent fallback、best-effort、兼容 alias 掩盖问题。
- 分支少；每个分支都能说出当前产品理由。
- 命名准确，少用 `Manager`、`Helper`、`Util`、`Adapter` 这类模糊词。
- 抽象只在减少真实重复、切开明确边界、或稳定外部契约时使用。
- 不把旧复杂度搬到新 helper 里。

如果清理后代码行数少了但理解成本高了，不算干净。
如果清理后 interface 多了但职责没有更清楚，不算干净。
如果清理后只是把旧兼容藏到另一个文件，不算干净。

## 2. 每轮粒度和依赖扫描

默认一轮清理一个主文件。主文件是提交和主改动粒度，不是分析边界。

每轮必须先做 dependency scan：

- `rg` 查谁调用主文件里的公开函数、类型、方法。
- 查主文件调用了哪些下游接口、repo、service、helper。
- 查同包文件是否共享状态、helper、错误、常量、接口。
- 查是否存在跨层调用、循环依赖、隐性耦合。
- 查是否存在旧测试、旧 scheduler、legacy alias、dev/mock 对主逻辑的污染。

只有一个职责天然跨少数文件时，才允许升级为小职责切片。升级前要写明：

- 为什么单文件清理会失败。
- 本轮切片包含哪些文件。
- 本轮切片不包含哪些相邻问题。

不要把 scheduler、workflow、chat、loop、server wiring 混在一轮。

## 3. 先契约，后代码

清理任何模块前，先写 `target/contracts/<module>.md`。

契约必须回答：

- 模块负责什么？
- 模块不负责什么？
- 输入输出是什么？
- 错误如何表达？
- 是否允许副作用？
- 是否允许真实外部依赖？
- 哪些旧兼容必须删除？
- 需要哪些单元测试？
- 模块完成后需要哪些集成测试？

核心模块必须先写模块级契约，再选文件：

- workflow
- scheduler
- chat
- loop
- server wiring

这些模块如果契约写不清，不要开始改代码。

## 4. 清理判断

优先删除：

- 只服务旧测试的逻辑
- 注释中已声明未来删除的旧路径
- tolerated alias / 伪兼容字段
- 重复 adapter / resolver / registry
- 混在核心路径里的 dev/mock 行为
- 巨型文件里的无关职责

优先保留：

- 当前产品真实需要
- 外部 API 明确契约
- 数据迁移边界
- 权限、安全、sandbox 防护
- LLM provider wire compatibility

如果主文件清不干净：

- 不要局部粉饰。
- 不要把旧复杂度搬到新 helper。
- 记录前置依赖，或升级为小职责切片。
- 升级后仍要保持边界小而明确。

## 5. 测试重建

文件清理后写单元测试：

- 测业务规则
- 测错误边界
- 测状态转换
- 不测私有实现细节
- 不依赖旧 harness

模块清理完写模块集成测试：

- store + service 协作
- HTTP contract
- SQLite schema / transaction
- SSE / eventlog / notification
- sandbox / permissions / provider 边界

所有模块完成后写全链路集成测试：

- 只测当前产品关键旅程
- 不恢复旧 coverage matrix

最小启动 smoke：

- 从 Phase 3 开始长期保留。
- 只验证后端可启动、health 可过、用户初始化可过。
- 它是防跑偏闸门，不是覆盖率工具。

## 6. 每轮记录模板

每轮在 `target/rounds/NNNN/round.md` 写：

```md
# Round NNNN — <文件或职责>

类型:

目标:

Dependency scan:

旧代码问题:

模块契约:

删除内容:

保留内容:

新实现:

新单元测试:

模块集成测试:

验证:

实现是否更干净:

覆盖状态:

遗留问题:

下一步:
```

## 7. 验证纪律

每轮至少：

```bash
gofmt
go test <target package>
```

如果改共享接口，跑直接依赖包。

如果改 HTTP contract，跑相关 handler/router。

如果改 scheduler/workflow/chat，模块完成时必须补集成测试，不能只靠 unit。

从 Phase 3 开始，如果最小启动 smoke 已建立，每轮结束也要跑 smoke；如果 smoke 尚未建立，记录原因。

## 7.1 覆盖闭环

完成不是只看 `go test` 或 coverage 数字。最终必须闭合三张清单：

1. 文件清单：每个后端生产文件都标记为 `cleaned` / `deleted` / `generated` / `boundary-kept`。
2. 模块清单：每个模块都有契约、文件状态、模块集成测试或明确豁免。
3. 产品旅程清单：全链路测试覆盖当前产品关键旅程。

每轮结束时，如果本轮处理了文件或模块，必须更新对应覆盖状态。

coverage 数字只用于发现盲区，不能作为完成定义。

## 8. 停止条件

遇到以下情况必须停下来记录：

- 当前产品行为不清楚
- 数据迁移可能破坏已有用户数据
- 权限/sandbox/security 语义不确定
- 需要跨多个核心模块才能完成
- 清理后只能靠新增抽象才能解释，说明设计还没想清楚

停止不是失败；硬改才是失败。
