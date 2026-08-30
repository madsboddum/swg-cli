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

## Releasing

Push a tag. That is the whole process.

```shell
$ git tag -a v0.2.0 -m v0.2.0
$ git push origin v0.2.0
```

The tag must start with `v` — Go modules require the prefix, and `.github/workflows/release.yml` only triggers on `v*`. Push the commit before the tag, since the workflow builds the tagged commit from the remote.

From there GoReleaser cross-compiles linux, darwin and windows on amd64 and arm64, publishes a GitHub release with the archives and checksums, writes the release notes from the commit prefixes, and commits an updated cask to the [tap](https://github.com/madsboddum/homebrew-swg-cli). Nothing needs to be built, uploaded or edited by hand, and the tap is never edited directly.

`swg version` reports the tag without the `v`, stamped via ldflags.

To check the release config without cutting a release:

```shell
$ mise run release-check
```

## Code layout

Format packages live at the repo root (`tre/`, `stf/`) so they stay importable on their own. Keep flag parsing, output formatting and exit codes in `cmd/swg/`.
