#!/usr/bin/env bash
# Downloads uv binary + python-build-standalone tarball for the CURRENT
# platform into FORGIFY_DEV_RESOURCES (default: ~/.forgify-dev-resources),
# named so internal/infra/sandbox.Bootstrap can find them:
#
#   uv-<platform>             ← bundled uv binary (no .tar.gz wrapper)
#   python-<platform>.tar.gz  ← python-build-standalone install_only tarball
#
# 下载当前平台的 uv 二进制 + python-build-standalone tarball 到
# FORGIFY_DEV_RESOURCES（默认 ~/.forgify-dev-resources），按
# internal/infra/sandbox.Bootstrap 期望的文件名命名。
#
# Pin versions via env vars (defaults: latest GitHub release):
#   UV_VERSION       e.g. 0.5.0
#   PBS_TAG          e.g. 20251014  (python-build-standalone release tag)
#   PYTHON_VERSION   e.g. 3.12      (matched as prefix against PBS assets)
#   FORGIFY_DEV_RESOURCES
#                    e.g. /opt/forgify/dev-resources
#
# 通过环境变量固定版本（默认拉 GitHub latest release）。

set -euo pipefail

# Pinned to match devbox.lock's uv@0.11 and the PBS release that ships
# the matching cpython-3.12 build. Bump in lockstep — see CLAUDE.md
# §"环境版本钉" for the upgrade procedure.
#
# 与 devbox.lock 的 uv@0.11 锁版本对齐 + 同步 cpython-3.12 的 PBS release。
# 升级时同步改——见 CLAUDE.md §"环境版本钉" 升级流程。
UV_VERSION="${UV_VERSION:-0.11.8}"
PBS_TAG="${PBS_TAG:-20260414}"
PYTHON_VERSION="${PYTHON_VERSION:-3.12}"
RESOURCES_DIR="${FORGIFY_DEV_RESOURCES:-$HOME/.forgify-dev-resources}"

# Map (uname -s, uname -m) to (sandbox-platform-key, upstream-release-naming).
# sandbox.platformKey() returns "<goos>-<goarch>"; upstream uv + PBS
# releases use Rust-style triplets like "aarch64-apple-darwin".
#
# 把 (uname -s, uname -m) 映射到 (sandbox 平台键, 上游 release 命名)。
# sandbox.platformKey() 返 "<goos>-<goarch>"；上游 uv + PBS release 用
# Rust 风格的 triplet 如 "aarch64-apple-darwin"。
case "$(uname -s)-$(uname -m)" in
	Darwin-arm64)  PLATFORM=darwin-arm64;  UPSTREAM=aarch64-apple-darwin ;;
	Darwin-x86_64) PLATFORM=darwin-amd64;  UPSTREAM=x86_64-apple-darwin ;;
	Linux-x86_64)  PLATFORM=linux-amd64;   UPSTREAM=x86_64-unknown-linux-gnu ;;
	Linux-aarch64) PLATFORM=linux-arm64;   UPSTREAM=aarch64-unknown-linux-gnu ;;
	*)
		echo "unsupported platform: $(uname -s)-$(uname -m)" >&2
		exit 1
	;;
esac

# Resolve latest tag for unpinned versions. Plain grep on the JSON is
# enough — we only need tag_name, no need for jq.
#
# 没固定版本时拿 latest tag。grep JSON 就够——只要 tag_name，不必 jq。
github_latest_tag() {
	curl -fsSL "https://api.github.com/repos/$1/releases/latest" \
		| grep '"tag_name"' | head -1 | cut -d'"' -f4
}

if [ -z "$UV_VERSION" ]; then
	echo "→ resolving latest uv version ..."
	UV_VERSION=$(github_latest_tag "astral-sh/uv")
fi
if [ -z "$PBS_TAG" ]; then
	echo "→ resolving latest python-build-standalone release ..."
	PBS_TAG=$(github_latest_tag "astral-sh/python-build-standalone")
fi

mkdir -p "$RESOURCES_DIR"

# ── uv ───────────────────────────────────────────────────────────────────────
echo "→ uv $UV_VERSION ($UPSTREAM) → $RESOURCES_DIR/uv-$PLATFORM"
TMP_UV=$(mktemp -d)
trap 'rm -rf "$TMP_UV"' EXIT
curl -fsSL "https://github.com/astral-sh/uv/releases/download/$UV_VERSION/uv-$UPSTREAM.tar.gz" \
	| tar -xz -C "$TMP_UV"
mv "$TMP_UV/uv-$UPSTREAM/uv" "$RESOURCES_DIR/uv-$PLATFORM"
chmod +x "$RESOURCES_DIR/uv-$PLATFORM"

# ── python-build-standalone ──────────────────────────────────────────────────
# Asset naming: cpython-<X.Y.Z>+<PBS_TAG>-<UPSTREAM>-install_only.tar.gz
# We match by Python prefix (e.g. "3.12") so the actual patch version is
# whatever the release shipped — caller doesn't need to track patches.
#
# Asset 命名见上方英文。按 Python 主版本前缀（如 "3.12"）匹配 patch 版本，
# 让调用方不必跟踪 patch 号。
echo "→ python-build-standalone $PBS_TAG (cpython-$PYTHON_VERSION-$UPSTREAM)"
ASSET_URL=$(curl -fsSL "https://api.github.com/repos/astral-sh/python-build-standalone/releases/tags/$PBS_TAG" \
	| grep -o "https://[^\"]*cpython-${PYTHON_VERSION}\.[^\"]*-${UPSTREAM}-install_only\.tar\.gz" \
	| head -1)
if [ -z "$ASSET_URL" ]; then
	echo "no matching asset in PBS release $PBS_TAG for cpython-${PYTHON_VERSION}.* and $UPSTREAM" >&2
	echo "browse https://github.com/astral-sh/python-build-standalone/releases/tag/$PBS_TAG" >&2
	exit 1
fi
curl -fsSL "$ASSET_URL" -o "$RESOURCES_DIR/python-$PLATFORM.tar.gz"

echo
echo "✓ resources ready: $RESOURCES_DIR"
echo "  uv:     $RESOURCES_DIR/uv-$PLATFORM"
echo "  python: $RESOURCES_DIR/python-$PLATFORM.tar.gz"
echo
echo "  Add to your shell:  export FORGIFY_DEV_RESOURCES=$RESOURCES_DIR"
