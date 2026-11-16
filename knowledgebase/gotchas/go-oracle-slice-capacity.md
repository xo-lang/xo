# A Go oracle's slices carry spare capacity an Xo List does not

Status: Observed (2026-11-16, task #98 template fuzz seed 35)

## Symptom

A fuzz mismatch on an error message about slicing: Go's text/template said
`error calling slice: invalid slice index: 5 > 4`, std/template said
`index out of range: 5`, on `{{$v := slice .mixed 1}}{{slice $v 5}}`.

## Cause

text/template's `slice` checks each index against the slice's capacity,
not its length. The oracle's data came from encoding/json, whose append
growth leaves capacity past the length (5 elements, capacity 8), so index 5
passed the capacity check and failed the i <= j check instead. An Xo List
has no spare capacity, so std/template (correctly, for exact slices) took
the first branch. The difference was the oracle's, not std's.

## Fix

Clip slices the oracle builds before handing them to the Go reference
(`t[:len(t):len(t)]`, scripts/stdcorpus/template.go tmplAny). Any oracle
whose Go reference reads `cap` (reflect's Cap, three-index slices,
text/template's slice) needs the same.

## Also: a cut inside a character (2026-11-16, seed 128)

`slice` of a string can cut a UTF-8 sequence. Go keeps the raw byte (its
urlquery gave `%A9`); an Xo Str cannot hold it, and std/template gives
U+FFFD for each cut byte (decision 0259). The oracle's `slice` is Go's code
with that rule applied (tmplXoSlice, installed with Funcs, which may
replace a builtin). Any oracle function returning a Go string with
invalid UTF-8 needs the same mapping.
