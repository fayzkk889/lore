# Lore

```
██╗      ██████╗ ██████╗ ███████╗
██║     ██╔═══██╗██╔══██╗██╔════╝
██║     ██║   ██║██████╔╝█████╗
██║     ██║   ██║██╔══██╗██╔══╝
███████╗╚██████╔╝██║  ██║███████╗
╚══════╝ ╚═════╝ ╚═╝  ╚═╝╚══════╝
```

**Open-source AI coding agent for your terminal. Bring your own API key.**

Lore is a tool-calling agent harness: the model works by reading and writing
files, running shell commands, and, crucially, **verifying its own work**. A
coding task is not "done" until the project builds, vets, tests, and passes
runtime checks against the real artifact. If verification fails, the agent goes
back to fixing.

No account. No telemetry. No middleman server. Lore talks directly to the model
provider you choose, with your key.

## Features

- **Verified completion** - build / vet / test / runtime smoke checks gate every
  coding task; the agent fixes failures until checks pass.
- **Verification ledger** - every `verify_app` run writes proof under
  `.lore/runs/`, and `/runs` lists recent ledgers from the TUI.
- **Local safety controls** - choose `full-auto`, `auto-safe`, `ask`, or
  `read-only`; tool activity is summarized in `.lore/audit.jsonl`.
- **Interactive TUI** - streamed responses, in-place tool status, bordered
  diffs, undo snapshots (`/rollback`), approval prompts, project status, and
  memory/wiki recall.
- **Headless mode** - `lore do "<task>"` runs one request to completion and exits
  non-zero if the result could not be verified. CI-friendly, with
  `--permission full-auto|read-only`.
- **Model-agnostic** - Anthropic's native API, any OpenAI-compatible endpoint
  (OpenAI, OpenRouter, DeepSeek, Together, self-hosted gateways), or local
  models via Ollama.
- **Project memory** - an optional `.lore/` wiki keeps project context across
  sessions (`lore init`).

## Install

Requires [Go](https://go.dev/dl/) 1.24+.

```sh
git clone https://github.com/fayzkk889/lore.git
cd lore
go build -o lore .        # lore.exe on Windows
```

Put the binary somewhere on your `PATH`, or `go install .`.

## Set Your API Key

Lore needs a provider and its API key. Three ways, in priority order
(flag > environment > config file):

**1. Environment variable** - set the key for your provider and run `lore`; the
provider is inferred:

| Provider   | Env var              | Default model                 |
|------------|----------------------|-------------------------------|
| Anthropic  | `ANTHROPIC_API_KEY`  | `claude-sonnet-4-6`           |
| OpenAI     | `OPENAI_API_KEY`     | `gpt-4o`                      |
| OpenRouter | `OPENROUTER_API_KEY` | `deepseek/deepseek-chat-v3.1` |
| DeepSeek   | `DEEPSEEK_API_KEY`   | `deepseek-chat`               |
| Ollama     | no key needed        | choose at setup               |
| Custom     | `LORE_API_KEY`       | choose at setup               |

```sh
export OPENROUTER_API_KEY=sk-or-v1-...
lore
```

**2. First-run prompt** - run `lore` with nothing configured and it asks for the
provider and key. Settings are stored in `~/.lore/config.toml` with mode `0600`.
Re-run setup any time with `lore config set`.

**3. Flags** - pass settings for one run:

```sh
lore --provider openrouter --api-key sk-or-... --model moonshotai/kimi-k2
```

Override the model or endpoint with `--model` / `--base-url`, or `LORE_MODEL` /
`LORE_BASE_URL`, or `~/.lore/config.toml`:

```toml
[engine]
provider = "openrouter"
model    = "deepseek/deepseek-chat-v3.1"
api_key  = ""   # empty = read from environment

[safety]
permission_mode = "auto-safe" # full-auto | auto-safe | ask | read-only
```

`lore config` shows what is active and where the key comes from, redacted.
Set the default safety mode with:

```sh
lore config permission auto-safe
```

### Local Models (Ollama)

```sh
ollama pull qwen3:4b
lore --provider ollama --model qwen3:4b
```

No key required. Any model with tool-calling support works; bigger is better.

### Any OpenAI-Compatible Endpoint

```sh
lore --provider custom \
     --base-url https://api.together.xyz/v1 \
     --model Qwen/Qwen2.5-Coder-32B-Instruct \
     --api-key $TOGETHER_API_KEY
```

## Quick Start

```sh
mkdir hello && cd hello
lore do "Build a CLI todo manager in Go: add <title>, list, done <id>. SQLite storage, include tests."
```

Lore plans, scaffolds the module, writes source and tests, builds, and runs
verification checks against the real binary. Exit code `0` means verified.

For interactive work, run `lore` in your project directory:

```text
> add a --json flag to the list command and cover it with a test
```

Useful TUI commands:

| Command | What it does |
|---------|--------------|
| `/status` | show engine, safety mode, memory/wiki, runs, and audit counts |
| `/permissions [mode]` | show or set `full-auto`, `auto-safe`, `ask`, or `read-only` for this session |
| `/approve` | shortcut that toggles interactive `ask` mode |
| `/runs` | list recent verification ledgers from `.lore/runs/` |
| `/audit` | show recent tool activity from `.lore/audit.jsonl` |
| `/wiki` | list Lore wiki documents |
| `/memory` | show project memory |
| `/remember <note>` | append a persistent project memory note |
| `/recall <query>` | search memory, wiki docs, and `LORE.md` |
| `/rollback <n>` | restore to a local snapshot |
| `/sh <cmd>` | run a shell command yourself |
| `/tokens` | show session token usage |

## Commands

| Command | What it does |
|---------|--------------|
| `lore` | interactive chat TUI in the current directory |
| `lore do "<task>"` | headless one-shot agent run; exit `0` means verified |
| `lore do --permission read-only "<task>"` | headless read-only audit |
| `lore config` | show / set / clear the provider configuration |
| `lore config permission [mode]` | show / set default permission mode |
| `lore init` | seed the optional `.lore/` project wiki |
| `lore history` | list undo snapshots |
| `lore rollback` | restore the project to a snapshot |

## Safety Modes

| Mode | Behavior |
|------|----------|
| `full-auto` | default; read, write, shell, setup, and verify tools run without prompting |
| `auto-safe` | TUI asks before shell, setup, delete, and verification actions |
| `ask` | TUI asks before writes, shell commands, setup, delete, and verification |
| `read-only` | allows read/search/list operations only; writes and shell execution are denied |

`lore do` is non-interactive, so it supports `full-auto` and `read-only`.
If your saved default is `ask` or `auto-safe`, headless runs fall back to
`full-auto` unless you explicitly pass `--permission read-only`:

```sh
lore do --permission read-only "Audit this codebase and report risks without changing files."
```

Use the TUI for `ask` and `auto-safe`, where Lore can prompt for approval.

## Notes

- Your API key is sent only to the provider endpoint you configured. Keep it in
  the environment if you prefer nothing on disk.
- Token usage for the session is shown in the TUI footer (`/tokens`). What those
  tokens cost is between you and your provider; Lore does no metering.
- `.lore/` is yours: project memory, snapshots, audit entries, and verification
  ledgers live there and are gitignored by default.

## Contributing

Contributions are welcome. Read [CONTRIBUTING.md](CONTRIBUTING.md) before
opening a pull request, and report security issues through
[SECURITY.md](SECURITY.md) instead of public issues.

## License

[MIT](LICENSE)
