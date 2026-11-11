# A merged std module's wire-named fields come out private

Status: Observed (2026-11-11, Xo Lang 3: std/trace's merge, 1b13a8dbb)

## Symptom

After a std module merges (decision 0588), a program that reads a field
of one of its structs stops building:

```
error XO0605: `parent_id` of `trace.Span` is not pub
```

The module's own tests and corpus pass, since they live inside the module.

## Cause

In a declaration file every field is public. A merged file is ordinary
Xo, where a field needs `pub`. The merge helper added `pub` with a pattern
for `name: Type` and missed `name as "wire": Type` (decision 0277's wire
names), so every field with a wire name became private: trace's Span lost
thirteen, Event one.

## Fix

Write `pub name as "wire": Type`. The merge helper now matches the wire
name form and compares the merged file's public fields with the
declaration file's before it writes. Before pushing a merge, build a
golden or program that uses the module from outside (here trace_spans),
not only the module's own tests.
