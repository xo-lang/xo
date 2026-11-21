# An OS specific amd64 lowering or a faster runtime loop broke a guard only the Mac tier runs

Status: Observed (2026-11-21, task #581, the 0.55.0 candidates 6 and 7)

Symptom: two internal/arm64 tests failed at every commit of main since
2026-11-18, seen first in the release candidate's Mac run:
TestWindowsAmd64Identity (103 "windows/amd64 differs from linux/amd64"
lines, e.g. linux `movq %fs:0, %r10` against windows
`callq xo_f_cabi_2ectx__unwinding`, and frame offsets in the Map log
helpers) and TestTextCost/trim (801M instructions over a 788M ceiling).
Both are not `-short` and the per push gates do not run them; the
instruction count is darwin only.

Cause: no wrong code. Three changes were right and the tests'
expectations were stale:

- 0eea20286 reads the unwinding word inline on linux and freebsd amd64;
  Windows still calls ctx.unwinding (scripts/bisect.sh named it).
- 7a308f44a splits a call free loop at 6 live registers on System V and
  at 8 on Windows (loopsplit.xo split_min_live).
- 5b364e9c3 made scan.in_set branch free: faster (host 1 --ab 0.938)
  with more instructions, so trim_chars' one pass form moved from 774M
  to 801M, the old count of the two call form the ceiling guards
  against (scripts/bisect.sh named it).

Fix: the inline unwinding check is the optional pass x86_unwind_inline
(on by default); the identity test turns it off for Linux and loop_split
off for both. The trim ceiling is 850M, between the one pass form (801M)
and the two call form measured on today's runtime (901M).

Avoid: an amd64 lowering that depends on the OS goes behind a pass name
and into TestWindowsAmd64Identity's XO_PASSES in the same change, and a
change to a loop a cost test guards reruns that test on the Mac
(`go test ./internal/arm64 -run 'TestWindowsAmd64Identity|TestTextCost'`).
