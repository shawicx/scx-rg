#!/bin/sh
# scx-rg 一键安装：从 GitHub Releases 下载对应平台的压缩包并校验安装。
# 用法：
#   curl -fsSL https://raw.githubusercontent.com/shawricx/scx-rg/main/scripts/install.sh | sh
#   curl -fsSL ... | sh -s -- --bin ~/.local/bin   # 自定义安装目录
set -eu

REPO="shawricx/scx-rg"
BIN_DIR="/usr/local/bin"

while [ $# -gt 0 ]; do
  case "$1" in
    --bin) BIN_DIR="$2"; shift 2 ;;
    *) echo "未知参数: $1（支持 --bin <目录>）" >&2; exit 1 ;;
  esac
done

# ---------- 平台探测 ----------
OS=$(uname -s)
ARCH=$(uname -m)
case "$OS" in
  Darwin) GOOS=darwin ;;
  Linux)  GOOS=linux ;;
  *) echo "不支持的操作系统: $OS（支持 darwin/linux）" >&2; exit 1 ;;
esac
case "$ARCH" in
  x86_64|amd64) GOARCH=amd64 ;;
  aarch64|arm64) GOARCH=arm64 ;;
  *) echo "不支持的架构: $ARCH（支持 amd64/arm64）" >&2; exit 1 ;;
esac

# ---------- 依赖检查 ----------
need() {
  command -v "$1" >/dev/null 2>&1 || { echo "缺少依赖: $1" >&2; exit 1; }
}
need curl
need tar

# ---------- 取最新版本 ----------
echo "查询 $REPO 最新版本…"
TAG=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" | grep -o '"tag_name": *"[^"]*"' | head -1 | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')
if [ -z "$TAG" ]; then
  echo "获取最新版本失败（网络或 GitHub API 限流）" >&2
  exit 1
fi
VER=${TAG#v}
ARCHIVE="scx-rg_${VER}_${GOOS}_${GOARCH}.tar.gz"
CHECKSUMS="scx-rg_${VER}_checksums.txt"
BASE="https://github.com/$REPO/releases/download/$TAG"
echo "最新版本: $TAG（$GOOS/$GOARCH）"

# ---------- 下载与校验 ----------
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT
echo "下载 $ARCHIVE …"
curl -fSL -o "$TMP/$ARCHIVE" "$BASE/$ARCHIVE"
curl -fsSL -o "$TMP/$CHECKSUMS" "$BASE/$CHECKSUMS" || echo "⚠ 校验和文件缺失，跳过校验" >&2

SHA_CMD=""
if command -v shasum >/dev/null 2>&1; then
  SHA_CMD="shasum -a 256"
elif command -v sha256sum >/dev/null 2>&1; then
  SHA_CMD="sha256sum"
fi
if [ -n "$SHA_CMD" ] && [ -s "$TMP/$CHECKSUMS" ]; then
  # 只校验本次下载的压缩包（--ignore-macos 下 sha256sum 无此参数，改用 grep 过滤）
  (cd "$TMP" && grep " $ARCHIVE\$" "$CHECKSUMS" > .checksum && $SHA_CMD -c .checksum) \
    || { echo "校验和不匹配，终止安装" >&2; exit 1; }
  echo "校验通过"
fi

# ---------- 安装 ----------
tar -xzf "$TMP/$ARCHIVE" -C "$TMP"
mkdir -p "$BIN_DIR"
if [ -w "$BIN_DIR" ]; then
  mv "$TMP/scx-rg" "$BIN_DIR/scx-rg"
else
  echo "$BIN_DIR 不可写，尝试 sudo …（或用 --bin ~/.local/bin 自定义目录）"
  sudo mv "$TMP/scx-rg" "$BIN_DIR/scx-rg"
fi
chmod +x "$BIN_DIR/scx-rg"

case ":$PATH:" in
  *":$BIN_DIR:"*) ;;
  *) echo "提示: $BIN_DIR 不在 PATH 中，请将其加入 shell 配置" >&2 ;;
esac

if [ "$GOOS" = darwin ] && [ "$(command -v xattr)" != "" ]; then
  echo "macOS 若被 Gatekeeper 拦截: xattr -d com.apple.quarantine $BIN_DIR/scx-rg"
fi

"$BIN_DIR/scx-rg" --version
echo "安装完成: $BIN_DIR/scx-rg"
