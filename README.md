# Lore

Lore is private, local project memory for desktop coding agents. In a new chat,
ask for relevant history; Lore retrieves cited messages from accessible local
Codex, Claude Code, and Qwen Code conversations. The same archive can be read
from connected Codex and Claude Desktop sessions on one Windows PC.

## Windows alpha.24

This release is free and closed source. There is no Lore account, hosted memory
server, WSL requirement, or shell-script installer.

1. Start with the app you already use: install the
   [Claude Desktop extension](website/downloads/lore-memory-windows.mcpb), or
   add the [Windows MCP server](website/downloads/lore-mcp.exe) to a **local
   Codex task** as a STDIO server named `lore`.
2. In that app, ask: **“Connect Lore to all my coding apps. Tell me which
   connected and which need attention.”** Review the settings change, then
   restart newly connected apps.
3. Ask: **“Read the relevant history through Lore, then continue.”** Check the
   cited source messages before relying on an important answer.

Read the [Windows setup guide](MCP_PREVIEW.md) or use the
[website setup page](website/docs.html). Download
[SHA-256 checksums](website/downloads/checksums.txt), the
[binary license](website/downloads/LICENSE.txt), and
[third-party notices](website/downloads/THIRD_PARTY_NOTICES.txt).

Automatic backfill covers accessible **local coding-agent** transcripts. A
Claude Desktop extension can retrieve this shared archive and save deliberate
checkpoints, but installing it does not expose ordinary Claude or ChatGPT cloud
chat history. Regular ChatGPT chat does not run this local MCP connection.
Memory stays on this PC; no cross-device sync is included. Retrieved chat
statements are evidence, not proof that an external action happened.

Alpha.24 passed automated fresh-profile tests and live recall checks in Claude
Desktop and Codex on one Windows PC. A clean-PC install is not yet verified,
and the binary is unsigned, so Windows may warn or block it. Keep Windows
protections enabled and [report the exact issue](https://github.com/fayzkk889/lore/issues)
with private content redacted.

The older standalone agent and WSL installers remain in the repository for
historical reference; they are **not** this release. Earlier MIT-licensed
source retains its original terms. The current executable is governed by its
bundled Free Binary License.
