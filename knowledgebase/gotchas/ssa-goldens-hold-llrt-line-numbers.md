# SSA goldens hold the line numbers of std/native/llrt modules

Status: Observed (2026-10-13, decision 0198's second round).

## Symptom

`go test -short ./internal/ssa` fails in TestGolden with "SSA differs
from testdata/golden/<name>/want.ssa" for about a dozen goldens after an
edit that only rewrote a comment in std/native/llrt/show.xo (or
std/native/prelude.xo).

## Cause

The SSA goldens print the runtime modules a program pulls in, with the
source position of each checked operation (`@ show.xo:74:43`). A comment
that gains or loses a line shifts every position below it, so the
goldens differ although no code changed. On 2026-10-13 the goldens
under internal/*/testdata held positions of show.xo (84) and prelude.xo
(3); num.xo, text.xo, netpolicy.xo, cabi.xo, scan.xo, rc.xo, sched.xo,
raw.xo, num_tables.xo, unicode.xo, and sock.xo appear in `@ <file>`
form too.

## Fix

Keep the line count of an edited header comment in those modules the
same (reflow it), or regenerate the goldens in the same change. Find the
modules goldens reference with
`grep -rhoE "[a-z_]+\.xo:[0-9]+" internal/*/testdata | sed 's/:.*//' | sort | uniq -c`.
