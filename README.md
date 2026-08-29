# swg-cli

`swg` is a command line tool for working with the file formats found in the game Star Wars Galaxies.

It is a single binary with a subcommand per task, in the style of `git` or `go`.

## Status

Early. Only the skeleton exists — `swg version` and `swg help`. Format support is being added subcommand by subcommand.

## Installation

Download a binary from the [Releases page](https://github.com/madsboddum/swg-cli/releases), or build from source:

```shell
$ go build -o swg ./cmd/swg
```

Building needs nothing installed but Go — `go.mod` pins the toolchain, so the right version is fetched on demand. Development tooling is pinned in `mise.toml`; see [CONTRIBUTING.md](CONTRIBUTING.md).

### Globally available in shells

```shell
$ sudo ln -s /path/to/swg /usr/local/bin/swg
```

## Usage

```shell
$ swg <command> [arguments]
$ swg help <command>
```

### Intended subcommands

| Command | Purpose |
| --- | --- |
| `tre` | List and extract entries from `.tre` archives |
| `stf` | Read string table files and print their entries |
| `iff` | Inspect the node tree of IFF-based files |
| `version` | Print the version |
| `help` | Show usage |

The table is a plan, not a promise. Only `version` and `help` are implemented today.

## Layout

Format packages live at the repo root (`tre/`, `stf/`) so they can be imported directly and extracted into standalone repos later. CLI concerns — flag parsing, printing, exit codes — stay in `cmd/swg/`.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). Commit messages follow [Conventional Commits](https://www.conventionalcommits.org/).

## License

GPL-3.0. See [LICENSE](LICENSE).
