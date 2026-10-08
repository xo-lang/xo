# Paper test 02: worker pool, notes

Files: `pool.xo` (Job, JobErr, Report, run_pool), `main.xo` (URL fetcher
program), `pool_test.xo` (the four required tests).

## 1. Spec gaps

| Spec section | Gap | What I guessed |
|---|---|---|
| 7.1 scope `timeout` | What a scope expression evaluates to when its deadline fires. Is it a Result, a propagated error, a fault? | It yields a Result-like value (e.g. `Result[T, Canceled]`), discarded with `let _ =` plus a comment. Expiry is detected by jobs with no recorded outcome. |
| 7.1 / 6.1 | Whether `scope ... timeout` needs the `clock` effect (only `select ... after` is said to). | Assumed yes; run_pool declares `uses net, clock` anyway. |
| 7.1 | Whether the task handle returned by `s.spawn` may be dropped as a bare expression statement. | Allowed because it is not a Result. |
| 6.2 | No effect polymorphism. A reusable pool cannot say "f uses whatever the caller wants". | Fixed `f` to `uses net, clock`, and assumed a less effectful closure (pure, or net only) is accepted where `uses net, clock` is expected. The spec only states the reverse direction (pure type rejects effectful fn). |
| 6.3 | No way to get time without a Clock capability, so the requested signature `run_pool(jobs, workers, timeout, f)` cannot compute `elapsed`. | Added a `clock: Clock` parameter. |
| 6.3 / 14 | Clock API: `now()`, `sleep(d)`. Time arithmetic. | `clock.now() -> Time`, `clock.sleep(Duration)` infallible, `Time - Time -> Duration`. |
| 2.1 | Duration arithmetic with Int. | `10ms * (5 - j.input)` yields a Duration. |
| 2.6 | How a `! E` function returns an error directly. Only `?` and `requires ... else` are shown. | `return err(TestErr.Boom(id: j.id))`. |
| 3.4 / 15 | Nested patterns in `match` (`some(ok(v))`). Grammar just says `Pattern`. | Allowed. |
| 3.4 | `for _ in 0..n` with `_` as a pattern. | Allowed. |
| 15 | Trailing commas and multi line parameter and argument lists. | Allowed, Go style. |
| 3.2 | Mixing positional and named arguments. | Avoided by naming every argument at run_pool call sites. |
| 6.4 | How a test block gets its fake `Os`. Test blocks have no parameter list. | An implicit `os` binding in every test block. |
| 6.4 | Whether the fake clock is virtual (sleep advances instantly) and how a scope timeout interacts with it. | Virtual: sleeps and deadlines use virtual time, so `clock.sleep(1h)` vs `timeout: 1s` finishes instantly. |
| 9.2 | `expect` only shown with `==`. No `<=` or other matchers. | `expect(max_seen <= 3) == true`. |
| 9.2 | Whether `expect` needs `use std/test`. | No import needed, as in the examples. |
| 5.4 | `Mutex.with` closure semantics with `var m`: is assigning `m = x` (not just calling a method) allowed, and what does `with` return? | Assigning works and writes back; `with` returns the closure's value (as in Appendix A). |
| 5.4 | Atomic API beyond `new` and `add` (no `load`, no return value of `add`). | Avoided Atomic entirely and used `Mutex[Int]` for counters. |
| 2.1 / 14 | List and Str methods: `len()`, `indexed()`, `push`, `lines()`, `trim()`, indexing `xs[i]`. | All exist with the obvious meaning. |
| 14 std/http | Client constructor and API. | `http.client(os.net)`, `client.get(url) -> http.Response ! http.ClientErr`, `resp.status: Int`, ClientErr implements Display. |
| 6.3 Stdio | Stdio API. | `os.stdio.read_all() -> Str ! Error`, `os.stdio.println(Str) -> () ! Error`. |
| 2.9 | Derive Display on a generic enum and whether interpolating a generic `E` needs a constraint. | Sidestepped: `JobErr[E]` has no derive and main matches on it, interpolating only the concrete `http.ClientErr`. |
| 8 | Whether a fault inside one job should be isolated. The spec says a fault cancels siblings. | Not isolated. "Failing job" taken to mean an error, not a fault. `std/task.isolate` could wrap `f`, but its interplay with `R ! E` (nested result types) is unspecified. |
| 7.2 | A job whose `f` completes but is cancelled before its outcome is recorded. | Reported as `TimedOut`, which is acceptable. |

