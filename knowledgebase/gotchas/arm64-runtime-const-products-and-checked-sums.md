# Runtime Xo: constant products are not folded and sums are checked on arm64

Status: Observed (2026-10-14, work item B03, std/native/llrt/stdio.xo).

Symptom: the first Xo version of standard output's buffer was 1.36
times the C path's instructions for 1,000,000 println on arm64 (1.10 on
LLVM). Its disassembly had `mov x10, #1; mov x9, #8; mul x9, x10, x9`
before every `ldr` of a word of its state, and `adds` plus `b.vs` (a
branch to an overflow fault) after every sum.

Cause: the self hosted arm64 emitter does not fold a product of two
named constants used as an argument (`rt.mem_load(w, LEN * 8, 8)` with
`const LEN = 1`), and Xo's `+` is checked (core 8: overflow faults), so
each sum of addresses or lengths carries a compare and branch, and the
fault path's code. LLVM folds both.

Fix: in runtime modules, write byte offsets as literal constants
(`const LEN = 8`, then `rt.mem_load(w, LEN, 8)`) and sums that cannot
overflow (sizes, addresses) with `rt.int_wrap(0, a, b)` (and
`rt.int_wrap(1, a, b)` for a difference). Keep the common path short and
separate from the rare one (stdio.xo's `out` and `out_slow`). With
these, the same workload is 0.788 of the C on arm64
(bench/results/stdio-2026-10-14.md).
