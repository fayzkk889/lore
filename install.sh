#!/bin/bash
set -e

VERSION="${LORE_VERSION:-0.10.0-alpha.4}"
REPO="fayzkk889/lore"
INSTALL_DIR="${LORE_INSTALL_DIR:-/usr/local/bin}"
INSTALL_TMP_DIR=$(mktemp -d "${TMPDIR:-/tmp}/lore-install.XXXXXX")
trap 'rm -rf "$INSTALL_TMP_DIR"' EXIT

# Detect OS and architecture
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case "$ARCH" in
    x86_64) ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

case "$OS" in
    linux|darwin) ;;
    *) echo "Unsupported OS: $OS. Use the Windows installer instead."; exit 1 ;;
esac

FILENAME="lore_${VERSION}_${OS}_${ARCH}.tar.gz"
URL="https://github.com/${REPO}/releases/download/v${VERSION}/${FILENAME}"
CHECKSUMS_URL="https://github.com/${REPO}/releases/download/v${VERSION}/checksums.txt"

echo "Downloading Lore ${VERSION} for ${OS}/${ARCH}..."
curl -fsSL "$URL" -o "${INSTALL_TMP_DIR}/${FILENAME}"
curl -fsSL "$CHECKSUMS_URL" -o "${INSTALL_TMP_DIR}/checksums.txt"

echo "Verifying checksum..."
EXPECTED=$(grep "  ${FILENAME}$" "${INSTALL_TMP_DIR}/checksums.txt" | awk '{print $1}')
if [ -z "$EXPECTED" ]; then
    echo "Checksum for ${FILENAME} not found"; exit 1
fi
if command -v sha256sum >/dev/null 2>&1; then
    ACTUAL=$(sha256sum "${INSTALL_TMP_DIR}/${FILENAME}" | awk '{print $1}')
else
    ACTUAL=$(shasum -a 256 "${INSTALL_TMP_DIR}/${FILENAME}" | awk '{print $1}')
fi
if [ "$ACTUAL" != "$EXPECTED" ]; then
    echo "Checksum verification failed"; exit 1
fi

echo "Extracting..."
tar -xzf "${INSTALL_TMP_DIR}/${FILENAME}" -C "$INSTALL_TMP_DIR"

echo "Installing to ${INSTALL_DIR}..."
if [ "$INSTALL_DIR" = "/usr/local/bin" ]; then
    sudo install -m 0755 "${INSTALL_TMP_DIR}/lore" "${INSTALL_DIR}/lore"
else
    mkdir -p "$INSTALL_DIR"
    install -m 0755 "${INSTALL_TMP_DIR}/lore" "${INSTALL_DIR}/lore"
fi

echo ""
echo "Lore ${VERSION} installed successfully!"
echo "Connect memory with: ${INSTALL_DIR}/lore connect"
echo "No API key is needed for memory. Lore's original coding agent is configured separately."
