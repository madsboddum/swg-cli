# 0002. Top-level commands are verbs, format is a flag

Status: Accepted
Date: 2026-08-30

## Context

Reading can sniff the format (#0001). Writing cannot — nothing exists yet to inspect, so the format has to be supplied. That asymmetry has to live somewhere in the interface.

The obvious place was a noun namespace for the write side: `swg tre pack`, `swg stf pack`. It works, but it puts nouns and verbs at the same level and leaves two different rules for how a format gets named, depending on which direction you are going.

## Decision

Every top-level command is a verb: `ls`, `cat`, `which`, `stat`, `extract`, `pack`. No command is a format name.

Format is a flag wherever it is needed:

```shell
$ swg cat --format iff foo.iff
$ swg pack --format tre -o patch_03.tre ./out
```

One rule covers both directions: reading defaults the format by sniffing, writing requires it. That is the whole of the asymmetry.

## Consequences

The top level stays small and stays a verb list, however many formats land. A new format package adds a `--format` value, not a command.

`--format` is required for `pack`, which is slightly more typing than a noun command would be, and the flag has to reject values that make no sense for the verb.

Commands that operate across the whole precedence stack — every read verb, plus `extract` — stay format-agnostic and need no noun at all.

## Rejected

**Noun-scoped subcommand groups**, `swg tre pack` and `swg stf pack`. Coherent, but it mixes nouns and verbs at the top level, and it invites the symmetric version on the read side, which #0001 rejects for good reasons.

**Welding the format into the command name**, `swg mktre`. The most authentically Unix option, and the least extensible — it is a new top-level command per format, forever.
