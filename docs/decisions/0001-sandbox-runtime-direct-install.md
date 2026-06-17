---
id: DOC-002
type: decision
status: active
owner: @weilin
created: 2026-06-11
reviewed: 2026-06-11
review-due: 2099-12-31
audience: [human, ai]
---

# 0001 — 沙箱运行时改自研直装，弃用 mise 内嵌

## 背景

sandbox 给 Function / Handler / MCP 提供隔离运行时。原实现内嵌 [`jdx/mise`](https://mise.jdx.dev/) 二进制（按平台 `go:embed`），启动时抽出，运行时调 `mise install <kind>@<ver>` 按需装运行时。

问题：**mise 在本项目里只干一件事——下载四个运行时**（python / node / uv / dotnet）。其余（venv / `npm install` / `uv pip` 的依赖管理、uvx/npx 解析）全是 Anselm 自己的 `EnvManager`，不经 mise。为这点活内嵌一个 **76MB** 的 mise，把 server 二进制从 ~30MB 撑到 **103MB**；且 `go:embed` 要求"目标平台二进制在场"，逼着跨平台 release 前必须 `cmd/setup --all` 预拉全 5 平台 mise——一整套 embed 装配舞蹈。

## 决策

**删除 mise 内嵌，自研 `directInstaller`**（`internal/infra/sandbox/direct.go`）：实现既有 `RuntimeInstaller` 接口（`MiseInstaller` 只是它的旧实现之一，接口边界让替换零外溢），按 recipe 直接从上游分发渠道**流式下载钉死版本的 tarball/zip → 校验 → 全树解压（剥 wrapper）**到 `<sandboxRoot>/runtimes/<kind>/<version>/`，布局与 mise 产出一致，消费方（`EnvManager`）零改。

四条 recipe（版本钉死 = 可复现，一如 uv/dotnet 一向如此；升级 = 改一行）：

| 运行时 | 渠道 | 校验和 | 解压 |
|---|---|---|---|
| python | astral python-build-standalone `install_only`（pbs tag `20260610`；3.11/3.12/3.13） | 每 release `SHA256SUMS` 清单 | 剥 `python/` |
| node | nodejs.org/dist（pin `v22.22.3`） | `SHASUMS256.txt` | 剥 `node-vX/` |
| uv | astral-sh/uv releases（`0.11.4`；release tag 即版本，任意版本套模板） | `.sha256` sidecar | 剥 `uv-<triple>/` |
| dotnet | builds.dotnet.microsoft.com（`10.0.300`） | `.sha512` sidecar | 扁平 |

全是 tar.gz + zip，Go 标准库（`archive/tar`+`compress/gzip`+`archive/zip`）全覆盖，无需 zstd/xz。macOS 解压后 `xattr -cr` 抹 quarantine/provenance 即可运行——上游二进制已由发布方/构建期签名，无需重签（真机验证 python/node/uv 在 darwin-arm64 直接 `--version` 跑通）。

## 取舍

**为何不选：**
- **保留 mise**：76MB 体积 + embed 跨平台舞蹈不可接受，而它在此只是个 tarball 下载器。
- **运行时本体也 `go:embed`**：4 × 5 平台 × 数十 MB = 二进制爆炸，且无离线收益（首用仍需联网下别的）。
- **动态版本解析**（GitHub API / `index.json` 取 latest）：放弃，改钉死——可复现、零元数据往返、无 GitHub 限流。仅 python 需拉一次 pbs `SHA256SUMS`（非 API、不限流）。
- **失去 mise 的广度**：以后加 ruby/php 各需一条 recipe。但现用且只用这 4 个、还钉死版本，YAGNI。

## 后果

- server 二进制 **103MB → 30MB**（省 73MB / 71%）；5 平台 release 省 ~380MB。
- **跨平台编译回到 plain `GOOS=x GOARCH=y go build`**——无 embed、无预拉、无 `cmd/setup`（已删）；运行时在目标机首用按需下，故 `go build` 可直接交叉编译、无平台依赖。
- 删除：`mise.go` + `codesign.go` + 6 个 `embed_mise_*.go` + 73MB 内嵌二进制 + `cmd/setup` + `MiseBin()` + bootstrap 的 mise 抽取（`Bootstrap` 简化为建 sandbox 根目录）。
- 净增 `direct.go`（含全树解压器 + 4 recipe）+ `direct_test.go`（离线 recipe 单测）+ `install_e2e_test.go`（`-tags e2e` 真装验证）。
- 离线无回归：mise embed 本就不省首用联网（运行时一向按需下）。
- 维护点：4 条 recipe 的版本/渠道随上游变需手动 bump（这些渠道极稳定）。
