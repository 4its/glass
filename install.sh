#!/bin/sh
set -e

REPO="https://github.com/xakepp35/glass"
BIN="glass"
INSTALL_DIR="/usr/local/bin"

# 1. Check dependencies
need() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "❌ Required tool '$1' not found in PATH"
    exit 1
  fi
}
need git
need go

# 2. Create temp dir (auto-cleaned on exit)
TMPDIR="$(mktemp -d)"
cleanup() { rm -rf "$TMPDIR"; }
trap cleanup EXIT

echo "📥 Cloning $REPO..."
git clone --depth 1 "$REPO" "$TMPDIR"

cd "$TMPDIR"

echo "⚙️ Building binary..."
go build -ldflags="-s -w" -trimpath -o "$BIN" .

# 3. Install
if [ ! -w "$INSTALL_DIR" ]; then
  echo "🔑 Installing to $INSTALL_DIR requires sudo"
  sudo install -m 755 "$BIN" "$INSTALL_DIR/$BIN"
else
  install -m 755 "$BIN" "$INSTALL_DIR/$BIN"
fi

echo "✅ Installed: $(command -v $BIN)"
echo "👉 Try: $BIN -h"
