#!/bin/sh
set -eu

PURGE_DATA=false
if [ "${1:-}" = "--purge-data" ]; then
    PURGE_DATA=true
elif [ -n "${1:-}" ]; then
    echo "Usage: sh uninstall.sh [--purge-data]" >&2
    exit 2
fi

if command -v lore >/dev/null 2>&1; then
    lore disconnect --all
fi

if [ -e /usr/local/bin/lore ]; then
    sudo rm -f /usr/local/bin/lore
fi

if [ "$PURGE_DATA" = true ]; then
    rm -rf -- "$HOME/.lore"
    echo "Lore and local Lore data were removed."
else
    echo "Lore was removed. Local memory remains in ~/.lore."
fi
