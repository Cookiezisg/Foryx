/* Anselm feature — reference 画廊的声明式目录（纯数据，唯一事实源）。
   结构：category{ cat, icon, items:[ item{ name, tag, blurb, specimens:[ spec ] } ] }
   spec（一种状态/变体，sea.js build() 实体化）：{ label, span?, center?, tag, attrs?, props?, text?, html?, children?, demo? }
     · attrs：setAttribute（true=布尔属性）  · props：JS 属性（.rows/.data/.graph/.options/.items 等）  · text：textContent  · html：innerHTML（富文本体）
     · children：递归子 spec（slot 组合，子 attrs.slot 指定具名槽）  · demo：交互浮层触发（toast/dialog/menu，由 sea.js demoTrigger 收口）
   加一个原语/态 = 加一条数据，零改渲染器。本表由 catalog 工作流按各原语源码扎根生成 + 对抗校验。 */
window.REF_CATALOG = [
  {
    "cat": "基础 Foundation",
    "icon": "circle",
    "items": [
      {
        "name": "状态点 status-dot",
        "tag": "an-status-dot",
        "blurb": "语义色状态点；state ∈ idle/run/wait/err/done，run 唯一带 accent 呼吸动效",
        "specimens": [
          {
            "label": "state=idle",
            "tag": "an-status-dot",
            "center": true
          },
          {
            "label": "state=run",
            "tag": "an-status-dot",
            "attrs": {
              "state": "run"
            },
            "center": true
          },
          {
            "label": "state=wait",
            "tag": "an-status-dot",
            "attrs": {
              "state": "wait"
            },
            "center": true
          },
          {
            "label": "state=err",
            "tag": "an-status-dot",
            "attrs": {
              "state": "err"
            },
            "center": true
          },
          {
            "label": "state=done",
            "tag": "an-status-dot",
            "attrs": {
              "state": "done"
            },
            "center": true
          }
        ]
      },
      {
        "name": "状态徽 badge",
        "tag": "an-badge",
        "blurb": "状态/标签药丸：tone 语义柔底着色，可选 dot 状态点（复用 status-dot）",
        "specimens": [
          {
            "label": "neutral",
            "tag": "an-badge",
            "text": "draft"
          },
          {
            "label": "tone=ok",
            "tag": "an-badge",
            "attrs": {
              "tone": "ok"
            },
            "text": "ready"
          },
          {
            "label": "tone=warn",
            "tag": "an-badge",
            "attrs": {
              "tone": "warn"
            },
            "text": "parked"
          },
          {
            "label": "tone=danger",
            "tag": "an-badge",
            "attrs": {
              "tone": "danger"
            },
            "text": "failed"
          },
          {
            "label": "tone=accent",
            "tag": "an-badge",
            "attrs": {
              "tone": "accent"
            },
            "text": "durable"
          },
          {
            "label": "dot=run + tone=accent",
            "tag": "an-badge",
            "attrs": {
              "dot": "run",
              "tone": "accent"
            },
            "text": "running"
          },
          {
            "label": "dot=done + tone=ok",
            "tag": "an-badge",
            "attrs": {
              "dot": "done",
              "tone": "ok"
            },
            "text": "replay 通过"
          }
        ]
      },
      {
        "name": "加载骨架 skeleton",
        "tag": "an-skeleton",
        "blurb": "灰条 + shimmer 占位；variant ∈ row/card/text/lines，count 控制条数（row 镜像 an-row 度量、0 layout-shift）",
        "specimens": [
          {
            "label": "variant=row count=3",
            "span": true,
            "tag": "an-skeleton",
            "attrs": {
              "variant": "row",
              "count": 3
            }
          },
          {
            "label": "variant=card",
            "span": true,
            "tag": "an-skeleton",
            "attrs": {
              "variant": "card"
            }
          },
          {
            "label": "variant=text count=2",
            "span": true,
            "tag": "an-skeleton",
            "attrs": {
              "variant": "text",
              "count": 2
            }
          },
          {
            "label": "variant=lines count=4",
            "span": true,
            "tag": "an-skeleton",
            "attrs": {
              "variant": "lines",
              "count": 4
            }
          }
        ]
      },
      {
        "name": "实体提及药丸 ref-pill",
        "tag": "an-ref-pill",
        "blurb": "实体提及 = kind 图标 + label；id 非空才可点（派 an-ref→Intent.select），空 id = 纯标注",
        "specimens": [
          {
            "label": "kind=function (可点)",
            "tag": "an-ref-pill",
            "attrs": {
              "kind": "function",
              "id": "fn_5e1a9c4d",
              "label": "normalize_payload"
            }
          },
          {
            "label": "kind=handler",
            "tag": "an-ref-pill",
            "attrs": {
              "kind": "handler",
              "id": "hd_2f7b1a30",
              "label": "slack_notify"
            }
          },
          {
            "label": "kind=agent",
            "tag": "an-ref-pill",
            "attrs": {
              "kind": "agent",
              "id": "ag_91c3de07",
              "label": "triage_agent"
            }
          },
          {
            "label": "kind=workflow",
            "tag": "an-ref-pill",
            "attrs": {
              "kind": "workflow",
              "id": "wf_a04f8b12",
              "label": "nightly_etl"
            }
          },
          {
            "label": "kind=trigger",
            "tag": "an-ref-pill",
            "attrs": {
              "kind": "trigger",
              "id": "trg_0c5e6a11",
              "label": "cron · 02:00"
            }
          },
          {
            "label": "kind=mcp",
            "tag": "an-ref-pill",
            "attrs": {
              "kind": "mcp",
              "id": "mcp_github",
              "label": "github"
            }
          },
          {
            "label": "kind=skill",
            "tag": "an-ref-pill",
            "attrs": {
              "kind": "skill",
              "id": "skill_pdf",
              "label": "pdf"
            }
          },
          {
            "label": "无 id (纯标注、不可点)",
            "tag": "an-ref-pill",
            "attrs": {
              "kind": "workflow",
              "label": "未登记的 workflow"
            }
          }
        ]
      },
      {
        "name": "分组小标题 group-label",
        "tag": "an-group-label",
        "blurb": "极薄段标题（uppercase + meta 字号 + 600 + ink-3）；文本走默认 slot，纵向外距走皮肤",
        "specimens": [
          {
            "label": "default",
            "tag": "an-group-label",
            "text": "Entities"
          },
          {
            "label": "default",
            "tag": "an-group-label",
            "text": "Triggers"
          },
          {
            "label": "default",
            "tag": "an-group-label",
            "text": "Flowrun nodes"
          }
        ]
      },
      {
        "name": "标签集 tags",
        "tag": "an-tags",
        "blurb": "可增删 pill 集 + 末尾虚线 add 入口；items 走 JS 属性（[{label,health}|string]），health ∈ ok/bad→绿/红点，mode single/multi",
        "specimens": [
          {
            "label": "items (multi 默认)",
            "tag": "an-tags",
            "props": {
              "items": [
                "etl",
                "nightly",
                "durable"
              ]
            }
          },
          {
            "label": "health=ok/bad 点",
            "tag": "an-tags",
            "props": {
              "items": [
                {
                  "label": "github",
                  "health": "ok"
                },
                {
                  "label": "postgres",
                  "health": "bad"
                }
              ]
            }
          },
          {
            "label": "mode=single + add-label",
            "tag": "an-tags",
            "attrs": {
              "mode": "single",
              "add-label": "选环境"
            },
            "props": {
              "items": [
                {
                  "label": "prod"
                }
              ]
            }
          },
          {
            "label": "空 (— 无 —)",
            "tag": "an-tags",
            "props": {
              "items": []
            }
          }
        ]
      },
      {
        "name": "警示条 callout",
        "tag": "an-callout",
        "blurb": "整行宽警示条：左 tone 图标 + 富文本体（slot，可含 <b>）；tone ∈ warn/danger/info/ok，icon 可覆盖默认图标",
        "specimens": [
          {
            "label": "tone=warn (默认)",
            "span": true,
            "tag": "an-callout",
            "html": "此 workflow 含 <b>dangerous</b> 工具调用，replay 时将逐次内存阻塞确认。"
          },
          {
            "label": "tone=danger",
            "span": true,
            "tag": "an-callout",
            "attrs": {
              "tone": "danger"
            },
            "html": "flowrun <b>fnr_4a8c…</b> 在 node <b>fetch_orders</b> 处崩溃；游标已 parked，可从断点 replay。"
          },
          {
            "label": "tone=info",
            "span": true,
            "tag": "an-callout",
            "attrs": {
              "tone": "info"
            },
            "html": "节点结果已记忆化（<b>record-once</b>）：解释器幂等重走只补未完成节点。"
          },
          {
            "label": "tone=ok",
            "span": true,
            "tag": "an-callout",
            "attrs": {
              "tone": "ok"
            },
            "html": "全部节点 <b>done</b>，flowrun 已确定性完成，842ms。"
          },
          {
            "label": "icon 覆盖 (tone=info)",
            "span": true,
            "tag": "an-callout",
            "attrs": {
              "tone": "info",
              "icon": "trigger"
            },
            "html": "webhook 触发已去重（<b>idx_trf_dedup</b>），同一 firing 不重复展开。"
          }
        ]
      },
      {
        "name": "占位态 state",
        "tag": "an-state",
        "blurb": "空/加载/错误占位：居中 icon 井 + title + hint + action slot；variant ∈ empty/loading/error，icon 缺省按 variant 兜底",
        "specimens": [
          {
            "label": "variant=empty (默认)",
            "span": true,
            "tag": "an-state",
            "attrs": {
              "variant": "empty",
              "title": "还没有 workflow",
              "hint": "创建第一个工作流，或从模板导入。"
            }
          },
          {
            "label": "empty + action slot",
            "span": true,
            "tag": "an-state",
            "attrs": {
              "variant": "empty",
              "title": "没有 trigger",
              "hint": "挂一个 cron 或 webhook 来自动触发。"
            },
            "children": [
              {
                "tag": "an-button",
                "attrs": {
                  "slot": "action",
                  "variant": "primary",
                  "icon": "plus"
                },
                "text": "新建 trigger"
              }
            ]
          },
          {
            "label": "variant=loading",
            "span": true,
            "tag": "an-state",
            "attrs": {
              "variant": "loading",
              "title": "正在加载 flowrun…",
              "hint": "拉取节点记忆化结果。"
            }
          },
          {
            "label": "variant=error",
            "span": true,
            "tag": "an-state",
            "attrs": {
              "variant": "error",
              "title": "加载失败",
              "hint": "sidecar 未就绪（/api/v1/health 超时），请重试。"
            },
            "children": [
              {
                "tag": "an-button",
                "attrs": {
                  "slot": "action",
                  "variant": "ghost",
                  "icon": "refresh-cw"
                },
                "text": "重试"
              }
            ]
          },
          {
            "label": "icon 覆盖",
            "span": true,
            "tag": "an-state",
            "attrs": {
              "variant": "empty",
              "icon": "search",
              "title": "无匹配结果",
              "hint": "换个关键词，或清空筛选。"
            }
          }
        ]
      }
    ]
  },
  {
    "cat": "控件 Atoms",
    "icon": "square",
    "items": [
      {
        "name": "按钮 button",
        "tag": "an-button",
        "blurb": "variant(ghost/primary/danger/icon) + size + icon + disabled + block 的统一动作钮，label 走默认 slot",
        "specimens": [
          {
            "label": "ghost (默认)",
            "tag": "an-button",
            "text": "取消",
            "center": true
          },
          {
            "label": "variant=primary icon",
            "tag": "an-button",
            "attrs": {
              "variant": "primary",
              "icon": "play"
            },
            "text": "Run",
            "center": true
          },
          {
            "label": "variant=danger",
            "tag": "an-button",
            "attrs": {
              "variant": "danger",
              "icon": "trash"
            },
            "text": "删除",
            "center": true
          },
          {
            "label": "variant=icon",
            "tag": "an-button",
            "attrs": {
              "variant": "icon",
              "icon": "more"
            },
            "text": "更多动作",
            "center": true
          },
          {
            "label": "size=sm",
            "tag": "an-button",
            "attrs": {
              "size": "sm"
            },
            "text": ":iterate",
            "center": true
          },
          {
            "label": "disabled",
            "tag": "an-button",
            "attrs": {
              "variant": "primary",
              "disabled": true
            },
            "text": ":trigger",
            "center": true
          },
          {
            "label": "block",
            "span": true,
            "tag": "an-button",
            "attrs": {
              "variant": "primary",
              "icon": "play",
              "block": true
            },
            "text": "Trigger workflow"
          }
        ]
      },
      {
        "name": "输入框 input",
        "tag": "an-input",
        "blurb": "值叶子：单行/multiline/mono/full，attribute 设 value·placeholder·disabled·readonly，输入派发 an-input",
        "specimens": [
          {
            "label": "default",
            "tag": "an-input",
            "attrs": {
              "value": "fn_a1b2c3d4e5f6a7b8"
            },
            "center": true
          },
          {
            "label": "placeholder",
            "tag": "an-input",
            "attrs": {
              "placeholder": "搜索实体…"
            },
            "center": true
          },
          {
            "label": "mono",
            "tag": "an-input",
            "attrs": {
              "mono": true,
              "value": "0 */6 * * *"
            },
            "center": true
          },
          {
            "label": "readonly",
            "tag": "an-input",
            "attrs": {
              "readonly": true,
              "value": "fne_5e1a0b… (replay)"
            },
            "center": true
          },
          {
            "label": "disabled",
            "tag": "an-input",
            "attrs": {
              "disabled": true,
              "value": "已锁定"
            },
            "center": true
          },
          {
            "label": "full + multiline",
            "span": true,
            "tag": "an-input",
            "attrs": {
              "full": true,
              "multiline": true,
              "mono": true,
              "value": "request.method == \"POST\" && request.headers[\"x-signature\"] != \"\""
            }
          }
        ]
      },
      {
        "name": "下拉选择 dropdown",
        "tag": "an-dropdown",
        "blurb": "受控单选下拉（替 select），options 走 property [{value,label,meta,icon}]，选中派发 an-change",
        "specimens": [
          {
            "label": "已选 + meta",
            "tag": "an-dropdown",
            "attrs": {
              "value": "agent"
            },
            "props": {
              "options": [
                {
                  "value": "function",
                  "label": "Function",
                  "meta": "fn",
                  "icon": "function"
                },
                {
                  "value": "handler",
                  "label": "Handler",
                  "meta": "hd",
                  "icon": "handler"
                },
                {
                  "value": "agent",
                  "label": "Agent",
                  "meta": "ag",
                  "icon": "agent"
                },
                {
                  "value": "workflow",
                  "label": "Workflow",
                  "meta": "wf",
                  "icon": "workflow"
                }
              ]
            },
            "center": true
          },
          {
            "label": "placeholder (未选)",
            "tag": "an-dropdown",
            "attrs": {
              "placeholder": "选择触发源…"
            },
            "props": {
              "options": [
                {
                  "value": "cron",
                  "label": "Cron",
                  "meta": "0 */6 * * *"
                },
                {
                  "value": "webhook",
                  "label": "Webhook",
                  "meta": "POST"
                },
                {
                  "value": "manual",
                  "label": "Manual"
                }
              ]
            },
            "center": true
          },
          {
            "label": "disabled",
            "tag": "an-dropdown",
            "attrs": {
              "value": "claude",
              "disabled": true
            },
            "props": {
              "options": [
                {
                  "value": "claude",
                  "label": "claude-opus-4",
                  "meta": "anthropic"
                },
                {
                  "value": "fake",
                  "label": "fake_llm",
                  "meta": "0 token"
                }
              ]
            },
            "center": true
          },
          {
            "label": "block",
            "span": true,
            "tag": "an-dropdown",
            "attrs": {
              "value": "ready",
              "block": true
            },
            "props": {
              "options": [
                {
                  "value": "draft",
                  "label": "draft"
                },
                {
                  "value": "ready",
                  "label": "ready",
                  "meta": "可执行"
                },
                {
                  "value": "archived",
                  "label": "archived"
                }
              ]
            }
          }
        ]
      },
      {
        "name": "分段切换 segmented",
        "tag": "an-segmented",
        "blurb": "灰药丸分段器，items 走 property（字符串或 {value,label}），value 设初选，弹簧滑动，切换派发 an-pick",
        "specimens": [
          {
            "label": "items (字符串)",
            "tag": "an-segmented",
            "props": {
              "items": [
                "全部",
                "Function",
                "Handler",
                "Agent",
                "Workflow"
              ]
            },
            "center": true
          },
          {
            "label": "value=初选",
            "tag": "an-segmented",
            "props": {
              "value": "parked",
              "items": [
                {
                  "value": "running",
                  "label": "running"
                },
                {
                  "value": "parked",
                  "label": "parked"
                },
                {
                  "value": "done",
                  "label": "done"
                }
              ]
            },
            "center": true
          },
          {
            "label": "disabled",
            "tag": "an-segmented",
            "attrs": {
              "disabled": true
            },
            "props": {
              "items": [
                "only",
                "readonly"
              ]
            },
            "center": true
          }
        ]
      },
      {
        "name": "标签页 tabs",
        "tag": "an-tabs",
        "blurb": "文字下划线切换器，items 走 property [{key,label,count?,render}]，懒建 pane，切换派发 an-pick",
        "specimens": [
          {
            "label": "items + count",
            "span": true,
            "tag": "an-tabs",
            "props": {
              "items": [
                {
                  "key": "overview",
                  "label": "概览"
                },
                {
                  "key": "runs",
                  "label": "Flowruns",
                  "count": 12
                },
                {
                  "key": "parked",
                  "label": "Parked",
                  "count": 2
                },
                {
                  "key": "code",
                  "label": "代码"
                }
              ]
            }
          },
          {
            "label": "value=初选",
            "span": true,
            "tag": "an-tabs",
            "props": {
              "value": "events",
              "items": [
                {
                  "key": "api",
                  "label": "端点"
                },
                {
                  "key": "schema",
                  "label": "Schema"
                },
                {
                  "key": "events",
                  "label": "SSE 事件",
                  "count": 3
                }
              ]
            }
          },
          {
            "label": "disabled",
            "span": true,
            "tag": "an-tabs",
            "attrs": {
              "disabled": true
            },
            "props": {
              "items": [
                {
                  "key": "a",
                  "label": "只读"
                },
                {
                  "key": "b",
                  "label": "禁用态"
                }
              ]
            }
          }
        ]
      },
      {
        "name": "动作组 action-group",
        "tag": "an-action-group",
        "blurb": "按钮组：统一间距/对齐/事件委托，子按钮 data-action 上抛 an-action；end/compact/block/stack 控布局",
        "specimens": [
          {
            "label": "default (左对齐)",
            "tag": "an-action-group",
            "children": [
              {
                "tag": "an-button",
                "attrs": {
                  "variant": "primary",
                  "icon": "play"
                },
                "text": "Run"
              },
              {
                "tag": "an-button",
                "text": "取消"
              }
            ]
          },
          {
            "label": "end + compact",
            "span": true,
            "tag": "an-action-group",
            "attrs": {
              "end": true,
              "compact": true,
              "block": true
            },
            "children": [
              {
                "tag": "an-button",
                "attrs": {
                  "variant": "danger"
                },
                "text": "丢弃"
              },
              {
                "tag": "an-button",
                "text": "保存草稿"
              },
              {
                "tag": "an-button",
                "attrs": {
                  "variant": "primary"
                },
                "text": ":iterate"
              }
            ]
          },
          {
            "label": "stack (纵向 block)",
            "tag": "an-action-group",
            "attrs": {
              "stack": true,
              "block": true
            },
            "children": [
              {
                "tag": "an-button",
                "attrs": {
                  "block": true,
                  "icon": "play"
                },
                "text": "Resume flowrun"
              },
              {
                "tag": "an-button",
                "attrs": {
                  "block": true,
                  "icon": "history"
                },
                "text": "Replay from node"
              },
              {
                "tag": "an-button",
                "attrs": {
                  "block": true,
                  "variant": "danger",
                  "icon": "trash"
                },
                "text": "Cancel run"
              }
            ]
          }
        ]
      }
    ]
  },
  {
    "cat": "行与卡 Rows & Cards",
    "icon": "list",
    "items": [
      {
        "name": "行 row",
        "tag": "an-row",
        "blurb": "唯一的“一行”：dot/icon · label · hint · meta + actions 槽；selected/collapsible/passive/depth 修饰",
        "specimens": [
          {
            "label": "default",
            "tag": "an-row",
            "span": true,
            "attrs": {
              "icon": "function",
              "label": "summarize_text",
              "meta": "function"
            }
          },
          {
            "label": "dot=run",
            "tag": "an-row",
            "span": true,
            "attrs": {
              "dot": "run",
              "label": "manual · 执行中",
              "meta": "842ms"
            }
          },
          {
            "label": "selected",
            "tag": "an-row",
            "span": true,
            "attrs": {
              "icon": "agent",
              "label": "triage-agent",
              "meta": "agent",
              "selected": true
            }
          },
          {
            "label": "hint",
            "tag": "an-row",
            "span": true,
            "attrs": {
              "icon": "handler",
              "label": "slack.post_message",
              "hint": "经 webhook 触发，向 #ops 频道投递 flowrun 失败通知",
              "meta": "handler"
            }
          },
          {
            "label": "collapsible open",
            "tag": "an-row",
            "span": true,
            "attrs": {
              "collapsible": true,
              "open": true,
              "label": "env · 环境变量",
              "meta": "3 项"
            }
          },
          {
            "label": "passive",
            "tag": "an-row",
            "span": true,
            "attrs": {
              "icon": "doc",
              "label": "只读说明行（不可选中）",
              "meta": "passive",
              "passive": true
            }
          },
          {
            "label": "depth=2",
            "tag": "an-row",
            "span": true,
            "attrs": {
              "dot": "done",
              "label": "subagent · 子节点",
              "meta": "120ms",
              "depth": 2
            }
          },
          {
            "label": "actions 槽",
            "tag": "an-row",
            "span": true,
            "attrs": {
              "icon": "workflow",
              "label": "nightly-report",
              "meta": "workflow"
            },
            "children": [
              {
                "tag": "an-button",
                "attrs": {
                  "slot": "actions",
                  "variant": "icon",
                  "icon": "play"
                }
              },
              {
                "tag": "an-button",
                "attrs": {
                  "slot": "actions",
                  "variant": "icon",
                  "icon": "edit"
                }
              }
            ]
          }
        ]
      },
      {
        "name": "可展开行 row-detail",
        "tag": "an-row-detail",
        "blurb": "一条 an-row（slot=row）+ 下方详情面板（默认 slot，常放 an-kv）；点行切 [open] 并高亮",
        "specimens": [
          {
            "label": "open（点行收起）",
            "tag": "an-row-detail",
            "span": true,
            "attrs": {
              "open": true
            },
            "children": [
              {
                "tag": "an-row",
                "attrs": {
                  "slot": "row",
                  "dot": "done",
                  "label": "manual · 完成",
                  "meta": "842ms"
                }
              },
              {
                "tag": "an-kv",
                "attrs": {
                  "wrap": true
                },
                "props": {
                  "rows": [
                    [
                      "flowrun ID",
                      "fr_5e1a9c2b4d7f0a13"
                    ],
                    [
                      "触发",
                      "manual"
                    ],
                    [
                      "节点数",
                      "5 / 5"
                    ],
                    [
                      "耗时",
                      "842ms"
                    ]
                  ]
                }
              }
            ]
          },
          {
            "label": "collapsed（点行展开）",
            "tag": "an-row-detail",
            "span": true,
            "children": [
              {
                "tag": "an-row",
                "attrs": {
                  "slot": "row",
                  "dot": "err",
                  "label": "cron · 失败",
                  "meta": "退出码 1"
                }
              },
              {
                "tag": "an-kv",
                "props": {
                  "rows": [
                    [
                      "flowrun ID",
                      "fr_91af3e7c08b25d6e"
                    ],
                    [
                      "触发",
                      "cron · 0 9 * * *"
                    ],
                    [
                      "失败节点",
                      "transform_rows"
                    ],
                    [
                      "错误码",
                      "FN_RUNTIME_PANIC"
                    ]
                  ]
                }
              }
            ]
          }
        ]
      },
      {
        "name": "信息卡 info-card",
        "tag": "an-info-card",
        "blurb": "无边信息单元：head（icon+title+meta）按需出现，body 走默认 slot，动作走 slot=actions",
        "specimens": [
          {
            "label": "title+icon+meta",
            "tag": "an-info-card",
            "span": true,
            "attrs": {
              "title": "Schedule",
              "icon": "clock",
              "meta": "UTC"
            },
            "children": [
              {
                "tag": "an-kv",
                "props": {
                  "rows": [
                    [
                      "cron",
                      "0 9 * * 1-5"
                    ],
                    [
                      "下次触发",
                      "2026-06-18 09:00"
                    ],
                    [
                      "时区",
                      "UTC"
                    ]
                  ]
                }
              }
            ]
          },
          {
            "label": "title only",
            "tag": "an-info-card",
            "span": true,
            "attrs": {
              "title": "输入契约"
            },
            "children": [
              {
                "tag": "an-kv",
                "attrs": {
                  "mono": true
                },
                "props": {
                  "rows": [
                    [
                      "text",
                      "string · required"
                    ],
                    [
                      "max_tokens",
                      "number · 256"
                    ],
                    [
                      "lang",
                      "string · auto"
                    ]
                  ]
                }
              }
            ]
          },
          {
            "label": "无 head（纯 body）",
            "tag": "an-info-card",
            "span": true,
            "children": [
              {
                "tag": "an-row",
                "attrs": {
                  "icon": "function",
                  "label": "summarize_text",
                  "meta": "function",
                  "passive": true
                }
              },
              {
                "tag": "an-row",
                "attrs": {
                  "icon": "agent",
                  "label": "triage-agent",
                  "meta": "agent",
                  "passive": true
                }
              }
            ]
          },
          {
            "label": "actions 槽",
            "tag": "an-info-card",
            "span": true,
            "attrs": {
              "title": "Trigger",
              "icon": "bolt",
              "meta": "webhook"
            },
            "children": [
              {
                "tag": "an-kv",
                "props": {
                  "rows": [
                    [
                      "URL",
                      "/hooks/inbound"
                    ],
                    [
                      "去重",
                      "idx_trf_dedup"
                    ],
                    [
                      "最近 firing",
                      "12s 前"
                    ]
                  ]
                }
              },
              {
                "tag": "an-button",
                "attrs": {
                  "slot": "actions",
                  "variant": "ghost"
                },
                "text": "编辑"
              },
              {
                "tag": "an-button",
                "attrs": {
                  "slot": "actions",
                  "variant": "ghost"
                },
                "text": "禁用"
              }
            ]
          }
        ]
      },
      {
        "name": "段 section",
        "tag": "an-section",
        "blurb": "小节标题 + 无边内容区；variant=plain 走文档型大标题，grid 转响应式 2 列块网格",
        "specimens": [
          {
            "label": "default（meta 大写小标）",
            "tag": "an-section",
            "span": true,
            "attrs": {
              "label": "执行"
            },
            "children": [
              {
                "tag": "an-row",
                "attrs": {
                  "dot": "run",
                  "label": "flowrun 进行中",
                  "meta": "3 / 5 节点"
                }
              },
              {
                "tag": "an-row",
                "attrs": {
                  "dot": "done",
                  "label": "上次成功",
                  "meta": "842ms"
                }
              }
            ]
          },
          {
            "label": "variant=plain（文档大标题）",
            "tag": "an-section",
            "span": true,
            "attrs": {
              "label": "Durable Execution",
              "variant": "plain"
            },
            "children": [
              {
                "tag": "an-row",
                "attrs": {
                  "icon": "doc",
                  "label": "节点结果记忆化 + 解释器幂等重走",
                  "passive": true
                }
              }
            ]
          },
          {
            "label": "grid（2 列块网格）",
            "tag": "an-section",
            "span": true,
            "attrs": {
              "label": "环境",
              "grid": true
            },
            "children": [
              {
                "tag": "an-info-card",
                "attrs": {
                  "title": "输入",
                  "icon": "arrow-down-to-line"
                },
                "children": [
                  {
                    "tag": "an-kv",
                    "props": {
                      "rows": [
                        [
                          "text",
                          "string"
                        ],
                        [
                          "lang",
                          "auto"
                        ]
                      ]
                    }
                  }
                ]
              },
              {
                "tag": "an-info-card",
                "attrs": {
                  "title": "输出",
                  "icon": "arrow-up-from-line"
                },
                "children": [
                  {
                    "tag": "an-kv",
                    "props": {
                      "rows": [
                        [
                          "summary",
                          "string"
                        ],
                        [
                          "tokens",
                          "204"
                        ]
                      ]
                    }
                  }
                ]
              }
            ]
          }
        ]
      },
      {
        "name": "键值大行 field",
        "tag": "an-field",
        "blurb": "label + 值键值大行；value 属性显值（editable 原地编辑），无 value 走默认 slot；editor=select 用 dropdown",
        "specimens": [
          {
            "label": "value（只读）",
            "tag": "an-field",
            "span": true,
            "attrs": {
              "label": "运行时",
              "value": "python3.12 · venv"
            }
          },
          {
            "label": "value + hint",
            "tag": "an-field",
            "span": true,
            "attrs": {
              "label": "超时",
              "value": "30s",
              "hint": "节点级硬超时，超时即标记 flowrun 失败并 park"
            }
          },
          {
            "label": "editable（原地编辑）",
            "tag": "an-field",
            "span": true,
            "attrs": {
              "label": "描述",
              "value": "把长文本压缩成一句摘要",
              "editable": true
            }
          },
          {
            "label": "editor=select",
            "tag": "an-field",
            "span": true,
            "attrs": {
              "label": "danger",
              "value": "cautious",
              "editable": true,
              "editor": "select"
            },
            "props": {
              "options": [
                "safe",
                "cautious",
                "dangerous"
              ]
            }
          },
          {
            "label": "空值（占位 —）",
            "tag": "an-field",
            "span": true,
            "attrs": {
              "label": "上次错误",
              "value": "",
              "editable": true
            }
          },
          {
            "label": "slot 值（无 value）",
            "tag": "an-field",
            "span": true,
            "attrs": {
              "label": "状态"
            },
            "children": [
              {
                "tag": "an-badge",
                "attrs": {
                  "dot": "run",
                  "tone": "accent"
                },
                "text": "running"
              }
            ]
          }
        ]
      },
      {
        "name": "定义列表 kv",
        "tag": "an-kv",
        "blurb": "紧凑键值列表；rows 是 PROP（[k,v] 或 {key,value,editable,editor,options}），值右贴边、过长自动换行；mono/wrap 修饰",
        "specimens": [
          {
            "label": "default",
            "tag": "an-kv",
            "span": true,
            "props": {
              "rows": [
                [
                  "类型",
                  "function"
                ],
                [
                  "运行次数",
                  "1,204"
                ],
                [
                  "平均耗时",
                  "842ms"
                ],
                [
                  "最近触发",
                  "12s 前"
                ]
              ]
            }
          },
          {
            "label": "mono",
            "tag": "an-kv",
            "span": true,
            "attrs": {
              "mono": true
            },
            "props": {
              "rows": [
                [
                  "flowrun ID",
                  "fr_5e1a9c2b4d7f0a13"
                ],
                [
                  "node ID",
                  "fnn_91af3e7c08b25d6e"
                ],
                [
                  "entity ID",
                  "fn_0a13c4d7f08b25e6"
                ]
              ]
            }
          },
          {
            "label": "wrap（长值多行）",
            "tag": "an-kv",
            "span": true,
            "attrs": {
              "wrap": true
            },
            "props": {
              "rows": [
                [
                  "stdout",
                  "Traceback (most recent call last): File \"main.py\", line 42, in transform raise ValueError(\"empty rows\")"
                ],
                [
                  "错误码",
                  "FN_RUNTIME_PANIC"
                ]
              ]
            }
          },
          {
            "label": "editable（原地编辑）",
            "tag": "an-kv",
            "span": true,
            "props": {
              "rows": [
                {
                  "key": "名称",
                  "value": "summarize_text",
                  "editable": true
                },
                {
                  "key": "标签",
                  "value": "nlp",
                  "editable": true
                }
              ]
            }
          },
          {
            "label": "editable + select",
            "tag": "an-kv",
            "span": true,
            "props": {
              "rows": [
                {
                  "key": "可见性",
                  "value": "workspace",
                  "editable": true,
                  "editor": "select",
                  "options": [
                    "private",
                    "workspace",
                    "shared"
                  ]
                }
              ]
            }
          },
          {
            "label": "空值（占位 —）",
            "tag": "an-kv",
            "span": true,
            "props": {
              "rows": [
                [
                  "上次错误",
                  ""
                ],
                [
                  "park 原因",
                  ""
                ]
              ]
            }
          }
        ]
      },
      {
        "name": "瘦表 thin-table",
        "tag": "an-thin-table",
        "blurb": "对齐多列非表格：columns/rows 走 PROP（columns=[{key,label,align}]·rows=[{k:v}]），无表格 chrome；selectable 行可点派 an-row-click",
        "specimens": [
          {
            "label": "default",
            "tag": "an-thin-table",
            "span": true,
            "props": {
              "columns": [
                {
                  "key": "node",
                  "label": "节点"
                },
                {
                  "key": "kind",
                  "label": "kind"
                },
                {
                  "key": "ms",
                  "label": "耗时",
                  "align": "right"
                }
              ],
              "rows": [
                {
                  "node": "fetch_rows",
                  "kind": "function",
                  "ms": "120ms"
                },
                {
                  "node": "transform",
                  "kind": "function",
                  "ms": "318ms"
                },
                {
                  "node": "post_slack",
                  "kind": "handler",
                  "ms": "404ms"
                }
              ]
            }
          },
          {
            "label": "align（center/right）",
            "tag": "an-thin-table",
            "span": true,
            "props": {
              "columns": [
                {
                  "key": "iter",
                  "label": "iteration",
                  "align": "center"
                },
                {
                  "key": "node",
                  "label": "节点"
                },
                {
                  "key": "status",
                  "label": "状态",
                  "align": "center"
                },
                {
                  "key": "ms",
                  "label": "耗时",
                  "align": "right"
                }
              ],
              "rows": [
                {
                  "iter": "0",
                  "node": "fetch_rows",
                  "status": "done",
                  "ms": "120ms"
                },
                {
                  "iter": "0",
                  "node": "transform",
                  "status": "done",
                  "ms": "318ms"
                },
                {
                  "iter": "1",
                  "node": "post_slack",
                  "status": "replay",
                  "ms": "—"
                }
              ]
            }
          },
          {
            "label": "selectable（行可点）",
            "tag": "an-thin-table",
            "span": true,
            "attrs": {
              "selectable": true
            },
            "props": {
              "columns": [
                {
                  "key": "id",
                  "label": "flowrun"
                },
                {
                  "key": "trigger",
                  "label": "触发"
                },
                {
                  "key": "result",
                  "label": "结果",
                  "align": "right"
                }
              ],
              "rows": [
                {
                  "id": "fr_5e1a9c2b",
                  "trigger": "manual",
                  "result": "done"
                },
                {
                  "id": "fr_91af3e7c",
                  "trigger": "cron",
                  "result": "parked"
                },
                {
                  "id": "fr_0a13c4d7",
                  "trigger": "webhook",
                  "result": "fail"
                }
              ]
            }
          },
          {
            "label": "empty（无数据行）",
            "tag": "an-thin-table",
            "span": true,
            "props": {
              "columns": [
                {
                  "key": "node",
                  "label": "节点"
                },
                {
                  "key": "ms",
                  "label": "耗时",
                  "align": "right"
                }
              ],
              "rows": []
            }
          }
        ]
      }
    ]
  },
  {
    "cat": "代码与数据 Code & Data",
    "icon": "code",
    "items": [
      {
        "name": "代码块 code-editor",
        "tag": "an-code-editor",
        "blurb": "唯一代码块/轻编辑原语：code 经 text 入，attrs lang/editable/compact/wrap/inline；非 inline editable 默认只读点编辑，inline editable 常驻编辑",
        "specimens": [
          {
            "label": "lang=py 只读",
            "span": true,
            "tag": "an-code-editor",
            "attrs": {
              "lang": "py"
            },
            "text": "def transform(event):\n    # 归一化 webhook payload\n    user = event[\"sender\"][\"login\"]\n    return {\"actor\": user, \"action\": event.get(\"action\", \"open\")}"
          },
          {
            "label": "lang=cel 只读",
            "span": true,
            "tag": "an-code-editor",
            "attrs": {
              "lang": "cel"
            },
            "text": "payload.action == \"opened\" && payload.pull_request.draft == false"
          },
          {
            "label": "editable（点编辑进编辑态）",
            "span": true,
            "tag": "an-code-editor",
            "attrs": {
              "lang": "py",
              "editable": true
            },
            "text": "def handler(ctx, args):\n    rows = ctx.db.query(args[\"sql\"])\n    return {\"count\": len(rows), \"rows\": rows}"
          },
          {
            "label": "inline editable（run-terminal args 常驻编辑）",
            "span": true,
            "tag": "an-code-editor",
            "attrs": {
              "lang": "sh",
              "inline": true,
              "editable": true
            },
            "text": "python ingest.py --workspace ws_4a2f --limit 50"
          },
          {
            "label": "inline 只读（version-diff 行内板）",
            "span": true,
            "tag": "an-code-editor",
            "attrs": {
              "lang": "json",
              "inline": true
            },
            "text": "{\"flowrunId\": \"flr_91c3e2\", \"status\": \"parked\"}"
          },
          {
            "label": "wrap 自动换行",
            "span": true,
            "tag": "an-code-editor",
            "attrs": {
              "lang": "js",
              "wrap": true
            },
            "text": "const url = `https://hooks.anselm.local/api/v1/triggers/${triggerId}/fire?token=${encodeURIComponent(secret)}&replay=true`;"
          },
          {
            "label": "compact 紧凑密度",
            "span": true,
            "tag": "an-code-editor",
            "attrs": {
              "lang": "yaml",
              "compact": true
            },
            "text": "schedule: \"0 */6 * * *\"\nentity: wf_nightly_sync\nparked_resume: true"
          }
        ]
      },
      {
        "name": "JSON 树 json-tree",
        "tag": "an-json-tree",
        "blurb": "唯一结构化展示原语：JSON 解析成可折叠树；数据优先 props.data={...} 否则 attrs.json 串；attrs label/open-depth/root（root=\"false\" 隐根行）",
        "specimens": [
          {
            "label": "props.data 默认",
            "span": true,
            "tag": "an-json-tree",
            "props": {
              "data": {
                "flowrunId": "flr_91c3e2a7",
                "status": "running",
                "entity": {
                  "kind": "workflow",
                  "name": "nightly-sync"
                },
                "nodes": [
                  {
                    "id": "n_fetch",
                    "state": "done",
                    "ms": 842
                  },
                  {
                    "id": "n_transform",
                    "state": "running"
                  }
                ],
                "parked": false
              }
            }
          },
          {
            "label": "label=trigger",
            "span": true,
            "tag": "an-json-tree",
            "attrs": {
              "label": "trigger"
            },
            "props": {
              "data": {
                "source": "webhook",
                "secret": "whsec_••••",
                "filter": "payload.action == \"opened\"",
                "dedup": true,
                "firings": 12
              }
            }
          },
          {
            "label": "root=false（隐根行）",
            "span": true,
            "tag": "an-json-tree",
            "attrs": {
              "root": "false"
            },
            "props": {
              "data": {
                "venv": ".venv",
                "python": "3.12.4",
                "packages": [
                  "requests==2.32",
                  "pydantic==2.7"
                ],
                "timeoutSec": 30
              }
            }
          },
          {
            "label": "open-depth=0（全折叠）",
            "span": true,
            "tag": "an-json-tree",
            "attrs": {
              "label": "agent",
              "open-depth": "0"
            },
            "props": {
              "data": {
                "name": "triage-bot",
                "model": "opus-4",
                "tools": [
                  "read_file",
                  "run_function",
                  "park"
                ],
                "memory": {
                  "window": 12,
                  "summarize": true
                }
              }
            }
          },
          {
            "label": "attrs.json 串",
            "span": true,
            "tag": "an-json-tree",
            "attrs": {
              "label": "mcp",
              "json": "{\"server\":\"filesystem\",\"transport\":\"stdio\",\"tools\":[\"list_dir\",\"read_text\"],\"enabled\":true}"
            }
          },
          {
            "label": "invalid JSON 兜底",
            "span": true,
            "tag": "an-json-tree",
            "attrs": {
              "json": "{ status: parked, }"
            }
          }
        ]
      },
      {
        "name": "版本对比 version-diff",
        "tag": "an-version-diff",
        "blurb": "单框 unified diff（旧→新逐行 LCS，+绿 −红）；内容经 props.before/after 设入；attrs lang/range/note/bare（隐顶栏内联）",
        "specimens": [
          {
            "label": "range+note · py 改动",
            "span": true,
            "tag": "an-version-diff",
            "attrs": {
              "lang": "py",
              "range": "v3 → v4",
              "note": "改 timeout 容错 + 增 parked 分支"
            },
            "props": {
              "before": "def run(ctx, args):\n    res = ctx.call(args[\"sql\"], timeout=10)\n    return res",
              "after": "def run(ctx, args):\n    res = ctx.call(args[\"sql\"], timeout=30)\n    if res.parked:\n        return ctx.park(\"awaiting approval\")\n    return res"
            }
          },
          {
            "label": "首版（before 空 → 整段 ctx）",
            "span": true,
            "tag": "an-version-diff",
            "attrs": {
              "lang": "py",
              "range": "v1",
              "note": "初版"
            },
            "props": {
              "before": "",
              "after": "def handler(ctx, args):\n    return {\"ok\": True}"
            }
          },
          {
            "label": "lang=cel · 纯改",
            "span": true,
            "tag": "an-version-diff",
            "attrs": {
              "lang": "cel",
              "range": "v2 → v3",
              "note": "收紧 webhook 过滤"
            },
            "props": {
              "before": "payload.action == \"opened\"",
              "after": "payload.action == \"opened\" && payload.pull_request.draft == false"
            }
          },
          {
            "label": "bare（隐顶栏 · 内联场景）",
            "span": true,
            "tag": "an-version-diff",
            "attrs": {
              "lang": "json",
              "bare": true
            },
            "props": {
              "before": "{\n  \"limit\": 20,\n  \"cursor\": null\n}",
              "after": "{\n  \"limit\": 50,\n  \"cursor\": null,\n  \"replay\": true\n}"
            }
          }
        ]
      }
    ]
  },
  {
    "cat": "执行块 Blocks",
    "icon": "blocks",
    "items": [
      {
        "name": "块流 block-tree",
        "tag": "an-block-tree",
        "blurb": "对话/agent transcript 统一渲染面，.blocks 走 JS 属性；9 块型（text/reasoning/tool_call/tool_result/progress/compaction/turnEnd/todo/subtree）；tool 结果按形态分派（终端/列表/JSON/error 标红）· turnEnd 按 stopReason 分态 · pokeText/pokeLog 流式增量",
        "specimens": [
          {
            "label": "text · user+assistant",
            "span": true,
            "tag": "an-block-tree",
            "props": {
              "blocks": [
                {
                  "type": "text",
                  "role": "user",
                  "text": "帮我把 `sync_inventory` function 接到每天 9 点的 cron trigger 上。"
                },
                {
                  "type": "text",
                  "text": "好的。我会先创建 **cron trigger**（`0 9 * * *`），再把它的 `firing` 接到 `sync_inventory` 的 `:run`。下面开始执行。"
                }
              ]
            }
          },
          {
            "label": "reasoning · open",
            "span": true,
            "tag": "an-block-tree",
            "props": {
              "blocks": [
                {
                  "type": "reasoning",
                  "label": "推理",
                  "open": true,
                  "text": "用户要每天定时同步库存。\n先确认 sync_inventory 是 function 实体且无未就绪 env，再建 cron。\ncron 表达式取 0 9 * * *（本地时区）。"
                }
              ]
            }
          },
          {
            "label": "tool_call · running",
            "span": true,
            "tag": "an-block-tree",
            "props": {
              "blocks": [
                {
                  "type": "tool_call",
                  "running": true,
                  "status": "正在创建 cron trigger…",
                  "items": [
                    {
                      "verb": "create_trigger",
                      "name": "cron · 0 9 * * *"
                    }
                  ]
                }
              ]
            }
          },
          {
            "label": "tool_call · settled+open",
            "span": true,
            "tag": "an-block-tree",
            "props": {
              "blocks": [
                {
                  "type": "tool_call",
                  "open": true,
                  "items": [
                    {
                      "verb": "run_function",
                      "name": "sync_inventory",
                      "danger": "safe",
                      "args": {
                        "warehouse": "SG-01",
                        "dryRun": false
                      },
                      "progress": {
                        "done": true,
                        "lines": [
                          "→ spawn venv",
                          "→ python sync_inventory.py",
                          "stdout: fetched 1820 SKUs",
                          "stdout: upserted 1820 rows"
                        ]
                      },
                      "result": {
                        "json": {
                          "synced": 1820,
                          "skipped": 0,
                          "ms": 4120
                        }
                      }
                    }
                  ]
                }
              ]
            }
          },
          {
            "label": "tool_call · danger gate",
            "span": true,
            "tag": "an-block-tree",
            "props": {
              "blocks": [
                {
                  "type": "tool_call",
                  "open": true,
                  "items": [
                    {
                      "verb": "Bash",
                      "name": "Bash",
                      "danger": "dangerous",
                      "gate": true,
                      "summary": "将在沙箱内删除过期快照目录。",
                      "args": "{\n  \"command\": \"rm -rf /data/snapshots/2024-*\"\n}"
                    }
                  ]
                }
              ]
            }
          },
          {
            "label": "tool_result · 独立块",
            "span": true,
            "tag": "an-block-tree",
            "props": {
              "blocks": [
                {
                  "type": "tool_result",
                  "label": "输出 · call handler",
                  "data": {
                    "status": 200,
                    "body": {
                      "ok": true,
                      "deliveryId": "del_9f2a3c1d7e0b4a55"
                    }
                  }
                }
              ]
            }
          },
          {
            "label": "progress · live",
            "span": true,
            "tag": "an-block-tree",
            "props": {
              "blocks": [
                {
                  "type": "progress",
                  "label": "日志 · stderr",
                  "done": false,
                  "lines": [
                    "→ flowrun frn_8a1c… 进入节点 fetch",
                    "→ 节点结果记忆化 (iteration=0)",
                    "→ 进入节点 transform"
                  ]
                }
              ]
            }
          },
          {
            "label": "turnEnd · max_steps",
            "span": true,
            "tag": "an-block-tree",
            "props": {
              "blocks": [
                {
                  "type": "turnEnd",
                  "code": "MAX_STEPS_REACHED",
                  "continueLabel": "继续"
                }
              ]
            }
          },
          {
            "label": "subtree · subagent",
            "span": true,
            "tag": "an-block-tree",
            "props": {
              "blocks": [
                {
                  "type": "subtree",
                  "label": "subagent · general-purpose",
                  "open": true,
                  "blocks": [
                    {
                      "type": "text",
                      "text": "诊断 flowrun `frn_8a1c…` 的 parked 节点。"
                    },
                    {
                      "type": "tool_call",
                      "open": true,
                      "items": [
                        {
                          "verb": "get_flowrun",
                          "name": "frn_8a1c…",
                          "result": {
                            "json": {
                              "flowrun": { "id": "frn_8a1c4f2e", "status": "running" },
                              "nodes": [{ "nodeId": "review_gate", "kind": "approval", "status": "parked" }]
                            }
                          }
                        }
                      ]
                    }
                  ]
                }
              ]
            }
          },
          {
            "label": "compaction",
            "span": true,
            "tag": "an-block-tree",
            "props": {
              "blocks": [
                {
                  "type": "compaction",
                  "coversUpToSeq": 18,
                  "summarizedCount": 6
                }
              ]
            }
          },
          {
            "label": "todo · 任务清单看板（3 态）",
            "span": true,
            "tag": "an-block-tree",
            "props": {
              "blocks": [
                {
                  "type": "todo",
                  "open": true,
                  "items": [
                    { "content": "检索竞品资料", "status": "completed" },
                    { "content": "抓取并摘要文档", "status": "in_progress", "activeForm": "正在抓取 temporal.io/docs" },
                    { "content": "汇总成要点", "status": "pending" }
                  ]
                }
              ]
            }
          },
          {
            "label": "tool_call · error 失败态",
            "span": true,
            "tag": "an-block-tree",
            "props": {
              "blocks": [
                {
                  "type": "tool_call",
                  "open": true,
                  "items": [
                    { "verb": "Read", "name": "docs/competitors.md", "error": "ENOENT: no such file or directory, open 'docs/competitors.md'" }
                  ]
                }
              ]
            }
          },
          {
            "label": "tool_result · 终端文本(Bash)",
            "span": true,
            "tag": "an-block-tree",
            "props": {
              "blocks": [
                {
                  "type": "tool_result",
                  "term": "removed /data/snapshots/2024-01 … 2024-12 (12 dirs)\nfreed 3.4 GB\n\n[exit code: 0]"
                }
              ]
            }
          },
          {
            "label": "tool_result · 搜索列表(WebSearch)",
            "span": true,
            "tag": "an-block-tree",
            "props": {
              "blocks": [
                {
                  "type": "tool_result",
                  "list": [
                    { "title": "Temporal: Durable Execution", "meta": "temporal.io", "hint": "事件溯源 + 确定性重放" },
                    { "title": "Restate: Durable Execution & State", "meta": "restate.dev", "hint": "日志式 durable，handler SDK 侵入" }
                  ]
                }
              ]
            }
          },
          {
            "label": "turnEnd · 终态变体（max_tokens/cancelled/error）",
            "span": true,
            "tag": "an-block-tree",
            "props": {
              "blocks": [
                { "type": "turnEnd", "stopReason": "max_tokens" },
                { "type": "turnEnd", "stopReason": "cancelled" },
                { "type": "turnEnd", "stopReason": "error", "code": "TOOL_ERROR_STORM" }
              ]
            }
          }
        ]
      },
      {
        "name": "试运行终端 run-terminal",
        "tag": "an-run-terminal",
        "blurb": "执行型实体试运行块：顶栏 run 钮(verb/vico)+语言标签+可编辑 args(lang)，点击逐行吐 stdout→终态摘要；data-trace 驱动 mock，gate 闸态只渲一行盾",
        "specimens": [
          {
            "label": "default · function",
            "span": true,
            "tag": "an-run-terminal",
            "attrs": {
              "verb": "运行",
              "vico": "play",
              "lang": "json",
              "args": "{\n  \"warehouse\": \"SG-01\",\n  \"dryRun\": false\n}",
              "data-trace": "{\"lines\":[\"→ spawn venv\",\"→ python sync_inventory.py\",\"stdout: fetched 1820 SKUs\",\"stdout: upserted 1820 rows\"],\"result\":{\"st\":\"ok\",\"out\":\"done\",\"ms\":4120,\"json\":{\"synced\":1820,\"skipped\":0}}}"
            }
          },
          {
            "label": "verb=:call · handler",
            "span": true,
            "tag": "an-run-terminal",
            "attrs": {
              "verb": "调用",
              "vico": "webhook",
              "lang": "json",
              "args": "{\n  \"event\": \"order.created\",\n  \"orderId\": \"ord_77c1\"\n}",
              "data-trace": "{\"lines\":[\"→ dispatch webhook handler\",\"stdout: 200 OK\"],\"result\":{\"st\":\"ok\",\"out\":\"delivered\",\"ms\":312,\"json\":{\"status\":200,\"deliveryId\":\"del_9f2a3c1d\"}}}"
            }
          },
          {
            "label": "err · result.st=error",
            "span": true,
            "tag": "an-run-terminal",
            "attrs": {
              "verb": "运行",
              "vico": "play",
              "lang": "python",
              "args": "warehouse = \"SG-99\"\nsync(warehouse, dry_run=True)",
              "data-trace": "{\"lines\":[\"→ spawn venv\",\"→ python sync_inventory.py\",\"stderr: KeyError: 'SG-99'\"],\"result\":{\"st\":\"error\",\"out\":\"KeyError: 'SG-99'\",\"ms\":880}}"
            }
          },
          {
            "label": "gate · env 未就绪",
            "span": true,
            "tag": "an-run-terminal",
            "attrs": {
              "gate": "env 未就绪：venv 缺 requests==2.* · 先安装依赖后方可运行"
            }
          }
        ]
      },
      {
        "name": "审批门 approval-gate",
        "tag": "an-approval-gate",
        "blurb": "人在环决策门，三 flavor 共皮：chat(danger 批准/始终批准/拒绝 + 三级徽 + args) / ask(ask_user 提交/跳过 + options 单选) / durable(flowrun :decide 通过/驳回 + 倒计时 + reason，仅 scheduler)；settled 收口",
        "specimens": [
          {
            "label": "chat · dangerous",
            "span": true,
            "tag": "an-approval-gate",
            "attrs": {
              "flavor": "chat",
              "title": "需要审批确认",
              "tool": "Bash",
              "danger": "dangerous",
              "summary": "将在沙箱内删除过期快照目录，操作不可逆。",
              "args": "{\n  \"command\": \"rm -rf /data/snapshots/2024-*\"\n}"
            }
          },
          {
            "label": "chat · cautious",
            "span": true,
            "tag": "an-approval-gate",
            "attrs": {
              "flavor": "chat",
              "title": "需要审批确认",
              "tool": "Write",
              "danger": "cautious",
              "summary": "将覆盖 config/triggers.yaml。",
              "args": "{\n  \"file_path\": \"config/triggers.yaml\"\n}"
            }
          },
          {
            "label": "chat · safe",
            "span": true,
            "tag": "an-approval-gate",
            "attrs": {
              "flavor": "chat",
              "title": "需要审批确认",
              "tool": "search_function",
              "danger": "safe",
              "summary": "只读列出当前 workspace 的 function 实体。"
            }
          },
          {
            "label": "ask · ask_user 提问门",
            "span": true,
            "tag": "an-approval-gate",
            "attrs": {
              "flavor": "ask",
              "title": "需要你的输入",
              "prompt": "要点用中文还是英文整理？",
              "options": "中文|英文|中英对照"
            }
          },
          {
            "label": "durable · 倒计时+reason",
            "span": true,
            "tag": "an-approval-gate",
            "attrs": {
              "flavor": "durable",
              "title": "审批收件箱",
              "ddl": "剩余 23h 41m · 截止 06-18 09:00",
              "prompt": "flowrun frn_8a1c… 在节点 review_gate 暂停：订单 ord_77c1 金额 ¥48,200 超过自动放行阈值，是否放行结算？",
              "allow-reason": true,
              "placeholder": "驳回理由（可选）…"
            }
          },
          {
            "label": "durable · 无 reason",
            "span": true,
            "tag": "an-approval-gate",
            "attrs": {
              "flavor": "durable",
              "title": "审批收件箱",
              "ddl": "剩余 02m 09s",
              "prompt": "flowrun frn_5e1a… 等待发布确认：将把 agent `support_bot` 切到 production。"
            }
          },
          {
            "label": "settled · 已决",
            "span": true,
            "tag": "an-approval-gate",
            "attrs": {
              "flavor": "durable",
              "title": "审批收件箱",
              "prompt": "flowrun frn_8a1c… 在节点 review_gate 暂停。",
              "settled": true
            }
          }
        ]
      },
      {
        "name": "输入条 composer",
        "tag": "an-composer",
        "blurb": "chat 输入条：多行 contenteditable + @ 提及内联药丸（复用地基 AnMention）+ 附件 chip（可删）+ Enter 发送 / Shift+Enter 换行；generating 切停止态。派 an-send{text,html,refs,attachments}/an-stop",
        "specimens": [
          {
            "label": "default · 空态（@ 起 picker）",
            "span": true,
            "tag": "an-composer",
            "props": {
              "mentions": [
                { "kind": "function", "id": "fn_5e1a9c4d", "label": "sync_inventory", "desc": "同步仓库库存" },
                { "kind": "agent", "id": "ag_91c3de07", "label": "triage_agent", "desc": "诊断失败执行" },
                { "kind": "workflow", "id": "wf_9f2a7c1b", "label": "pr_merge_flow", "desc": "PR 合并流程" }
              ]
            }
          },
          {
            "label": "附件 chip · 可删",
            "span": true,
            "tag": "an-composer",
            "props": {
              "attachments": [
                { "name": "spec.md", "icon": "doc" },
                { "name": "screenshot.png", "icon": "doc" }
              ]
            }
          },
          {
            "label": "generating · 停止态",
            "span": true,
            "tag": "an-composer",
            "attrs": {
              "generating": true
            }
          }
        ]
      },
      {
        "name": "接线组 wire-list",
        "tag": "an-wire-list",
        "blurb": "可增删的 key→表达式(field→CEL) 映射行组；props.rows 设入(map 或 [{field,expr}])，attr keyph/exprph 占位、addlabel 增行钮文案；变更派 an-wire-change{map}",
        "specimens": [
          {
            "label": "filled · input 接线",
            "span": true,
            "tag": "an-wire-list",
            "attrs": {
              "addlabel": "添加映射"
            },
            "props": {
              "rows": {
                "warehouse": "trigger.firing.payload.warehouse",
                "dryRun": "false",
                "sku": "node.fetch.output.items[0].sku"
              }
            }
          },
          {
            "label": "rows=[] · 空",
            "span": true,
            "tag": "an-wire-list",
            "attrs": {
              "addlabel": "添加映射"
            },
            "props": {
              "rows": []
            }
          },
          {
            "label": "数组形式 + 自定占位",
            "span": true,
            "tag": "an-wire-list",
            "attrs": {
              "keyph": "port",
              "exprph": "when (CEL)",
              "addlabel": "添加分支"
            },
            "props": {
              "rows": [
                {
                  "field": "approved",
                  "expr": "node.review.output.decision == \"yes\""
                },
                {
                  "field": "rejected",
                  "expr": "node.review.output.decision == \"no\""
                }
              ]
            }
          }
        ]
      },
      {
        "name": "节点甘特 node-gantt",
        "tag": "an-node-gantt",
        "blurb": "单 flowrun 的逐节点甘特：每节点一行，时段条沿 run 内相对时间轴（atPct/wPct ∈[0,100]）铺。nodes 经 JS 属性注入（[{id,kind,label,status,atPct,wPct,iters?,parked?}]）；一眼看出谁慢（条长）·循环几轮（iters 多段 + ×N 徽）·在哪 parked（warn 虚框等待条）·谁未起（future 占位）。点行高亮并派 an-node-pick{id}。status→色：done 绿 / err 红 / parked warn / future 灰。",
        "specimens": [
          {
            "label": "parked run（iters×N + parked 虚框 + future 占位）",
            "span": true,
            "tag": "an-node-gantt",
            "props": {
              "nodes": [
                {
                  "id": "on_pr_merged",
                  "kind": "trigger",
                  "label": "on_pr_merged",
                  "status": "done",
                  "atPct": 0,
                  "wPct": 4
                },
                {
                  "id": "run_tests",
                  "kind": "action",
                  "label": "run_tests",
                  "status": "done",
                  "iters": [
                    {
                      "atPct": 5,
                      "wPct": 26
                    },
                    {
                      "atPct": 40,
                      "wPct": 28
                    }
                  ]
                },
                {
                  "id": "branch_result",
                  "kind": "control",
                  "label": "branch_result",
                  "status": "done",
                  "iters": [
                    {
                      "atPct": 32,
                      "wPct": 5
                    },
                    {
                      "atPct": 69,
                      "wPct": 5
                    }
                  ]
                },
                {
                  "id": "approve_rollback",
                  "kind": "approval",
                  "label": "approve_rollback",
                  "status": "parked",
                  "atPct": 75,
                  "wPct": 23,
                  "parked": true
                },
                {
                  "id": "do_rollback",
                  "kind": "action",
                  "label": "do_rollback",
                  "status": "future",
                  "atPct": 0,
                  "wPct": 0
                }
              ]
            }
          },
          {
            "label": "failed run（err 红条 + retry 2 轮均 fail）",
            "span": true,
            "tag": "an-node-gantt",
            "props": {
              "nodes": [
                {
                  "id": "on_pr_merged",
                  "kind": "trigger",
                  "label": "on_pr_merged",
                  "status": "done",
                  "atPct": 0,
                  "wPct": 6
                },
                {
                  "id": "run_tests",
                  "kind": "action",
                  "label": "run_tests",
                  "status": "err",
                  "iters": [
                    {
                      "atPct": 7,
                      "wPct": 30
                    },
                    {
                      "atPct": 45,
                      "wPct": 30
                    }
                  ]
                },
                {
                  "id": "branch_result",
                  "kind": "control",
                  "label": "branch_result",
                  "status": "future",
                  "atPct": 0,
                  "wPct": 0
                }
              ]
            }
          },
          {
            "label": "running run（agent 在途 done 长条 + 后继 future）",
            "span": true,
            "tag": "an-node-gantt",
            "props": {
              "nodes": [
                {
                  "id": "ticket_in",
                  "kind": "trigger",
                  "label": "ticket_in",
                  "status": "done",
                  "atPct": 0,
                  "wPct": 8
                },
                {
                  "id": "triage",
                  "kind": "agent",
                  "label": "triage",
                  "status": "done",
                  "atPct": 9,
                  "wPct": 70
                },
                {
                  "id": "severity",
                  "kind": "control",
                  "label": "severity",
                  "status": "future",
                  "atPct": 0,
                  "wPct": 0
                },
                {
                  "id": "notify",
                  "kind": "action",
                  "label": "notify",
                  "status": "future",
                  "atPct": 0,
                  "wPct": 0
                }
              ]
            }
          },
          {
            "label": "completed run（全 done 满轴）",
            "span": true,
            "tag": "an-node-gantt",
            "props": {
              "nodes": [
                {
                  "id": "cron",
                  "kind": "trigger",
                  "label": "cron",
                  "status": "done",
                  "atPct": 0,
                  "wPct": 4
                },
                {
                  "id": "extract",
                  "kind": "action",
                  "label": "extract",
                  "status": "done",
                  "atPct": 5,
                  "wPct": 55
                },
                {
                  "id": "load",
                  "kind": "action",
                  "label": "load",
                  "status": "done",
                  "atPct": 61,
                  "wPct": 38
                }
              ]
            }
          },
          {
            "label": "空（nodes=[] → 无行）",
            "span": true,
            "tag": "an-node-gantt",
            "props": {
              "nodes": []
            }
          }
        ]
      },
      {
        "name": "运行看板 run-board",
        "tag": "an-run-board",
        "blurb": "单 workflow 运行看板：左列每次 run（trigger 多次 → 多条 flowrun，状态点 + id/trigger·when + ↻replay 徽），右栏内嵌 an-node-gantt 随选中 run 同步逐节点甘特；runs 经 JS 属性注入，run.selected 定初选，点行派 an-run-pick{id}。",
        "specimens": [
          {
            "label": "default · parked 选中（左列三态混排）",
            "span": true,
            "tag": "an-run-board",
            "props": {
              "runs": [
                {
                  "id": "fr_b7e0c431",
                  "status": "parked",
                  "trigger": "webhook · pr #1287",
                  "when": "12:09 · 在途",
                  "replay": 0,
                  "selected": true,
                  "gantt": [
                    {
                      "id": "on_pr_merged",
                      "kind": "trigger",
                      "label": "on_pr_merged",
                      "status": "done",
                      "atPct": 0,
                      "wPct": 4
                    },
                    {
                      "id": "run_tests",
                      "kind": "action",
                      "label": "run_tests",
                      "status": "done",
                      "iters": [
                        {
                          "atPct": 5,
                          "wPct": 26
                        },
                        {
                          "atPct": 40,
                          "wPct": 28
                        }
                      ]
                    },
                    {
                      "id": "branch_result",
                      "kind": "control",
                      "label": "branch_result",
                      "status": "done",
                      "iters": [
                        {
                          "atPct": 32,
                          "wPct": 5
                        },
                        {
                          "atPct": 69,
                          "wPct": 5
                        }
                      ]
                    },
                    {
                      "id": "approve_rollback",
                      "kind": "approval",
                      "label": "approve_rollback",
                      "status": "parked",
                      "atPct": 75,
                      "wPct": 23,
                      "parked": true
                    },
                    {
                      "id": "do_rollback",
                      "kind": "action",
                      "label": "do_rollback",
                      "status": "future",
                      "atPct": 0,
                      "wPct": 0
                    }
                  ]
                },
                {
                  "id": "fr_a1c89f02",
                  "status": "completed",
                  "trigger": "webhook · pr #1284",
                  "when": "10:30",
                  "replay": 0,
                  "gantt": [
                    {
                      "id": "on_pr_merged",
                      "kind": "trigger",
                      "label": "on_pr_merged",
                      "status": "done",
                      "atPct": 0,
                      "wPct": 6
                    },
                    {
                      "id": "run_tests",
                      "kind": "action",
                      "label": "run_tests",
                      "status": "done",
                      "atPct": 7,
                      "wPct": 34
                    },
                    {
                      "id": "branch_result",
                      "kind": "control",
                      "label": "branch_result",
                      "status": "done",
                      "atPct": 42,
                      "wPct": 6
                    },
                    {
                      "id": "approve_rollback",
                      "kind": "approval",
                      "label": "approve_rollback",
                      "status": "done",
                      "atPct": 49,
                      "wPct": 30
                    },
                    {
                      "id": "do_rollback",
                      "kind": "action",
                      "label": "do_rollback",
                      "status": "done",
                      "atPct": 80,
                      "wPct": 18
                    }
                  ]
                },
                {
                  "id": "fr_c3d471a8",
                  "status": "failed",
                  "trigger": "webhook · pr #1279",
                  "when": "08:15",
                  "replay": 1,
                  "gantt": [
                    {
                      "id": "on_pr_merged",
                      "kind": "trigger",
                      "label": "on_pr_merged",
                      "status": "done",
                      "atPct": 0,
                      "wPct": 6
                    },
                    {
                      "id": "run_tests",
                      "kind": "action",
                      "label": "run_tests",
                      "status": "err",
                      "iters": [
                        {
                          "atPct": 7,
                          "wPct": 30
                        },
                        {
                          "atPct": 45,
                          "wPct": 30
                        }
                      ]
                    },
                    {
                      "id": "branch_result",
                      "kind": "control",
                      "label": "branch_result",
                      "status": "future",
                      "atPct": 0,
                      "wPct": 0
                    }
                  ]
                }
              ]
            }
          },
          {
            "label": "running 选中 · agent 长条（support_triage）",
            "span": true,
            "tag": "an-run-board",
            "props": {
              "runs": [
                {
                  "id": "fr_9a40e1d7",
                  "status": "running",
                  "trigger": "webhook · ticket #4821",
                  "when": "12:18 · 在途",
                  "replay": 0,
                  "selected": true,
                  "gantt": [
                    {
                      "id": "ticket_in",
                      "kind": "trigger",
                      "label": "ticket_in",
                      "status": "done",
                      "atPct": 0,
                      "wPct": 8
                    },
                    {
                      "id": "triage",
                      "kind": "agent",
                      "label": "triage",
                      "status": "done",
                      "atPct": 9,
                      "wPct": 70
                    },
                    {
                      "id": "severity",
                      "kind": "control",
                      "label": "severity",
                      "status": "future",
                      "atPct": 0,
                      "wPct": 0
                    },
                    {
                      "id": "notify",
                      "kind": "action",
                      "label": "notify",
                      "status": "future",
                      "atPct": 0,
                      "wPct": 0
                    }
                  ]
                },
                {
                  "id": "fr_2f7a0931",
                  "status": "completed",
                  "trigger": "webhook · ticket #4790",
                  "when": "09:20",
                  "replay": 0,
                  "gantt": [
                    {
                      "id": "ticket_in",
                      "kind": "trigger",
                      "label": "ticket_in",
                      "status": "done",
                      "atPct": 0,
                      "wPct": 6
                    },
                    {
                      "id": "triage",
                      "kind": "agent",
                      "label": "triage",
                      "status": "done",
                      "atPct": 7,
                      "wPct": 60
                    },
                    {
                      "id": "severity",
                      "kind": "control",
                      "label": "severity",
                      "status": "done",
                      "atPct": 68,
                      "wPct": 6
                    },
                    {
                      "id": "notify",
                      "kind": "action",
                      "label": "notify",
                      "status": "done",
                      "atPct": 75,
                      "wPct": 24
                    }
                  ]
                }
              ]
            }
          },
          {
            "label": "failed 选中 · ↻replay 徽（无 selected → 首条兜底）",
            "span": true,
            "tag": "an-run-board",
            "props": {
              "runs": [
                {
                  "id": "fr_c3d471a8",
                  "status": "failed",
                  "trigger": "webhook · pr #1279",
                  "when": "08:15",
                  "replay": 1,
                  "gantt": [
                    {
                      "id": "on_pr_merged",
                      "kind": "trigger",
                      "label": "on_pr_merged",
                      "status": "done",
                      "atPct": 0,
                      "wPct": 6
                    },
                    {
                      "id": "run_tests",
                      "kind": "action",
                      "label": "run_tests",
                      "status": "err",
                      "iters": [
                        {
                          "atPct": 7,
                          "wPct": 30
                        },
                        {
                          "atPct": 45,
                          "wPct": 30
                        }
                      ]
                    },
                    {
                      "id": "branch_result",
                      "kind": "control",
                      "label": "branch_result",
                      "status": "future",
                      "atPct": 0,
                      "wPct": 0
                    }
                  ]
                },
                {
                  "id": "fr_5e80b21c",
                  "status": "completed",
                  "trigger": "cron · 02:00",
                  "when": "02:00",
                  "replay": 0,
                  "gantt": [
                    {
                      "id": "cron",
                      "kind": "trigger",
                      "label": "cron",
                      "status": "done",
                      "atPct": 0,
                      "wPct": 4
                    },
                    {
                      "id": "extract",
                      "kind": "action",
                      "label": "extract",
                      "status": "done",
                      "atPct": 5,
                      "wPct": 55
                    },
                    {
                      "id": "load",
                      "kind": "action",
                      "label": "load",
                      "status": "done",
                      "atPct": 61,
                      "wPct": 38
                    }
                  ]
                }
              ]
            }
          },
          {
            "label": "单 run · 全 done（最小看板）",
            "span": true,
            "tag": "an-run-board",
            "props": {
              "runs": [
                {
                  "id": "fr_5e80b21c",
                  "status": "completed",
                  "trigger": "cron · 02:00",
                  "when": "02:00",
                  "replay": 0,
                  "selected": true,
                  "gantt": [
                    {
                      "id": "cron",
                      "kind": "trigger",
                      "label": "cron",
                      "status": "done",
                      "atPct": 0,
                      "wPct": 4
                    },
                    {
                      "id": "extract",
                      "kind": "action",
                      "label": "extract",
                      "status": "done",
                      "atPct": 5,
                      "wPct": 55
                    },
                    {
                      "id": "load",
                      "kind": "action",
                      "label": "load",
                      "status": "done",
                      "atPct": 61,
                      "wPct": 38
                    }
                  ]
                }
              ]
            }
          }
        ]
      }
    ]
  },
  {
    "cat": "文档 Documents",
    "icon": "doc",
    "items": [
      {
        "name": "块编辑器 doc-editor",
        "tag": "an-doc-editor",
        "blurb": "🪂 Notion 式块编辑器（全 demo 唯一自画 contenteditable 逃生舱）：blocks 经 JS 属性注入一次渲富文本（h1/h2/h3 · p[spans 含 @ref] · bullet · todo · quote · code · callout · divider），之后编辑活在 DOM。三能力：斜杠「/」开块型菜单 · 「@」边打边滤插 an-ref-pill · 悬停 pill 浮信息卡。mentions 走 JS 属性喂 @ picker / 悬卡；左槽 ＋ 手柄插块。",
        "specimens": [
          {
            "label": "全块型 + @ref + mentions 悬卡",
            "span": true,
            "tag": "an-doc-editor",
            "props": {
              "mentions": [
                {
                  "kind": "function",
                  "id": "fn_5e1a9c4d",
                  "label": "fetch_article",
                  "desc": "抓取 URL 正文"
                },
                {
                  "kind": "agent",
                  "id": "ag_91c3de07",
                  "label": "triage_agent",
                  "desc": "诊断失败执行"
                },
                {
                  "kind": "workflow",
                  "id": "wf_9f2a7c1b",
                  "label": "pr_merge_flow",
                  "desc": "PR 合并后跑测试 + 审批回滚"
                },
                {
                  "kind": "approval",
                  "id": "apf_release",
                  "label": "approve_rollback",
                  "desc": "回滚审批门"
                },
                {
                  "kind": "doc",
                  "id": "doc_durable",
                  "label": "Durable 执行设计",
                  "desc": "引擎设计文档"
                },
                {
                  "kind": "trigger",
                  "id": "trg_3a1f",
                  "label": "webhook · pr",
                  "desc": "监听 GitHub PR webhook"
                }
              ],
              "blocks": [
                {
                  "type": "h2",
                  "text": "背景"
                },
                {
                  "type": "p",
                  "spans": [
                    {
                      "t": "团队现在靠人肉串起"
                    },
                    {
                      "ref": {
                        "kind": "function",
                        "id": "fn_5e1a9c4d",
                        "label": "fetch_article"
                      }
                    },
                    {
                      "t": " 抓取 → "
                    },
                    {
                      "ref": {
                        "kind": "agent",
                        "id": "ag_91c3de07",
                        "label": "triage_agent"
                      }
                    },
                    {
                      "t": " 诊断 → 人工审批回滚，链路脆且不可重放。本文定义把它编排成一条 durable workflow。"
                    }
                  ]
                },
                {
                  "type": "h2",
                  "text": "目标"
                },
                {
                  "type": "bullet",
                  "text": "节点结果记忆化：崩溃后从断点续跑，绝不重跑已完成节点。"
                },
                {
                  "type": "bullet",
                  "text": "失败分支挂人工审批门，决策 first-wins、支持超时自动驳回。"
                },
                {
                  "type": "code",
                  "lang": "cel",
                  "text": "branch_result.exitCode != 0 && payload.branch == \"main\""
                },
                {
                  "type": "quote",
                  "text": "Durable 为魂——节点记忆化 + 解释器幂等重走，非事件溯源。"
                },
                {
                  "type": "divider"
                },
                {
                  "type": "p",
                  "spans": [
                    {
                      "t": "相关："
                    },
                    {
                      "ref": {
                        "kind": "doc",
                        "id": "doc_durable",
                        "label": "Durable 执行设计"
                      }
                    },
                    {
                      "t": " · "
                    },
                    {
                      "ref": {
                        "kind": "trigger",
                        "id": "trg_3a1f",
                        "label": "webhook · pr"
                      }
                    }
                  ]
                }
              ]
            }
          },
          {
            "label": "待办 todo（checked 勾/空）",
            "span": true,
            "tag": "an-doc-editor",
            "props": {
              "blocks": [
                {
                  "type": "h2",
                  "text": "待办"
                },
                {
                  "type": "todo",
                  "checked": true,
                  "text": "图校验：全节点从 trigger 可达、回边只出自 control/approval"
                },
                {
                  "type": "todo",
                  "checked": true,
                  "text": "pin 闭包：跑前冻结引用实体的 active 版本"
                },
                {
                  "type": "todo",
                  "checked": false,
                  "text": "审批超时 timer（5s tick 扫 parked 行）"
                },
                {
                  "type": "todo",
                  "checked": false,
                  "text": "前端 scheduler 面：执行时间线 + 运行图 + 节点调试"
                }
              ]
            }
          },
          {
            "label": "提示条 callout（info / warn · 含 <b>）",
            "span": true,
            "tag": "an-doc-editor",
            "props": {
              "blocks": [
                {
                  "type": "callout",
                  "tone": "info",
                  "html": "设计原则 #2 的落点：<b>节点结果记忆化 + 解释器幂等重走</b>（非事件溯源）。"
                },
                {
                  "type": "h3",
                  "text": "两张表讲完所有状态"
                },
                {
                  "type": "bullet",
                  "text": "flowruns（run 头）= 冻结拓扑 + 冻结引用版本（pinned_refs）+ 状态。"
                },
                {
                  "type": "bullet",
                  "text": "flowrun_nodes（★真相表）= 每行一个 (节点, 轮次) 的记忆化 result；UNIQUE(flowrun_id,node_id,iteration) = record-once。"
                },
                {
                  "type": "callout",
                  "tone": "warn",
                  "html": "结论：现有 agent 平台多是 <b>SaaS + 云编排</b>，本地优先 + durable 是差异点。"
                }
              ]
            }
          },
          {
            "label": "代码块 code（lang 标签）",
            "span": true,
            "tag": "an-doc-editor",
            "props": {
              "blocks": [
                {
                  "type": "h3",
                  "text": "引擎是一个幂等函数 Advance(runID)"
                },
                {
                  "type": "p",
                  "spans": [
                    {
                      "t": "读 frn 行 + 冻结图 → 算 ready (节点,轮次) → 跑/求值 → 写行 → 重复。崩溃 = 再调一遍：completed 行被「抄」（record-once 拒绝重写），绝不重跑。"
                    }
                  ]
                },
                {
                  "type": "code",
                  "lang": "text",
                  "text": "节点行只写终态（completed/failed/parked）\nparked 是唯一非终态：审批挂起、派生审批收件箱"
                }
              ]
            }
          },
          {
            "label": "空编辑器（聚焦空块显占位提示，可 / @ 起手）",
            "span": true,
            "tag": "an-doc-editor",
            "props": {
              "mentions": [
                {
                  "kind": "function",
                  "id": "fn_5e1a9c4d",
                  "label": "fetch_article",
                  "desc": "抓取 URL 正文"
                },
                {
                  "kind": "workflow",
                  "id": "wf_9f2a7c1b",
                  "label": "pr_merge_flow",
                  "desc": "PR 合并后跑测试 + 审批回滚"
                }
              ],
              "blocks": [
                {
                  "type": "p",
                  "text": ""
                }
              ]
            }
          }
        ]
      },
      {
        "name": "文档大纲 outline",
        "tag": "an-outline",
        "blurb": "文档大纲 / 目录（ToC）：左导引线 + 按 level 缩进的可点标题，active 节叠 accent 短条高亮；items（[{text,level}]，level∈2/3）/ active（当前节索引）走 JS 属性，点条目派 an-outline-pick{index}（消费方滚到对应标题）。",
        "specimens": [
          {
            "label": "items + active=0（PRD 大纲）",
            "tag": "an-outline",
            "props": {
              "active": 0,
              "items": [
                {
                  "level": 2,
                  "text": "背景"
                },
                {
                  "level": 2,
                  "text": "目标"
                },
                {
                  "level": 2,
                  "text": "核心编排"
                },
                {
                  "level": 2,
                  "text": "待办"
                }
              ]
            }
          },
          {
            "label": "active=1（高亮中段）",
            "tag": "an-outline",
            "props": {
              "active": 1,
              "items": [
                {
                  "level": 2,
                  "text": "两张表讲完所有状态"
                },
                {
                  "level": 2,
                  "text": "引擎是一个幂等函数 Advance(runID)"
                }
              ]
            }
          },
          {
            "label": "level=3 嵌套缩进",
            "tag": "an-outline",
            "props": {
              "active": 2,
              "items": [
                {
                  "level": 2,
                  "text": "H1 · 本地优先 v0.3"
                },
                {
                  "level": 3,
                  "text": "后端全实体 + durable 引擎"
                },
                {
                  "level": 3,
                  "text": "前端设计系统 + 能力画廊"
                },
                {
                  "level": 2,
                  "text": "H2 · 协作与可观测"
                }
              ]
            }
          },
          {
            "label": "无 active（无高亮）",
            "tag": "an-outline",
            "props": {
              "items": [
                {
                  "level": 2,
                  "text": "对比维度"
                },
                {
                  "level": 3,
                  "text": "执行模型"
                },
                {
                  "level": 3,
                  "text": "部署形态"
                }
              ]
            }
          },
          {
            "label": "empty（暂无标题）",
            "tag": "an-outline",
            "props": {
              "items": []
            }
          }
        ]
      }
    ]
  },
  {
    "cat": "图与壳 Graph & Chrome",
    "icon": "layout-grid",
    "items": [
      {
        "name": "编排画布 graph-canvas",
        "tag": "an-graph-canvas",
        "blurb": "🪂 自绘 SVG 画布：workflow 编排图 + flowrun 运行态。graph 经 props 注入（{nodes:[{id,kind,ref}],edges:[{from,to,port}]}），属性 framed 嵌实体页定高框 / toolbar 缩放组 / mode=edit|run / dir=LR|TB；节点 5 kind（trigger/action/agent/control/approval），回边只能从 control/approval 出。",
        "specimens": [
          {
            "label": "mode=edit framed toolbar dir=LR",
            "span": true,
            "tag": "an-graph-canvas",
            "attrs": {
              "framed": true,
              "toolbar": true,
              "mode": "edit",
              "dir": "LR"
            },
            "props": {
              "graph": {
                "nodes": [
                  {
                    "id": "trigger",
                    "kind": "trigger",
                    "ref": "trg_webhook"
                  },
                  {
                    "id": "fetch",
                    "kind": "action",
                    "ref": "fn_fetch_pr"
                  },
                  {
                    "id": "review",
                    "kind": "approval",
                    "ref": "apf_merge"
                  },
                  {
                    "id": "merge",
                    "kind": "action",
                    "ref": "fn_merge"
                  }
                ],
                "edges": [
                  {
                    "from": "trigger",
                    "to": "fetch"
                  },
                  {
                    "from": "fetch",
                    "to": "review"
                  },
                  {
                    "from": "review",
                    "to": "merge",
                    "port": "yes"
                  }
                ]
              }
            }
          },
          {
            "label": "mode=edit dir=TB control 分支",
            "span": true,
            "tag": "an-graph-canvas",
            "attrs": {
              "framed": true,
              "toolbar": true,
              "mode": "edit",
              "dir": "TB"
            },
            "props": {
              "graph": {
                "nodes": [
                  {
                    "id": "trigger",
                    "kind": "trigger",
                    "ref": "trg_cron"
                  },
                  {
                    "id": "score",
                    "kind": "agent",
                    "ref": "ag_triage"
                  },
                  {
                    "id": "route",
                    "kind": "control",
                    "ref": "ctl_branch"
                  },
                  {
                    "id": "escalate",
                    "kind": "action",
                    "ref": "fn_notify"
                  }
                ],
                "edges": [
                  {
                    "from": "trigger",
                    "to": "score"
                  },
                  {
                    "from": "score",
                    "to": "route"
                  },
                  {
                    "from": "route",
                    "to": "escalate",
                    "port": "branch1"
                  },
                  {
                    "from": "route",
                    "to": "score",
                    "port": "retry"
                  }
                ]
              }
            }
          },
          {
            "label": "mode=run flowrun 运行态",
            "span": true,
            "tag": "an-graph-canvas",
            "attrs": {
              "framed": true,
              "toolbar": true,
              "mode": "run",
              "dir": "LR"
            },
            "props": {
              "graph": {
                "nodes": [
                  {
                    "id": "trigger",
                    "kind": "trigger",
                    "ref": "trg_webhook"
                  },
                  {
                    "id": "fetch",
                    "kind": "action",
                    "ref": "fn_fetch_pr"
                  },
                  {
                    "id": "review",
                    "kind": "approval",
                    "ref": "apf_merge"
                  },
                  {
                    "id": "merge",
                    "kind": "action",
                    "ref": "fn_merge"
                  }
                ],
                "edges": [
                  {
                    "from": "trigger",
                    "to": "fetch",
                    "id": "e1"
                  },
                  {
                    "from": "fetch",
                    "to": "review",
                    "id": "e2"
                  },
                  {
                    "from": "review",
                    "to": "merge",
                    "id": "e3",
                    "port": "yes"
                  }
                ]
              },
              "run": {
                "state": {
                  "trigger": "completed",
                  "fetch": "completed",
                  "review": "parked",
                  "merge": "future"
                },
                "iters": {
                  "trigger": 1,
                  "fetch": 1,
                  "review": 1,
                  "merge": 0
                },
                "memo": {
                  "review": {
                    "parked": true
                  }
                },
                "taken": [
                  "e1",
                  "e2"
                ],
                "live": null
              }
            }
          }
        ]
      },
      {
        "name": "节点图例 kind-legend",
        "tag": "an-kind-legend",
        "blurb": "图节点 5 类（trigger/action/agent/control/approval）只读颜色图例——自 window.AnGraph 取数、零属性、零 props。rail 与 reference 画廊同用。",
        "specimens": [
          {
            "label": "default",
            "tag": "an-kind-legend",
            "center": true
          }
        ]
      },
      {
        "name": "工具条 toolbar",
        "tag": "an-toolbar",
        "blurb": "三段对齐骨架 left|main|right（无边无卡）。title/meta 属性渲染标准标题 + 次级 meta，缺省时 main 走默认 slot；bordered 作顶栏（底描边 + island 底）、compact 矮一档；左附件入 slot=left、右动作入 slot=right。",
        "specimens": [
          {
            "label": "title + meta",
            "span": true,
            "tag": "an-toolbar",
            "attrs": {
              "title": "fn_fetch_pr · 概览",
              "meta": "v3 · 已就绪"
            }
          },
          {
            "label": "bordered 顶栏 + right 动作",
            "span": true,
            "tag": "an-toolbar",
            "attrs": {
              "title": "工作流编辑器",
              "meta": "wf_release_gate",
              "bordered": true
            },
            "children": [
              {
                "tag": "an-action-group",
                "attrs": {
                  "slot": "right",
                  "end": true
                },
                "children": [
                  {
                    "tag": "an-button",
                    "attrs": {
                      "variant": "icon",
                      "icon": "history"
                    }
                  },
                  {
                    "tag": "an-button",
                    "attrs": {
                      "variant": "primary",
                      "icon": "trigger"
                    },
                    "text": "Trigger"
                  }
                ]
              }
            ]
          },
          {
            "label": "compact + left 附件",
            "span": true,
            "tag": "an-toolbar",
            "attrs": {
              "title": "节点 review",
              "compact": true
            },
            "children": [
              {
                "tag": "an-badge",
                "attrs": {
                  "slot": "left",
                  "dot": "wait",
                  "tone": "warn"
                },
                "text": "parked"
              }
            ]
          },
          {
            "label": "自定义 main（默认 slot）",
            "span": true,
            "tag": "an-toolbar",
            "children": [
              {
                "tag": "an-badge",
                "attrs": {
                  "tone": "accent",
                  "dot": "run"
                },
                "text": "flowrun 进行中"
              }
            ]
          }
        ]
      },
      {
        "name": "海洋页头 ocean-header",
        "tag": "an-ocean-header",
        "blurb": "海面页头（无卡）：crumb 面包屑（'|' 分隔自动插 /）+ 大标题 + meta 行 + 右动作。editable 标题就地改名（hover 现铅笔、原地 contenteditable、派 an-title-change）；右动作入 slot=actions、meta 入 slot=meta。",
        "specimens": [
          {
            "label": "crumb + title",
            "span": true,
            "tag": "an-ocean-header",
            "attrs": {
              "crumb": "Entities|Workflow",
              "title": "release-gate 发布闸门"
            }
          },
          {
            "label": "editable 就地改名",
            "span": true,
            "tag": "an-ocean-header",
            "attrs": {
              "crumb": "Entities|Function",
              "title": "fetch_pr",
              "editable": true
            }
          },
          {
            "label": "meta 徽 + actions 执行动作",
            "span": true,
            "tag": "an-ocean-header",
            "attrs": {
              "crumb": "Entities|Agent",
              "title": "triage 诊断智能体",
              "editable": true
            },
            "children": [
              {
                "tag": "an-badge",
                "attrs": {
                  "slot": "meta",
                  "dot": "done",
                  "tone": "ok"
                },
                "text": "ready"
              },
              {
                "tag": "an-action-group",
                "attrs": {
                  "slot": "actions",
                  "end": true
                },
                "children": [
                  {
                    "tag": "an-button",
                    "attrs": {
                      "variant": "primary",
                      "icon": "agent"
                    },
                    "text": "Invoke"
                  },
                  {
                    "tag": "an-button",
                    "attrs": {
                      "variant": "icon",
                      "icon": "more"
                    }
                  }
                ]
              }
            ]
          }
        ]
      },
      {
        "name": "右岛 right-island",
        "tag": "an-right-island",
        "blurb": "右岛内容壳（宽度由外层 shell 控制）：head（icon + title）+ 默认 slot 堆叠若干 an-info-card。正文区可滚不显滚轮，卡间距走 margin。",
        "specimens": [
          {
            "label": "title + icon + info-card 堆叠",
            "span": true,
            "tag": "an-right-island",
            "attrs": {
              "title": "试运行 · Run",
              "icon": "run"
            },
            "children": [
              {
                "tag": "an-info-card",
                "attrs": {
                  "title": "入参",
                  "icon": "doc",
                  "meta": "json"
                },
                "children": [
                  {
                    "tag": "an-kv",
                    "props": {
                      "rows": [
                        [
                          "prNumber",
                          "428"
                        ],
                        [
                          "dryRun",
                          "true"
                        ]
                      ]
                    }
                  }
                ]
              },
              {
                "tag": "an-info-card",
                "attrs": {
                  "title": "Schedule",
                  "icon": "scheduler",
                  "meta": "UTC"
                },
                "children": [
                  {
                    "tag": "an-row",
                    "attrs": {
                      "icon": "trigger",
                      "label": "cron · 0 9 * * 1",
                      "meta": "每周一 09:00"
                    }
                  }
                ]
              }
            ]
          },
          {
            "label": "触发器岛（trigger meta）",
            "span": true,
            "tag": "an-right-island",
            "attrs": {
              "title": "触发器 · webhook",
              "icon": "trigger"
            },
            "children": [
              {
                "tag": "an-info-card",
                "attrs": {
                  "title": "最近 firing",
                  "icon": "history"
                },
                "children": [
                  {
                    "tag": "an-row",
                    "attrs": {
                      "dot": "done",
                      "label": "trf_5e1a · 已激活",
                      "meta": "842ms"
                    }
                  },
                  {
                    "tag": "an-row",
                    "attrs": {
                      "dot": "err",
                      "label": "trf_3b9c · 去重丢弃",
                      "meta": "dedup"
                    }
                  }
                ]
              }
            ]
          }
        ]
      },
      {
        "name": "实体工作台 entity-workspace",
        "tag": "an-entity-workspace",
        "blurb": "chat 右岛实体工作台（v2）= entities 流的实体面板镜像，跟着对话长出来：自绘头（真名 + 右上下拉选择器）+ body 双态（item 态=该 item canonical 全量 facet an-tabs / picker 态=分类列表，仅非空分类 + 搜索 + 状态筛）。每种 item（5 实体 kind + Todo + Subagent）一套固定 facet，未触及显空态。命令式 ensure/setActive/focus/setTodo 供 chat sea 流式驱动；auto attr = 静态展示（画廊）全 ensure + 停首项。",
        "specimens": [
          {
            "label": "多 item（Function/Workflow/Subagent/Todo）+ 下拉选择器（auto 静态）",
            "span": true,
            "tag": "an-entity-workspace",
            "attrs": { "auto": "", "active": "fn_demo" },
            "props": {
              "model": {
                "items": [
                  {
                    "id": "fn_demo", "category": "entity", "kind": "function", "name": "sync_inventory", "lang": "python", "status": "done", "meta": "v2 · env ready", "revert": "revert 回 v1",
                    "facets": [
                      { "key": "overview", "label": "概览", "rows": [["version", "v2"], ["env_status", "ready"], ["inputs", "warehouse: str"]] },
                      { "key": "versions", "label": "版本", "range": "v1 → v2", "note": "加指数退避重试",
                        "before": "import requests\n\ndef sync_inventory(wh):\n    return upsert(fetch_skus(wh))\n",
                        "after": "import time, requests\n\ndef sync_inventory(wh):\n    for i in range(3):\n        try:\n            return upsert(fetch_skus(wh))\n        except requests.RequestException:\n            if i == 2: raise\n            time.sleep(2 ** i)\n" },
                      { "key": "code", "label": "源码", "empty": { "icon": "function", "title": "未触及", "hint": "本对话未 create/edit" } },
                      { "key": "run", "label": "终端", "empty": { "icon": "run", "title": "尚无本对话运行" } },
                      { "key": "history", "label": "历史", "empty": { "icon": "history", "title": "尚无执行记录" } }
                    ]
                  },
                  {
                    "id": "wf_demo", "category": "entity", "kind": "workflow", "name": "pr_merge_flow", "lang": "json", "status": "err", "meta": "v5 · failed",
                    "facets": [
                      { "key": "overview", "label": "概览", "rows": [["lifecycle", "live"], ["concurrency", "serial"]] },
                      { "key": "flowrun", "label": "运行图", "nodes": [
                        { "id": "trigger", "kind": "trigger", "label": "pr.webhook", "status": "completed", "atPct": 0, "wPct": 12 },
                        { "id": "fetch", "kind": "action", "label": "fetch", "status": "failed", "atPct": 14, "wPct": 38 },
                        { "id": "transform", "kind": "action", "label": "transform", "status": "completed", "atPct": 14, "wPct": 26 }
                      ] },
                      { "key": "graph", "label": "图", "empty": { "icon": "workflow", "title": "未触及" } },
                      { "key": "versions", "label": "版本", "empty": { "icon": "diff", "title": "本对话未升版" } },
                      { "key": "history", "label": "历史", "empty": { "icon": "history", "title": "尚无运行记录" } }
                    ]
                  },
                  {
                    "id": "sub_demo", "category": "subagent", "name": "Explore · 核对接线", "status": "done", "meta": "Explore · 2 步",
                    "facets": [
                      { "key": "trace", "label": "轨迹", "blocks": [
                        { "type": "text", "text": "核对 cron trigger 是否真接到 sync_inventory。" },
                        { "type": "tool_call", "items": [{ "verb": "get_trigger", "name": "trg_3a1f8c2d", "result": { "json": { "listening": true } } }] },
                        { "type": "text", "text": "接线正确，trigger 监听中。" }
                      ] },
                      { "key": "overview", "label": "概览", "rows": [["type", "Explore"], ["steps", "2"], ["landsIn", "message_blocks"]] }
                    ]
                  },
                  {
                    "id": "todo", "category": "todo", "name": "Todo", "status": "run", "meta": "1/3",
                    "facets": [
                      { "key": "board", "label": "看板", "items": [
                        { "content": "建图并三层校验", "status": "completed" },
                        { "content": "trigger 冒烟", "status": "in_progress", "activeForm": "正在冒烟" },
                        { "content": "接真实 webhook", "status": "pending" }
                      ] }
                    ]
                  }
                ]
              }
            }
          }
        ]
      },
      {
        "name": "记录页骨架 page",
        "tag": "an-page",
        "blurb": "记录页滚动壳：居中 --w-content 列 + 唯一滚动区 + 浮动 overlay 滚轮（rAF 节流、空闲隐藏，不占 gutter）。零属性、零 props；header/tabs/sections 全塞默认 slot。",
        "specimens": [
          {
            "label": "default 滚动壳",
            "span": true,
            "tag": "an-page",
            "children": [
              {
                "tag": "an-ocean-header",
                "attrs": {
                  "crumb": "Entities|Workflow",
                  "title": "release-gate 发布闸门"
                }
              },
              {
                "tag": "an-section",
                "attrs": {
                  "title": "概览"
                },
                "children": [
                  {
                    "tag": "an-row",
                    "attrs": {
                      "icon": "workflow",
                      "label": "wf_release_gate",
                      "meta": "4 节点 · 1 回边"
                    }
                  }
                ]
              }
            ]
          }
        ]
      },
      {
        "name": "侧栏列表 sidebar-list",
        "tag": "an-sidebar-list",
        "blurb": "左岛 rail 复合件：New / 过滤 / 分组 / 类型头 / 实体行共用 Row 三列网格（行首槽对齐）。model 经 props 注入递归结构 groups→types→rows；属性 more 给每行加 … 动作。动作 an-new / an-filter / an-select。",
        "specimens": [
          {
            "label": "单组平铺（无 group label）",
            "span": true,
            "tag": "an-sidebar-list",
            "props": {
              "model": {
                "newLabel": "新建实体",
                "filterPlaceholder": "过滤实体…",
                "groups": [
                  {
                    "types": [
                      {
                        "icon": "function",
                        "label": "Function",
                        "count": 3,
                        "open": true,
                        "rows": [
                          {
                            "id": "fn_fetch_pr",
                            "label": "fetch_pr",
                            "dot": "done",
                            "meta": "v3"
                          },
                          {
                            "id": "fn_merge",
                            "label": "merge",
                            "dot": "done",
                            "meta": "v1"
                          },
                          {
                            "id": "fn_notify",
                            "label": "notify",
                            "dot": "idle",
                            "meta": "草稿"
                          }
                        ]
                      },
                      {
                        "icon": "workflow",
                        "label": "Workflow",
                        "count": 1,
                        "open": true,
                        "rows": [
                          {
                            "id": "wf_release_gate",
                            "label": "release-gate",
                            "dot": "run",
                            "meta": "运行中",
                            "selected": true
                          }
                        ]
                      }
                    ]
                  }
                ]
              }
            }
          },
          {
            "label": "可折叠大组 + more 行动作",
            "span": true,
            "tag": "an-sidebar-list",
            "attrs": {
              "more": true
            },
            "props": {
              "model": {
                "newLabel": "New",
                "filterPlaceholder": "Search…",
                "groups": [
                  {
                    "label": "Pinned",
                    "open": true,
                    "types": [
                      {
                        "icon": "agent",
                        "label": "Agent",
                        "count": 1,
                        "open": true,
                        "rows": [
                          {
                            "id": "ag_triage",
                            "label": "triage",
                            "dot": "done",
                            "meta": "ready"
                          }
                        ]
                      }
                    ]
                  },
                  {
                    "label": "Recents",
                    "open": true,
                    "types": [
                      {
                        "icon": "trigger",
                        "label": "Trigger",
                        "count": 2,
                        "open": true,
                        "rows": [
                          {
                            "id": "trg_webhook",
                            "label": "webhook · pr",
                            "dot": "run",
                            "meta": "active"
                          },
                          {
                            "id": "trg_cron",
                            "label": "cron · weekly",
                            "dot": "wait",
                            "meta": "暂停"
                          }
                        ]
                      },
                      {
                        "icon": "mcp",
                        "label": "MCP",
                        "count": 1,
                        "open": false,
                        "rows": [
                          {
                            "id": "mcp_github",
                            "label": "github",
                            "dot": "done",
                            "meta": "12 tools"
                          }
                        ]
                      }
                    ]
                  }
                ]
              }
            }
          }
        ]
      }
    ]
  },
  {
    "cat": "浮层 Overlays",
    "icon": "layers",
    "items": [
      {
        "name": "Toast toast",
        "tag": "an-toast",
        "blurb": "瞬时提示（window.AnToast.show）",
        "specimens": [
          {
            "label": "show()",
            "tag": "an-button",
            "demo": "toast",
            "icon": "bell",
            "text": "弹一条 toast"
          }
        ]
      },
      {
        "name": "对话框 dialog",
        "tag": "an-dialog",
        "blurb": "模态确认（AnDialog.open：title + content + actions）",
        "specimens": [
          {
            "label": "open()",
            "tag": "an-button",
            "demo": "dialog",
            "icon": "trash",
            "text": "打开对话框"
          }
        ]
      },
      {
        "name": "菜单 menu",
        "tag": "an-menu",
        "blurb": "锚定浮动菜单（AnMenu.open(anchor)）",
        "specimens": [
          {
            "label": "open(anchor)",
            "tag": "an-button",
            "demo": "menu",
            "icon": "sliders",
            "text": "打开菜单"
          }
        ]
      },
      {
        "name": "浮层底座 floating",
        "tag": "an-floating",
        "blurb": "浮层定位底座——被 menu / dropdown 复用，自身无独立 UI",
        "specimens": [
          {
            "label": "基础设施",
            "span": true,
            "tag": "an-callout",
            "attrs": {
              "tone": "info"
            },
            "text": "an-floating 是浮层定位底座（锚定 / 翻转 / 避让 viewport），由 an-menu、an-dropdown 复用，不直接展示。"
          }
        ]
      }
    ]
  }
];
