# Lore alpha.24: Windows desktop setup

The current instructions and downloads are at [loredev.co/docs](https://loredev.co/docs). Lore is a free, closed-source local MCP memory tool for Windows. This release does not need WSL, a Lore account, a Lore API key, or a manual import of supported local coding-agent histories.

## Install in one app

- **Claude Desktop:** [Download the Lore extension (.mcpb)](https://loredev.co/downloads/lore-memory-windows.mcpb). In Claude Desktop, open **Settings → Extensions → Advanced settings → Install Extension**, select the file, review the computer-access warning, and start a new chat.
- **Local Codex task in ChatGPT desktop:** [Download the Windows MCP server (.exe)](https://loredev.co/downloads/lore-mcp.exe). Add it as a local **STDIO MCP server** named `lore`, using its full path as the command. Restart the app and start a new Codex task. This does not attach to regular ChatGPT chat.

Ask your connected app: **“Connect Lore to all my coding apps. Tell me which connected and which need attention.”** Review the settings changes and restart newly connected apps. Then, in a project chat, ask: **“Read the relevant history through Lore, then continue.”**

Lore indexes accessible local Codex, Claude Code, and Qwen Code transcripts and cites source messages. The source app need not be open. Ordinary ChatGPT and Claude Desktop cloud chats are not automatically backfilled or continuously captured by an MCP connection. Claude Desktop can read Lore's existing archive and save a deliberate checkpoint when asked. Memory does not sync across PCs.

The Windows binary is unsigned; Windows may warn or block it. Keep protections enabled and [report the exact warning](https://github.com/fayzkk889/lore/issues). Check the [SHA-256 checksums](https://loredev.co/downloads/checksums.txt), [license](https://loredev.co/downloads/LICENSE.txt), and [third-party notices](https://loredev.co/downloads/THIRD_PARTY_NOTICES.txt).
