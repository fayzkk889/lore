# Lore

Lore is local project memory for desktop coding agents. In a new project chat, ask for relevant history; Lore retrieves cited messages from accessible local Codex, Claude Code, and Qwen Code conversations. Connected Codex and Claude Desktop sessions can use the same archive on one Windows PC.

## Windows alpha.24

This release is free and closed source. There is no Lore account, hosted memory service, WSL requirement, or shell-script installer.

1. Start in the app you already use: [install the Claude Desktop extension](https://loredev.co/downloads/lore-memory-windows.mcpb), or [add the Windows MCP server](https://loredev.co/downloads/lore-mcp.exe) to a **local Codex task** as a STDIO server named `lore`.
2. Ask: **“Connect Lore to all my coding apps. Tell me which connected and which need attention.”** Review the settings changes and restart newly connected apps.
3. In a project chat, ask: **“Read the relevant history through Lore, then continue.”** Check cited source messages before relying on an important answer.

See the [setup guide](https://loredev.co/docs), [SHA-256 checksums](https://loredev.co/downloads/checksums.txt), [binary license](https://loredev.co/downloads/LICENSE.txt), and [third-party notices](https://loredev.co/downloads/THIRD_PARTY_NOTICES.txt).

Automatic backfill covers accessible **local coding-agent** transcripts. A Claude Desktop extension can retrieve the shared archive and save deliberate checkpoints, but installing it does not expose ordinary Claude or ChatGPT cloud chat history. Regular ChatGPT chat does not run this local MCP connection. There is no cross-device sync. Retrieved chat statements are evidence, not proof that an external action happened.

Alpha.24 passed fresh-profile tests and live recall checks in Claude Desktop and Codex on one Windows PC. A clean-PC install remains unverified. The binary is unsigned, so Windows may warn or block it; keep protections enabled and [report the exact issue](https://github.com/fayzkk889/lore/issues) with private content redacted.

Older standalone-agent and WSL files in this repository are historical, not this release. Earlier MIT-licensed source retains its original terms. The current executable uses its bundled Free Binary License.
