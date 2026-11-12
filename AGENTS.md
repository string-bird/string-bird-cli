# AGENTS.md: StringBird CLI

`sb` connects a codebase to **StringBird**, the translation management service: push source
strings, pull translations, machine-translate and approve, check locale completeness, and
create or promote releases. The repo is also a GitHub Action (`string-bird/string-bird-cli@v0`).

## Stack

Go 1.27 · cobra · standard library HTTP client · GoReleaser · install scripts for
macOS/Linux (`install.sh`) and Windows (`install.ps1`) · Homebrew tap `string-bird/tap/sb`.

## Layout

```
cmd/sb/            main
internal/cli/      commands (push, pull, translate, approve, status, release, env, init, api, keys, ...)
internal/api/      StringBird API client
internal/config/   stringbird.yaml, env vars and flag overrides
action.yml         the GitHub Action
.goreleaser.yaml   release builds
```

## Commands

```bash
go build -o sb ./cmd/sb
gofmt -l . && go vet ./... && go test ./...    # run before finishing a change
```

Tests run every command against an in-process fake of the StringBird API
(`internal/cli/fake_api_test.go`) and build and run the binary itself.

## Rules

- Naming: `StringBird` where people read the name (UI, emails, docs); `stringbird` in identifiers
  (domains, binary, npm scope, env vars). Never "String Bird". GitHub names stay `string-bird`
  (org, repos, module paths, Action `uses:`), because `stringbird` is taken there.
- `sb` never prompts outside a terminal, exits non-zero on any failure, and prefixes errors with
  `sb: `. Keep output lines stable: the website and docs quote them.
- Commands and flags are documented in [README.md](README.md); update it with any change to them.
- Releases: pushing a `vX.Y.Z` tag runs GoReleaser and moves the `v0` major tag the Action uses.

## Git

- Commits: one-line Conventional Commits ([.claude/rules/commits.md](.claude/rules/commits.md)).
- **Ask before every `git push`**, and before anything else that changes the remote: tags, force
  pushes, deleting branches, opening or merging pull requests.
  Tags publish a release, so never tag without asking. Committing locally is fine.
- Commits are signed (1Password SSH signing on the owner's machine), so a commit can wait for that
  approval.

## The other repos

All under `github.com/string-bird`, checked out side by side in `D:\Projects\string-bird`:

| Repo                  | What                                                                     |
|-----------------------|--------------------------------------------------------------------------|
| `string-bird-api`     | Go API and worker (DigitalOcean); plans, billing, releases, MT           |
| `string-bird-cli`     | `sb` CLI and GitHub Action                                               |
| `string-bird-js`      | JavaScript SDKs: core, React Native, i18next, FormatJS                   |
| `string-bird-website` | Astro marketing site and blog                                            |
| `string-bird-infra`   | Terraform for Cloudflare (HCP Terraform)                                 |
