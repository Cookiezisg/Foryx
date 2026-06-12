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