## 2. Habits

- Go `sync.WaitGroup` plus `go func()`: started to reach for a WaitGroup. Replaced
  with `scope s { s.spawn(...) }`, which already waits for all children.
- Go `context.WithTimeout` and passing `ctx` into `f`: wanted `f(ctx, job)`.
  Replaced with `scope s timeout timeout { ... }`; cancellation flows implicitly.
- Go `results := make([]R, len(jobs))` and writing `results[i] = r` from
  workers: not possible (no shared mutable state, lists are immutable). Changed
  to a `Mutex[Map[Int, Result[R, E]]]` and assembling ordered lists afterwards.
- Go `for { j, ok := <-ch; if !ok { break } }`: changed to
  `while { if let i = queue.recv() { ... } else { break } }`.
- Naming the closure error binding `err` (Go habit, `if err != nil`). `err` is a
  predeclared identifier that cannot be shadowed, so the variant field is
  `cause` and the match arm binds `cause`. Same trap for `result`: I almost
  named the success list `result`, which collides with the `ensures`
  identifier. Used `results`.
- Rust `Ok(v)` / `Err(e)` capitalized constructors: wrote lowercase `ok(v)` /
  `err(e)`.
- Go `if err != nil { return err }` to fail in a test closure: changed to
  `return err(TestErr.Boom(id: j.id))` (itself a guess).
- Rust `impl<T> Report<T>` and `<T>` generics: used `[T]`.
- Go `atomic.AddInt64` and `atomic.Load` for the concurrency counter: switched to
  `Mutex[Int]` because only `Atomic.add` is specified.
- Go `defer wg.Done()` style decrement at function end: kept `defer` but noted it
  is block scoped, which is what I want inside the closure body.

## 3. Friction

- **No effect polymorphism.** A reusable pool must hard code what `f` may do.
  The task says net, the tests want to sleep, so the signature became:
  ```
  f: fn(Job[T]) -> R ! E uses net, clock,
  ```
  Anyone wanting `fs` in a job cannot use this pool. A generic `uses fx`
  parameter would fix it.
- **Capabilities leak into signatures.** Measuring elapsed time forced an extra
  `clock: Clock` parameter that the task's signature did not have:
  `run_pool(jobs: jobs, workers: 8, timeout: 30s, clock: os.clock, f: ...)`.
- **No shared mutable output.** Workers cannot write into a results slot, so
  outcomes go through a mutex and are re-ordered afterwards:
  ```
  done.with(fn(var m) { m.put(i, r) })
  ...
  let outcomes = done.with(fn(m) { m })
  for (i, job) in jobs.indexed() { match outcomes.get(i) { ... } }
  ```
  Correct and clear, but more ceremony than `results[i] = r`.
- **Timed out scope result is undefined.** I had to write
  `let _ = scope s timeout timeout { ... }` and rely on side data to detect
  expiry. Also `timeout timeout` (contextual word followed by a parameter named
  by the task) reads oddly.
- **Errors must be bound, not dropped, to avoid cancelling siblings.** The
  natural `f(jobs[i])?` would fail the worker task and cancel the whole scope.
  Binding `let r = f(jobs[i])` is the fix, and it is easy to get wrong.
- **Predeclared names.** `err`, `result`, `ok` cannot be used as identifiers, so
  `Failed(err: E)` became `Failed(cause: E)`.
- **Test assertions.** Only `expect(x) == y` exists, so range checks read as
  `expect(max_seen <= 3) == true`.
- **Counters via Mutex.** Incrementing and reading in one step needs a
  multi line closure:
  ```
  let now_active = active.with(fn(var n) {
    n += 1
    n
  })
  ```

## 4. Confidence

| File | Estimated remaining spec violations |
|---|---|
| `pool.xo` | 1 to 3. Most likely: the type and handling of a timed out `scope` expression, dropping the spawn handle, and the trailing comma parameter list. |
| `main.xo` | 2 to 4, almost all from guessed std APIs (`read_all`, `lines`, `trim`, `http.client`, `println`). The language usage itself I believe is correct. |
| `pool_test.xo` | 2 to 4. Most likely: the implicit `os` in tests, `return err(...)`, `Duration * Int`, and whether a pure-ish closure is accepted for an `uses net, clock` parameter. |
