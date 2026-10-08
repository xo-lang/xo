# Paper test notes: 02 worker pool (spec v0.3)

Files: `pool.xo` (library: `Job`, `Report`, `run_pool`), `main.xo` (URL
fetcher), `pool_test.xo` (the four required tests plus helpers).

## 1. Spec gaps

1. **Semaphore copy semantics** (std 7, std 0). std 0 says capabilities and
   fakes are handles, and core 5.1 lists `Chan`, `Mutex`, `Atomic` as handle
   types, but `task.Semaphore` is in neither list. A semaphore that copies by
   value would be useless once captured by each spawned closure. Guessed: it is
   a handle. Marked `// GUESS` in `pool.xo`.
2. **What a cancelled task sees after a call returns** (core 7.2). A failing
   blocking call returns its `Canceled` variant, but `f` is generic over `E`,
   so the pool cannot recognise "this error is really a cancellation". I used
   `task.is_canceled()` after `f` returns to avoid recording a cancelled job as
   its own failure. The spec does not say whether `Mutex.with` in an already
   cancelled task that does not need to wait still runs its closure, or
   unwinds. I avoided the question by not calling it when cancelled. A
   `Canceled` marker interface for errors (or `task.Timed` style wrapping
   done by the runtime) would make this cleaner.
3. **Closure error type inference** (core 3.3). "Effects of a closure are
   inferred from its body unless declared" but nothing says the error type is
   inferred. For `fn() { scope s { ... } }` passed to `task.with_timeout`, I
   assumed `E` is inferred as `Never` because no child can fail. For closures
   that use `?` I declared the error type explicitly (`fetch` in `main.xo`,
   `worker` in the test file) to be safe.
4. **Error type of a `scope` expression** (core 7.1). A scope can propagate a
   child failure "as if by `?`", so its type is effectively `T ! E` for some E.
   When every spawned closure returns `()` with no error, I assumed the scope
   cannot fail and the enclosing closure stays `! Never`.
5. **Module identity vs. directory name** (core 1.1, 4.1). The folder is
   `02-worker-pool`, which maps to `02_worker_pool`, not a valid identifier. Fine
   as long as nothing imports it, but the spec does not say whether a module
   that contains `main` may also export a library, or whether `run_pool` should
   live in a separate directory. I put everything in one module.
6. **Fault inside a match arm block** (core 3.5, 8). `fault(...)` has type
   `Never`, used as a statement inside `if` in a match arm block. Assumed fine.
7. **Tuple element access**. There is no `.0` / `.1` in the grammar, and
   closure parameters cannot be patterns (`LParam = [var] ident [: Type]`). So
   `urls.indexed().map(fn((i, u)) { ... })` is impossible. I used a `for`
   loop with a tuple pattern instead. Not a guess, but a gap worth noting.
8. **Fairness / ordering of semaphore waiters**. The test asserting
   `elapsed == 40ms` assumes 10 jobs at 3 permits finish in exactly 4 rounds
   of 10ms on the virtual clock. That holds for any waiter order, but it also
   assumes a released permit is handed over without the clock moving. std 8.2
   only says time jumps when every task is blocked; a task holding a just
   released permit is runnable, so I believe this is right.
9. **`Display` of `task.Timed[E]`** (std 7). It is `derive Display`, which
   prints Debug text like `Failed(Connect(msg: "..."))`. For user facing output
   I wrote a `describe` helper. Not a gap, just a note that the derived text is
   not meant for humans.

## 2. Habits

- Started to write a Go style pool: `jobs := make(chan Job)`, N goroutines
  ranging over the channel, a `sync.WaitGroup`, and `context.WithTimeout`.
  Replaced with `task.with_timeout` + `scope s { s.spawn(...) }` +
  `task.semaphore(workers)`. The scope replaces the WaitGroup and the
  `with_timeout` child scope replaces the context.
- Wanted a preallocated `results := make([]R, len(jobs))` and `results[i] = r`
  from each goroutine. Not possible: no zero values, and closures capture by
  value. Switched to `Mutex[Map[Int, Result[R, E]]]` keyed by index, then a
  sequential pass in job order.
- Wrote `if err != nil` shape in my head for `f(job.input)`; changed to binding
  the `Result` (`let r = f(job.input)`), which counts as handling it (core 2.6)
  and keeps the error from escaping the spawned task and cancelling siblings.
- Wanted Rust style `.enumerate().map(|(i, u)| Job { ... }).collect()` in
  `main.xo`. Closure params cannot be patterns, so it became a `for (i, url) in
  urls.indexed()` loop pushing onto a `var` list.
- Wanted `g.running++`. No `++`; wrote `g.running += 1`.
- Almost wrote `Atomic` with a compare and swap loop for the peak gauge (Go
  `atomic.CompareAndSwap` habit). A `Mutex[Gauge]` with `with(fn(var g) {...})`
  is shorter and obviously correct.
- Almost defined my own `PoolErr[E] { TimedOut, Failed(E) }`. `task.Timed[E]`
  already has exactly that shape, so I reused it.

## 3. Friction

- **Getting results out of concurrent tasks needs a Mutex.** Value semantics
  plus capture by value means the natural "write into slot i" is a mutex round
  trip, and the final read is another closure:
  ```
  outcomes.with(fn(var m) { m.put(i, r) })
  ...
  let done = outcomes.with(fn(m) { m })
  ```
  Alternative was to keep the `Task` handles in a list and `await` each, but
  handles created inside the `with_timeout` closure cannot escape it, and an
  await after a timeout is not specified.
- **Cancellation versus job failure.** Because `E` is generic, the pool cannot
  tell a real job error from a `Canceled` produced by the deadline. The
  workaround is a runtime check that is inherently racy at the boundary:
  ```
  let r = f(job.input)
  if !task.is_canceled() {
    outcomes.with(fn(var m) { m.put(i, r) })
  }
  ```
- **Four levels of nested closures** in `run_pool`
  (`with_timeout` -> `scope` -> `spawn` -> `permits.with` -> `outcomes.with`).
  Each is idiomatic, but the result reads like callback code, and each level
  raises the inference questions in gap 3 and 4.
- **Capability threading.** `run_pool` needs a `Clock` both for the deadline
  and for `elapsed`, so the task's signature `run_pool(jobs, workers, timeout,
  f)` grows a `clock` parameter. Correct per the design, but it means the
  required API cannot be matched exactly.
- **Rebuilding order in main.** `Report.results` holds only successes, without
  ids, so printing one line per URL in input order needs a failures map plus a
  cursor into `results`:
  ```
  if let Some(e) = failed.get(job.id) { ... } else {
    os.stdio.println("${job.input} ${report.results[next]}")
    next += 1
  }
  ```
- **Named argument rule.** Positional before named means
  `run_pool(os.clock, jobs, workers: 8, timeout: 30s, f: fetch)`; fine, but I
  had to check the rule twice.

## 4. Confidence

- `pool.xo`: 1 to 2 likely violations. Most at risk: closure error type
  inference for the `with_timeout` closure containing a `scope` (gaps 3, 4), and
  whether `fault` in an `if` with no `else` inside a match arm block typechecks
  as `()`.
- `main.xo`: 0 to 1. Most at risk: `os.stdio.lines()?` followed by a
  `.map` continuation line (should be fine per 1.3), and `fn(l) { l.trim() }`
  inference through `map`'s effect variable signature.
- `pool_test.xo`: 1 likely. Most at risk: the exact `elapsed` assertions on the
  virtual clock (behavioural, not syntactic), and `expect(report.failures) ==
  [...]` needing `Eq` on `task.Timed[WorkErr]` (should be automatic).
