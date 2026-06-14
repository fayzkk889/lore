# Contributing to Lore

Thanks for helping improve Lore.

Lore is an open-source terminal coding agent with a local-first, bring-your-own
API key model. The most valuable contributions keep that promise intact:
predictable local behavior, clear verification, cautious file handling, and a
better user experience in the terminal.

## Development Setup

Requirements:

- Go 1.24 or newer
- Git
- A provider API key if you want to test live agent sessions

Common commands:

```sh
go test ./...
go vet ./...
go build -o lore .
```

On Windows:

```powershell
go test ./...
go vet ./...
go build -o lore.exe .
```

## Before Opening a Pull Request

Run:

```sh
go test ./... -count=1
go vet ./...
```

For user-facing CLI changes, also smoke test:

```sh
lore --help
lore config
lore init
lore history
```

If you change agent behavior, verification, rollback, shell execution, config,
or permissions, add focused tests.

## Design Principles

- Lore connects directly to the user's chosen provider. Do not add a central
  proxy, billing meter, account requirement, or telemetry.
- Keep user files safe. Model-supplied paths and shell behavior must stay inside
  the project unless the user explicitly chooses otherwise.
- Prefer verified work over optimistic summaries. Coding tasks should build,
  test, and exercise the real artifact.
- Keep terminal output useful and calm. Avoid noisy logs, vague success claims,
  and hidden destructive behavior.
- Do not commit secrets, generated binaries, or local `.lore/` state.

## Issue Triage

Useful bug reports include:

- Operating system and shell.
- Lore version or commit.
- Provider/model, if relevant.
- The command run.
- Expected behavior.
- Actual output or error text.

Please redact provider keys, tokens, private file paths, and project secrets.

