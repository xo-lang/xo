# LLVM keeps the checks of loops whose bounds live in loop phis

Status: Observed (2026-10-12, decision 0192, bench/perf/unicode on the
LLVM backend)

Symptom: Xo code compiled with `--backend=llvm` retired more
instructions than the same work in C through the same clang with full
LTO: the lower compare of `bench/perf/unicode` 1.126x the C path, even
though C and Xo are inlined across the language boundary. The assembly
of the binary searches of `std/native/llrt/unicode.xo` kept `b.vs` after
`hi - lo`, `lo + (hi - lo) / 2`, `m * 12`, `m + 1`, and `b.hs` before
every table read; the UTF-8 decoder kept the checks of `i + k`, `i + 1`,
`j + 1`. The C path has none of these (C's arithmetic is not checked).

Cause: LLVM's value tracking (LVI, CorrelatedValuePropagation) does not
iterate over loop phis, so it never learns that `lo` and `hi` stay in
[0, 328]; nothing in LLVM knows the relation `lo < hi` that bounds the
midpoint; a Str's length is an `extractvalue` of an argument, unbounded,
so `i + k` after `i < n` may still overflow for LLVM. Separately, its
inliner (threshold about 225 at -O2) leaves `decode` and `to_case` (two
call sites each, about 200 instructions) as calls in the case mapping
loop.

Fix: the backend proves what it can itself and tells LLVM the rest
(decision 0192): its own interval analysis (decision 0184's, with facts
from dominating branches and passed bounds checks) lowers proved
arithmetic as `nsw`, and `llvm.assume` gives LLVM the interval of each
binary search midpoint so the checks of the table reads it inlines
later fold away; calls of functions the arm64 cost model inlines are
`alwaysinline` at the call site.

Trap inside the fix, 1: telling LLVM the range of every loop parameter
(`llvm.assume(i >= 0)` in each loop header) made `bench/perf/llvm/xo`
sieve retire 3.37 billion instructions instead of 2.09 (cycles 725
million instead of 409): with the assumes LLVM proved the inner loop
`j += i` free of wrapping, computed its trip count in 128 bits, and
unrolled it around a call of the 128 bit division (`__udivti3`). The unicode
workloads gained nothing from those assumes (the backend proves the
arithmetic itself), so only binary search midpoints are told. Measure
the bench programs, not only the workload being tuned, when adding
assumes.

Trap inside the fix, 2: forcing a callee into a small wrapper can make the
wrapper too big for LLVM to inline into the hot loop. With `to_case`
forced into `prelude.rune_case` (264 instructions, 2 call sites, just
over the copy budget of 500), Rune case mapping went from 222 million
(LLVM's own choices) to 247 million instructions. Compare the whole
chain's inlining, not one call site, when changing the model.

To find such checks: `objdump -d --no-show-raw-insn <binary>` and look
for `b.vs` / `b.hs` branches to `_xo_fault_overflow` and
`_xo_fault_bytes_index` in the hot function; the build directory's
`prog.ll` can be edited by hand (an `llvm.assume`, an attribute) and
rebuilt with `clang -O2 -flto -Wno-override-module -fexceptions
-funwind-tables -isysroot <SDK> prog.ll xo_rt.c` to try a fact before
writing the analysis.
