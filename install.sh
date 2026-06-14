#!/bin/bash
set -e

VERSION="0.9.0-beta"
REPO="fayzkk889/lore"

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
curl -fsSL "$URL" -o "/tmp/${FILENAME}"
curl -fsSL "$CHECKSUMS_URL" -o "/tmp/lore-checksums.txt"

echo "Verifying checksum..."
EXPECTED=$(grep "  ${FILENAME}$" "/tmp/lore-checksums.txt" | awk '{print $1}')
if [ -z "$EXPECTED" ]; then
    echo "Checksum for ${FILENAME} not found"; exit 1
fi
if command -v sha256sum >/dev/null 2>&1; then
    ACTUAL=$(sha256sum "/tmp/${FILENAME}" | awk '{print $1}')
else
    ACTUAL=$(shasum -a 256 "/tmp/${FILENAME}" | awk '{print $1}')
fi
if [ "$ACTUAL" != "$EXPECTED" ]; then
    echo "Checksum verification failed"; exit 1
fi

echo "Extracting..."
tar -xzf "/tmp/${FILENAME}" -C /tmp

echo "Installing to /usr/local/bin..."
sudo mv /tmp/lore /usr/local/bin/lore
chmod +x /usr/local/bin/lore
rm "/tmp/${FILENAME}"

echo ""
echo "Lore ${VERSION} installed successfully!"
echo "Set your provider API key (e.g. export OPENROUTER_API_KEY=...) then run 'lore'."
