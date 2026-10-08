# Lore

Lore is local project memory for coding agents. It helps a fresh chat recover
relevant decisions, constraints and unfinished work from accessible earlier
conversations, with references to the source messages. You ask in your agent;
Lore runs as an MCP server on your computer. Ordinary memory operations do not
need a Lore account or hosted memory service.

## Current developer preview

**0.10.0-alpha.9-mcp** is a free, closed-source MCP-only preview for **Codex on
Windows through Ubuntu WSL 2**. It does not include Lore's original standalone
coding agent, a new native Windows executable, or the browser extension. The
Windows/WSL/Codex flow was tested end to end on one PC with Smart App Control
enabled. Other MCP clients have not yet been validated with this package.

Download [lore-mcp-linux](website/downloads/lore-mcp-linux) and
[install-codex.ps1](website/downloads/install-codex.ps1) into the same folder,
then follow the [setup guide](MCP_PREVIEW.md). The
[checksums](website/downloads/checksums.txt),
[license](website/downloads/LICENSE.txt) and
[third-party notices](website/downloads/THIRD_PARTY_NOTICES.txt) accompany the
download. The same static site files are in [website/](website/) for manual
Cloudflare Pages upload.

Once installed, start a **new** Codex task in your project and say:

```text
Read the relevant history, then continue.
```

The installer adds managed Codex guidance for that plain request. You can name
Lore explicitly when needed. Lore discovers accessible local Codex, Claude
Code and Qwen Code session files, but it cannot automatically read cloud-only
ChatGPT or Claude account history. Retrieved chat statements are evidence to
inspect, not proof that an external action happened.

Earlier alpha.7 downloads and browser-extension documentation remain in
[legacy documentation](LEGACY_ALPHA7.md); those are outside this MCP-only
preview. Source from earlier MIT-licensed releases retains its original terms.
The current binary is governed by its bundled Free Binary License.
