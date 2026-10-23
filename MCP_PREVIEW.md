# Lore alpha.24: Windows desktop setup

Lore shares one local memory archive between connected desktop coding tools.
You do not need WSL, a Lore account, an API key for Lore, or a manual import of
supported local coding-agent histories.

## Install in one app

- **Claude Desktop:** Download
  [lore-memory-windows.mcpb](website/downloads/lore-memory-windows.mcpb). In
  Claude Desktop, open **Settings → Extensions → Advanced settings → Install
  Extension**, select the file, and review its computer-access prompt. Start a
  new chat.
- **Local Codex task in the ChatGPT desktop app:** Download
  [lore-mcp.exe](website/downloads/lore-mcp.exe). Add it as a local **STDIO**
  MCP server named `lore`, using the full downloaded path as its command.
  Restart the app and start a new Codex task. This does not attach Lore to a
  regular ChatGPT conversation.

In the connected app, ask:

> Connect Lore to all my coding apps. Tell me which connected and which need attention.

Review the settings change. Lore backs up each settings file before changing
it, reports apps it cannot connect, and copies its server to a stable local
path for newly connected clients. Restart newly connected apps. Codex may ask
you to trust Lore's prompt hook.

Then ask in a project chat:

> Read the relevant history through Lore, then continue.

For a specific question, include a project name, ticket ID, or a few keywords.
Lore cites project, conversation, and message IDs so you can inspect the
original source. Supported local Codex, Claude Code, and Qwen Code transcripts
are indexed on startup and refreshed before recall; the source app need not be
open. The archive is `%USERPROFILE%\.lore\archive.json`.

## Scope and help

Ordinary ChatGPT and Claude Desktop cloud conversations are not automatically
backfilled or continuously captured merely by adding this MCP. Claude Desktop
can read the local archive and save a deliberate checkpoint when asked. There
is no cross-device sync. Lore cannot verify external events from a chat report
alone. Its answers depend on the connected model using the tools correctly.

The Windows executable is unsigned. If Windows blocks it, keep Defender and
Smart App Control enabled and [report the exact warning](https://github.com/fayzkk889/lore/issues).
For a missing tool, restart the client and start a new chat. Privately
distributed Claude Desktop extensions must be installed again for updates.
Check the [SHA-256 checksums](website/downloads/checksums.txt),
[license](website/downloads/LICENSE.txt), and
[third-party notices](website/downloads/THIRD_PARTY_NOTICES.txt).
