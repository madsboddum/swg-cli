# 0004. Command vocabulary: `cat`, `extract`, `pack`

Status: Accepted
Date: 2026-08-30

## Context

The read verbs came from Unix without much thought — `ls`, `cat`, `which`, `stat` all mean here roughly what they mean in a shell, and a reader guesses right on the first try.

The write verb had no such gift. Unix never settled on one. Archive creation is either the `mk*` family (`mkfs`, `mksquashfs`, `mkisofs`) or a flag local to the tool (`tar c`, `ar r`, `cpio -o`). There was nothing obvious to borrow, and the name had to be chosen rather than inherited.

## Decision

- `cat` — write a rendering of a path to standard output.
- `extract` — write paths out to a directory tree.
- `pack` — build a new archive from a directory tree.

## Consequences

`pack` and `extract` are not a matched pair the way `pack`/`unpack` would be. That is deliberate: `extract` reads the whole precedence stack — loose files, then archives in order — while `pack` writes a single file. Different scope, different word.

`cat` renders rather than dumping bytes, which is a departure from the shell's `cat`. `extract` is the one that reproduces bytes exactly, so the distinction has somewhere to live.

## Rejected

**`create`** — tar's word, and the first choice. It reads as vague once the tool understands the formats inside the archives as well as the archives themselves: create *what*? `--format` answers that (#0002), but the verb should not need the flag to be legible.

**`write`** — collides with what `cat` already does. `swg write` sounds like it writes to standard output.

**`build`** — implies deriving something through a toolchain. Nothing is compiled here; existing bytes are packed.

**`mktre`** — the most Unix-authentic of the lot, rejected under #0002 for welding the format into the command name.
