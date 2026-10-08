# Lore MCP-only developer preview

This preview is free, closed-source local project memory for Codex on Windows
through Ubuntu WSL 2. It ships a Linux MCP server, not a new unsigned Windows
Lore executable. This avoids the Smart App Control block seen on our test PC,
but installing WSL is a real prerequisite.

## Set up Codex

1. Complete [Microsoft's Ubuntu WSL installation](https://learn.microsoft.com/en-us/windows/wsl/install)
   and launch Ubuntu once. Ensure `codex` is available in PowerShell.
2. Download [lore-mcp-linux](website/downloads/lore-mcp-linux) and
   [install-codex.ps1](website/downloads/install-codex.ps1) into the same folder.
3. Open PowerShell in that folder and run `.\install-codex.ps1`. If the WSL
   distribution has another name, use `.\install-codex.ps1 -Distro YourName`.
4. Open a **new** Codex task in your project and ask:
   `Read the relevant history, then continue.`

The installer verifies the binary, backs up existing Codex configuration and
instructions, and adds a managed instruction so a plain history request calls
Lore. It preserves the local archive at `%USERPROFILE%\.lore\archive.json`.
If PowerShell blocks a downloaded script, inspect it and use manual setup
instead of disabling Windows protection.

See the [SHA-256 checksums](website/downloads/checksums.txt),
[Free Binary License](website/downloads/LICENSE.txt), and
[third-party notices](website/downloads/THIRD_PARTY_NOTICES.txt).

## Scope

Codex on Windows with Ubuntu WSL 2 was tested end to end. The same local MCP
server may be configurable in other MCP clients, but they have not been
validated with this package. Lore can discover accessible local Codex, Claude
Code, and Qwen Code session files. Cloud-only browser chats still need capture
or import; the older browser extension is not included here. Lore returns
candidate evidence with source references, not independent proof of actions
outside the chat. There is no hosted Lore memory service or cross-device sync.
