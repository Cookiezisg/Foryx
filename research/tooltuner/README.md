# tooltuner

持续、开放式的 **tool-call 质量优化引擎**,由 Claude Code agent 操作。让一套工具的全部 LLM-facing 面
(描述 / schema / 系统 prompt / 教学 / 示例)被模型**选得更对、用得更对**。没有终点,越转越好。

## 想跑一轮?
**读 [`PLAYBOOK.md`](./PLAYBOOK.md),然后照它做。** 那是司机手册(操作步骤 + 血换纪律 + G0-G10 先验 + 信任边界)。

## 这里有什么
| 文件 | 是什么 |
|---|---|
| `PLAYBOOK.md` | **跑一轮前读这个**(方法论皇冠)|
| `PRD.md` | 为啥做、做成啥样(产品) |
| `SPEC.md` | 怎么搭(架构 + 三层记忆数据契约) |
| `engine/` | 零件:`memory` `model_client` `run_model` `score` `ab` + `gen`/`judge` Workflow |
| `target/` | 被优化的 Forgify 工具集(91 工具 + 三层记忆) |

## 状态
- ✅ 建成 + 验证:`python3 engine/smoke_mock.py`(零 token 管线自测)· `python3 engine/smoke_live.py`(1 次真 DeepSeek,~¥0.002,全链路通)
- ⏳ 第一轮真迭代:读 PLAYBOOK 跑(烧少量预算)
- ⏳ 退役 `../llm-experiments/`(本工具取代它;验证满意后删)

## 信任边界(别忘)
闭环里**绝对分是 Claude 审美,不是生产真相;只信相对 / paired**。真执行兜 code,真实使用日志待产品上线补。
