# ThreadSanitizer misses the edges of the runtime's reference count paths

Status: Observed (2026-10-30, TestTSanSharedMaps after a81de74f2)

Symptom: `go test ./internal/llvm -run TestTSanSharedMaps` reported a
data race in every variant (map_log 0, 1, branch) from a81de74f2 on: a
`with_read` snapshot of a shared Map read in one task against another
task's `with` put, with the race's stacks in the program's reads and
writes. It looked like a value semantics miscompile (a snapshot whose
contents change under a reader), and the bisect named a borrow
inference change (used_after in compiler/ssa/borrow.xo) that lets
Mutex.with move its payload instead of retaining every leaf.

Cause: the semantics were sound (the snapshot program on arm64, LLVM
and Go, and an 8 worker stress of 10000 rounds without --race, never
saw a changed snapshot). ThreadSanitizer saw no happens-before edge
between the writer's release of the shared value and the reader's
reuse of it, because the runtime (std/native/llrt) is not instrumented
under --race (decision 0273, race_unchecked) and the two edges live in
hand written runtime paths:

- `rc.claim` in obj.xo rl_copy, which proves a shared count is the last
  reference with an acquire re-load before the value is mutated in
  place (the `reuse` slow path);
- the shared count decrement in the compiler's generated drop helpers
  (compiler/arm64 rc.xo dec_text, `_slow`: `ldaddal`), which is
  generated text and not a runtime function, so neither the LLVM
  backend's sanitizer nor the arm64 backend's instrumentation sees it.

Before a81de74f2 the with path retained every leaf of the payload and
the counts never reached the claim, so the gap never showed; moving the
payload made the reader's reuse (claim) and the writer's last drop meet.

Fix (xl104, 2026-10-30): the runtime's rc paths are annotated by hand.
obj.xo rl_copy calls `sanhook.acquire(l)` after a successful claim (and
the string reserve path the same); a shared decrement under --race goes
through `obj.dec_shared_race` (`sanhook.release` before the atomic
decrement, `sanhook.acquire` after it when it was the last reference),
exported as `xo_x_obj_dec_shared_race` and called by the generated drop
helpers only when the build is a race build (rc.xo shared_dec); a
plain build keeps the inline `ldaddal`. The edges are the ones Go's
runtime gives its sync primitives and LLVM gives an instrumented atomic.

Avoid: a new hand written runtime path that transfers ownership between
threads through a count or a flag (a claim, a last-reference check, a
park handoff) needs its own `sanhook.release` / `sanhook.acquire` pair,
since --race never instruments llrt; test it with a TestTSan* case that
moves a value across tasks, not only with the program goldens. A
"snapshot changed" report under --race in a program whose plain runs
are sound is this gap until proven otherwise: run the semantics program
without --race on every backend first (gotcha ordering: breakdown
before hypotheses).
