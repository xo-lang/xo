# A word skip at the top of a byte loop reruns on every byte of dense input

Observed 2026-11-06 (Xo Lang 1, the SIMD survey's html escape step).

## Symptom

`std/native/html.xo` escape_string with `if n - i >= 8 && !esc8(b, i) {
i += 8; continue }` at the top of its byte loops measured 2.44 times
the old wall on bench/perf/html escape (host 1, linux/arm64), while a
clean text would have run faster.

## Cause

The benchmark's text has a special character about every 5 bytes, so
almost every word holds one. When the word test fails the loop handles
one byte and comes back to the top, where it loads and tests the next
word (five broadcast compares) again: the word test runs once per byte,
on top of the byte work.

## Fix

Use the word's mask, not only whether it is zero: jump to its first hit
(`i += rt.ctz(m).shr_logical(3)`), and where the loop only counts, count
the hits of each kind with `rt.popcount` and step a whole word. The
reworked escape_string measured 0.943 of the old wall on the same text.
Measure a word skip on dense input as well as on clean input.
