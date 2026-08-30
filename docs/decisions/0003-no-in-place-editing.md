# 0003. Writing produces a new archive; no in-place editing

Status: Accepted
Date: 2026-08-30

## Context

Once `swg` can write a `.tre`, the natural next ask is editing one — replace a member, add a file, drop a file. `tar` and `zip` both offer it, so the expectation comes ready-made.

The client does not work that way. `base.tre` is never mutated. A change ships as a higher-numbered archive that shadows the old path, and the precedence rules resolve it at load time. Patching *is* the edit mechanism.

## Decision

There is no in-place member editing. The write side is one verb that builds a whole archive from a directory tree.

The round trip is `extract` → edit the files with ordinary tools → `pack`.

## Consequences

The filesystem is the editor. Every tool the user already has works on the extracted tree, and `swg` does not have to grow a member-replacement path, an append mode, or a way to rewrite an index in place.

It also matches how the result gets used. An archive built this way is a patch archive, which is what the client expects to load.

Extracting a large tree to change one file is more I/O than an in-place edit would be. That is the cost, and it is paid on a tool run, not on the client.

`pack` never modifies its inputs, so a bad build costs an output file and nothing else.

## Rejected

**tar's full verb family** — `create`, `add`, `update`, `delete`. Three of the four have no counterpart in how the client loads data, so they would be `swg` inventing a capability the format's own ecosystem does not use.
