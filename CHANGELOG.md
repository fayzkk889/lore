# Changelog

## v0.10.0-alpha.24 — original-source recall

- Favors original chat turns over copied answers that cite them, including
  cross-workspace fallback when a copied answer is the only local match.
- Clarifies source attribution: recorded client roles do not prove human
  authorship, and Lore cites project, conversation, and message IDs.
- Rechecked fresh-profile onboarding, two-way local memory, missing-source
  handling, and citation reopening against the release binary. Live Claude
  Desktop and Codex checks passed on one Windows PC.
- Ships a native Windows MCP server and Claude Desktop extension. A clean-PC
  install remains unverified and Windows may warn about the unsigned binary.

## v0.10.0-alpha.23 — desktop handoff and settings backups

- Tested a native Windows Lore handoff between Claude Desktop and Codex.
- Created a fresh pre-change backup whenever Lore updates an app's settings.

## v0.10.0-alpha.11-mcp — MCP memory reliability

- Imports browser chat exports from ordinary Windows file paths when the MCP
  server runs in WSL.
- Hides synthetic Codex approval-review sessions from the all-sources memory
  list, consistent with their exclusion from recall.
- Passed 32 isolated MCP user-journey checks and the archive, autosync, MCP,
  connection, configuration and path test suites under WSL.

## v0.10.0-alpha.10-mcp — cross-harness preview

- Adds a tested Claude Code installer. Fresh Claude Code and Codex sessions
  recovered each other's project work with source-message citations.
- Excludes synthetic Codex approval-review helper sessions from search and
  continuation without deleting archived records.
- Routes plain "what did Codex/Claude Code do last?" questions to recent history
  for the named harness, avoiding older textually similar sessions.
- Installs the WSL binary atomically so a running MCP process is not corrupted
  by an in-place Windows copy.
- Remains a Windows/Ubuntu WSL 2 developer preview; browser-only histories still
  require capture or import, and other MCP clients are not yet validated.

## v0.10.0-alpha.9-mcp — developer preview

- Ships a dedicated local MCP memory server for Windows via Ubuntu WSL 2, without
  a new unsigned native Windows executable or standalone coding-agent UI.
- Connects to Codex and adds managed guidance so ordinary history requests call
  Lore while preserving existing Codex instructions.
- Normalizes Windows and WSL workspace paths for project-scoped recall.
- Filters transcript scaffolding from evidence and tightens uncertain counts and
  named-subject answers.
- Passed local archive/autosync/MCP tests and fresh Codex recall sessions on one
  Windows PC. Other MCP clients and broad Windows installation remain untested.

## v0.10.0-alpha.7

- Adds a native browser bridge and optional ChatGPT/Claude extension preview.
- Captures opted-in visible conversations into the same project memory used by MCP.
- Retrieves bounded project context and inserts it into empty browser drafts.
- Preserves existing drafts, pauses capture, and reports unsupported layouts.
- Deduplicates stable message IDs and preserves partially mounted history.
- Labels browser timestamps as capture times, not original authored dates.
- Uses updated app executables without reinstalling extension code.
- Fixes Windows exclusive lock-file contention for archive and history sync.
- Passes six native and exact-package platform gates plus 16 Chromium fixture
  journeys and a fresh browser-to-coding-agent memory recovery test.
- Browser-store publishing and authenticated live-site validation remain pending.

## v0.10.0-alpha.6

- Adds managed startup memory guidance for Codex, Claude Code and Qwen Code.
- Uses recent visible turns, including the latest user request, for continuation.
- Excludes transport/tool-log scaffolding from search and deduplicates copied hits.
- Returns focused, cited excerpts within one strict 6,000-character default budget.
- Tightens weak overlap and exact-subject matching for credential questions.
- Preserves explicit version/number/alias constraints across alternate queries.
- Automatically rebuilds incompatible search caches.
- Validated against actual local histories and fresh model sessions, in addition
  to six native OS/CPU regression and installer gates.
- Windows remains unsigned; macOS signing/notarization remains incomplete.

## v0.10.0-alpha.5

- Tries bounded automatic alternatives after an empty engineering-topic search.
- Reports supported and accessible history sources separately from new imports.
- Checks candidate execution/version before replacing an existing installation.
- Detects Windows ARM64 from emulated shells.
- Fixes lost shell output and incomplete final output aggregation.
- Adds native OS/CPU, MCP and installer lifecycle checks.
- Windows remains unsigned; macOS signing/notarization is not yet completed.

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
