# 0001. Reading sniffs the format; `--format` overrides

Status: Accepted
Date: 2026-08-30

## Context

The archives hold several formats. String tables decode to key/value lines, IFF files to a node tree, datatables to rows. `swg cat` has to decide which renderer to use.

The file already answers the question. String tables are named `.stf`, IFF containers open with a `FORM` magic, datatables with a `DTII` form inside it. The bytes declare what they are before anything is asked of them.

## Decision

`cat` sniffs. The user names a path and gets a sensible rendering of whatever is there, without saying what it is.

A `--format` flag overrides the sniff, for the cases where it guesses wrong or where a lower-level view is wanted — rendering a `DTII` as a raw node tree rather than as rows, say.

## Consequences

`swg cat 'string/en/**.stf'` works, and so does a glob spanning several formats in one invocation. The user does not have to know the answer before asking the question.

Each new format package adds a sniff rule, and the sniff has to stay cheap — it runs on every file a glob matches.

An unrecognised or malformed file must degrade rather than fail. A `DTII` that does not parse as rows still renders as a node tree; a file that is nothing recognisable still renders as bytes.

## Rejected

**Naming the format on the command line**, as `swg iff cat <path>`. It forces the user to know the format up front, makes a category error out of an ordinary typo — `.stf` is not IFF — and breaks mixed globs outright, since one fixed noun cannot cover a tree.
