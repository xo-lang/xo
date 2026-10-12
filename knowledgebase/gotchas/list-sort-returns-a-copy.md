# `xs.sort()` returns a sorted copy; a bare call does nothing

Status: Observed (2026-10-12, decision 0178 stage 5, selfhost/cli/servers
json.xo).

## Symptom

A port of Go's `sort.Strings(keys)` written as `ks.sort()` left the keys
in insertion order: the JSON of a decoded `map[string]any` came out
`{"b":...,"a":...}` where Go writes `{"a":...,"b":...}`. The checker
said nothing.

## Cause

`List.sort`, `sort_by`, and the other list operations return a new list
(std.md 2.3, lists are values); there is no in place sort. A call whose
result is dropped is not a diagnostic for a non `Result` value.

## Fix

Assign the result: `ks = ks.sort()`. When porting Go code, read every
`sort.X(s)` and `slices.Sort(s)` as an assignment.
