**Current launch: Windows only (amd64 and arm64).** Start with the Windows installer below. macOS and Linux expansion is deferred; historical platform instructions do not define current launch support.

# Lore

**Private local memory for coding agents.**

Lore lets Codex, Claude Code, Cursor, Qwen Code, and Claude Desktop recall
useful context from earlier AI work. Memory stays on the user's computer and
ordinary memory operations require no Lore account, API key, or hosted memory
service.

Lore is free during its alpha. Its current implementation is closed source.
Earlier versions that were published under MIT remain governed by MIT.

The current release is **v0.10.0-alpha.7**. This launch supports Windows amd64 and arm64.
Other operating systems are deferred.

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

For Codex, Claude Code and Qwen Code, connection also adds a short memory
instruction block to the client's default global guidance file. Existing
guidance is preserved. This makes history requests visible to the agent before
it discovers Lore's MCP tools. Disconnect removes Lore's instruction block.

```text
Read the relevant history and continue this work.
Get the production database decision from Lore.
Remember through Lore that production uses port 4318.
What does Lore remember about this project?
Forget the obsolete deployment conversation from Lore.
```

The model calls Lore through MCP. Terminal commands are only needed for the
one-time connection and administrative operations.

To upgrade from an earlier beta, rerun the installer and `lore connect`, then
restart the client. Reconnecting refreshes the stable MCP executable and the
new memory guidance. Other clients may require explicitly saying “through Lore.”

## Current coverage

### Browser project memory — developer preview

Alpha.7 includes `lore browser install` and an optional Chrome/Edge extension
archive on the release page. Extract the extension ZIP, load its folder through
the browser's **Load unpacked** option, then run `lore browser install` once from
the installed app. In a saved ChatGPT/Claude conversation, open Lore, select the
project and explicitly enable capture. Capture starts off and is per conversation.

In a new browser chat, choose that project and click **Get project context**.
Insert it into an empty draft or copy it, review it, and send yourself. Connected
coding agents recall the same captured history through Lore. Choose an existing
workspace for default agent scope; request a named project explicitly otherwise.

The bridge uses the registered installed executable, so app updates replacing
that path are used without reinstalling the extension. This does not introduce
an app auto-updater. Extension code still requires an extension update; the
unpacked preview does not have store-managed updates. Moving the app requires
rerunning `lore browser install`.

Store publishing and authenticated live-site validation are pending. Chromium
tests use controlled site-layout fixtures. Capture covers recognized rendered
messages, not all account history, hidden messages, attachments or every branch.
Stable-ID edits replace their saved turn; without IDs, content hashes preserve
partially mounted history but older edited versions can remain. Browser timestamps
are capture times, not original message dates. The popup reports capture failures.

### Local coding agents and other clients

Lore automatically discovers local **Codex**, **Claude Code**, and **Qwen Code**
histories. Cursor and Claude Desktop can read and write shared Lore memory, but
their older native chats are not automatically imported yet. ChatGPT and Claude
history not captured by the optional extension needs an accessible export file.

OpenCode can be configured manually with a local stdio MCP server running
`lore mcp`; automatic OpenCode setup/history capture is not included in this beta.
Ollama models need a tool-executing client connected to Lore; bare Ollama does not
automatically access chat history through Lore.

Recall uses local vocabulary-based search and bounded automatic alternatives for
common engineering questions. It can miss paraphrases; a model may need to retry
with more specific terms. General continuation retrieves recent visible turns;
default recall returns up to four evidence windows within 6,000 characters.
The agent should check whether the excerpts answer the question and explain gaps.
Use a consistent workspace path: switching between
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

