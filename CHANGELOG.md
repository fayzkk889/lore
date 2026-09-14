# Changelog

## v0.10.0-alpha.4

- Automatically discovers current Qwen Code session history alongside Codex
  and Claude Code.
- Supports Qwen's default history directory plus `QWEN_HOME` and
  `QWEN_RUNTIME_DIR` overrides.
- Excludes Qwen system events, hidden thoughts, tool calls, and tool results
  from the searchable archive.

## v0.10.0-alpha.3

- Prevents the Windows uninstaller from deleting unrelated files in a custom
  install directory.
- Cleans Windows installer temporary files after failed downloads.
- Makes Unix uninstall resilient to MCP disconnect failures and adds an
  explicit `--skip-disconnect` option.
- Adds configurable data directories for safe, testable purge behavior.
- Documents ZIP and text imports accurately in CLI help.

## v0.10.0-alpha.2

- Adds local cross-client memory through MCP.
- Automatically discovers Codex and Claude Code histories.
- Adds cited recall, source inspection, explicit memories, checkpoints,
  imports, list, update, and permanent forget.
- Adds exact workspace isolation and live MCP root tracking.
- Adds a validated persistent search index and bounded retrieval.
- Adds safe configuration for Codex, Claude, Cursor, and Qwen clients.
- Adds safe disconnect and uninstall paths.

This is an alpha release. Automatic historical discovery for Cursor, Qwen Code,
Claude Desktop, and browser-hosted chats is not included yet.
