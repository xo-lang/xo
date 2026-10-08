# Notes: 02-worker-pool (paper test, spec v0.2)

Files: `pool.xo` (Job, Report, run_pool), `main.xo` (URL fetcher program),
`pool_test.xo` (the four required tests).

Design in one paragraph: `run_pool` spawns one task per job inside a `scope`,
gates the work with `task.semaphore(workers)`, wraps the whole scope in
`task.with_timeout`, and records each finished job's `Result` in a
`Mutex[Map[Int, Result[R, E]]]` keyed by job position. After the timeout
scope returns (every task has finished or unwound), it walks `jobs` in order:
a recorded `Ok` is a result, a recorded `Err` is `Timed.Failed(e)`, and a
missing entry is `Timed.TimedOut`. I reused `task.Timed[E]` as the per job
error type instead of defining my own.

## 1. Spec gaps

1. **`uses` with several effects in a declaration (core 6.2 vs core 15).** The
   grammar `FnClause = "uses" EffectList` has no parentheses and the note
   under the grammar says parens are needed only in a function *type*. But
   the examples `retry` (core 6.2) and `with_timeout` (std 7) write
   `uses (clock, fx)` on a declaration. Guessed: parens are accepted in both
   places; I wrote `uses (clock, fx)`.
2. **Is `Mutex.with` a cancellation point when uncontended? (core 7.2).** The
   list says "`Mutex.with` while waiting". A job whose `f` returned
   `Err(ClientErr.Canceled)` because the timeout fired could therefore still
   store that error if the lock is free, and the report would show a
   transport error instead of a timeout. Guessed: not guaranteed, so I check
   `task.is_canceled()` before recording.
3. **Value of a block ending in a `for` statement (core 3.2, 7.1).** "The last
   expression of a block is its value" but `for` is a statement (grammar
   `Stmt`). Guessed: such a block is `()`. This matters for the scope inside
   `with_timeout`, whose closure type is inferred from it.
4. **Generic struct literal type arguments (core 2.2, 2.7).** Inference is
   specified for calls, not for struct literals. Guessed `Job{id: i, input: url}`
   infers `Job[Str]` (the appendix's `Mutex.new(...)` without `[T]` is the
   nearest precedent).
5. **Generic enum variant construction across modules (core 2.3).** Nothing
   shows constructing a variant of a generic enum from another module.
   Guessed `task.Timed.Failed(e)` and `task.Timed.TimedOut` with `E`
   inferred. Also unclear whether the prefix can be dropped on the right side
   of `expect(x) == ...` (is the expected type "known" there?), so I kept it.
6. **`Int` implements `Display`? (core 1.6, std 1).** Interpolation requires
   `Display`. std 1 says scalars, `Str`, `Duration`, `Time` are `Ord`, and
   std 2.6 says how `Duration` displays, but never says `Int` is `Display`.
   Guessed yes (`"{urls[id]} {status}"`).
7. **Does `derive Display` on a generic enum need `E: Display`? (std 7).**
   `Timed[E]` derives `Display`; I interpolate `Timed[http.ClientErr]`. Since
   std errors are `Display`, this works either way, but the rule for generic
   derives is not stated.
8. **Semaphore fairness (std 7).** Order in which waiting tasks get permits is
   unspecified. I wrote the timeout test so it passes under any order, but a
   pool user cannot rely on jobs starting in input order.
9. **Virtual clock during unwinding (core 9.2, std 8.2).** Unclear whether time
   can move between the timer firing and `with_timeout` returning. Guessed it
   cannot, so `expect(report.elapsed) == 5s`.
10. **Contract fuzzing of higher order generic functions (core 9.1).** "`xo test`
    fuzzes every function with contracts". `run_pool` has `requires workers > 0`
    but takes a `Clock` and an `fn` parameter. Capabilities get seeded fakes,
    but nothing says how a function argument or a generic `T` is generated.
    Left as is; I assume such functions are skipped or `T` is instantiated
    with something trivial.
11. **Faults in a job.** Not a contradiction, but the task says "a failing job
    does not stop the other jobs" and the spec separates errors from faults.
    I chose: errors are isolated per job, faults cancel the pool. Wrapping `f`
    in `task.isolate` would contain faults too, at the cost of a third error
    variant (`Faulted`), so I did not.
12. **Discarding a non `Result` call inside `defer` (core 3.4).** `defer
    running.add(-1)` defers an expression whose value is `Int`. Discarding a
    non `Result` value is allowed as an expression statement (2.6), so I
    assume the same holds for `defer stmt`.

## 2. Habits

