---
id: DOC-037
type: how-to
status: active
owner: @weilin
created: 2026-06-12
reviewed: 2026-06-12
review-due: 2026-12-12
audience: [human, ai]
---

# 数据目录与跨机迁移

## 数据都在哪

一切在 `$FORGIFY_DATA_DIR`（默认 `~/.forgify`）：

| 路径 | 内容 |
|---|---|
| `forgify.db` | SQLite 全库（实体/版本/执行日志/消息/索引） |
| `workspaces/<ws>/` | 文件式存储：memories / blobs（SHA256 CAS）/ skills |
| `sandbox/` | 运行时（python/node/uv/llamasrv/embedmodel）+ 各 env——**纯派生缓存，可不迁** |
| `logs/forgify.log` | 轮转日志（10MB×3，gzip）——报障就发这个文件 |

## 备份

直接拷贝整个数据目录（建议先停 app，让 SQLite WAL checkpoint 干净落盘）。同一台机器上的恢复 = 拷回去，完整无损。

## 跨机迁移：三类密文需要重填

落盘加密密钥从**机器指纹**派生（防「拷库即解」，见 CR-20）——换机器后密钥不同，下列三类**密文**不可解，迁移后需在新机重新填写：

1. **API keys**（模型密钥）——重新录入 + `:test`
2. **Handler init-config**（init 参数，含密钥类）——重新 `PUT /handlers/{id}/config`
3. **MCP server 的 env/headers**——重新配置或重 import

**其余一切数据完整可用**：实体与版本、对话与消息、执行日志、workflow/flowrun、文档、记忆、技能、blob 附件——密文只覆盖上述三类配置。`sandbox/` 不必迁移，新机首用按需重装（directInstaller）。

> 完整 export/import（用户口令重加密密文）在 roadmap，未排期。
