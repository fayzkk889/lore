#!/bin/sh
set -eu

PURGE_DATA=false
SKIP_DISCONNECT=false
INSTALL_DIR="${LORE_INSTALL_DIR:-/usr/local/bin}"
DATA_DIR="${LORE_DATA_DIR:-$HOME/.lore}"
for argument in "$@"; do
    case "$argument" in
        --purge-data) PURGE_DATA=true ;;
        --skip-disconnect) SKIP_DISCONNECT=true ;;
        *) echo "Usage: sh uninstall.sh [--purge-data] [--skip-disconnect]" >&2; exit 2 ;;
    esac
done

if [ "$SKIP_DISCONNECT" = false ] && [ -x "$INSTALL_DIR/lore" ]; then
    if ! "$INSTALL_DIR/lore" disconnect --all; then
        echo "Warning: Lore could not disconnect every client. The binary will still be removed; inspect client MCP settings if a stale Lore entry remains." >&2
    fi
fi

if [ -e "$INSTALL_DIR/lore" ]; then
    if [ "$INSTALL_DIR" = "/usr/local/bin" ]; then
        sudo rm -f "$INSTALL_DIR/lore"
    else
        rm -f "$INSTALL_DIR/lore"
    fi
fi
rmdir "$INSTALL_DIR" 2>/dev/null || true

if [ "$PURGE_DATA" = true ]; then
    rm -rf -- "$DATA_DIR"
    echo "Lore and local Lore data were removed."
else
    echo "Lore was removed. Local memory remains in ~/.lore."
fi
