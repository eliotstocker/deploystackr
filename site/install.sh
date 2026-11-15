#!/bin/sh
set -e

# DeployStackr Install Script
# https://deploystac.kr

REPO="eliotstocker/deploystackr"
INSTALL_DIR="/usr/local/bin"
BIN_NAME="deploystackr"

echo "🐬 Installing DeployStackr..."

# Detect OS & Architecture
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"

case "$ARCH" in
  x86_64|amd64)
    ARCH="amd64"
    ;;
  arm64|aarch64)
    ARCH="arm64"
    ;;
  *)
    echo "Unsupported architecture: $ARCH"
    exit 1
    ;;
esac

case "$OS" in
  darwin|linux)
    ;;
  *)
    echo "Unsupported operating system: $OS"
    exit 1
    ;;
esac

LATEST_TAG=$(curl -s "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/' || true)

if [ -z "$LATEST_TAG" ]; then
  LATEST_TAG="v0.1.0"
fi

DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${LATEST_TAG}/deploystackr_${OS}_${ARCH}.tar.gz"

echo "Fetching ${BIN_NAME} ${LATEST_TAG} for ${OS}/${ARCH}..."

TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT

if curl -sSLf "$DOWNLOAD_URL" -o "$TMP_DIR/deploystackr.tar.gz" 2>/dev/null; then
  tar -xzf "$TMP_DIR/deploystackr.tar.gz" -C "$TMP_DIR"
  TARGET="$INSTALL_DIR/$BIN_NAME"
  if [ -w "$INSTALL_DIR" ]; then
    mv "$TMP_DIR/$BIN_NAME" "$TARGET"
  else
    echo "Sudo permissions required to install to $INSTALL_DIR"
    sudo mv "$TMP_DIR/$BIN_NAME" "$TARGET"
  fi
  chmod +x "$TARGET"
  echo "✅ Successfully installed DeployStackr to $TARGET"
  echo ""
  echo "Run 'deploystackr -h' to get started!"
else
  echo "Pre-built binary not found. You can run directly via Go or Docker:"
  echo "  go install github.com/${REPO}/cmd/server@latest"
fi
