---
id: WRK-014
type: working
status: active
owner: @weilin
created: 2026-06-12
reviewed: 2026-06-12
review-due: 2026-09-12
expires: 2026-09-12
landed-into: ""
audience: [human, ai]
---

# findings —— 验收发现（AC-N，每条真机复现 + 亲验定性）

> 严重度：🔴 功能不可用/语义错 / 🟡 体验或一致性 / 🟢 轻症。处置：fixed / pending / wontfix（带理由）/ doc-fix。

## W0 环境与座架

- **AC-1 🟢 function 创建响应嵌套形 `{function, version}` 与 workspace 创建扁平形不一致**（观察，待 W1 定性）
  真机：POST /functions 返 `{"function":{...},"version":{...}}`，POST /workspaces 返扁平对象。前端两套解析。是否统一留 W1 对照全实体后定。
- **AC-2 🟡 function 创建同步阻塞 env 物化**（观察，待 W1 定性）
  真机：首建 function 的 POST 阻塞 26.3s（冷启动下载 python + venv + pip）；运行时缓存命中后仍 ~3-6s（venv 构建）。创建即编辑器场景，秒级以上同步阻塞值得裁决（异步物化 + envStatus=installing 已有列支撑）。

## W1 锻造域

- **AC-3 🟢 api.md `:run` body 写 `{input}` 实为 `{args}`**（doc-fix）
  真机：`POST /functions/{id}:run` 带 `{"input":{...}}` 被严格解码拒 400 INVALID_REQUEST；代码收 `{args, version}`（handlers/function.go:167-170），与 run_function 工具一致。api.md 已重述。黑盒按文档打、被代码拒——正是验收要抓的契约漂移。
- **AC-4 🔴 SpawnLongLived 用 CommandContext——常驻实例绑死在首个请求的 ctx 上**（fixed）
  真机：首个 `:call` 懒 spawn 实例 → 请求结束 ctx 取消 → **实例被连带杀死** → 后续调用全部撞尸体重 spawn（每次调用都付 spawn 代价，且首调后实例必死）。单测从未暴露（测试 ctx 活到断言后）。修复：`exec.Command` 替换 `CommandContext`，生命周期由 handle.Kill/Shutdown 显式拥有（infra/sandbox/spawn.go:141）。**只有真机验收能抓到的级别**。
- **AC-5 🟡 handler driver 无协议护盾——用户 print() 直接污染 JSON-RPC 判死实例**（fixed）
  真机：方法体一行 print → 协议帧解析失败 → 实例判 crash 废弃重生，用户代码「合法操作」杀进程。修复：DriverScript 启动即重定向用户态 stdout→stderr（import/__init__/method/shutdown 全程受保护），协议只经保存的真 stdout 写——与 function driver 同款护盾；print 自此变成调用日志（assemble.go）。
- **AC-6 🟡 stderr 窗口竞态——print 先写却可能后到、被 detach 关在门外**（fixed）
  真机：单跑绿、全波跑挂——stdout（return 帧）与 stderr（print）两条独立管道各自 goroutine 在读，无跨管道顺序保证。修复：detach 前 30ms stderr 宽限（call.go stderrGrace），count=2 复跑确定性绿。
- **AC-7 🟡 approval 孤 timeoutBehavior 不校验——垃圾值静默落库**（fixed）
  真机：`{timeoutBehavior:"explode"}`（无 timeout）201 落库；ValidateForm 只在 timeout 非空时校验 behavior。今天无害、补上 timeout 即毒化该行。修复：behavior 非空必合法（domain/approval ValidateForm）。
- **AC-1 复定性：创建响应嵌套形是六版本实体的统一约定**（by-design 关闭）
  真机对照：fn/hd/ctl/apf 创建一律返 `{<entity>, version}`（双对象都需要：实体头+版本体）；workspace 无版本故扁平。前端按「版本实体 vs 平实体」两类解析即可，约定一致。
- **AC-2 复定性 → AC-PD-1**：创建同步阻塞 env 物化（冷 26s/暖 3-6s）——进 DECISIONS 待裁。

