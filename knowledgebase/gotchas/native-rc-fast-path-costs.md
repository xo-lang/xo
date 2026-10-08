# Small runtime changes cost the strings benchmark several percent

Status: Observed (2026-10-08, decision 0058, bench/perf/llvm strings)

Symptom: the first version of thread safe counts and per thread heaps
made `strings` 12% slower on both native backends (wall time), although
the added work was a few instructions per allocation. Each fix below
was found by relinking the same generated code with runtime variants
(`clang prog-darwin.s variant.c`) and comparing process cycles.

Causes, each observed:
- A `noinline` call on a cold path (`heap_slow()` in `xo_free`) made the
  function non leaf: it saved registers and the frame on every call.
  Fix: make the threaded path a tail call (`return alloc_mt(n)`), so the
  single threaded path stays a leaf.
- clang's SLP vectorizer merged `live++; total++` on adjacent fields into
  one 128 bit load, add, and store; `xo_free` then stores `live` with 64
  bits, which keeps the next 128 bit load from store forwarding (seen in
  the disassembly; its share of the cost was not measured on its own).
  Fix: one counter per call (`allocs` in `xo_alloc`, `frees` in
  `xo_free`; the leak check reports the difference).
- `__builtin_expect` on the release made clang split the non unique
  branch of `rs_append` into `rs_append.cold.1`, a real call; appending
  to a literal (`"w${i}"`) took it 2 million times. Fix: test the
  immortal count first, inline.
- With plain counts LLVM folded `retain w` and the release of the
  `Option` it came from (`if let Some(w) = xs.pop()`) into almost
  nothing; the branches of the shared paths hide the pair from it. Fix:
  `ssa.CancelledRelease`, which the arm64 backend already did, in the
  LLVM backend too.
- Changing only the literal's count (2^60 to INT64_MIN) moved cycles by
  2% with the same instruction count (branch layout); and code layout
  alone moves fib or nbody by up to 2% with identical instructions.

Fix (method): compare `cycles elapsed` and `instructions retired` from
`/usr/bin/time -l` (per process, so other load on the host matters less
than for wall time), minimum of 11 to 31 interleaved runs, and check the
instruction count before believing a cycle difference under 2%. Use
`objdump -d` on the runtime object to see frames, vector stores, and
`.cold` calls.
