# Commits

Versions and changelogs are generated from commit messages, so every commit follows [Conventional Commits](https://www.conventionalcommits.org).

- **One line only.** No body, no footers, no trailers (no `Co-Authored-By`).
- Format: `type(scope)!: summary`, scope and `!` optional.
- Summary: imperative, lowercase, no trailing period, whole line ≤ 72 characters.
- Types and their version effect:

| Type       | Use for                                         | Version |
|------------|-------------------------------------------------|---------|
| `feat`     | new command, flag or visible behavior           | minor   |
| `fix`      | bug fix                                         | patch   |
| `perf`     | performance improvement                         | patch   |
| `refactor` | code change with no behavior change             | —       |
| `test`     | tests or test harness only                      | —       |
| `docs`     | docs, README, AGENTS.md, rules                  | —       |
| `build`    | dependencies, build config                      | —       |
| `ci`       | CI workflows                                    | —       |
| `chore`    | anything else (scripts, config, repo setup)     | —       |
| `revert`   | reverting a previous commit                     | —       |

- Breaking change: add `!` (`feat(cli)!: rename --env to --environment`) → major. One line, so never a `BREAKING CHANGE:` footer.
- Scope: the area, short: `cli`, `api`, `config`, `push`, `pull`, `release`, `action`, `install`.
- One logical change per commit; if the summary needs "and", consider two commits.

Examples:

```
feat(cli): add sb init and --org, --project, --files overrides
fix(push): keep key order when writing json
ci: release with goreleaser
```
