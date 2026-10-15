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

### Reading a palette

```shell
$ swg cat palette/creature_frog.pal
index	hex	r	g	b
0	#5d3534	93	53	52
1	#6a3e3d	106	62	61
2	#794a49	121	74	73
...
```

On a terminal each line gets a swatch of the colour itself and the columns line up. Redirect the output, or pass `-color never`, for the tab-separated form above.

### Reading the inside of a building

A `.pob` portal object is the interior of anything a player can walk inside: the cells it is divided into and the portals between them. It prints as two tables, cells first:

```shell
$ swg cat appearance/thm_newbie_hall.pob
cell	name	appearance	floor	lights
0	r0	appearance/mesh/thm_newbie_hall_r0_mesh_r0.msh	appearance/collision/thm_newbie_hall_r0_floor0.flr	0
1	r1	appearance/mesh/thm_newbie_hall_r1_mesh_r1.msh	appearance/collision/thm_newbie_hall_r1_collision_floor0.flr	7
...

cell	portal	target	passable	disabled	clockwise	door	vertices	x	y	z
0	0	13	true	false	true		4	52.88	17.46	-415.95
1	1	2	true	false	true		4	0.00	3.50	-28.00
1	2	12	false	false	true		4	-12.07	3.08	-3.01
...
```

Each portal is listed from both sides, once per cell it joins, and its `x`, `y` and `z` are the centre of the opening in the building's own coordinates.

### Reading an object template

The files under `object/` are object templates: what the client knows about an item, a creature, a building or a draft schematic. Each is a stack of layers, one per class in the client's template hierarchy, and prints most specific first, with the parameters each layer sets. A list of structs, like a draft schematic's attribute ranges, gets one line per struct:

```shell
$ swg cat object/draft_schematic/weapon/shared_pistol_blaster_cdef.iff
SDSC (draft schematic) 0003, derived from object/draft_schematic/base/shared_base_draft_schematic.iff
  slots:
    name=@craft_weapon_ingredients_n:frame_assembly hardpoint=""
    ...
  attributes:
    name=@crafting:minDamage experiment=@crafting:expDamage value=17..34
    name=@crafting:maxDamage experiment=@crafting:expDamage value=51..67
    ...
  craftedSharedTemplate = "object/weapon/ranged/pistol/shared_pistol_cdef.iff"

SITN (intangible) 0000, derived from object/draft_schematic/base/shared_base_draft_schematic.iff
  (nothing set)

SHOT (object) 0010, derived from object/draft_schematic/base/shared_base_draft_schematic.iff
  (nothing set)
```

Parameters a template leaves unset are left out, since the value comes from the parent it derives from. What is set prints as stored: string ids stay string ids rather than the text they name, and `@:` is an empty one.

### Reading the raw bytes

`-format hex` skips the decoding above and dumps the bytes, sixteen to a line as an offset, hex and ASCII. An IFF container keeps its node tree, with each chunk dumped in full beneath it and offsets counted from the start of the chunk. It is the view to work from when a form is not decoded yet:

```shell
$ swg cat -format hex object/draft_schematic/weapon/shared_pistol_blaster_cdef.iff
FORM SDSC (2825 bytes)
  FORM DERV (72 bytes)
    XXXX (60 bytes)
      00000000  6f 62 6a 65 63 74 2f 64  72 61 66 74 5f 73 63 68  object/draft_sch
      00000010  65 6d 61 74 69 63 2f 62  61 73 65 2f 73 68 61 72  ematic/base/shar
      00000020  65 64 5f 62 61 73 65 5f  64 72 61 66 74 5f 73 63  ed_base_draft_sc
      00000030  68 65 6d 61 74 69 63 2e  69 66 66 00              hematic.iff.
  FORM 0003 (1913 bytes)
    PCNT (4 bytes)
      00000000  03 00 00 00                                       ....
...
```

Anything that is not a string table, a datatable, a palette or a portal object is written out as the bytes it holds, so pulling one file out is a redirect:

```shell
$ swg cat texture/lambda_glass.dds > lambda_glass.dds
```

### Extracting a tree onto disk

`extract` is the other half of `cat`: it resolves paths the same way, then writes them out as files rather than to standard output. `-C` says where they land and the archive path becomes directories below it.

```shell
$ swg extract -C ./out 'string/en/**.stf'
string/en/badge_d.stf
string/en/badge_n.stf
...
```

Bytes are written exactly as stored, whatever the format, so nothing is decoded and an existing file is overwritten. Each path is printed as it is written, so the output is a manifest of what landed on disk; `-q` silences it.

`-archive` extracts everything one archive holds, ignoring precedence:

```shell
$ swg extract -C ./out -archive patch_02.tre
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
| `cat` | Decode paths from the archives into readable text |
| `extract` | Write paths from the archives out as files |
| `ls` | List paths across the archives |
| `which` | Show which archive a path is read from |
| `version` | Print the version |

```shell
$ swg <command> --help
```

## Layout

Format packages live at the repo root (`tre/`, `toc/`, `stf/`, `archive/`) so they can be imported directly and extracted into standalone repos later. CLI concerns like flag parsing, printing and exit codes stay in `cmd/swg/`.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). Commit messages follow [Conventional Commits](https://www.conventionalcommits.org/).

## License

GPL-3.0. See [LICENSE](LICENSE).
