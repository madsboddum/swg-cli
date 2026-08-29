# swg-cli

`swg` is a command line tool for the file formats found in Star Wars Galaxies.

## Verification

Run `mise run verify` before reporting any code change as done. It runs gofmt, `go vet`, `golangci-lint`, tests and a build — the same set CI runs. Report failures rather than working around them.

Individual steps exist as `mise run fmt`, `vet`, `lint`, `test`, `build`.

## Layout

- `cmd/swg/` — CLI concerns only: flag parsing, output formatting, exit codes.
- Repo root (`tre/`, `stf/`, …) — format packages, one per format. They stay importable on their own so they can be extracted into standalone repos later, so they must not depend on `cmd/swg/`.

Subcommands are entries in the `commands` table in `cmd/swg/command.go`, each with a `run(args []string, stdout, stderr io.Writer) int`. Return an exit code; do not call `os.Exit` outside `main`.

## Tooling

The Go version lives in `go.mod`'s `toolchain` directive and nowhere else. Never set `GOTOOLCHAIN=local` — it disables the automatic fetch of the pinned toolchain.

Non-Go tools are pinned in `mise.toml`. Do not add `tool` directives to `go.mod`; golangci-lint's maintainers advise against installing it that way.

## Commits

Conventional Commits — `feat:`, `fix:`, `chore:`, `docs:`, `refactor:`, `test:`. Scope is the package or subcommand, e.g. `feat(tre): add extract subcommand`. Release changelogs are generated from these prefixes.

When the work comes from an issue, add a `Refs: #N` footer, or `Closes: #N` on the commit that finishes it.

A `commit-msg` hook enforces this with `cog verify`, and a `pre-push` hook runs `mise run verify`. Both are wired by `mise install`. Do not pass `--no-verify` to work around a failing hook; fix the message or the code.
