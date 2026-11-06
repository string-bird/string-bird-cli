# String Bird CLI

[![CI](https://github.com/string-bird/string-bird-cli/actions/workflows/ci.yml/badge.svg)](https://github.com/string-bird/string-bird-cli/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/string-bird/string-bird-cli)](https://github.com/string-bird/string-bird-cli/releases)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

`sb` connects your codebase to [String Bird](https://stringbird.io), the translation
management platform. Push source strings, pull translations, and publish releases to the
CDN your apps load them from, from your terminal or CI.

```bash
sb push                                        # upload new and changed source strings
sb pull                                        # download translations for every locale
sb release create --env staging --wait         # publish approved translations to staging
sb release promote production --from staging   # ship exactly what staging serves
```

## Contents

- [Quick start](#quick-start)
- [Installation](#installation)
- [Configuration](#configuration)
- [Authentication](#authentication)
- [Commands](#commands)
- [Continuous integration](#continuous-integration)
- [Troubleshooting](#troubleshooting)
- [Development](#development)

## Quick start

1. **Install** `sb` (more options under [Installation](#installation)):

   ```bash
   curl -fsSL https://raw.githubusercontent.com/string-bird/string-bird-cli/main/install.sh | sh
   ```

2. **Add `stringbird.yaml`** to your project root. The ids are in the project's settings in
   the dashboard.

   ```yaml
   org_id: 0193a1b2-0000-7000-8000-000000000001
   project_id: 0193a1b2-0000-7000-8000-000000000002
   files: src/locales/{locale}.json
   ```

3. **Set an API key**, created in the dashboard under organization settings → API keys:

   ```bash
   export STRINGBIRD_API_KEY=sb_...
   ```

4. **Sync:**

   ```bash
   sb push --dry-run   # preview
   sb push             # upload src/locales/en.json (your base locale)
   sb pull             # write src/locales/de.json, fr.json, ...
   ```

## Installation

Prebuilt binaries for Linux, macOS and Windows (x64 and ARM64) are attached to every
[release](https://github.com/string-bird/string-bird-cli/releases).

| Platform | Command |
|---|---|
| macOS, Linux | `curl -fsSL https://raw.githubusercontent.com/string-bird/string-bird-cli/main/install.sh \| sh` |
| Windows (PowerShell) | `irm https://raw.githubusercontent.com/string-bird/string-bird-cli/main/install.ps1 \| iex` |
| Homebrew | `brew install string-bird/tap/sb` |
| Scoop | `scoop bucket add string-bird https://github.com/string-bird/scoop-bucket` then `scoop install string-bird/sb` |
| Go 1.26+ | `go install github.com/string-bird/string-bird-cli/cmd/sb@latest` |

The install scripts verify the download against the release's `checksums.txt` and accept
two optional variables:

| Variable | Default |
|---|---|
| `SB_VERSION` | Latest release. Pin one with e.g. `v0.1.0`. |
| `SB_INSTALL_DIR` | `/usr/local/bin` if writable, otherwise `~/.local/bin`. On Windows `%LOCALAPPDATA%\Programs\sb`, added to your user `PATH`. |

Every release archive carries signed build provenance. To confirm an archive was built by
this repository's release workflow:

```bash
gh attestation verify sb_0.1.0_linux_amd64.tar.gz --repo string-bird/string-bird-cli
```

## Configuration

`sb` reads `stringbird.yaml` from the current directory (or the path given with
`--config`). Commit it: it holds no secrets. A full example is in
[stringbird.example.yaml](stringbird.example.yaml).

```yaml
org_id: 0193a1b2-0000-7000-8000-000000000001
project_id: 0193a1b2-0000-7000-8000-000000000002
files: src/locales/{locale}.json
format: json
structure: flat
```

| Field | Required | Description |
|---|---|---|
| `org_id` | yes | Organization id, from the dashboard. |
| `project_id` | yes | Project id, from the project's settings. |
| `files` | yes | Path to the translation files, relative to the config file. `{locale}` is replaced by the locale code (`en`, `de-AT`) and may appear more than once, e.g. `i18n/{locale}/messages.json`. |
| `format` | no | `json` (default): values are ICU MessageFormat, as used by FormatJS and i18next-icu. `i18next`: classic i18next v4 JSON (`{{name}}`, `key_one` / `key_other`); the server converts it to and from ICU. |
| `structure` | no | Key layout of pulled files: `flat` (`"auth.login.title"`) or `nested`. Defaults to `flat` for `json` and `nested` for `i18next`. |

Unknown fields are rejected, so typos surface immediately.

> [!NOTE]
> Quote a `files` value that starts with `{`, otherwise YAML reads it as a map:
> `files: "{locale}/messages.json"`.

## Authentication

`sb` authenticates with an API key from the `STRINGBIRD_API_KEY` environment variable, or
the `--api-key` flag. Prefer the variable: flags end up in shell history and process lists.
The key never belongs in `stringbird.yaml`, and `sb` refuses a config that contains one.

The key's role decides what `sb` may do:

| Action | Minimum role |
|---|---|
| `sb pull` | viewer |
| `sb push` to existing keys | translator |
| `sb push` adding keys, `--delete-missing` | developer |
| `sb release create`, `sb release promote` | developer |

Keys can also be limited to certain projects, and to certain environments for releasing
and promoting. Use the narrowest key that does the job, e.g. a viewer key for builds that
only pull.

## Commands

Global flags:

| Flag | Description |
|---|---|
| `-c, --config <path>` | Config file. Default `stringbird.yaml`. |
| `--api-key <key>` | API key. Default `$STRINGBIRD_API_KEY`. |
| `-h, --help` | Help for any command. |
| `--version` | Print the version. |

### `sb push`

Uploads translation files to String Bird. By default only the base-locale (source) file
is pushed: new keys are added and changed source texts are saved, which marks their
translations as outdated.

| Flag | Description |
|---|---|
| `--dry-run` | Show what would change without saving anything. |
| `--translations` | Also push the other locales' files that exist. Translations that differ are overwritten. |
| `--skip-existing` | Never overwrite an existing translation; only fill in missing ones. |
| `-l, --locale <code>` | Push only these locales. Repeatable. |
| `--delete-missing` | Delete keys the base-locale file no longer contains. |

Each file is imported all or nothing. If any entry is invalid (broken ICU syntax, a key
that is also another key's prefix, a non-string value), nothing from that file is saved
and every problem is listed:

```console
$ sb push --dry-run --delete-missing
dry run en (src/locales/en.json): 1 key added, 1 updated, 1 key deleted, 40 unchanged
  + checkout.title
  ~ auth.login.title: "Sign in" → "Log in"
  - home.legacy_banner
```

> [!TIP]
> Translators usually work in the dashboard, and pushing translation files overwrites
> their changes. Treat the dashboard as the source of truth for translations: push the
> source file, pull the rest. To seed translations once, use
> `sb push --translations --skip-existing`.

### `sb pull`

Downloads every project locale and writes it to the `files` path, replacing the existing
file.

| Flag | Description |
|---|---|
| `-l, --locale <code>` | Pull only these locales. Repeatable. |
| `--fallback` | Fill missing translations from the fallback chain (`de-AT` → `de` → base locale). |
| `--include-unapproved` | Include machine translations that are still awaiting approval. |

Keys that the chosen format cannot express (for example ICU `select` in i18next format)
are left out with a warning.

### `sb release create`

Takes a snapshot of all approved translations as the next release (`v1`, `v2`, ...) and
publishes it to the CDN. Releases are immutable.

| Flag | Description |
|---|---|
| `-e, --env <name>` | Also point this environment at the new release. |
| `--note <text>` | Note shown with the release in the dashboard. |
| `--wait` | Wait until the release is published, and fail if publishing fails. |
| `--timeout <duration>` | How long `--wait` waits. Default `5m`. |

### `sb release promote <environment>`

Points an environment at an existing release. Nothing is rebuilt: the environment serves
exactly what the release contains. Promoting an older release is a rollback.

| Flag | Description |
|---|---|
| `--from <environment>` | Use the release another environment currently serves. |
| `--release <ref>` | Use a release by version (`12` or `v12`) or id. |

```bash
sb release promote production --from staging   # promote
sb release promote production --release v11    # roll back
```

## Continuous integration

`sb` is built for CI: it never prompts, exits with a non-zero status on any failure, and
retries rate-limited requests automatically.

### GitHub Actions

This repository is also a GitHub Action that installs `sb` on Linux, macOS and Windows
runners.

```yaml
name: translations

on:
  push:
    branches: [main]
    paths: [src/locales/en.json]

jobs:
  sync:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v5
      - uses: string-bird/string-bird-cli@v0
      - run: sb push
      - run: sb release create --env staging --wait --note "${{ github.sha }}"
    env:
      STRINGBIRD_API_KEY: ${{ secrets.STRINGBIRD_API_KEY }}
```

To preview a pull request's effect on your strings, run `sb push --dry-run` in a
`pull_request` workflow. It needs a key that is allowed to push (translator or developer),
but saves nothing.

| Input | Description |
|---|---|
| `version` | Release to install, e.g. `v0.1.0`. Default `latest`. |

| Output | Description |
|---|---|
| `version` | The release that was installed. |

### Other CI systems

Install a pinned version with the script, then run `sb` with `STRINGBIRD_API_KEY` taken
from your CI's secret store:

```bash
curl -fsSL https://raw.githubusercontent.com/string-bird/string-bird-cli/main/install.sh | SB_VERSION=v0.1.0 sh
sb pull
```

## Troubleshooting

Errors include the API's error code, which tells you what to fix:

```console
$ sb pull
sb: get project: invalid_api_key: the API key is invalid, revoked or expired (HTTP 401)
```

| Code | Meaning |
|---|---|
| `invalid_api_key` | The key is wrong, revoked or expired. Check `STRINGBIRD_API_KEY`. |
| `org_not_found`, `project_not_found` | Wrong id in `stringbird.yaml`, or the key has no access to that project. |
| `forbidden` | The key's role is too low for this action. See [Authentication](#authentication). |
| `environment_not_allowed` | The key is limited to other environments. |
| `locale_not_found` | The locale isn't in the project. Add it in the dashboard first. |
| `invalid_import` | The file has invalid entries; each one is listed above the error. |
| `plan_limit_reached` | The change would exceed your plan (e.g. number of keys). Nothing was saved. |
| `nothing_to_release` | The project has no approved translations yet. |
| `rate_limited` | Too many requests. `sb` retries automatically; this appears only if retries run out. |

## Development

Requires Go 1.26 or newer.

```bash
go test ./...
go build -o sb ./cmd/sb
```

Tests run every command against an in-process fake of the String Bird API, and build and
run the binary itself. Releases are cut by pushing a `vX.Y.Z` tag; see
[.goreleaser.yaml](.goreleaser.yaml) and the [release workflow](.github/workflows/release.yml).

## License

[MIT](LICENSE)
