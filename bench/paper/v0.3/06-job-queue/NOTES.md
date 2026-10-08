# Paper test notes: 06 job queue (spec v0.3)

Files: `queue.xo` (queue core), `worker.xo` (runner), `api.xo` (HTTP),
`queue_test.xo` (all tests from the task plus one HTTP test).

## 1. Spec gaps

1. **`?` on Option inside a closure** (core 2.5). The rule says "in a function
   that returns `U?`". I used `st.next_visible(now)?` inside a
   `fn(var st) -> Lease?` closure passed to `Mutex.with`. Guessed closures
   count as functions here. Marked GUESS in `queue.xo`.
2. **Private types in pub structs** (core 1.5, 2.2). `pub struct Job` has a non
   pub field `lease: LeaseInfo?` where `LeaseInfo` is not pub. The spec never
   says whether that is allowed or a "private type in public API" error.
   Guessed allowed. Marked GUESS.
3. **Scope body ending in an `Err` value** (core 7.1). `let outcome = scope s {
   ...; handler(l.job) }` evaluates to a `Result`. The spec covers failing
   child tasks but not a body whose tail value happens to be a `Result`.
   Guessed it is a plain value and does not propagate. Marked GUESS.
4. **`Never` as an error type satisfying `E: Error`** (std 4.1, core 6.2). A
   non failing handler `fn(_req) { http.json(...) }` is passed to
   `handle[E: Error, ...]`. Subsumption says non failing functions are
   accepted where failing ones are expected, but the bound is on a type
   parameter, not a function type. Guessed it works. Marked GUESS.
5. **Mixed `T` and `Err(e)` arms in a tail `match`** (core 2.6). The return
   rule lists what a "returned value" may be, but does not say whether each
   arm of a tail `match` is judged separately. I avoided the question with
   `_ => return Err(...)` (type `Never`) in `leased_job`. Not a guess in code,
   but the spec should say.
6. **`Mutex.with` discards mutations on `Err`** (core 5.4). That is specified,
   but it is silent on whether a "lazy maintenance" step done in the same
   closure (expiring leases) can be kept when the closure then fails. It
   cannot, so `ack`, `nack`, and `extend` call `q.expire(now)` in a separate
   critical section first. Specified, but surprising; see Friction.
7. **Is `Task[T, E]` a nameable prelude type?** (std 2.7). It appears only in a
   method signature. I first wanted `var tasks: List[Task[(), Never]]` to await
   workers inside `task.with_timeout`. Also unspecified: can a task handle
   from scope `s` be awaited inside the child scope that `with_timeout`
   creates? I sidestepped both with a `finished: Chan[()]` that each worker
   signals from a `defer`.
8. **Is `clock.now()` a cancellation point?** (core 7.2). "Effect calls" are
   cancellation points, and `now` is an effect call that cannot fail, so in a
   cancelled task it would unwind. My runner relies on this: after shutdown
   cancels a worker, any normal path `ack`/`nack` unwinds and the shielded
   `defer` does the nack. If `now()` were not a cancellation point the design
   still works (the `settled` flag prevents a double nack).
9. **Exact timing of `with_timeout` on the fake clock** (std 7, 8.2). The
   shutdown test asserts `clock.now() - started == cfg.grace`. That depends on
   "time does not move while cancelled tasks unwind", which std 8.2 states, so
   I believe it is exact, but `defer` blocks doing Mutex work in many tasks at
   that instant is not discussed.
10. **Invariant on structs updated with `with`** (core 9.1). "Checked after
    construction and after every var method". Unclear whether `j with {...}`
    counts as construction. I assumed yes (`Job` has an invariant tying
    `state == Leased` to `lease.is_some()`).
11. **Enum variant field patterns nested through a `from` variant**
    (core 3.5). `QueueOp(NotFound(..) | NotDead(..)) => 404` relies on the
    inner variants being resolvable bare because the field type of the `from`
    variant is known. The "expected type is known" rule (2.3) is listed for
    expressions; for patterns 3.5 says UpperCamel always means a variant but
    not how bare names resolve inside nested patterns. Assumed it works.
12. **Range of `<<` on `Int`** (core 15). Shift is listed in precedence but its
    overflow behavior is not specified (does `1 << 63` fault?). I guarded with
    `if attempts > 6 { return MAX_BACKOFF }` so it does not matter.
13. **Graceful shutdown vs process signals** (core 7.5). A signal cancels the
    root scope immediately, which skips the runner's grace period. There is no
    documented way to observe "a signal arrived" as a value (to close `stop`
    instead). I did not write a `main` for that reason.

## 2. Habits

