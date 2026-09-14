# Lore privacy policy

Effective: September 14, 2026

Lore is local software. The Lore memory service does not require a Lore
account, does not contain telemetry, and does not send the local memory archive
to Lore or to a Lore-operated server.

## Data Lore reads

When memory is connected, Lore may read supported AI-client transcripts stored
on the same computer. Automatic discovery currently covers Codex and Claude
Code. Lore can also read a file or export when the user explicitly asks to
import it.

Lore's original coding-agent commands may read project files and execute tools
according to the permission mode selected by the user.

## Data Lore stores

Lore stores its normalized memory archive, search index, source manifest, and
configuration under the user's local Lore directory, normally `~/.lore`.
Project wiki, audit, verification, and rollback data may also be stored in a
project's `.lore` directory.

The user can inspect remembered items through Lore, update explicit memories,
and permanently forget cited conversations. Forgotten automatically discovered
conversations are suppressed during later synchronization unless the user
explicitly restores them.

## Network activity

Local memory import, indexing, search, citation lookup, update, and deletion do
not require network access. The installer downloads release files and checksums
from GitHub.

When the user runs Lore's original coding agent, prompts, selected context, and
tool results may be sent directly to the model provider configured by the user.
That transfer is governed by the provider's terms and privacy policy. The MCP
client using Lore memory may likewise send retrieved excerpts to its configured
model provider as part of the client's normal operation.

## Data sharing and sale

Lore does not operate a memory backend, collect the local archive, sell user
data, or share the local archive with advertisers.

## Security and deletion

Lore uses restrictive local file permissions where the operating system
supports them and treats retrieved conversation text as untrusted evidence.
Deleting Lore's local data directory removes its archive from that computer.
Source transcripts managed by Codex, Claude Code, or another client must be
deleted separately in that client if the user wants those originals removed.

Security reports can be sent through the private contact described in
`SECURITY.md`.
