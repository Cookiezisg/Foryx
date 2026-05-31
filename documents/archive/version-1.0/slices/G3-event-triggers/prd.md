# G3 · 事件触发 — 产品需求文档

**切片**：G3  
**状态**：待 Review

---

## 1. 背景

除定时触发外，工作流还支持两种事件触发：
- **文件监听**（`trigger_file`）：监控本地目录，有新文件时触发
- **Webhook**（`trigger_webhook`）：监听本地 HTTP 端口，收到 POST 请求时触发

---

## 2. 文件触发

### 配置

| 配置项 | 说明 |
|---|---|
| `path` | 监控的目录路径 |
| `pattern` | 文件名 glob 模式，如 `*.xlsx` |
| `event` | 触发事件：`created`（默认）/ `modified` / `deleted` |

### 行为

- 监控使用操作系统文件系统事件（macOS FSEvents）
- 新文件匹配 pattern 时，将文件路径作为 `trigger_input.file_path` 传入工作流
- 同一文件 5 秒内只触发一次（防抖）

---

## 3. Webhook 触发

### 配置

| 配置项 | 说明 |
|---|---|
| `port` | 监听端口（默认 8080） |
| `path` | URL 路径（如 `/trigger/my-workflow`） |
| `secret` | 验签密钥（可选），用于验证来源 |

### 行为

- 工作流部署时，在指定端口+路径启动 HTTP 监听
- 收到 POST 请求时，将请求体（JSON）作为 `trigger_input` 传入工作流
- 响应：立即返回 `{"status":"accepted","runId":"..."}` 不等待工作流完成

### 安全

- 配置了 `secret` 时，验证 `X-Webhook-Secret` 请求头
- 不匹配则返回 401

---

## 4. 触发输入注入

两种触发方式都将触发数据注入 `trigger_input` 变量，工作流中可通过 `{{trigger_input.file_path}}` 或 `{{trigger_input.body.field}}` 引用。

---

## 5. 验收测试

```
1. 文件触发：部署工作流 → 向监控目录放入匹配文件 → 工作流自动运行
2. 文件触发：5 秒内放入同名文件两次 → 只触发一次
3. Webhook：部署 → curl POST → 工作流运行，返回 runId
4. Webhook：错误 secret → 返回 401
5. 工作流暂停 → 文件监听和 Webhook 停止
```
