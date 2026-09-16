# Lore

**Private local memory for coding agents.**

Lore lets Codex, Claude Code, Cursor, Qwen Code, and Claude Desktop recall
useful context from earlier AI work. Memory stays on the user's computer and
ordinary memory operations require no Lore account, API key, or hosted memory
service.

Lore is free during its alpha. Its current implementation is closed source.
Earlier versions that were published under MIT remain governed by MIT.

The current release is **v0.10.0-alpha.5**. Windows is the primary beta download;
Linux is available too. macOS builds are previews until Apple signing and
notarization are completed.

### Windows protection notices

This beta's executable and PowerShell installer are **unsigned**. SmartScreen may
show an unknown-publisher warning, and Smart App Control or a workplace policy
may block them. We cannot promise installation on every Windows machine yet.
Do not disable system protections to install Lore. If blocked, report the exact
message and your OS/client versions without sharing private history.

The macOS archives are not Developer ID signed or notarized; Gatekeeper may block
them. Code signing is separate from the archive checksum verification below.

## Install

### Windows

Open PowerShell and run:

```powershell
Invoke-WebRequest https://raw.githubusercontent.com/fayzkk889/lore/main/install.ps1 -OutFile install-lore.ps1
.\install-lore.ps1
```

Restart the terminal afterward.

### macOS or Linux

```sh
curl -fsSLO https://raw.githubusercontent.com/fayzkk889/lore/main/install.sh
sh install.sh
```

The installers detect the operating system and CPU, download the matching
archive from GitHub Releases, and verify its SHA-256 checksum before installing.
They also check that the candidate runs and reports the expected version before
replacing an existing installation.
Set `LORE_INSTALL_DIR` to use a custom directory. The Windows installer also
accepts `-NoPath` when the user does not want it to change the user PATH.

## Connect

Run once:

```sh
lore connect
```

Or choose clients explicitly:

```sh
lore connect codex claude-code
lore connect --all
```

Restart the connected client. Then use normal prompts:

```text
Read the relevant history and continue this work.
Get the production database decision from Lore.
Remember through Lore that production uses port 4318.
What does Lore remember about this project?
Forget the obsolete deployment conversation from Lore.
```

The model calls Lore through MCP. Terminal commands are only needed for the
one-time connection and administrative operations.

## Current coverage

Lore automatically discovers local **Codex**, **Claude Code**, and **Qwen Code**
histories. Cursor and Claude Desktop can read and write shared Lore memory, but
their older native chats are not automatically imported yet. Cloud-only ChatGPT
and Claude histories require an accessible export file.

OpenCode can be configured manually with a local stdio MCP server running
`lore mcp`; automatic OpenCode setup/history capture is not included in this beta.
Ollama models need a tool-executing client connected to Lore; bare Ollama does not
automatically access chat history through Lore.

Recall uses local vocabulary-based search and bounded automatic alternatives for
common engineering questions. It can miss paraphrases; a model may need to retry
with more specific terms. Use a consistent workspace path: switching between
symlink aliases can miss path-scoped history. Memory does not sync across devices.

## Privacy

- The memory archive stays on the local computer.
- Lore memory has no telemetry and requires no Lore account.
- Search, indexing, update, and deletion make no Lore network request.
- Retrieved excerpts may be sent to the model provider by the connected AI
  client as part of that client's normal operation.
- Every result retains source conversation and message identifiers.

Read the complete [privacy policy](PRIVACY.md).

## Disconnect and uninstall

To remove Lore from client configurations while preserving unrelated settings:

```sh
lore disconnect --all
```

Windows users can then run `uninstall.ps1`; macOS and Linux users can run
`sh uninstall.sh`. The uninstallers remove only Lore's binary and leave other
files in a custom install directory untouched. Local memory is retained by
default so reinstalling does not lose it. Pass `-PurgeData` on Windows or
`--purge-data` on Unix only when the archive should also be permanently removed.

## Feedback and security

Use [GitHub Issues](https://github.com/fayzkk889/lore/issues) for reproducible
bugs and feature requests. Read [SECURITY.md](SECURITY.md) before reporting a
vulnerability.

## License

Official unmodified binaries are free for personal and internal business use
under the [Lore Free Binary License](LICENSE).
