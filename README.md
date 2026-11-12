# StringBird CLI

[![CI](https://github.com/string-bird/string-bird-cli/actions/workflows/ci.yml/badge.svg)](https://github.com/string-bird/string-bird-cli/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/string-bird/string-bird-cli)](https://github.com/string-bird/string-bird-cli/releases)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

`sb` connects your codebase to [StringBird](https://stringbird.io), the translation
management platform. Sync translation files, machine-translate and approve, check how
complete each locale is, and publish releases to the CDN your apps load them from, from
your terminal or CI.

```bash
sb push                                        # upload new and changed source strings
sb translate --wait                            # machine-translate what's missing
sb status --min 100                            # fail unless every locale is ready
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

2. **Set an API key**, created in the dashboard under organization settings → API keys:

   ```bash
   export STRINGBIRD_API_KEY=sb_...
   ```

3. **Create `stringbird.yaml`** in your project root. `sb init` lists the projects your key
   can see and finds your locale files:

   ```console
   $ sb init --org 0193a1b2-0000-7000-8000-000000000001
   Projects:
     1) Mobile (mobile)
     2) Web (web)
   Project number [1]: 2
   Translation files, {locale} where the locale goes [src/locales/{locale}.json]:
   Wrote stringbird.yaml for project Web (web): json files at src/locales/{locale}.json
   ```

4. **Sync your strings:**

   ```bash
   sb push --dry-run   # preview
   sb push             # upload src/locales/en.json (your base locale)
   sb pull             # write src/locales/de.json, fr.json, ...
   ```

5. **Release** once translations are approved:

   ```bash
   sb status                                  # what's still missing
   sb release create --env production --wait  # publish to the CDN
   ```

## Installation

Prebuilt binaries for Linux, macOS and Windows (x64 and ARM64) are attached to every
[release](https://github.com/string-bird/string-bird-cli/releases).

| Platform | Command |
|---|---|
| macOS, Linux | `curl -fsSL https://raw.githubusercontent.com/string-bird/string-bird-cli/main/install.sh \| sh` |
| Windows (PowerShell) | `irm https://raw.githubusercontent.com/string-bird/string-bird-cli/main/install.ps1 \| iex` |
| Homebrew | `brew install string-bird/tap/sb` |
| Scoop | `scoop bucket add stringbird https://github.com/string-bird/scoop-bucket` then `scoop install stringbird/sb` |
| Go 1.27+ | `go install github.com/string-bird/string-bird-cli/cmd/sb@latest` |

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
`--config`). Create it with [`sb init`](#sb-init) and commit it: it holds no secrets, and
it keeps every developer and CI job on the same project and paths. A full example is in
[stringbird.example.yaml](stringbird.example.yaml).

```yaml
org_id: 0193a1b2-0000-7000-8000-000000000001
project_id: 0193a1b2-0000-7000-8000-000000000002 # Web (web)
files: src/locales/{locale}.json
format: json
structure: flat
```

| Field | Required | Description |
|---|---|---|
| `org_id` | yes | Organization id, from the dashboard. |
| `project_id` | yes | Project id or slug (`web`). An id saves a lookup on every run. |
| `files` | for `push`, `pull` | Path to the translation files, relative to the config file. `{locale}` is replaced by the locale code (`en`, `de-AT`) and may appear more than once, e.g. `i18n/{locale}/messages.json`. |
| `format` | no | `json` (default): values are ICU MessageFormat, as used by FormatJS and i18next-icu. `i18next`: classic i18next v4 JSON (`{{name}}`, `key_one` / `key_other`); the server converts it to and from ICU. |
| `structure` | no | Key layout of pulled files: `flat` (`"auth.login.title"`) or `nested`. Defaults to `flat` for `json` and `nested` for `i18next`. |

Unknown fields are rejected, so typos surface immediately.

> [!NOTE]
> Quote a `files` value that starts with `{`, otherwise YAML reads it as a map:
> `files: "{locale}/messages.json"`.

### Overrides

Flags and environment variables take precedence over the file, so one run can target
another project, or `sb` can run with no config file at all, e.g. in a monorepo or a
one-off CI job:

| Setting | Flag | Environment variable |
|---|---|---|
| `org_id` | `--org` | `STRINGBIRD_ORG_ID` |
| `project_id` | `--project` | `STRINGBIRD_PROJECT_ID` |
| `files` | `--files` | |

Precedence is flag, then environment variable, then `stringbird.yaml`. A `--files` path is
relative to the working directory; the one in the file is relative to the file.

```bash
sb pull --org 0193a1b2-... --project web --files "public/locales/{locale}.json"
```

## Authentication

`sb` authenticates with an API key from the `STRINGBIRD_API_KEY` environment variable, or
the `--api-key` flag. Prefer the variable: flags end up in shell history and process lists.
The key never belongs in `stringbird.yaml`, and `sb` refuses a config that contains one.

The key's role decides what `sb` may do:

| Action | Minimum role |
|---|---|
| `sb pull`, `sb status`, and every `list` command | viewer |
| `sb push` to existing keys, `sb translate`, `sb approve` | translator |
| `sb push` adding keys, `--delete-missing` | developer |
| `sb release create`, `sb release promote`, `sb env create`, `sb locales add` | developer |
| `sb api` | whatever the route needs |

Keys can also be limited to certain projects, and to certain environments for releasing
and promoting. Use the narrowest key that does the job, e.g. a viewer key for builds that
only pull.

API keys work inside their own organization only: they can't list your other
organizations, or manage members, settings or billing. Use the dashboard for those.

## Commands

| Command | Description |
|---|---|
| **Sync** | |
| [`sb init`](#sb-init) | Create `stringbird.yaml` |
| [`sb push`](#sb-push) | Upload the source file, and optionally translations |
| [`sb pull`](#sb-pull) | Download translated files |
| **Translate** | |
| [`sb status`](#sb-status) | How complete each locale is; gate CI on it |
| [`sb translate`](#sb-translate) | Machine-translate what's missing |
| [`sb approve`](#sb-approve) | Approve machine translations |
| **Release** | |
| [`sb release create`](#sb-release-create) | Publish approved translations as a release |
| [`sb release promote`](#sb-release-promote) | Point an environment at a release, or roll back |
| [`sb release list`, `sb release diff`](#sb-release-list-and-sb-release-diff) | Release history and what changed |
| [`sb env`](#sb-env) | List and create environments |
| **Browse** | |
| [`sb projects`, `sb locales`, `sb keys`](#sb-projects-sb-locales-sb-keys) | Projects, locales and keys |
| [`sb api`](#sb-api) | Call any API route |

The `list` commands, `sb status` and `sb translate` take `--json` for scripts.

Global flags:

| Flag | Description |
|---|---|
| `-c, --config <path>` | Config file. Default `stringbird.yaml`. |
| `--api-key <key>` | API key. Default `$STRINGBIRD_API_KEY`. |
| `--org <id>` | Organization id. Overrides `org_id`; see [Overrides](#overrides). |
| `--project <id or slug>` | Project. Overrides `project_id`. |
| `--files <path>` | Translation files path with `{locale}`. Overrides `files`. |
| `-h, --help` | Help for any command. |
| `-v, --version` | Print the version. |

### Sync

#### `sb init`

Creates `stringbird.yaml`. Values come from `--org`, `--project` and `--files` (or the
environment variables); anything missing is asked for when running in a terminal:

- With an API key set, the projects the key can see are listed to pick from (a single
  project is picked automatically), and a slug given with `--project` is checked and saved
  as the project's id.
- The files path is suggested from locale files found in the folder, such as
  `src/locales/en.json` or `public/locales/en/translation.json`.
- The format (`json` or `i18next`) is detected from the base-locale file's content.

| Flag | Description |
|---|---|
| `--format <json\|i18next>` | Set the format instead of detecting it. |
| `--force` | Replace an existing `stringbird.yaml`. |

Outside a terminal `sb init` never prompts: pass every value as a flag.

```bash
sb init --org 0193a1b2-... --project web --files "src/locales/{locale}.json"
```

#### `sb push`

Uploads translation files to StringBird. By default only the base-locale (source) file
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

#### `sb pull`

Downloads every project locale and writes it to the `files` path, replacing the existing
file.

| Flag | Description |
|---|---|
| `-l, --locale <code>` | Pull only these locales. Repeatable. |
| `--fallback` | Fill missing translations from the fallback chain (`de-AT` → `de` → base locale). |
| `--include-unapproved` | Include machine translations that are still awaiting approval. |

Keys that the chosen format cannot express (for example ICU `select` in i18next format)
are left out with a warning.

### Translate

#### `sb status`

Counts, per locale, the keys that still need work, and the share that is ready:
translated, approved and up to date, which is what a release ships.

```console
$ sb status
LOCALE  READY  MISSING  OUTDATED  UNAPPROVED
de      96.5%  3        2         4
fr      100%   0        0         0
```

| Flag | Description |
|---|---|
| `-l, --locale <code>` | Only these locales. Default every locale but the base. |
| `--min <percent>` | Exit with 1 when a locale is less ready than this. |

#### `sb translate`

Starts a machine translation run. Results are saved awaiting approval and never replace a
person's translation. Nothing to translate is not an error.

| Flag | Description |
|---|---|
| `-l, --locale <code>` | Only these locales. Default every locale but the base. |
| `--scope <scope>` | `missing` (default), `outdated` (base text changed since) or `needed` (both). |
| `--wait` | Follow the run; exit with 1 if it fails or a string can't be translated. |
| `--timeout <duration>` | How long `--wait` waits. Default `30m`. |

#### `sb approve`

Approves machine translations so releases include them. Outdated translations are skipped:
translate them again first.

| Flag | Description |
|---|---|
| `--run <id>` | What one `sb translate` run wrote; `sb translate --wait` prints the id. |
| `-l, --locale <code>` | A locale's machine translations. |
| `--prefix <prefix>` | Only keys starting with this. Combines with either of the above. |

```bash
sb approve --locale de --prefix checkout.
```

### Release

#### `sb release create`

Takes a snapshot of all approved translations as the next release (`v1`, `v2`, ...) and
publishes it to the CDN. Releases are immutable.

| Flag | Description |
|---|---|
| `-e, --env <name>` | Also point this environment at the new release. |
| `--note <text>` | Note shown with the release in the dashboard. |
| `--wait` | Wait until the release is published, and fail if publishing fails. |
| `--timeout <duration>` | How long `--wait` waits. Default `5m`. |

#### `sb release promote`

`sb release promote <environment>` points an environment at an existing release. Nothing
is rebuilt: the environment serves exactly what the release contains. Promoting an older
release is a rollback.

| Flag | Description |
|---|---|
| `--from <environment>` | Use the release another environment currently serves. |
| `--release <ref>` | Use a release by version (`12` or `v12`), id, or environment name. |

```bash
sb release promote production --from staging   # promote
sb release promote production --release v11    # roll back
```

#### `sb release list` and `sb release diff`

`sb release list` shows releases newest first, with the environments each is live in.
`sb release diff <release>` shows the keys added, changed and removed per locale, against
the previous release or `--against` another. A release is a version, an id, or an
environment name for the release it serves, so you can review a promotion first:

```console
$ sb release diff staging --against production
v14 against v13
de: 1 added, 1 changed, 0 removed
  + checkout.title: "Kasse"
  ~ auth.login: "Anmelden" → "Einloggen"
```

| Flag | Description |
|---|---|
| `--limit <n>` | `list`: how many releases, up to 200. Default 20. |
| `--against <release>` | `diff`: what to compare with. Default the release before. |
| `--stat` | `diff`: counts per locale only. |

#### `sb env`

`sb env list` shows each environment and the release it serves. `sb env create <name>`
adds one (lowercase letters, digits and dashes), for example `staging` or `pr-42`.

### Browse

#### `sb projects`, `sb locales`, `sb keys`

| Command | Description |
|---|---|
| `sb projects list` | Projects your key can see; `*` marks the configured one. Needs only `--org`. |
| `sb locales list` | The project's locales. |
| `sb locales add <code>...` | Add locales; codes are stored canonical (`pt-br` → `pt-BR`). |
| `sb keys list` | Keys with their base-locale text. |

`sb keys list` filters with `--prefix auth.`, shows another locale with `-l de`, and lists
what needs work in one locale with `--missing de`, `--outdated de` or `--unapproved de`.
`--limit` caps the count.

#### `sb api`

Calls any route an API key can reach, with authentication and your org and project filled
in. A path is relative to your project, unless it starts with `/v1/`, where `{org}` and
`{project}` are replaced.

```bash
sb api "/keys?status=missing&locale=de"
sb api /v1/orgs/{org}/projects
sb api /locales -f locale=it                         # fields make it a POST
sb api -X GET /keys -f prefix=auth.                  # or query parameters with -X GET
sb api "/import?locale=de&dry_run=true" --input locales/de.json
```

| Flag | Description |
|---|---|
| `-X, --method <method>` | Default `GET`, or `POST` with fields or `--input`. |
| `-f, --field key=value` | String field. Query parameter for `GET`/`DELETE`, JSON body otherwise. |
| `-F, --typed-field key=value` | Same, with the value read as JSON (`true`, `42`, `["a"]`). |
| `--input <file>` | Send a file as the body, `-` for stdin. |
| `-i, --include` | Print the status line and headers. |

The response is printed as returned, with JSON indented; an error status exits with 1.

> [!NOTE]
> Git Bash on Windows rewrites arguments that start with `/` into Windows paths. Leave out
> the leading slash (`sb api keys`) or set `MSYS_NO_PATHCONV=1`.

## Continuous integration

`sb` is built for CI: it never prompts outside a terminal, exits with a non-zero status on
any failure, and retries rate-limited requests automatically.

### GitHub Actions

This repository is also a GitHub Action that installs `sb` on Linux, macOS and Windows
runners.

| Input | Description |
|---|---|
| `version` | Release to install, e.g. `v0.1.0`. Default `latest`. |

| Output | Description |
|---|---|
| `version` | The release that was installed. |

Push source strings on every change to `main` and publish to staging:

```yaml
name: translations

on:
  push:
    branches: [main]
    paths: [src/locales/en.json]

jobs:
  sync:
    runs-on: ubuntu-latest
    env:
      STRINGBIRD_API_KEY: ${{ secrets.STRINGBIRD_API_KEY }}
    steps:
      - uses: actions/checkout@v5
      - uses: string-bird/string-bird-cli@v0
      - run: sb push
      - run: sb release create --env staging --wait --note "${{ github.sha }}"
```

Block a production promotion until every locale is fully translated and approved:

```yaml
      - run: sb status --min 100
      - run: sb release promote production --from staging
```

To preview a pull request's effect on your strings, run `sb push --dry-run` in a
`pull_request` workflow. It needs a key that may push (translator or developer), but saves
nothing.

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

| Message or code | Meaning |
|---|---|
| `no stringbird.yaml here` | Run `sb init`, `cd` to the project root, or pass `--org`, `--project` and `--files`. |
| `project "…" not found` | No project with that slug for this key; the message lists the ones it can see. |
| `invalid_api_key` | The key is wrong, revoked or expired. Check `STRINGBIRD_API_KEY`. |
| `org_not_found`, `project_not_found` | Wrong id in `stringbird.yaml`, or the key has no access to that project. |
| `forbidden` | The key's role is too low for this action. See [Authentication](#authentication). |
| `environment_not_allowed` | The key is limited to other environments. |
| `api_key_not_allowed` | The route needs a signed-in person, not an API key (members, settings, billing). |
| `locale_not_found` | The locale isn't in the project. Add it with `sb locales add`. |
| `invalid_import` | The file has invalid entries; each one is listed above the error. |
| `plan_limit_reached` | The change would exceed your plan (e.g. number of keys). Nothing was saved. |
| `nothing_to_release` | The project has no approved translations yet. |
| `mt_disabled`, `mt_budget_exhausted` | Machine translation is off for the organization, or this month's words are used up. |
| `rate_limited` | Too many requests. `sb` retries automatically; this appears only if retries run out. |

## Development

Requires Go 1.27 or newer.

```bash
go test ./...
go build -o sb ./cmd/sb
```

Tests run every command against an in-process fake of the StringBird API, and build and
run the binary itself. Releases are cut by pushing a `vX.Y.Z` tag; see
[.goreleaser.yaml](.goreleaser.yaml) and the [release workflow](.github/workflows/release.yml).

## License

[MIT](LICENSE)
