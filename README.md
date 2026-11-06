# sb: the String Bird CLI

`sb` connects your repository to [String Bird](https://github.com/string-bird), a hosted
translation management service. It uploads your source strings, downloads translations,
and publishes releases to the CDN your apps load them from.

`sb` is a thin HTTP client: the server parses, validates and converts files. The CLI only
uploads and downloads them.

```bash
sb push                                    # upload the source (base-locale) file
sb pull                                    # download every locale
sb release create --env staging --wait     # snapshot approved translations, serve them on staging
sb release promote production --from staging
```

## Install

Prebuilt binaries for Linux, macOS and Windows (amd64 and arm64) are on
[GitHub releases](https://github.com/string-bird/string-bird-cli/releases).

**macOS and Linux:**

```bash
curl -fsSL https://raw.githubusercontent.com/string-bird/string-bird-cli/main/install.sh | sh
```

**Windows (PowerShell):**

```powershell
irm https://raw.githubusercontent.com/string-bird/string-bird-cli/main/install.ps1 | iex
```

Both scripts check the download against the release's `checksums.txt`. `SB_VERSION=v0.1.0`
pins a version and `SB_INSTALL_DIR` picks the folder (default `/usr/local/bin` or
`~/.local/bin`; on Windows `%LOCALAPPDATA%\Programs\sb`, added to your PATH).

**Homebrew:**

```bash
brew install string-bird/tap/sb
```

**Scoop:**

```powershell
scoop bucket add string-bird https://github.com/string-bird/scoop-bucket
scoop install string-bird/sb
```

**Go 1.26 or newer:**

```bash
go install github.com/string-bird/string-bird-cli/cmd/sb@latest
```

Release archives come with build provenance, so you can check one was built by this
repository's release workflow:

```bash
gh attestation verify sb_0.1.0_linux_amd64.tar.gz --repo string-bird/string-bird-cli
```

## Configure

Put a `stringbird.yaml` at your project root and commit it
(see [stringbird.example.yaml](stringbird.example.yaml)):

```yaml
org_id: 0193a1b2-0000-7000-8000-000000000001
project_id: 0193a1b2-0000-7000-8000-000000000002
files: src/locales/{locale}.json          # relative to this file
format: json                              # json (ICU) or i18next
# structure: nested                       # pulled key layout: flat or nested
```

| Field | Meaning |
|---|---|
| `org_id`, `project_id` | Ids from the project's settings in the dashboard. |
| `files` | One file per locale; `{locale}` is replaced by the BCP 47 code (`en`, `de-AT`). |
| `format` | `json`: values are ICU MessageFormat (FormatJS, i18next-icu). `i18next`: classic i18next v4 JSON (`{{name}}`, `key_one`/`key_other`), converted to and from ICU by the server. Default `json`. |
| `structure` | Key layout of pulled files, `flat` or `nested`. Default: flat for `json`, nested for `i18next`. |

Unknown fields are errors, so typos don't pass silently. Use `--config` for a config
elsewhere.

### API key

Create an API key in the dashboard (organization settings → API keys) and pass it in
`STRINGBIRD_API_KEY`. `--api-key` works too, but ends up in shell history and process
lists. **The key never goes in `stringbird.yaml`**: `sb` refuses a config with an
`api_key` field.

The key's role decides what `sb` can do:

| Command | Needs |
|---|---|
| `sb pull` | any role |
| `sb push` changing texts | translator or above |
| `sb push` adding keys, `--delete-missing` | developer or above |
| `sb release create`, `sb release promote` | developer or above |

A key limited to some projects answers `project_not_found` for the others. A key limited
to some environments releases into and promotes only those (`environment_not_allowed`).

## Commands

### `sb push`

Uploads files through the import endpoint. Each file is all or nothing: one bad entry
(broken ICU, a key next to its own prefix, a non-string value) rejects the whole file,
and `sb` lists every problem.

By default only the base-locale file is pushed: new keys are added and changed source
texts saved, which marks their translations outdated.

| Flag | |
|---|---|
| `--translations` | Also push every other project locale whose file exists. Translations that differ are overwritten. |
| `--skip-existing` | Only add what's missing; never overwrite an existing translation. |
| `-l, --locale de` | Push only these locales (repeatable). |
| `--delete-missing` | Delete keys the base-locale file no longer has. |
| `--dry-run` | Show what would change (keys added, texts updated, keys deleted) and save nothing. |

Translators edit in the dashboard; pushing translations back from the repository
overwrites their work. Prefer `--skip-existing` with `--translations`, or keep the
dashboard as the source of truth for translations and only `pull` them.

### `sb pull`

Downloads every project locale (or `--locale` ones) and writes them to `files`,
replacing what's there.

| Flag | |
|---|---|
| `-l, --locale de` | Pull only these locales (repeatable). |
| `--fallback` | Fill gaps from the fallback chain (`de-AT` → `de` → base locale). |
| `--include-unapproved` | Include machine translations still awaiting approval. |

Keys the format can't express (i18next: `select`, `=0`, offsets) are left out with a
warning.

### `sb release create`

Snapshots the approved translations as the next release (`v13`) and publishes it to the
CDN. Releases are immutable.

| Flag | |
|---|---|
| `-e, --env staging` | Also point this environment at the new release. |
| `--note "..."` | A note shown with the release. |
| `--wait` | Wait until the release is published, and fail if publishing fails. |
| `--timeout 5m` | How long `--wait` waits. |

### `sb release promote <environment>`

Points an environment at a release, serving byte for byte what the release holds.
Pointing it at an older release rolls back.

| Flag | |
|---|---|
| `--from staging` | The release another environment serves. |
| `--release v12` | A release by version (`12`, `v12`) or id. |

## CI

`sb` exits non-zero on any error and prints the API's error code (`invalid_import`,
`plan_limit_reached`, …). Rate-limited requests are retried after the `Retry-After` wait.

### GitHub Actions

`string-bird/string-bird-cli` is also an action that installs `sb` and puts it on the PATH,
on Linux, macOS and Windows runners. Pushing source strings on `main` and releasing to
staging:

```yaml
name: translations
on:
  push:
    branches: [main]
    paths: [src/locales/en.json]

jobs:
  push:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v5
      - uses: string-bird/string-bird-cli@v0 # or with: { version: v0.1.0 }
      - run: sb push
        env:
          STRINGBIRD_API_KEY: ${{ secrets.STRINGBIRD_API_KEY }}
      - run: sb release create --env staging --wait --note "${{ github.sha }}"
        env:
          STRINGBIRD_API_KEY: ${{ secrets.STRINGBIRD_API_KEY }}
```

| Input | |
|---|---|
| `version` | Release tag to install, e.g. `v0.1.0`. Default `latest`. |

| Output | |
|---|---|
| `version` | The tag that was installed. |

### Other CI

Run the install script, then `sb` with `STRINGBIRD_API_KEY` set from your CI's secrets:

```bash
curl -fsSL https://raw.githubusercontent.com/string-bird/string-bird-cli/main/install.sh | SB_VERSION=v0.1.0 sh
```

On pull requests, `sb push --dry-run` shows what merging would change without saving
anything. A key with the viewer role is enough for `sb pull` in builds that bundle
translations.

## Development

```bash
go vet ./...
go test ./...
go build -o sb ./cmd/sb
```

Tests run the commands against a hand-written fake of the API (`internal/cli/fake_api_test.go`);
`cmd/sb` tests build the binary and run it.

Releases: push a `v*` tag. The release workflow runs GoReleaser (`.goreleaser.yaml`), which
builds the archives, publishes the GitHub release with checksums and provenance, and updates
the Homebrew tap and Scoop bucket when `HOMEBREW_TAP_TOKEN` and `SCOOP_BUCKET_TOKEN` are set.
Try it locally with `goreleaser release --snapshot --clean`.

## License

[MIT](LICENSE)
