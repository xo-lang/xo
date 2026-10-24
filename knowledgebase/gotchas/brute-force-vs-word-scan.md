# Porting Go's brute-force search can lose to the word scan already there

Observed 2026-10-24, while closing the `cut` row's search cost
(bench/results/strfind-2026-10-24.md).

## Symptom

Porting Go's `bytes.Index` short-haystack path (its brute force for
`len(s) <= bytealg.MaxBruteForce`, 64 on amd64; on arm64 a register compare
in `IndexString`) into `scan.find_bytes` produced the same answers on every
corpus case and every benchmark, and cost more on the rows that search:
`cut` 19.9% more instructions, `split_after` 6.3%, `strsplit split` 5.9%,
`split_multi` 12.4% and `split_absent` 42.6% (same-revision A/B, 10 rounds,
XO_INLINE_STRICT=1).

## Cause

The port replaced `find_bytes`' skip with a byte-at-a-time candidate walk.
That skip is `find_byte`, a 16-byte word compare (`eq16_1`) of the
separator's first byte, which is what Go's own `bytes.Index` uses
(`IndexByte`) in the same place. Go's brute force wins in Go because its
`Index` is written in assembly that compares the separator in registers; it
is not a statement that a byte loop beats a word scan, and in Xo it does
not.

## Fix

Keep the skip, port only what removes work per candidate: Go's second byte
test before the whole compare (`s[i+1] == c1` ahead of `Equal`), and the
`n == len(s)` case as one compare. That measured 4.6% fewer instructions on
`cut`, 4.6% on `split_multi`, 0.8% on `split_after`, no row worse. Rule of
thumb: before porting a Go search path, find which of its parts is a skip
and which is a compare, and never replace an existing word-wide skip with a
byte loop; port the compare-side savings instead.
