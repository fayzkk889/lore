# Lore MCP developer preview

Lore gives Codex and Claude Code one local, cited memory of accessible coding
chats. It is free and closed source in this preview. The supported setup is
Windows with Ubuntu WSL 2; the Linux MCP server runs through WSL, avoiding a
new unsigned native Windows executable.

## Set up your agent

1. Complete [Microsoft's Ubuntu WSL installation](https://learn.microsoft.com/en-us/windows/wsl/install)
   and launch Ubuntu once. Have `codex` or `claude` available in PowerShell.
2. Download [lore-mcp-linux](website/downloads/lore-mcp-linux) and the
   [Codex installer](website/downloads/install-codex.ps1) or
   [Claude Code installer](website/downloads/install-claude-code.ps1) into the
   same folder.
3. Run `.\install-codex.ps1` or `.\install-claude-code.ps1` in PowerShell. Run
   both if you use both agents. If your WSL distribution is not named Ubuntu,
   add `-Distro YourName`.
4. Start a **new** agent session in the project and ask: “Read the relevant
   history, then continue.” For a targeted question, say, “Through Lore, what
   did Codex do last on this project?”

The installers verify the binary hash, back up existing client configuration
and instructions, install the local MCP server, and leave the memory archive at
`%USERPROFILE%\.lore\archive.json` intact. They replace the WSL binary
atomically and roll back on setup failure. If PowerShell blocks a downloaded
script, inspect it and use manual setup instead of disabling Windows
protection. Check the [SHA-256 checksums](website/downloads/checksums.txt),
[Free Binary License](website/downloads/LICENSE.txt), and
[third-party notices](website/downloads/THIRD_PARTY_NOTICES.txt).

## Scope

Codex and Claude Code were tested end to end in both directions on one Windows
PC. Lore also discovers accessible local Qwen Code sessions, but this package
does not configure Qwen Code as an MCP client. Browser ChatGPT and Claude
account histories are not automatically available: uncaptured cloud chats need
a separate capture or import path. Other MCP hosts, including Ollama-based
clients, need their own validation. Lore supplies historical excerpts with
source IDs; it cannot prove an external action happened. There is no hosted
Lore account or cross-device sync. The chosen AI provider receives excerpts
its agent requests, not the entire archive.
