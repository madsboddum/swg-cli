# Contributing

## Commit messages

This repo uses [Conventional Commits](https://www.conventionalcommits.org/).

```
<type>[optional scope]: <description>
```

Types in common use here:

- `feat:` — a new user-facing capability
- `fix:` — a bug fix
- `chore:` — tooling, CI, dependencies, housekeeping
- `docs:` — documentation only
- `refactor:` — behaviour-preserving code changes
- `test:` — tests only

The rest of the Angular set — `perf`, `build`, `ci`, `style`, `revert` — is accepted too. Anything outside it is rejected.

Scope is the package or subcommand, e.g. `feat(tre): add extract subcommand`.

Breaking changes get a `!` after the type — `feat(stf)!: ...` — and a `BREAKING CHANGE:` footer.

Work that comes from an issue references it in a footer, so the commit is traceable back to the reasoning behind it:

```
Refs: #12
```

Use `Closes: #12` on the commit that finishes the issue.

This matters because release changelogs are grouped by these prefixes. A `commit-msg` hook runs `cog verify` (cocogitto), so a malformed message is rejected at commit time. Merge and fixup commits are exempt.

## Setup

Non-Go tooling is pinned in `mise.toml`. With [mise](https://mise.jdx.dev) installed:

```shell
$ mise install
```

That also installs the git hooks via lefthook, as a `postinstall` hook.

The Go version is not pinned there — `go.mod`'s `toolchain` directive is the single source of truth, and any Go 1.21+ on your PATH will fetch the right one automatically. Do not set `GOTOOLCHAIN=local`; that disables the fetch.

## Before pushing

```shell
$ mise run verify
```

That runs gofmt, `go vet`, `golangci-lint`, tests and a build — the same set CI runs on every push and pull request. Individual steps are available as `mise run fmt`, `vet`, `lint`, `test`, `build`.

A `pre-push` hook runs it for you. Use `git push --no-verify` to skip it.

`golangci-lint` also enforces complexity limits (`cyclop`, `gocognit`, `maintidx`, `funlen`, `nestif`) and copy-paste detection (`dupl`), tuned to what the tree already passes. If a function genuinely can't fit — a flat, table-driven parser, say — add a targeted `//nolint:<linter> // reason` rather than loosening the limit in `.golangci.yml`.

## Releasing

Merge to `main` and wait for the night. That is the whole process.

`.github/workflows/tag.yml` runs `cog bump --auto` on a nightly schedule. Cocogitto reads the commits since the last tag and picks the next version from their prefixes — `feat:` bumps the minor, `fix:` the patch, a `BREAKING CHANGE:` footer the major — then tags `main` and pushes the tag. A day's merges ship as one release rather than one release each.

A night with nothing but `docs:`, `chore:`, `ci:` and the like is not a release, and neither is a night with no merges at all. `cog` tags nothing and exits cleanly, and the release job is skipped. That exemption is cocogitto's own rule, not a pattern match in the workflow, so `feat!:` and breaking-change footers are read correctly.

To release without waiting, run the **Tag** workflow from the Actions tab. Its `level` input defaults to `auto`, the same choice the schedule makes; set it to `patch`, `minor` or `major` to force a version no commit warrants.

The workflow calls `release.yml` directly rather than letting the pushed tag trigger it. A push authenticated with the default `GITHUB_TOKEN` does not fire other workflows, so a tag trigger alone would never run. `release.yml` keeps its `v*` trigger for tags pushed by hand.

The same bump runs from your own machine:

```shell
$ mise run release
```

Add `--dry-run` to print the version it would pick and stop. It refuses to run on a dirty tree.

Config lives in `cog.toml`. `pre_bump_hooks` runs `mise run verify` and aborts the release if it fails. No changelog is written — GoReleaser generates the release notes from the same commits, and a second copy would drift.

The tag must start with `v` — Go modules require the prefix, and `release.yml`'s own trigger only matches `v*`. `tag_prefix` in `cog.toml` handles that.

From there GoReleaser cross-compiles linux, darwin and windows on amd64 and arm64, publishes a GitHub release with the archives and checksums, writes the release notes from the commit prefixes, and commits an updated cask to the [tap](https://github.com/madsboddum/homebrew-swg-cli). Nothing needs to be built, uploaded or edited by hand, and the tap is never edited directly.

`swg version` reports the tag, `v` and all, stamped via ldflags. That is `.Tag` rather than GoReleaser's `.Version`, which strips the prefix. Keeping it matches the pseudo-version an untagged local build falls back to, so both look alike.

To check the release config without cutting a release:

```shell
$ mise run release-check
```

## Code layout

Format packages live at the repo root (`tre/`, `stf/`) so they stay importable on their own. Keep flag parsing, output formatting and exit codes in `cmd/swg/`.

## Decisions

Design choices that the code cannot explain on its own are recorded in [`docs/decisions/`](docs/decisions/) — why `cat` sniffs the format rather than being told it, why every command is a verb, why there is no in-place archive editing.

Read them before changing the shape of the CLI. They exist so a rejected option is not quietly reintroduced as a fix. If you disagree with one, supersede it with a new file rather than editing the old one — the trail is the point.
