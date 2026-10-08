# Fewer instructions in a hot native loop can be slower

Status: Observed (2026-10-08, internal/arm64, bench nbody on Apple M4 Pro)

Symptom: dropping two never taken bounds checks (`cmp`, `b.hs`) from
nbody's `advance` inner loop retired 8 percent fewer instructions and
ran 1.4 percent slower (694 against 684M cycles, interleaved, 7 to 9
runs). Computing an invariant element address before the loop instead
of in it: 0.9 percent slower. Moving `advance` by 4 bytes (a change in
another function) moved it by 0.7 percent either way.

Cause: not known. The loop is bound by a chain through memory (each
iteration stores `bs[i].vx` and the next loads it back) and by
`fdiv`/`fsqrt` latency, not by instruction count. Two `nop`s in place of
the removed check recovered half the loss; aligning `advance` to each of
0 to 28 bytes past a 64 byte boundary did not recover it. The removal
changes when the loads issue relative to the stores of the previous
iteration, which the core's memory dependence prediction is sensitive
to.

Fix (workaround): measure every code shape change on every bench
program, in cycles (`/usr/bin/time -l`, "cycles elapsed") as well as
wall time; keep a change only if no program is slower. To tell a layout
artifact from a real cost, rebuild the variant with the function aligned
at several offsets (`.p2align 6` plus 0 to 7 `nop`s before its label in
the `.s`, linked by hand) and compare the spread. The bounds check
removal was dropped for this reason (decision 0054).
