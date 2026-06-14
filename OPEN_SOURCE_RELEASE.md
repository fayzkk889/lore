# Open-Source Release Checklist

Use this checklist when publishing Lore from the private development repository
to the public `fayzkk889/lore` repository.

## 1. Freeze

- Stop feature work on the release branch.
- Run `go test ./... -count=1`.
- Run `go vet ./...`.
- Build a local binary and smoke test `lore --help`, `lore config`, `lore init`,
  `lore history`, and `lore rollback --last`.

## 2. Secret Review

- Search the working tree for provider keys, private endpoints, OAuth remnants,
  payment links, internal notes, and local logs.
- Confirm `.env`, `.env.*`, `.lore/`, `dist/`, and generated binaries are not
  tracked.
- Rotate any exposed provider key. If a key ever entered git history, assume it
  is compromised even if the latest commit removes it.

## 3. Public Surface

- README describes open-source BYO-key behavior.
- LICENSE is present.
- SECURITY.md is present.
- CONTRIBUTING.md is present.
- CODE_OF_CONDUCT.md is present.
- `.env.example` contains placeholders only.
- Install scripts point to `fayzkk889/lore`.
- Website copy contains no pricing tiers, OAuth/account flow, hosted metering,
  or LemonSqueezy checkout links.

## 4. Clean Public Repository

Recommended path:

1. Keep the private development repository private.
2. Create or reuse the public `fayzkk889/lore` repository.
3. Push the cleaned current source as a fresh public history.
4. Do not import private commit history unless it has been audited.

Example from a clean export directory:

```sh
git init
git add .
git commit -m "Initial open-source release"
git branch -M main
git remote add origin https://github.com/fayzkk889/lore.git
git push -u origin main
```

If `fayzkk889/lore` already has release-only files, archive them first or push
to a temporary branch and merge intentionally.

## 5. Release

- Tag the first public source release, for example `v0.9.1-beta`.
- Let GitHub Actions run GoReleaser.
- Confirm release archives, checksums, and installer downloads work.
- Test the install command on a clean machine or temporary environment.

## 6. Launch

- Make the repository public.
- Pin a short issue or discussion explaining the roadmap.
- Link the website to the public repo and latest release.
- Keep next-version features out of the launch branch.
