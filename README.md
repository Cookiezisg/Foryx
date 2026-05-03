# Forgify

本地优先的 Agentic Workflow Platform — 目标 Wails 桌面 app。

## 快速开始

```bash
git clone <repo>
cd Forgify
make environment       # 首次：装 devbox + Nix + Go 工具 + 沙箱资源（Nix 安装时要 sudo 一次）
make test-console      # 起 dev server + 自动开浏览器，Ctrl+C 停
```

**就这两步**。`make` 会自动进 devbox shell，你不用手动切换。

## 五个核心命令

```bash
make environment    # 首次环境装配（一次性）
make test-console   # 起 dev server + 浏览器，live reload
make test-unit      # 单测套件
make test-pipeline  # 真依赖 / e2e 套件（需 .env 里 DEEPSEEK_API_KEY）
make stop           # 杀绑在 8742 端口的进程
```

每个 `test-*` 命令都自动进 devbox 环境跑——不用 `devbox shell`。

## 环境一致性保证

所有版本通过这几个文件钉死，新电脑跑出来字节级一致：

| 文件 | 钉的内容 |
|---|---|
| `devbox.lock` | Nix 包的精确 commit（go 1.25.9 / python 3.12.13 / uv 0.11.8 / make 4.4.1）|
| `scripts/download-sandbox-resources.sh` | PBS bundle 版本（uv 0.11.8 / cpython 3.12.13）|
| `devbox.json` | go install 工具版本（air / staticcheck / deadcode）|
| `backend/go.mod` | Go 依赖（zap 1.28 / modernc.org/sqlite 1.50 等）|

**升级版本**：改对应文件，跑一次 `devbox install` 重新生成 lock，提交。

## 文档

- 项目愿景 / 架构 / Phase 路线：[`documents/version-1.2/backend-design.md`](documents/version-1.2/backend-design.md)
- 当前进展 / 决策日志：[`documents/version-1.2/progress-record.md`](documents/version-1.2/progress-record.md)
- 工程纪律：[`CLAUDE.md`](CLAUDE.md)