- Wanted a Go `sync.Mutex` with `Lock()/defer Unlock()` around a struct with
  plain fields. Changed to one `Mutex[QueueState]` and `q.state.with(fn(var st)
  { ... })`, with helpers as `var` methods on `QueueState` (`st.reap(now)`).
- Wanted Go `context.Context` / `ctx.Done()` for shutdown and a
  `sync.WaitGroup` for waiting. Changed to a `scope`, an `Atomic[Bool]` stop
  flag, a `Chan[()]` that workers signal from `defer`, and `task.with_timeout`
  plus `s.cancel()`.
- Wanted Rust `let Some(l) = q.lease(..) else { sleep; continue }`. xo has no
  let else, so it became a two arm `match`.
- Started writing a free function `reap(st, now)` with a `var` parameter, then
  realized the call site syntax for an in out argument is not shown anywhere.
  Changed to a `var` receiver method `st.reap(now)`, which is documented.
- Wanted `2.pow(n)` / Go `1 << n` with `time.Duration(...)`. Used
  `1s * (1 << (n - 1))` since `Duration * Int` is defined, and avoided a
  method call on an integer literal (lexing of `2.pow` is not discussed).
- Wanted Go style sentinel `time.Time{}` for "no lease deadline". Changed to
  `lease: LeaseInfo?` with the token, worker, and deadline together.
- Wrote `return Enqueued{...}` inside the `with` closure expecting it to return
  from `enqueue` (Go habit of thinking of the critical section as inline code).
  It returns from the closure, which is what I want here, but it is easy to
  misread.
- Reached for `j.lease_token != token` comparing `Str?` with `Str` (implicit
  wrap). Used a `match` with a guard instead, to avoid relying on implicit
  `Some` inside a comparison.

## 3. Friction

1. **`with` rolls back on error, so maintenance and validation cannot share a
   critical section.** Expiring stale leases inside the same closure that then
   returns `StaleToken` would lose the expiry. Every fallible op does two
   locks:
   ```
   q.expire(now)
   q.state.with(fn(var st) -> () ! QueueErr {
     let j = st.leased_job(job_id, token)?
     st.jobs.put(job_id, j with {state: JobState.Done, lease: None})
   })
   ```
   The alternative (return `Result` as a plain `R` with explicit `Ok`) is
   heavier.
2. **Long effect generic parameter lists.** Passing a handler with effect
   variable `fx` through three functions repeats the full signature each time:
   ```
   fn run_one[fx: Effects](q: Queue, clock: Clock, cfg: RunnerConfig, l: Lease,
     handler: fn(Job) -> () ! Error uses fx) uses (clock, fx)
   ```
   A struct holding the handler would need a struct with an effect parameter,
   which the spec does not show.
3. **No obvious way to wait for N tasks with a deadline.** `task.with_timeout`
   runs a closure in a child scope, but whether it may await handles of the
   parent scope is unclear, so I built a counting channel:
   ```
   let drained = task.with_timeout(clock, cfg.grace, fn() {
     for _ in 0..cfg.workers {
       let _ = finished.recv()
     }
   })
   if drained.is_err() {
     s.cancel()
   }
   ```
4. **Nack on cancellation needs a flag plus a shielded `defer`.** Because a
   cancelled task cannot do normal blocking work, the nack has to live in
   `defer` and be suppressed on the happy path:
   ```
   var settled = false
   defer {
     if !settled {
       let _ = q.nack(l.job.id, l.token, "canceled")
     }
   }
   ```
5. **Mixing a Result tail with early exits.** In `leased_job` I wanted a tail
   `match` with one `T` arm and one `Err(...)` arm, but the spec does not
   clearly bless that, so one arm became `return Err(...)`.
6. **Capabilities stored in a struct** (`Queue.clock`) force every pub method
   to say `uses clock`, including `get` and `stats`, because they expire leases
   lazily. That is correct but makes the read API look effectful.

## 4. Confidence

- `queue.xo`: about 2 to 3 remaining violations. Most likely spots: the
  private type in a pub struct field, `?` on Option in a closure, the nested
  `filter` closure with a tuple destructure, the struct `invariant` calling a
  method.
- `worker.xo`: about 2 to 4. Most likely: effect variable plumbing through non
  pub generic functions, `let outcome = scope s { ... }` holding a `Result`,
  `s.spawn` handle discarded inside a `for`, `defer finished.send(())` form.
- `api.xo`: about 1 to 2. Most likely: non failing handler vs `E: Error`, and
  the nested or pattern inside a `from` variant in `status()`.
- `queue_test.xo`: about 2 to 3. Most likely: exact virtual clock timing in
  the shutdown test, `expect(x) == None` on a struct carrying `Bytes` and
  `Time` (relies on auto `Eq`), and `os.clock` (a `FakeClock`) passed where
  `Clock` is expected through a closure capture.
