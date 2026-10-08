# 0058: Reference counts shared between threads: a sign bit, marked on crossing

Status: Accepted (2026-10-08). Implemented in `std/llrt/xo_rt.c`
("Reference counts", per thread heaps), `internal/llvm/fromssa.go`
(`rcInc`, `rcDec`, `rcMark`, share helpers), `internal/arm64/rc.go`
(`incText`, `decText`, `markText`, `atomicInc`, `atomicDec`),
`internal/arm64/isel.go` (`inc`), `internal/arm64/closures.go`
(`decOrDefer`, `envShare`), `internal/arm64/boxes.go` (`boxShare`),
`internal/ssa/share.go` (`InsertShareProbes`). Evidence:
`bench/results/rc-shared-2026-10-08.md`. Update 2026-10-08: tasks lower
natively (decision 0068) and emit `OpShare` at every crossing listed in
item 3 (spawned closures, task results, channel sends, Mutex values).

## Context
Decision 0046 made native memory reference counted with plain (non
atomic) counts, noting that tasks would need atomic counts for values
that cross tasks or a per task heap with copying. The HTTP server and
every `scope`/`spawn` program need values to cross threads on the native
backends. Value semantics (core 5.1) mean a value is never mutated
through a shared reference, but its count is: two threads retaining and
releasing one list race on its count, and an in place update (Perceus
reuse, decision 0046 item 7) of a list another thread still reads is a
data race on its contents. Making every count atomic costs every retain
and release a locked read-modify-write; Swift pays that and Koka and
Lean showed most values never leave their thread.

## Decision
1. **Encoding.** The count word of every heap object (strings, lists,
   boxes, closure environments) is: `c > 0` thread local with c
   references (plain loads and stores); `c = -n` shared with n
   references (atomic); `XO_RC_IMMORTAL = INT64_MIN` for literals, never
   written. Every shared count has bit 62 set; the immortal count does
   not. This is Lean 4's scheme (positive local, negative multi threaded)
   with Koka's "mark shared on crossing"; it needs no extra header bit.
2. **Fast paths.** Retain: `c > 0 ? c + 1 : slow` (arm64 `tbnz x, #63`,
   one more predicted branch than a plain increment). Release:
   `c > 1 ? c - 1 : slow` (`b.le` where the plain code had `b.eq`, no
   extra instruction). Slow paths, out of the hot layout: count 1 frees;
   bit 62 clear is immortal (nothing); otherwise atomic. Retaining a
   shared object is a relaxed atomic decrement; releasing it is an
   atomic increment with acquire and release order, and the thread that
   moves it from -1 to 0 frees (Rust's Arc uses release plus an acquire
   fence; ThreadSanitizer does not model fences, and on arm64 both are
   one `ldaddal`). arm64 uses LSE (`stadd`, `ldaddal`) on Darwin, whose
   arm64 machines all have it, and exclusive load and store loops on
   Linux, which also runs on ARMv8.0 cores. LLVM uses `atomicrmw`, and
   reads counts with `load atomic ... unordered` (a racing read of a
   shared count is not undefined, and the optimizer still forwards it).
3. **Marking (crossing).** `OpShare v` (SSA, borrows v, no result) marks
   v shared: every thread local count reachable from v is negated,
   stopping at counts already negative, so a shared object only points to
   shared or immortal objects. Generated `share` helpers walk the heap
   leaves of each type like `dup`; lists call `xo_rl_share` with an
   element callback; boxes and closure environments go through the
   `xo_rc_defer` worklist (bounded C stack on long chains). An
   environment gains a third header word, its share function: `{rc,
   drop, share, captures...}`. Where a value crosses (spawn and
   spawn_background captures, channel send, Mutex and Atomic stores,
   closures passed to concurrency primitives) the lowering emits
   `OpShare` before the handoff; the handoff itself (thread start, a
   locked queue) orders the marking before the other thread's first
   access. No construct lowers to `OpShare` yet: tasks do not lower on
   the native backends.
