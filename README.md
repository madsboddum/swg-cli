# swg-cli

`swg` is a command line tool for the file formats found in the game Star Wars Galaxies.

It reads `.stf` and everything else straight out of the `.tre` archives, so there is nothing to extract first. Point it at a game directory and the whole tree is one namespace, resolved the way the client resolves it.

It is a single binary with a subcommand per task, in the style of `git` or `go`.

## Installation

Install from the Homebrew tap:

```shell
$ brew install madsboddum/swg-cli/swg
```

That works on macOS and Linux.

Otherwise download a binary from the [Releases page](https://github.com/madsboddum/swg-cli/releases), or build from source:

```shell
$ go build -o swg ./cmd/swg
```

Building needs nothing installed but Go. `go.mod` pins the toolchain, so the right version is fetched on demand.

### Globally available in shells

```shell
$ sudo ln -s /path/to/swg /usr/local/bin/swg
```

### Shell completion

```shell
# bash
$ swg completion bash > /usr/local/etc/bash_completion.d/swg

# zsh
$ swg completion zsh > "${fpath[1]}/_swg"

# fish
$ swg completion fish > ~/.config/fish/completions/swg.fish
```

Restart the shell, or source the file, to pick it up. It completes subcommands,
flags, `-archive` names, and paths inside the archives a directory at a time:

```shell
$ swg cat str<TAB>
$ swg cat string/<TAB>
string/en/  string/ja/
$ swg cat string/en/ba<TAB>
badge_d.stf  badge_n.stf
```

## The archive directory

Every subcommand works against a directory of `.tre` archives, meaning a game install. Name it with `--dir`, after the subcommand:

```shell
$ swg ls --dir ~/Games/ProjectSWG/CU
```

Or set `SWG_DIR` once and drop the flag:

```shell
$ export SWG_DIR=~/Games/ProjectSWG/CU
$ swg ls
```

The flag wins over the environment. Without either, commands exit 2:

```shell
$ swg ls
swg ls: no archive directory set; pass --dir or set SWG_DIR
```

## Examples

### Reverse string lookup

The main reason to reach for this. You have a line of text from a screenshot or an old video and you need the string id an emulator can reference. `cat` decodes string tables to `@file:key|value`, one entry per line, so `grep` does the searching:

```shell
$ swg cat 'string/en/**.stf' | grep -i "you have been incapacitated"
@base_player:prose_victim_incap|You have been incapacitated by %TT.
```

Quote the pattern, or the shell expands it first. `*` and `?` match within a path segment, `**` across segments, and patterns match whole paths.

That is every English string in the game in one stream:

```shell
$ swg cat 'string/en/**.stf' | wc -l
222838
```

### Listing

With no argument, `ls` lists the top level across every archive:

```shell
$ swg ls
GLCache/
abstract/
animation/
appearance/
...
```

A directory lists its immediate entries, subdirectories marked with a trailing slash:

```shell
$ swg ls string/
string/en/
string/ja/

$ swg ls string/en/quest | head -4
string/en/quest/corvetter_trap.stf
string/en/quest/crafting_contract/
string/en/quest/crowd_pleaser/
string/en/quest/force_sensitive/
```

Patterns work here too:

```shell
$ swg ls 'string/*/badge_n.stf'
string/en/badge_n.stf
string/ja/badge_n.stf

$ swg ls '**.stf' | wc -l
6628
```

`-archive` narrows the listing to one archive, ignoring precedence:

```shell
$ swg ls -archive bottom.tre '**' | wc -l
808
```

### Reading a single file

```shell
$ swg cat string/en/space/space_faction.stf
@space/space_faction:blacksun|Black Sun
@space/space_faction:civilian|Civilian
@space/space_faction:corsair|Corsair
@space/space_faction:default|
@space/space_faction:hutt|Hutt
...

$ swg cat string/en/badge_n.stf | wc -l
185
```

Anything that is not a string table is written out as the bytes it holds, so extracting is a redirect:

```shell
$ swg cat texture/lambda_glass.dds > lambda_glass.dds
```

### Which archive a path comes from

```shell
$ swg which string/en/combat_effects.stf
string/en/combat_effects.stf  ->  patch_24_shared_00.tre
```

`-all` lists every archive holding it, winner first:

```shell
$ swg which -all shader/e_invisible_collidable.sht
shader/e_invisible_collidable.sht
  patch_sku3_24_client_00.tre  (winner)
  patch_sku1_24_client_00.tre
  patch_24_client_00.tre
```

## Precedence

A path can live in several archives. Loose files in the game directory beat every archive; among archives, filename sort order decides and the last one wins, so `patch_02.tre` shadows `patch_01.tre`. `cat` reads the winner, `ls` lists each path once, and `which -all` shows the full order.

Only files inside subdirectories count as loose overrides. Files sitting directly in the game directory are the client's own: executables, configuration, the archives themselves. None of that is game data.

Archives that carry no index of their own are read through the client `.toc` files beside them.

## Supersedes

This replaces the separate [stf](https://github.com/madsboddum/stf) and [toc](https://github.com/madsboddum/toc) tools. Both required Java and operated on files already extracted from an archive; `swg` needs neither.

The `@file:key|value` output format carries over from the stf tool, minus the `.stf` extension in the id, so lines read the way the game's own data writes a string id.

## Commands

| Command | Purpose |
| --- | --- |
| `cat` | Write paths from the archives to standard output |
| `ls` | List paths across the archives |
| `which` | Show which archive a path is read from |
| `version` | Print the version |
| `help` | Show usage for swg or a subcommand |

```shell
$ swg help <command>
```

## Layout

Format packages live at the repo root (`tre/`, `toc/`, `stf/`, `archive/`) so they can be imported directly and extracted into standalone repos later. CLI concerns like flag parsing, printing and exit codes stay in `cmd/swg/`.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). Commit messages follow [Conventional Commits](https://www.conventionalcommits.org/).

## License

GPL-3.0. See [LICENSE](LICENSE).
