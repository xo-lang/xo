# 0431: A fault inside Mutex.with poisons the mutex

Status: Rejected (owner, 2026-10-30). Rollback stays: a fault inside
Mutex.with leaves the protected value unchanged (core 5.4), and the
mutex is not poisoned. The Data section below showed no hot path in
std, compiler/ or bench paying the rollback copy after 02271995b.
(Proposed by Xo Lang 1, 2026-10-30, asked for by the lead after Xo Lang
2's metricsum profile.)

## Context

Core 5.4: "Mutations to the `var` parameter are written back when the
closure returns successfully. If it returns an error or faults, the
protected value is left unchanged." Unwinding for cancellation keeps the
value too (golden conc_mutex_rollback, "canceled kept").

The native backends and LLVM implement this as follows. The packed
closure (compiler/ssa map.xo closure_with, decision 0085) gets the value
moved out of the mutex. On `Err`, on a fault, and on cancellation, its
unwind exit hands back the value it was given (build.xo packed_value,
finish_fn: `pack_orig`). So the closure keeps that original value alive
across every later point that can fail: an `Err` return, a `?`, a
cancellation point, and every point that can fault (an index, a call
that can fault, arithmetic overflow, a `requires`). Reference counting
then makes the first write to each collection reachable from the value
copy that collection, once per `with`. The Go backend does the same
(std/rt sync.go Mutex_with: MarkP, then a copy on write).

The cost is the size of every collection the body writes, per call:

| program (arm64, Mac, instructions) | 20k calls | 40k | 80k |
|---|---|---|---|
| m: `l[i % 2].b.push(x)`, no fault point after the push | 32.6M | 51.4M | 88.9M |
| q: the push in a helper, the list reset every 500 | 78.2M | 142M | 271M |
| qp: `for k in 0..st.streams.len() { st.streams[k].b.push(x) }` | 1.25G | 7.24G | 50.97G |
| p: the same push with no Mutex at all | 17.1M | 15.2M | 16.4M |

These were measured on 2026-10-30 with
~/xo-lead/notes/xo-lang-2/repro-place/main.xo, xo at lang1-mxwith
02271995b.

qp is quadratic. After the first push, the next iteration's index into
`st.streams` can fault, so the original `St` stays alive, its `streams`
list is shared, and each `b` gets copied whole before its push.
std/metrics' Summary (metricsum, 2M observations, 1.825x Go on
linux/arm64) has the same shape: summary_observe calls
qinsert(st.streams[i], v, objs), a call that can fault. Its profile puts
26% in rl_copy, `dupp QStream` and `share List[QSample]` (Xo Lang 2,
2026-10-30).
Update (2026-10-30): std/metrics no longer pays it. e3a99876b gives
QState a hot buffer of Atomic slots, so observe's `with` body writes no
collection, and metricsum went from 1,559 to 655 ms on linux/arm64
(0.766x Go). The rule's cost still falls on user code with the qp shape,
which is what this decision is about; the metricsum estimate below is
kept as the measure of the shape, not of std today.

The copy is not an RC bug. Commit 02271995b fixed the one that was:
Mutex.with's payload was a borrowed projection. Since that fix a write
after the body's last failure point changes the value in place (m
above), and only a write followed by such a point pays. Go has no
rollback: a panic inside a locked region leaves the state as the
goroutine left it. Rust's Mutex is poisoned by a panic, and later `lock`
calls return `Err(PoisonError)` whose guard still gives the value.

Who relies on rollback after a fault today:

- golden fault_tasks: a fault in `bump`'s `with` after a push, then
  "after the fault: 2 values" and "mutex still works: 3 values";
- every HTTP handler that faults inside a `with` on server state: the
  server answers 500 (decision 0083) and the next request sees the state
  from before the faulting request;
- conc_mutex_rollback's cancellation case, and the `Err` cases of it and
  conc_mutex (both unchanged by this proposal).

std has seven files that call `with(fn(var`. None of them reads a value
back after a fault on purpose. Each one's `with` would be re-read for the
poisoned case if this is accepted.

## Data: which `with` bodies still copy (2026-10-30)

