# 0173: Log attribution and resource budgets

Status: Accepted, implemented (2026-10-11): log attribution on every
backend; resource budgets on the Go backend (native XO1201 planned).
Follow-ups of decision 0156 ("left / future" in
`bench/results/supply-chain-2026-10-11.md`).

## Context

Decision 0156 made logging the effect `log` but left each record
unattributed: when a consumer allows a dependency to log, a leaked secret
in the logs cannot be traced to the module that wrote it. It also left
compute denial of service out of scope: a dependency that loops forever
or allocates without bound performs no effect, so no ceiling, lock, or
attenuation applies, and `task.with_timeout` does not help either, since
a pure loop has no cancellation point (core 7.2) and the timed-out scope
waits for it forever.

## Decision

### 1. Per-record log attribution

- A call of `log.debug/info/warn/error` whose source file belongs to a
  required module version (`check.Module.Dependency`, not the main module
  or a workspace module) writes a record attributed to
  `<module path>@<version>`. The call's source file decides, not the
  function being generated, so a closure of the main module inlined into
  a dependency's specialized copy stays the main module's.
- Text records: `<time> <LEVEL> [<module>@<version>] <msg> k=v ...`.
  JSON records: the key `"module"`, which replaces a field of that name,
  so a dependency cannot attribute its record to another module. Tests:
  `log.Record` gains `module: Str` (`""` for the main module). The main
  module's records are unchanged.
- Go backend: internal/gen emits `rt.Log_<level>From(tk, from, msg,
  fields)`. Native backends: the SSA builder emits `rt.log_from(<const>)`
  after the arguments and just before the call (`callAsVia`), and
  std/native/log.xo takes it at entry (`xo_log_from_take`, a thread
  local in xo_http.c; no task switch happens in between).
- `xo audit` lists each dependency's log call sites and the attribution
  its records carry (`log_attribution`, `log_sites`).
- Not covered: `log.info` used as a function value from a dependency
  (the value adapter does not attribute), std's own records.

### 2. Resource budgets: `task.with_budget`

```
struct Budget { cpu: Duration, alloc: Int, steps: Int }   // 0: no ceiling
fn budget(cpu: Duration = 0s, alloc: Int = 0, steps: Int = 0) -> Budget
enum Budgeted[E] { Exceeded(resource: Str), Failed(from E) }
fn with_budget[T, E, fx](clock: Clock, b: Budget, f: fn() -> T ! E uses fx) -> T ! Budgeted[E] uses (clock, fx)
```

- Shape: like `task.with_timeout` (decision 0004's structured
  concurrency): f runs in a child scope; a passed ceiling cancels that
  scope, waits for it to unwind, and returns `Err(Exceeded("cpu" |
  "alloc" | "steps"))`. A budget is not authority, so it is not a
  capability rooted at `os`; it limits what the call may consume. It
  takes `clock` because running time is observed time: the outcome is
  not deterministic, and the effect says so (as for `with_timeout`).
  `steps` is the deterministic unit for tests and replay.
- Scope: every task started inside the call works under the budget,
  wherever it is started (`Task.child` inherits it), and nested budgets
  all apply (each charge walks the chain).
- Enforcement: generated code calls `tk.Tick()` at the start of every
  loop iteration and at the entry of every function or closure whose
  body calls user code (a user function, a function value, or an
  interface method). Unbounded work needs a loop or recursion, and every
  function on a call cycle calls something, so leaf functions get no
  poll and keep their size for Go's inliner. Without a budget the poll is
  a nil check. Under one, a thread charges its steps and running time
  every 1024 steps (the last quantum before a `steps` ceiling is cut to
  hit it exactly) and the poll unwinds the task when a budget of its
  chain is exceeded or the task is canceled: under a budget, pure loops
  and recursion are cancellation points.
- Running time: wall time on the monotonic clock outside blocking waits
  (`blockOn`/`unblock` stop and restart the stretch), summed over the
  budget's threads. It overcounts when goroutines are descheduled by an
  oversubscribed machine.
- Allocation (Go backend): Go keeps no allocation count per goroutine,
  so `alloc` is the process wide `/gc/heap/allocs:bytes` since the budget
  started, an upper bound that also counts other tasks running at the
  same time (fails closed). Read only at a charge with an `alloc`
  ceiling.
- Deferred code runs shielded and is not unwound; budgeted shielded code
  that runs past twice a ceiling faults with the new `XO-F008`.
- Native backends: `with_budget` is XO1201 (planned). The plan: polls at
  loop headers and call entries in the SSA builder, and an exact per task
  allocation count in std/llrt's allocator.

### Measured

- `cmd/xo` TestSupplyChainAttacks: 12 of 17 scenarios blocked (fetch 1,
  check 7, lock 1, runtime 3; 5 not blocked), up from 10 of 15: the two
  new rows are the unbounded loop under `with_budget(steps)` and
  `with_budget(cpu)`, blocked at run time. The unbudgeted loop stays not
  blocked (`bench/results/supply-chain-2026-10-11.md`).
- internal/gen golden `budget`: a pure loop (steps and cpu), exponential
  recursion with no loop, allocation, tasks spawned inside, a result
  within budget, a failure, and nested budgets.
- Poll overhead without a budget, with polls everywhere: fib(35) +45%,
  a sieve +18%, a tight `while` loop unchanged; so polls are emitted only
  when the program calls `task.with_budget` (a program without one
  generates the same Go as before). See
  `bench/results/budget-poll-2026-10-11.md`.

## Consequences

- In a program that calls `task.with_budget`, every loop and every
  non-leaf generated Go function carries one `tk.Tick()` line, budgeted
  or not (+45% on a call-bound recursion, +18% on a list sieve);
  programs without a budget are unchanged. Narrowing the polls to code
  reachable from budgeted closures is the next experiment.
- `log.Record` has a fourth field; code constructing it needs `module`.
- `NetErr`, `ProcErr` unchanged; the new fault code is `XO-F008`.
- The default posture is unchanged: a dependency called without a
  budget can still burn CPU. As with ceilings and attenuation, the
  consumer opts in per call.

## Alternatives

- A budget as a capability (`os.budget(...)` passed to the dependency):
  the dependency would have to accept and use it; a budget the caller
  wraps around the call needs no cooperation, which is the point
  against a hostile dependency.
- A `task` spawn option instead of a wrapper: possible later as sugar
  for `s.spawn(fn() { task.with_budget(...) })`; the wrapper also
  covers the synchronous call, the common case for a library.
- Fail-stop (fault the whole process on overrun) without polls: no code
  generation cost, but it cannot return an error the consumer handles,
  and Go cannot stop a goroutine from outside.
- Polls at every function entry: simpler, but leaf functions would grow
  and lose inlining for no gain (a leaf cannot be on a cycle).
