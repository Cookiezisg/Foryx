/* Anselm feature — entities 实体注册表（mock 实例；rail 分组 + sea schema 渲染共享）。Phase 3.0 deps 加载，Phase 3.1+ 换真后端 List/Get。
   每条 = { id, kind, label, meta, dot, data（renderEntity 消费）, args?/trace?（可执行实体的右岛试运行 mock）}。 */
window.ENTITY_REGISTRY = [
  {
    id: "function", kind: "function", label: "fetch_article", meta: "v4 · ready", dot: "done",
    data: {
  description: "抓取指定 URL 的页面正文，返回纯文本与标题。无状态，每次调用拉起全新沙箱。",
  meta: [
    ["name", "fetch_article"],
    ["tags", "scrape · http"],
    ["活动版本", "v4"],
    ["Python", "3.12"],
  ],
  code: "import httpx\nfrom selectolax.parser import HTMLParser\n\ndef fetch_article(url: str, timeout: int = 10):\n    resp = httpx.get(url, timeout=timeout, follow_redirects=True)\n    resp.raise_for_status()\n    tree = HTMLParser(resp.text)\n    title = tree.css_first(\"title\")\n    body = tree.css_first(\"article\") or tree.body\n    return {\n        \"title\": title.text(strip=True) if title else \"\",\n        \"text\": body.text(separator=\"\\n\", strip=True) if body else \"\",\n    }",
  inputs: [
    ["url", "string · 必填"],
    ["timeout", "int · 默认 10"],
  ],
  outputs: [
    ["title", "string"],
    ["text", "string"],
  ],
  dependencies: [
    { icon: "box", label: "httpx", meta: ">=0.27" },
    { icon: "box", label: "selectolax", meta: ">=0.3.21" },
  ],
  env: [
    ["env_id", "fnenv_9a4c1f0e7b2d3a6e"],
    ["状态", "ready"],
    ["最近同步", "2026-06-16 14:08"],
    ["错误", "—"],
  ],
  runs: [
    { icon: "check", dot: "done", label: "manual · 完成", meta: "842ms", hint: "2026-06-16 14:12", detail: [["运行 ID","fne_5e1a0c4b7d20f3a6"],["触发","manual · @weilin（HTTP :run）"],["沙箱","venv fnenv_9a4c…3a6e · py3.12 · v4"],["输入","url=https://example.com/post/42, timeout=10"],["输出","title=「本地优先 Agent 平台技术解析」 · text 4200 字"],["耗时","842ms"],["时间","2026-06-16 14:12:33"]] },
    { icon: "bot", dot: "done", label: "agent · 完成", meta: "1.1s", hint: "2026-06-15 09:30", detail: [["运行 ID","fne_b7e0a91c2f6d4e08"],["触发","agent · triage_bot（挂载工具 fn_<id>）"],["沙箱","venv fnenv_9a4c…3a6e · py3.12 · v4 · 全新拉起"],["输入","url=https://blog.acme.dev/release-notes, timeout=10"],["输出","title=「Acme v2.4 Release Notes」 · text 2.6k 字"],["耗时","1.1s"],["时间","2026-06-15 09:30:12"]] },
    { icon: "workflow", dot: "done", label: "workflow · 完成", meta: "910ms", hint: "2026-06-14 22:04", detail: [["运行 ID","fne_3c2d9a017e4b6f51"],["触发","workflow · nightly_digest（dispatch.RunAction）"],["溯源","flowrun fr_a4f1…9c · node fetch"],["沙箱","venv fnenv_9a4c…3a6e · py3.12 · v4"],["输入","url=https://news.ycombinator.com/item?id=40421, timeout=10"],["输出","title=「Show HN: …」 · text 1.9k 字"],["耗时","910ms"],["时间","2026-06-14 22:04:51"]] },
    { icon: "clock", dot: "wait", label: "manual · 超时", meta: "10.0s", hint: "2026-06-14 18:51", detail: [["运行 ID","fne_8f01b6c3d29a45e7"],["触发","manual · @weilin（HTTP :run）"],["沙箱","venv fnenv_9a4c…3a6e · py3.12 · v4"],["输入","url=https://slow.origin.test/article, timeout=10"],["状态","timeout（ctx.Err → status=timeout）"],["错误","httpx.ReadTimeout：10s 内未返回响应头"],["耗时","10.0s（命中 timeout 上限）"],["时间","2026-06-14 18:51:07"]] },
    { icon: "play", dot: "err", label: "chat · 失败", meta: "320ms", hint: "FUNCTION_ENV_NOT_READY · 2026-06-13 11:20", detail: [["运行 ID","fne_d4a90f1e6b2c7038"],["触发","chat · @weilin（run_function 工具）"],["溯源","conversation cv_2b7e… · tool_call blk_91a4…"],["沙箱","venv fnenv_9a4c…3a6e 状态 failed（selectolax 装包未完成）"],["输入","url=https://example.com/post/7, timeout=10"],["错误码","FUNCTION_ENV_NOT_READY"],["错误","env 未 ready，run 前置检查拒绝执行；需 Edit 空 ops 重装依赖（function.env_rebuilt）"],["耗时","320ms（未进沙箱即拒绝）"],["时间","2026-06-13 11:20:44"]] },
  ],
},
    args: "{\n  \"url\": \"https://example.com/post/42\",\n  \"timeout\": 10\n}",
    trace: {"lines":["→ spawn venv（httpx · selectolax）","→ GET https://example.com/post/42 → 200","解析正文…"],"result":{"st":"ok","out":"completed","ms":842,"json":{"title":"本地优先 Agent 平台技术解析","text":"正文 4200 字…"}}},
    versLang: "py",
    versions: [
      { v: "v4", active: true, t: "2026-06-16", reason: "跟随重定向 + article 优先选择器", src: "def fetch_article(url, timeout=10):\n    resp = httpx.get(url, timeout=timeout, follow_redirects=True)\n    resp.raise_for_status()\n    tree = HTMLParser(resp.text)\n    body = tree.css_first(\"article\") or tree.body\n    return {\"text\": body.text(strip=True)}" },
      { v: "v3", t: "2026-06-10", reason: "加超时与 raise_for_status", src: "def fetch_article(url, timeout=10):\n    resp = httpx.get(url, timeout=timeout)\n    resp.raise_for_status()\n    tree = HTMLParser(resp.text)\n    body = tree.body\n    return {\"text\": body.text(strip=True)}" },
      { v: "v2", t: "2026-06-02", reason: "初版：纯 httpx 抓取", src: "def fetch_article(url):\n    resp = httpx.get(url)\n    tree = HTMLParser(resp.text)\n    return {\"text\": tree.body.text(strip=True)}" },
    ],
  },
  {
    id: "handler", kind: "handler", label: "slack_client", meta: "常驻 · running", dot: "done",
    data: {
  description: "常驻 Slack 客户端——持有鉴权后的 WebClient，跨调用复用连接发消息 / 拉频道。",
  meta: [
    ["ID", "hd_4c1a9f02e7b3d680"],
    ["活动版本", "v5"],
    ["python", "3.12"],
    ["执行单位", "常驻进程上的一次 RPC"],
    ["更新于", "2026-06-16 14:22"],
  ],
  runtime: [
    ["状态", "running"],
    ["实例 ID", "hdi_b7e0c4319a26f5d1"],
    ["spawn 于", "2026-06-17 09:03"],
    ["最近调用", "send · 11s 前"],
  ],
  configState: [
    ["完整度", "ready"],
    ["已配置", "2 / 2"],
    ["加密", "AES-GCM 整 blob"],
  ],
  missingConfig: [
    { icon: "check", label: "无缺失必填项", meta: "可正常 spawn", passive: true },
  ],
  initArgs: [
    ["api_key", "required · sensitive · ********"],
    ["base_url", "optional · default https://slack.com/api"],
  ],
  config: {
    api_key: "********",
    base_url: "https://slack.com/api",
  },
  methods: [
    { icon: "send", label: "send", meta: "to:str, text:str → str", hint: "timeout 8000ms" },
    { icon: "book-open", label: "list_channels", meta: "→ list", hint: "" },
    { icon: "git-branch", label: "stream_history", meta: "channel:str → progress*", hint: "generator" },
  ],
  code: "class HandlerImpl:\n    def __init__(self, api_key: str, base_url: str = \"https://slack.com/api\"):\n        from slack_sdk import WebClient\n        self.client = WebClient(token=api_key, base_url=base_url)\n\n    def shutdown(self):\n        self.client = None\n\n    def send(self, to: str, text: str) -> str:\n        resp = self.client.chat_postMessage(channel=to, text=text)\n        return resp[\"ts\"]\n\n    def list_channels(self) -> list:\n        return [c[\"name\"] for c in self.client.conversations_list()[\"channels\"]]",
  calls: [
    { dot: "done", icon: "send", label: "send", meta: "ok · 142ms", hint: "by chat · 11s 前",
      detail: [["调用 ID", "hdc_5e1a…c20"], ["实例", "hdi_b7e0c4319a26f5d1"], ["参数", "to=#ops, text=deploy done"], ["返回", "ts=1718533201.0042"], ["耗时", "142ms"], ["触发", "chat · @weilin"], ["时间", "2026-06-17 14:22:09"]] },
    { dot: "done", icon: "book-open", label: "list_channels", meta: "ok · 318ms", hint: "by agent · 4m 前",
      detail: [["调用 ID", "hdc_4b8c…a90"], ["参数", "（无）"], ["返回", "12 个频道"], ["耗时", "318ms"], ["触发", "agent triage_agent"], ["时间", "2026-06-17 14:18:31"]] },
    { dot: "err", icon: "git-branch", label: "stream_history", meta: "HANDLER_RPC_TIMEOUT · 8000ms", hint: "by workflow · 22m 前",
      detail: [["调用 ID", "hdc_3a7b…f22"], ["参数", "channel=#dev"], ["错误码", "HANDLER_RPC_TIMEOUT"], ["错误", "RPC 超时：8000ms 内未返回首帧"], ["触发", "workflow pr_merge_flow"], ["时间", "2026-06-17 14:00:12"]] },
    { dot: "done", icon: "send", label: "send", meta: "ok · 96ms", hint: "by chat · 1h 前",
      detail: [["调用 ID", "hdc_29df…118"], ["参数", "to=#release, text=nightly ok"], ["返回", "ts=1718529600.0011"], ["耗时", "96ms"], ["触发", "chat"], ["时间", "2026-06-17 13:20:45"]] },
  ],
},
    args: "{\n  \"method\": \"send\",\n  \"to\": \"#ops\",\n  \"text\": \"deploy done\"\n}",
    trace: {"lines":["→ RPC → 常驻实例 hdi_b7e0…","send #ops"],"result":{"st":"ok","out":"ts=1718533201.0042","ms":142,"json":{"ts":"1718533201.0042","ok":true}}},
    versLang: "py",
    versions: [
      { v: "v5", active: true, t: "2026-06-16", reason: "send 返回消息 ts", src: "def send(self, to, text):\n    resp = self.client.chat_postMessage(channel=to, text=text)\n    return resp[\"ts\"]\n\ndef list_channels(self):\n    return [c[\"name\"] for c in self.client.conversations_list()[\"channels\"]]" },
      { v: "v4", t: "2026-06-08", reason: "补 list_channels 方法", src: "def send(self, to, text):\n    self.client.chat_postMessage(channel=to, text=text)\n\ndef list_channels(self):\n    return [c[\"name\"] for c in self.client.conversations_list()[\"channels\"]]" },
      { v: "v3", t: "2026-05-30", reason: "初版：仅 send", src: "def send(self, to, text):\n    self.client.chat_postMessage(channel=to, text=text)" },
    ],
  },
  {
    id: "agent", kind: "agent", label: "triage_agent", meta: "v4 · 挂载 3", dot: "done",
    data: {
  description: "技术分诊 Agent：读取失败执行的 transcript，定位根因并给出修复建议。",
  meta: [
    ["ID", "ag_9c2f1a7b04d3e6f8"],
    ["活动版本", "v4"],
    ["模型", "claude-opus-4 (覆盖)"],
    ["挂载健康", "1 项异常"],
    ["最近运行", "2026-06-17 09:42"],
  ],
  prompt: "You are a triage agent.\nYour role: read the transcript of a failed execution and locate the root cause.\n\n- Use only the mounted tools; do not fabricate capabilities.\n- Restate the failure first, then narrow the scope step by step.\n- The final answer must be a single JSON object with exactly {rootCause, fix, confidence}.",
  tools: [
    { icon: "code", label: "parse_traceback", meta: "fn_a1b2c3d4e5f60718", hint: "解析 Python traceback" },
    { icon: "git-branch", label: "git_blame__line", meta: "hd_77ee...__blame", hint: "handler 方法绑定" },
    { icon: "plug", label: "github__search_issues", meta: "mcp:github/search_issues", hint: "需在线 server" },
  ],
  skill: "triage-runbook（注入 system prompt 执行指南）",
  knowledge: [
    { icon: "file-text", label: "事故复盘手册", meta: "doc_3f8a91c2", passive: true },
    { icon: "file-text", label: "错误码对照表", meta: "doc_b4d70e15", passive: true },
  ],
  modelOverride: [
    ["provider", "anthropic"],
    ["model", "claude-opus-4"],
    ["temperature", "0.2"],
  ],
  mountHealth: [
    { dot: "done", label: "parse_traceback", meta: "fn_a1b2c3d4e5f60718", hint: "可解析" },
    { dot: "done", label: "git_blame__blame", meta: "hd_77ee...__blame", hint: "可解析" },
    { dot: "err", label: "github__search_issues", meta: "mcp:github/search_issues", hint: "MCP server 离线 — invoke 将失败" },
  ],
  inputs: [
    ["executionId", "string · 必填 · 待分诊的执行 ID"],
    ["hint", "string · 可选 · 人工补充线索"],
  ],
  outputs: [
    ["rootCause", "string · 根因一句话"],
    ["fix", "string · 修复建议"],
    ["confidence", "number · 0–1 置信度"],
  ],
  executions: [
    { dot: "done", label: "agx_5e9d...c01", meta: "manual · 4 步 · 1.8k tok", hint: "09:42 · ok",
      detail: [["执行 ID", "agx_5e9d…c01"], ["触发", "manual · @weilin"], ["步数", "4（读 transcript → parse_traceback → 缩范围 → 归纳）"], ["Token", "1.8k"], ["输出", "{ rootCause, fix, confidence: 0.82 }"], ["时间", "2026-06-17 09:42 · 1.8s"]] },
    { dot: "err", label: "agx_4b8c...a90", meta: "chat · mount 解析失败", hint: "08:15 · AGENT_MOUNT_INVALID",
      detail: [["执行 ID", "agx_4b8c…a90"], ["触发", "chat"], ["错误码", "AGENT_MOUNT_INVALID"], ["错误", "github MCP server 离线，工具挂载解析失败，invoke 前置预检未过"], ["时间", "2026-06-17 08:15"]] },
    { dot: "done", label: "agx_3a7b...f22", meta: "workflow · 6 步 · 3.1k tok", hint: "昨天 · ok",
      detail: [["执行 ID", "agx_3a7b…f22"], ["触发", "workflow pr_merge_flow"], ["步数", "6"], ["Token", "3.1k"], ["输出", "{ rootCause, fix, confidence: 0.91 }"], ["时间", "昨天 18:30 · 3.4s"]] },
  ],
},
    args: "{\n  \"executionId\": \"agx_4b8c…a90\",\n  \"hint\": \"\"\n}",
    trace: {"lines":["→ 读取失败 transcript","→ parse_traceback","→ 缩小范围 · 归纳根因"],"result":{"st":"ok","out":"completed · 4 步 · 1.8k tok","ms":1800,"json":{"rootCause":"venv 未就绪即调用","fix":"等 env_status=ready 再 :run","confidence":0.82}}},
    versLang: "md",
    versions: [
      { v: "v4", active: true, t: "2026-06-15", reason: "强制输出三字段 JSON", src: "You are a triage agent.\nRead the transcript of a failed execution and locate the root cause.\n\n- Use only the mounted tools.\n- Restate the failure first, then narrow the scope.\n- The final answer must be a single JSON: {rootCause, fix, confidence}." },
      { v: "v3", t: "2026-06-07", reason: "加挂载工具约束", src: "You are a triage agent.\nRead the transcript of a failed execution and locate the root cause.\n\n- Use only the mounted tools.\n- Restate the failure first, then narrow the scope." },
      { v: "v2", t: "2026-05-29", reason: "初版：自由格式根因", src: "You are a triage agent.\nRead the failed transcript and explain the likely root cause." },
    ],
  },
  {
    id: "workflow", kind: "workflow", label: "pr_merge_flow", meta: "active · serial", dot: "done",
    data: {
  description: "PR 合并后跑测试，失败则审批是否回滚——监听 GitHub webhook，按分支结果路由。",
  meta: [
    ["ID", "wf_9f2a7c1b3e8d4602"],
    ["当前版本", "v7（active）"],
    ["节点", "5 个 · 边 6 条"],
    ["入口 trigger", "trg_3a1f… (webhook)"]
  ],
  graph: {
    nodes: [
      { id: "on_pr_merged", kind: "trigger", ref: "trg_3a1f9c", input: {} },
      { id: "run_tests", kind: "action", ref: "fn_5b2e1a", input: { sha: "on_pr_merged.payload" }, retry: { maxAttempts: 2, backoff: "fixed", delayMs: 500 } },
      { id: "branch_result", kind: "control", ref: "ctl_7d4c", input: { passed: "run_tests.passed" } },
      { id: "approve_rollback", kind: "approval", ref: "apf_2e9b", input: { failures: "run_tests.failures" } },
      { id: "do_rollback", kind: "action", ref: "hd_8a3f.rollback", input: { sha: "on_pr_merged.payload" } },
    ],
    edges: [
      { id: "g1", from: "on_pr_merged", to: "run_tests" },
      { id: "g2", from: "run_tests", to: "branch_result" },
      { id: "g3", from: "branch_result", to: "approve_rollback", port: "fail" },
      { id: "g4", from: "branch_result", to: "run_tests", port: "retry" },
      { id: "g5", from: "approve_rollback", to: "do_rollback", port: "yes" },
    ],
    run: {
      state: { on_pr_merged: "completed", run_tests: "completed", branch_result: "completed", approve_rollback: "parked", do_rollback: "future" },
      taken: ["g1", "g2", "g3"], live: null, iters: { run_tests: 2 },
      memo: {
        on_pr_merged: { out: "PR #1287 merged → main" },
        run_tests: { loop: [["#0", "3 failed"], ["#1", "1 failed"]] },
        branch_result: { __port: "fail", passed: false },
        approve_rollback: { parked: true, prompt: "main 分支测试连续两轮失败，是否回滚本次合并？", ddl: "8h 后自动驳回", form: "apf_2e9b v4" },
      },
    },
  },
  lifecycle: [
    ["状态", "active（监听中）"],
    ["归因", "user · @weilin"],
    ["在途 run", "1"]
  ],
  concurrency: [
    ["策略", "serial"],
    ["含义", "在途则推迟、下次 drain 生效"],
    ["firing 处置", "失败不静默丢弃"]
  ],
  attention: [
    { icon: "shield-check", dot: "done", label: "无告警", meta: "最近一次 run 已 completed", hint: "失败时告警；run 完成后自动清除", passive: true }
  ],
  flowruns: [
    { icon: "check", dot: "done", label: "fr_a1c… completed", meta: "5 节点全记忆化 · 1.4s", hint: "trigger: webhook · 12:04",
      detail: [["flowrun ID", "fr_a1c8…9f02"], ["触发", "webhook · firing trf_a1c8…"], ["payload", "pr=#1284, branch=main"], ["节点记忆化", "5/5 nodes recorded（idx_frn_once）"], ["路径", "on_pr_merged → run_tests → branch_result(fail) → approve_rollback(yes) → do_rollback"], ["终态节点", "do_rollback completed"], ["状态", "completed"], ["耗时", "1.4s"], ["时间", "2026-06-17 12:04:31"]] },
    { icon: "clock", dot: "run", label: "fr_b7e… running", meta: "在途 · approve_rollback 待决", hint: "trigger: webhook · 12:09",
      detail: [["flowrun ID", "fr_b7e0…c431"], ["触发", "webhook · firing trf_b7e0…"], ["payload", "pr=#1287, branch=main"], ["节点记忆化", "4/5 node_id 已落行；approve_rollback parked、do_rollback future"], ["当前节点", "approve_rollback（parked · 待人工决策）"], ["在途", "run_tests×2 轮均 fail → branch_result=fail → 等 yes/no"], ["DDL", "8h 后自动驳回"], ["耗时", "1.4s（至 parked）"], ["时间", "2026-06-17 12:09:07"]] },
    { icon: "play", dot: "wait", label: "fr_c3d… failed", meta: "run_tests 退出码非 0", hint: "可 :replay 修复 · 昨日 18:21",
      detail: [["flowrun ID", "fr_c3d4…71a8"], ["触发", "webhook · firing trf_c3d4…"], ["payload", "pr=#1279, branch=main"], ["节点记忆化", "2/5 nodes recorded（on_pr_merged, run_tests）"], ["终态节点", "run_tests failed（fn_5b2e1a 退出码非 0）"], ["状态", "failed（action 节点出错、未走分支路由）"], ["错误", "run_tests 子进程退出码 1，retry 2 轮仍非 0"], ["replay", ":replay 清 failed 行、保留前置记忆化、自 run_tests 续跑"], ["时间", "2026-06-16 18:21:44"]] },
  ],
},
    args: "{\n  \"payload\": { \"pr\": 1287, \"branch\": \"main\" }\n}",
    trace: {"lines":["→ 起 flowrun fr_b7e…","run_tests… 退出码非 0","branch_result → fail","approve_rollback parked（待人工）"],"result":{"st":"running","out":"parked at approve_rollback","ms":1420}},
    versLang: "json",
    versions: [
      { v: "v7", active: true, t: "2026-06-14", reason: "失败分支加审批门", src: "{\n  \"nodes\": [\"on_pr_merged\", \"run_tests\", \"branch_result\", \"approve_rollback\", \"do_rollback\"],\n  \"edges\": [\"merged→tests\", \"tests→branch\", \"branch→approve(fail)\", \"branch→tests(retry)\", \"approve→rollback(yes)\"]\n}" },
      { v: "v6", t: "2026-06-05", reason: "失败重试回边", src: "{\n  \"nodes\": [\"on_pr_merged\", \"run_tests\", \"branch_result\", \"do_rollback\"],\n  \"edges\": [\"merged→tests\", \"tests→branch\", \"branch→rollback(fail)\", \"branch→tests(retry)\"]\n}" },
      { v: "v5", t: "2026-05-22", reason: "初版：直跑测试无回滚", src: "{\n  \"nodes\": [\"on_pr_merged\", \"run_tests\", \"branch_result\"],\n  \"edges\": [\"merged→tests\", \"tests→branch\"]\n}" },
    ],
  },
  {
    id: "control", kind: "control", label: "route_approval", meta: "3 分支", dot: "idle",
    data: {
  description: "依工单优先级与金额路由审批流——高优先级或大额走 escalate 出口，其余兜底直通。",
  meta: [
    ["ID", "ctl_9f3a1c20e4b78d56"],
    ["当前版本", "ctlv_2b6e0a91f3c4d870 · v4"],
    ["分支数", "3"],
    ["求值", "解释器内联 · first-true-wins"],
    ["状态", "active"],
  ],
  inputs: [
    ["priority", "string"],
    ["amount", "number"],
    ["region", "string"],
  ],
  branches: [
    { icon: "git-branch", dot: "run", label: "escalate", meta: "priority == 'high' || amount > 10000", hint: "emit: tier, reviewer" },
    { icon: "git-branch", dot: "run", label: "regional", meta: "region in ['EU', 'APAC']", hint: "emit: queue" },
    { icon: "check", dot: "done", label: "default", meta: "true", hint: "catch-all · 透传 input", passive: true },
  ],
  when: "priority == 'high' || amount > 10000",
  emit: {
    tier: "input.amount > 50000 ? 'exec' : 'manager'",
    reviewer: "input.region == 'EU' ? 'eu-board' : 'global-board'",
    __port: "escalate"
  },
},
    versLang: "json",
    versions: [
      { v: "v4", active: true, t: "2026-06-15", reason: "新增 regional 分支", src: "[\n  { \"when\": \"priority == 'high' || amount > 10000\", \"port\": \"escalate\" },\n  { \"when\": \"region in ['EU','APAC']\", \"port\": \"regional\" },\n  { \"when\": \"true\", \"port\": \"default\" }\n]" },
      { v: "v3", t: "2026-06-09", reason: "调高 amount 阈值至 10000", src: "[\n  { \"when\": \"priority == 'high' || amount > 10000\", \"port\": \"escalate\" },\n  { \"when\": \"true\", \"port\": \"default\" }\n]" },
      { v: "v2", t: "2026-05-28", reason: "补 EU emit 重写", src: "[\n  { \"when\": \"priority == 'high' || amount > 5000\", \"port\": \"escalate\" },\n  { \"when\": \"true\", \"port\": \"default\" }\n]" },
    ],
  },
  {
    id: "approval", kind: "approval", label: "release_gate", meta: "v4", dot: "done",
    data: {
  description: "上线发布前的人工放行闸：把待发布摘要渲染给审批人，等 yes/no 决策再放下游。",
  meta: [
    ["实体", "apf_3f9c1a7b20d4e8f1"],
    ["当前版本", "v4 (active)"],
    ["状态", "已激活"],
    ["更新于", "2026-06-15 14:22"],
  ],
  template: "## 发布放行确认\n\n**服务**：{{ input.service }}\n**版本**：`{{ input.version }}`\n**变更条数**：{{ input.changeCount }}\n\n> 由 {{ input.requestedBy }} 于发布窗口提交，请确认是否放行。",
  inputs: [
    ["service", "string · 必填"],
    ["version", "string · 必填"],
    ["changeCount", "number"],
    ["requestedBy", "string"],
  ],
  decision: [
    ["允许填备注", "是 (allowReason)"],
    ["超时", "30d"],
    ["超时行为", "reject（默认否决）"],
    ["出口", "yes / no（固定两口）"],
  ],
  ports: [
    { icon: "check", dot: "done", label: "yes", meta: "已批准", hint: "连向下游放行分支" },
    { icon: "shield-check", dot: "wait", label: "no", meta: "已否决 / 超时", hint: "连向回退分支" },
  ],
},
    versLang: "md",
    versions: [
      { v: "v4", active: true, t: "2026-06-15", reason: "新增 changeCount 字段", src: "## 发布放行确认\n\n**服务**：{{ input.service }}\n**版本**：`{{ input.version }}`\n**变更条数**：{{ input.changeCount }}\n\n> 由 {{ input.requestedBy }} 于发布窗口提交，请确认是否放行。" },
      { v: "v3", t: "2026-06-03", reason: "超时由 7d 改 30d", src: "## 发布放行确认\n\n**服务**：{{ input.service }}\n**版本**：`{{ input.version }}`\n\n> 由 {{ input.requestedBy }} 提交，请确认是否放行。" },
      { v: "v2", t: "2026-05-25", reason: "allowReason 置真", src: "## 发布放行确认\n\n**服务**：{{ input.service }}\n**版本**：`{{ input.version }}`\n\n> 请确认是否放行。" },
    ],
  },
  {
    id: "trigger", kind: "trigger", label: "daily_digest", meta: "cron · listening", dot: "run",
    data: {
  description: "每个工作日早 9 点对最近 24h 的支持工单做一次摘要扇出——cron 刻度 fire，扇给所有监听本 trigger 的 active workflow。",
  meta: [
    ["源类型", "cron"],
    ["监听态", "listening · 3 个 workflow 引用"],
    ["引用计数", "3"],
    ["最近 fire", "12 分钟前 · 起 2 个 flowrun"],
    ["待命", "常驻（非 once）"],
  ],
  sourceMeta: [
    ["kind", "cron"],
    ["表达式", "0 9 * * 1-5（robfig 5 段，分钟粒度）"],
    ["热更", "Edit 即对监听中 listener 重 Register"],
  ],
  config: {
    kind: "cron",
    cron: { expression: "0 9 * * 1-5", timezone: "Asia/Singapore" }
  },
  dedup: [
    ["策略", "trigger + tick 时刻"],
    ["折叠", "同一刻度的重复材化"],
    ["约束", "idx_trf_dedup UNIQUE(workflow_id, trigger_id, dedup_key)"],
  ],
  outputs: [
    ["firedAt", "datetime · 本次刻度时间"],
    ["tick", "string · 刻度槽标识"],
    ["payload", "object · 扇给监听者的信号体"],
  ],
  activations: [
    { icon: "zap", dot: "done", label: "fired=true · 9:00 刻度", meta: "12 分钟前", hint: "firingCount=2 · 扇给 2 个监听者",
      detail: [["activation ID", "tra_9c2a4f1e8b00d3a7"], ["判定", "fired=true"], ["刻度", "2026-06-17 09:00 · tick 0 9 * * 1-5"], ["timezone", "Asia/Singapore"], ["firingCount", "2"], ["监听者", "wf 工单日报 · wf 升级值班通知"], ["触发", "cron 刻度"], ["时间", "2026-06-17 09:00:02 · 12 分钟前"]] },
    { icon: "zap", dot: "done", label: "fired=true · 昨日 9:00 刻度", meta: "1 天前", hint: "firingCount=3",
      detail: [["activation ID", "tra_5b71e0c93a2f6d18"], ["判定", "fired=true"], ["刻度", "2026-06-16 09:00 · tick 0 9 * * 1-5"], ["timezone", "Asia/Singapore"], ["firingCount", "3"], ["监听者", "wf 工单日报 · wf 升级值班通知 · wf 周报归档"], ["触发", "cron 刻度"], ["时间", "2026-06-16 09:00:01 · 1 天前"]] },
    { icon: "clock", dot: "idle", label: "fired=false · 手动 :fire 探测", meta: "2 天前", hint: "detail：当时 0 个监听者，仅记审计",
      detail: [["activation ID", "tra_2d0f7a16c4e9b35c"], ["判定", "fired=false"], ["刻度", "manual :fire 探测（不走 cron tick）"], ["firingCount", "0"], ["监听者", "无（当时 0 个 active listener）"], ["原因", "无监听者可扇出，仅落 activation 审计行（D1 Log · 永不删除）"], ["触发", "manual :fire · @weilin"], ["时间", "2026-06-15 16:30:44 · 2 天前"]] },
  ],
  firings: [
    { icon: "play", dot: "done", label: "started · wf 工单日报", meta: "12 分钟前", hint: "flowrun fr_3a9f… 已起", detail: [["Firing ID","trf_3a9f…d10"],["目标 workflow","wf_工单日报 (wf_1c4e…)"],["dedup 键","wf_1c4e + trg_daily + tick:2026-06-17T09:00"],["状态","started → flowrun fr_3a9f…（persist-before-act：先落收件箱再起 run）"],["claim","scheduler tick @ 09:00:03 单事务认领"],["payload","{ firedAt:09:00, tick:'09:00', window:'24h' }"],["时间","2026-06-17 09:00:03"]] },
    { icon: "play", dot: "done", label: "started · wf 升级值班通知", meta: "12 分钟前", hint: "flowrun fr_77c2… 已起", detail: [["Firing ID","trf_77c2…a48"],["目标 workflow","wf_升级值班通知 (wf_9b32…)"],["dedup 键","wf_9b32 + trg_daily + tick:2026-06-17T09:00"],["状态","started → flowrun fr_77c2…（同刻度第 2 个扇出）"],["claim","scheduler tick @ 09:00:03 单事务认领"],["payload","{ firedAt:09:00, tick:'09:00', window:'24h' }"],["时间","2026-06-17 09:00:03"]] },
    { icon: "git-branch", dot: "wait", label: "skipped · wf 工单日报", meta: "1 天前", hint: "overlap：上一轮 run 未完，本刻度跳过", detail: [["Firing ID","trf_5e80…b21"],["目标 workflow","wf_工单日报 (wf_1c4e…)"],["dedup 键","wf_1c4e + trg_daily + tick:2026-06-16T09:00"],["状态","skipped（overlap：该 workflow 上一轮 flowrun 仍 running，singleton 策略本刻度不并发起新 run）"],["flowrun","—（未起，仅记 Log）"],["payload","{ firedAt:09:00, tick:'09:00' }"],["时间","2026-06-16 09:00:04"]] },
    { icon: "clock", dot: "idle", label: "pending · 待 scheduler drain", meta: "—", hint: "收件箱 5s tick 下一轮认领", detail: [["Firing ID","trf_c0a4…f93（已 persist，待处置）"],["目标 workflow","wf_升级值班通知 (wf_9b32…)"],["dedup 键","wf_9b32 + trg_daily + tick:next"],["状态","pending（durable 收件箱行已落库，等 scheduler 每 5s 逐 workspace drain 时 ClaimFiring）"],["flowrun","—（尚未认领，claim 事务内才决 started/skipped）"],["payload","{ firedAt:next-tick }"],["时间","— · 下一轮 5s tick 认领"]] },
  ],
},
    args: "{}",
    trace: {"lines":["→ FireManual（手动探测）","扇给 2 个监听 workflow"],"result":{"st":"ok","out":"activation trf_9c2a… · 2 firings","ms":60,"json":{"fired":true,"firingCount":2}}},
  },
  {
    id: "mcp", kind: "mcp", label: "github", meta: "ready · 7 工具", dot: "done",
    data: {
  description: "GitHub 官方 MCP server——仓库 / issue / PR / 代码搜索工具集，经 stdio 常驻进程接入。",
  meta: [
    ["来源", "registry"],
    ["registryId", "io.github/github-mcp-server"],
    ["runtime", "node"],
    ["超时", "180s"],
    ["创建", "2026-06-11 09:42"],
  ],
  status: "ready · 7 工具已缓存",
  lastError: "—",
  transport: [
    ["transport", "stdio"],
    ["command", "npx"],
    ["args", "-y @modelcontextprotocol/server-github"],
    ["env", "GITHUB_TOKEN（已加密注入）"],
  ],
  tools: [
    { icon: "box", label: "search_repositories", meta: "tools/list", hint: "按关键词检索仓库" },
    { icon: "box", label: "get_file_contents", meta: "tools/list", hint: "读取仓库文件内容" },
    { icon: "box", label: "create_issue", meta: "tools/list", hint: "新建 issue" },
    { icon: "box", label: "list_pull_requests", meta: "tools/list", hint: "列出 PR" },
    { icon: "box", label: "search_code", meta: "tools/list", hint: "代码全文搜索" },
  ],
  calls: [
    { dot: "done", label: "search_repositories", meta: "ok · 412ms", hint: "chat · 2026-06-17 14:20",
      detail: [["工具", "search_repositories"], ["参数", "q=anselm in:name"], ["返回", "5 个仓库"], ["耗时", "412ms"], ["触发", "chat"], ["时间", "2026-06-17 14:20"]] },
    { dot: "done", label: "get_file_contents", meta: "ok · 188ms", hint: "agent invoke · 2026-06-17 14:18",
      detail: [["工具", "get_file_contents"], ["参数", "repo=anselm, path=README.md"], ["返回", "正文 6.2KB"], ["耗时", "188ms"], ["触发", "agent invoke"], ["时间", "2026-06-17 14:18"]] },
    { dot: "err", label: "create_issue", meta: "failed · 9.0s", hint: "403 rate limited · 2026-06-17 13:55",
      detail: [["工具", "create_issue"], ["参数", "repo=anselm, title=…"], ["错误", "403 secondary rate limit；退避 9s 后仍失败"], ["触发", "chat"], ["时间", "2026-06-17 13:55"]] },
    { dot: "done", label: "list_pull_requests", meta: "ok · 660ms", hint: "workflow · 2026-06-17 11:02",
      detail: [["工具", "list_pull_requests"], ["参数", "repo=anselm, state=open"], ["返回", "3 个 PR"], ["耗时", "660ms"], ["触发", "workflow"], ["时间", "2026-06-17 11:02"]] },
  ],
  stderr: "[github-mcp] connected to GitHub API (rest+graphql)\n[github-mcp] tools/list -> 7 tools\n[github-mcp] warn: secondary rate limit hit, backing off 9s",
},
  },
  {
    id: "skill", kind: "skill", label: "summarize-diff", meta: "ai · 文件式", dot: "idle",
    data: {
  description: "把杂乱的 git diff 总结成一段结构化的 PR 描述，自动归类改动并标注潜在风险。",
  meta: [
    ["name (slug)", "summarize-diff"],
    ["source", "ai"],
    ["持久化", "skills/summarize-diff/SKILL.md"],
    ["版本", "无（编辑即覆盖文件）"],
  ],
  frontmatter: {
    name: "summarize-diff",
    description: "把杂乱的 git diff 总结成一段结构化的 PR 描述。",
    source: "ai",
    "allowed-tools": ["read_file", "bash:git"],
    agent: "code-reviewer",
    license: "MIT",
  },
  body: "# Summarize Diff\n\n你将收到一段 git diff，参数为 $ARGUMENTS。\n\n步骤：\n1. 读取改动范围（$1 = base 分支，默认 main）。\n2. 按「功能 / 修复 / 重构 / 文档」归类每个 hunk。\n3. 标注任何触及鉴权或迁移的高风险改动。\n\n会话：${CLAUDE_SESSION_ID}\n输出一段可直接贴进 PR 的 Markdown 描述。",
  inline: [
    { icon: "file-text", label: "渲染正文", hint: "$ARGUMENTS / $1..$n / 命名占位 / ${CLAUDE_SESSION_ID} 替换" },
    { icon: "send", label: "注入当前对话", hint: "设为 active skill" },
    { icon: "shield-check", label: "预授权 allowed-tools", hint: "本次运行内免危险确认" },
    { icon: "shield-check", label: "拒绝 shell 注入", hint: "刻意不支持 !`cmd`", passive: true },
  ],
  fork: [
    { icon: "bot", label: "派给隔离 subagent", hint: "frontmatter.agent = code-reviewer" },
    { icon: "git-branch", label: "渲染正文随提示带入", hint: "独立上下文执行" },
    { dot: "wait", label: "缺 agent 则报错", hint: "SKILL_FORK_REQUIRES_AGENT", passive: true },
  ],
  allowedTools: [
    { icon: "check", label: "read_file", meta: "equip 出边" },
    { icon: "check", label: "bash:git", meta: "equip 出边" },
    { icon: "shield-check", label: "预授权 ≠ 限制白名单", hint: "由危险确认流消费、非门控", passive: true },
  ],
},
  },
];

// 能力挂载图标与左岛同源：每条工具挂载图标 = 被挂实体 kind 图标（据 meta 引用 ID 前缀派生），
// 与左岛分组图标逐一对齐；非实体引用（如文档 doc_）派生为空则保留自带图标。
window.ENTITY_REGISTRY.forEach((e) => {
  const tools = e.data && e.data.tools;
  if (Array.isArray(tools)) tools.forEach((t) => { const ic = window.kindIconOf(t.meta); if (ic) t.icon = ic; });
});
