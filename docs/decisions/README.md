# Decisions

Design decisions that are hard to infer from the code, kept here so the reasoning outlives the conversation that produced it.

One file per decision, numbered, never renumbered. A decision that stops being true is not deleted — it gets a `Superseded by` line and a new file, so the trail stays readable.

Not everything belongs here. A decision earns a file when someone could plausibly arrive later, see the current shape, and "fix" it back to the option that was already rejected.

Where a decision governs specific code, that code carries a one-line comment pointing at the file. Browsing this directory finds the set; editing the code finds the one that matters.

| # | Decision |
|---|---|
| [0001](0001-reading-sniffs-the-format.md) | Reading sniffs the format; `--format` overrides |
| [0002](0002-commands-are-verbs.md) | Top-level commands are verbs, format is a flag |
| [0003](0003-no-in-place-editing.md) | Writing produces a new archive; no in-place editing |
| [0004](0004-command-vocabulary.md) | Command vocabulary: `cat`, `extract`, `pack` |

## Template

```markdown
# NNNN. Title

Status: Accepted
Date: YYYY-MM-DD

## Context

What forced a choice.

## Decision

What was chosen.

## Consequences

What this costs, and what it rules out.

## Rejected

The alternatives, and why not.
```