- **Go worker loop with a jobs channel and a `sync.WaitGroup`.** First draft
  in my head was N goroutines reading `jobs <-chan Job` and writing to a
  results channel. Changed to one `s.spawn` per job gated by
  `task.semaphore`, because the structured scope already waits for everything
  and the semaphore expresses "at most N" directly.
- **`ctx, cancel := context.WithTimeout(...)`.** Wanted to thread a context into
  `f`. Changed to `task.with_timeout(clock, timeout, fn() { ... })`; the
  deadline flows down the scope tree with no parameter.
- **Preallocated `results := make([]R, len(jobs))` written by index.** Not
  possible: closures capture by value and lists are immutable. Changed to a
  `Mutex[Map[Int, Result[R, E]]]` and a final in order pass over `jobs`.
- **`for i, job := range jobs`.** Wrote `for (i, job) in jobs.indexed()`.
- **Rust closure tuple destructuring `|(i, u)| ...`.** Wanted
  `urls.indexed().map(fn((i, u)) { Job{...} })`. Closure params are plain
  identifiers (`LParam`), so I used a `var jobs` and a `for` loop with `push`.
- **`atomic.AddInt64(&running, 1)` plus `if now > max { max = now }` on a shared
  local.** The captured local would be a copy, so the peak lives in a
  `Mutex[Int]` updated with `peak.with(fn(var p) { p = p.max(now) })`.
- **Go `defer` at function level.** `defer running.add(-1)` is block scoped in
  xo; here the closure body is the block, so it behaves as intended.
- **`fmt.Printf("%s %d\n", ...)`.** Changed to interpolation `"{urls[id]} {status}"`.
- **Rust `Result<Option<..>>` handling with `if let`.** Used a single nested
  `match outcomes.get(i) { Some(Ok(v)) => ... }` instead.
- **Writing `select` with a timer arm** (Go `case <-time.After(timeout)`).
  Dropped in favor of `with_timeout`, which also cancels and waits for unwind.

## 3. Friction

- **Capture by value forces a mutex for every collected result.** Collecting
  outputs from tasks needs an explicit shared type even when each task writes
  a distinct slot:
  ```
  let done = Mutex[Map[Int, Result[R, E]]].new({})
  ...
  done.with(fn(var m) { m.put(i, r) })
  ...
  let outcomes = done.with(fn(m) { m })
  ```
  Alternative is awaiting task handles in order, but `with_timeout` cancels
  the scope and handles of cancelled tasks cannot be inspected afterwards, so
  partial results would be lost.
- **Cancellation vs recorded error ambiguity.** I had to reason about whether
  an interrupted `f` returns `Err(Canceled)` and whether the subsequent
  `Mutex.with` unwinds, then add a guard:
  ```
  let r = f(job.input)
  if !task.is_canceled() {
    done.with(fn(var m) { m.put(i, r) })
  }
  ```
- **Effects plumbing on a `pub` generic function.** The signature carries a
  type param kind, a function type with an effect variable, and an explicit
  `uses (clock, fx)` that must also include the capability needed to measure
  elapsed time:
  ```
  pub fn run_pool[T, R, E, fx: Effects](clock: Clock, jobs: List[Job[T]], ..., f: fn(T) -> R ! E uses fx) -> Report[R, E]
    uses (clock, fx)
  ```
  Needing a `Clock` just to report elapsed time is honest but adds a
  parameter Go would not have.
- **Closure parameters cannot be patterns.** Building jobs from
  `urls.indexed()` needed a `var` list and a loop instead of one `map`.
- **Nested closures pile up.** `with_timeout(fn) > scope > spawn(fn) >
  sem.with(fn) > done.with(fn)` is five levels of nesting for the core loop.
- **Results and failures as separate lists.** To print one line per URL in
  input order, `main` has to re-merge them through a `Map[Int, Str]` keyed by
  job id. That is a consequence of the required `Report` shape, not the spec.
- **`let _ = task.with_timeout(...)`.** Because the outcome is fully reflected
  in `done`, the `Result` is discarded explicitly, which reads like ignoring
  an error even though it is intentional.

## 4. Confidence

- `pool.xo`: about 1 to 2 likely violations. Most at risk: the parenthesized
  `uses` in a declaration, the type of the scope block ending in `for`, and
  the `task.Timed.Failed(e)` construction form.
- `main.xo`: about 1 likely violation. Risks: struct literal type inference,
  `Int` interpolation, closure type inference for `fn(l) { l.trim() }` on the
  result of `lines()?`.
- `pool_test.xo`: about 1 to 2 likely violations. Risks: `Timed` variant paths
  in `expect` comparisons, `defer` of a non `Result` expression, the exact
  `elapsed == 5s` assumption on the virtual clock, and the `peak == 3`
  assertion depending on scheduler behavior.