Counted for the owner's decision, with xo at main b55d33aec plus 02271995b.
The call sites come from `xo refs Mutex.with`. Each body was classified from
`xo dev dump ssa --stage=rc`: does it write the protected value, and does a
fault point follow that write on some path (the rule's cost)?

Left out: 12 sites in `_test.xo` files and 110 in bench/planted-xo* (the
planted-bug variants, which the checker rejects on purpose).

| where | sites | write the value | a write, then a fault point |
|---|---|---|---|
| std | 41 | 36 | 2 |
| compiler | 23 | 17 | 0 |
| bench (no planted variants) | 53 | 44 | 7 |
| goldens and testdata | 67 | 47 | 6, all rollback or fault tests |
| total | 184 | 144 | 15 |

The 9 outside the tests, each measured or read:

- **Map.update inside a with** (6 bench sites: chathub main.xo 139, 157,
  224, 242, 254, per connection or message; bench/xo v0.4 rate limiter
  limiter.xo 96, per request). The static rule flags these: the update takes
  the entry out, then calls a closure that can fault. They do not pay,
  though. A repro made 20,000 `with` + `update` calls: 0.00 s with 1,000
  keys and 0.27 s with 100,000 keys, building the map included. A copy per
  call would be 2 billion entry copies.
- **std/native/trace.xo 37** (trace.push, per request through
  ring_middleware; the ring holds 1,000 spans by default). It pushes, then
  `l = l[l.len() - cap..]`. That pattern copies the ring with or without
  rollback. At cap 100,000, 20,000 calls took 1.97 s with no Mutex and
  2.60 s inside `with`, so rollback adds about 30%. A ring with a write
  index removes both (a std item).
- **std/native/metrics.xo 471** (expose, a summary's quantiles): the
  streams are copied once per scrape and series. A cold path; the size is
  bounded.
- **bench/xo v0.4 job-queue queue.xo 177** (dead_jobs): the jobs map is
  copied once per call. An admin path.

So after 02271995b, no hot path in std, compiler/ or bench pays the
rollback copy, beyond trace.push's 30% over its own copy. The expensive
shape is qp's: a write, then a fault point, on an unbounded collection, per
item. Today's code has no instance of it. std/metrics had one until
e3a99876b. The decision is about user code with that shape.

## Decision (proposed)

1. A fault that unwinds out of a `Mutex.with` body poisons the mutex.
   The body's changes are not written back, and the value is dropped (it
   may be half changed, and natively it may be mid-update, such as an
   element taken out of a list). From then on every `with` and
   `with_read` on that mutex faults with a new code, `XO-F010` (`mutex
   poisoned by a fault at LOC`, with the original fault's location),
   until `m.reset(v)` stores a new value and clears the poison. `Mutex`
   gains `fn (m: Mutex[T]) reset(v: T)` and `fn (m: Mutex[T])
   is_poisoned() -> Bool`.
2. An `Err` (an `Err` return or `?`) and cancellation still leave the
   value unchanged, as today. They are the expected outcomes of a body,
   and cancellation is how scopes end normally.
3. The packed closure keeps its original value alive only across the
   points that can return `Err` or be canceled: an `Err` exit, a `?`, a
   wait, and a call to a function that can be canceled (the cancellation
   facts the native backends already keep, stdfacts `cancels`). A fault
   point no longer keeps it. A body that cannot fail and does not wait
   changes the value in place: qp drops to m's cost.
4. The Go backend gets the same semantics: on a panic it marks the mutex
   poisoned instead of restoring `m.v`, and it skips MarkP when the
   closure's type cannot fail.

## Consequences

- qp at 80k goes from 50.97G instructions to about m's 89M (estimated,
  one in-place push per write). metricsum loses its rl_copy share, an
  estimated 26% of its time, which would bring it from 1.825x Go to
  about 1.35x. Both are to be confirmed by `--ab` on the arm64 host when
  this is built.
- A faulting HTTP handler that holds server state in a `Mutex` takes that
  state down: every later request that touches it faults (500) until
  code calls `reset`. This is the main cost. A server that wants today's
  behavior keeps the work that can fault outside `with`, or makes the
  body return `Err` instead of faulting.
- `task.isolate` callers see a poisoned mutex after the fault they
  caught. fault_tasks' "mutex still works" line becomes an `XO-F010`
  fault unless it calls `reset` (the golden changes).
- The spec changes core 5.4's bullet, core 5.x's performance note, the
  `XO-F` table (XO-F010), and std.md 2.7 (`reset`, `is_poisoned`).
- `xo vet` could note a `with` body whose fault points follow a write to
  a large collection. That is optional and not part of this decision.

## Alternatives

- **Keep today's rule (status quo).** Correct, and simple to explain. A
  write followed by a fault point copies each collection once per call.
  Code avoids it by writing last, by moving the fallible work out of the
  body, or by bounding what each call writes (q's reset at 500).
  02271995b already makes the last-write case free.
- **Rollback only when a fault point follows a write.** This is what the
  code does since 02271995b. It cannot go further without proving that
  the later points cannot fault. Bounds-check elimination for `for k in
  0..x.len()` while `x`'s length cannot change would remove qp's only
  fault point, but not metricsum's (a call that can fault).
- **An undo log per write.** Record each in-place write's old value
  (the element, the field, the list length) and replay the log
  backwards on a fault. The cost is per write, not per collection size,
  but every write in a `with` body pays a log entry, the log must hold
  references (more counting), and nested collections need nested logs.
  It is a large compiler change for the native backends and LLVM, and
  the Go backend has no hook for it.
- **Keep the half-changed value (Go's behavior).** It needs no poison
  state, but natively the value can be mid-update at the fault (an
  element taken out of a list by `list.take` has not been put back), so
  handing it back would need every in-place operation to be
  fault-consistent. It also silently keeps a broken invariant.
- **Opt in per call (`m.with_in_place(f)`, faults poison) and keep
  `with` as it is.** No existing program changes, and hot paths (std
  metrics, caches) choose the cheap form. Two spellings of one operation,
  and the default stays the slow one for the shape agents write first.
  If the owner prefers not to change `with`'s fault semantics, this is
  my recommendation instead of the decision above.