4. **In place updates.** `reuse` keeps its inline `c == 1` test; its
   slow path (`xo_rl_copy`) first tries `xo_rc_claim`: a shared count of
   -1 read again with acquire order proves the caller holds the only
   reference (every other thread's release happened before), and the
   object goes back to thread local (count 1) and is updated in place;
   otherwise it copies and drops its reference (freeing the original
   when the others went meanwhile; `xo_rl_copy` now takes the drop
   callback too). String appends (`rs_append`) do the same. Static
   uniqueness (decision 0046) is unaffected: crossing needs a retain or a
   move, which ends the proof.
5. **Runtime.** Per thread heaps once `xo_rt_threads_begin` runs (the
   first thread keeps the global heap, later threads get their own on
   first use; a block may be freed on another thread's list); while one
   thread runs, `xo_alloc` and `xo_free` load one pointer (`solo_heap`)
   and branch, and the threaded paths are tail calls so the fast paths
   stay leaf functions. `xo_rc_defer`'s worklist is per heap, the output
   buffer is locked when threaded, the leak check sums per heap counters
   (`allocs` and `frees`, one counter per call). Literals live in read
   only memory (LLVM `constant`, arm64 `__TEXT,__const`/`.rodata`), so a
   stray write faults.
6. **Testing without tasks.** `build.Options.ShareProbes` (and
   `ssa.InsertShareProbes`) marks every heap value shared as it is
   defined; every golden program passes on both backends with it (the
   atomic paths, the claim, every share walker), and breaking an atomic
   path fails 11 to 27 of them. `std/llrt/testdata/rc_threads.c` hands
   shared strings, lists, box chains, and literals to pthreads (retain,
   release, copy on write, claim races, appends, cross thread frees)
   under ThreadSanitizer and optimized; `-DXO_RT_TEST_THREADS=4` runs
   whole programs (LLVM IR with probes) on four threads at once under
   ThreadSanitizer.

## Consequences
- Single threaded programs: within 2% on every bench/perf/llvm workload
  on both backends (process cycles, minimum of 11 interleaved runs; most
  within 1%). Getting there needed: the literal case of `rs_append`
  tested first, the threaded paths of the allocator as tail calls, one
  counter per allocator call, and cancelling `retain w; release
  Some(w)` in the LLVM backend too (`ssa.CancelledRelease`), because
  LLVM no longer folds the pair through the branchy count code.
- A value that crossed stays shared (atomic) until an in place update
  claims it; values derived from it (copies, new lists of its elements)
  start thread local. Marking is linear in the thread local part of the
  value's graph, once.
- What a task runtime still needs on the native backends: lowering
  `scope`/`spawn`/channels/Mutex/Atomic to SSA and to threads (or green
  threads), emitting `OpShare` at each crossing, calling
  `xo_rt_threads_begin` before the first thread, cancellation, and the
  network effects for the HTTP server. Mutex and Atomic handles can form
  cycles through shared state; reference counting alone does not collect
  those (decision 0046 noted it).
- A thread's heap is never freed or handed to another thread; a program
  that starts and ends many threads keeps their free lists. A future
  task runtime with a thread pool reuses heaps per worker.

## Alternatives
- **Atomic counts everywhere** (Swift): simplest, but every retain and
  release becomes an atomic read-modify-write, including the ones in
  single threaded loops; rejected by the 2% budget (not measured here).
- **Biased reference counting** (Choi et al., PACT 2018; Swift's
  experiment): an owner thread id plus two counters per object; the
  owner updates its counter without atomics, others use the shared
  counter and a queue to merge. More header space (Xo objects have one
  word) and a merge protocol; the sign scheme gives the same fast path
  for values that never cross, which is most of them.
- **Per task heaps with deep copies at boundaries** (Erlang, Pony's
  iso): no shared counts at all, but every send copies the value
  (an HTTP request's body, a cache's map), and value semantics already
  make sharing safe; rejected for the copy cost, kept in mind for small
  messages.
- **A header bit instead of the sign** (the task's first sketch): needs
  a mask on every update or a second word; the sign is free to test
  (`tbnz #63`) and leaves the release fast path unchanged.
