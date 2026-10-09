# Aligning loop tops on Apple M4 makes speed stable, not faster

Status: Observed (2026-10-08, internal/arm64, bench/perf/llvm on Apple
M4 Pro)

Symptom: aligning the top of every innermost loop to 64 bytes (padding
jumped over, functions aligned to 64) removed collatz's and nbody's
sensitivity to where the code lands (spread over 16 shifts of the code:
collatz 2.15 to 0.74 percent, nbody 1.12 to 0.20 percent, against 0.57
and 0.26 for identical binaries), yet both got slower than the
unaligned code's median (collatz 1.0 percent, nbody 0.8 percent), and
sort got slower and more sensitive (0.75 to 1.66 percent spread). 16 and
32 byte loop alignment, outer loops, and running through the nops
instead of jumping over them were no better.

Cause: the speed of these loops depends on where particular instructions
fall, not on the loop starting at a line boundary. collatz is fastest
with its 13 instruction loop starting 44 bytes into a 64 byte line
(crossing the boundary); sort's scan loops prefer an 8 byte aligned top;
nbody's inner loop is 688 to 694M cycles at every offset of its top but
684M at most unaligned shifts. Aligning picks one fixed point, which is
not the good one for any of them. A `b` over the padding also costs a
taken branch per loop entry, which matters for nbody's inner loop (1 to
4 iterations per entry).

Fix: none by default (decision 0075: no alignment). To judge a code
shape change, do not align; sweep `XO_ARM64_LAYOUT=pad=K` (K = 0 to 15)
and compare medians over the shifts, with 16 copies of one binary as
the noise floor. Only collatz, nbody, and sort have spreads above run
noise on a shared host.
