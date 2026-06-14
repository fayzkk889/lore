# Security Policy

Lore is a local-first, bring-your-own-key coding agent. It does not use a Lore
proxy, hosted billing meter, or central account service.

## Supported Versions

Security fixes are applied to the latest public release and the default branch.
If you are using an older beta, upgrade before reporting an issue unless the
issue still reproduces on the latest release.

## Reporting a Vulnerability

Please do not open a public issue for vulnerabilities that could expose user
data, credentials, local files, or provider API keys.

Report privately by contacting the maintainer:

- GitHub: https://github.com/fayzkk889
- X / Twitter: https://x.com/tana_shahh

Include:

- The affected Lore version or commit.
- Your operating system.
- Clear reproduction steps.
- Whether the issue can expose files, command output, API keys, or project
  memory.

## Secrets and API Keys

If you accidentally commit a provider key, rotate it with the provider
immediately. Removing the key from a later commit is not enough; git history may
still contain it.

Lore tries to keep local state private:

- `~/.lore/config.toml` stores configuration with private file permissions.
- Project `.lore/` data is private where the operating system supports POSIX
  permission bits.
- Verification ledgers redact obvious sensitive output lines.
- Rollback snapshots skip obvious secrets, binaries, and large files.

These protections are defense-in-depth, not a substitute for keeping secrets out
of source files.

