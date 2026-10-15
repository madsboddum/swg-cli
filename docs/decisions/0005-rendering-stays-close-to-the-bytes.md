# 0005. Rendering stays close to what is stored

Status: Accepted
Date: 2026-10-15

## Context

`cat` decodes formats into text, and every decoder faces the same pull: make the output friendlier by interpreting it. A draft schematic's attribute is stored as `@crafting:minDamage`, and the string table next door says that means "Minimum Damage". A range stored as `35..35` could print as `35`. An empty string id `@:` could print as a dash.

Each of these reads better, and each hides something. The resolved string no longer says which table and key the file names. The collapsed range no longer says the file stores a range. The dash reads the same as a parameter that was never set.

The people reading this output are working out what the client was shipped with, for documentation and for modding. What the file says is the answer they came for; an interpretation of it is a second claim they would have to verify.

## Decision

Rendering shows what is stored, in a readable shape, and stops there.

A decoder may lay data out, name its fields and label opaque tags, so `SDSC` prints as `SDSC (draft schematic)`. It does not replace a stored value with what that value refers to or means: string ids stay string ids, numbers stay the numbers stored, and two encodings the file keeps distinct stay distinct on screen.

## Consequences

Output is a faithful reading of the file, which a person or an LLM can quote back as fact and compare against the bytes with `-format hex`.

It is less convenient than it could be. Looking up what `@crafting:minDamage` says, or what `gameObjectType = 8203` stands for, is a second step: `swg cat` the string table, or consult the client's enums.

## Rejected

**Resolving references in place**, such as printing a string id's English text beside it. Even appended rather than substituted, it makes the output a blend of two files, and it leans on the string table of one locale.

**Normalising encodings**, such as collapsing an equal-ended range to a single value. The file chose the encoding; the rendering does not get to un-choose it.
