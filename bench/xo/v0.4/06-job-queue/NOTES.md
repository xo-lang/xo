# Notes: 06-job-queue (Xo 0.4, checker arm)

Result: the first full draft checked with 0 errors and 0 warnings. Two
reverted probes confirmed that test blocks and closure effects are actually
checked, so the clean result is not just the checker skipping things. The
checker cannot run code, so none of the runtime behavior (fake clock
auto advance, cancellation, shielded defer) is verified.

## Design summary

- `queue.xo`: `Queue` holds `Mutex[State]`, a `Clock`, and `max_attempts`.
  Every public method reads `now` outside the lock, then runs pure `var State`
  methods inside `Mutex.with`. Every operation first expires leases, so an
  expired lease is counted as a failed attempt lazily (visible again at
  `lease_until + backoff`, or dead lettered). Tokens are increasing `Int`s.
  Lease selection is a linear scan (highest priority, then lowest id).
- `worker.xo`: `Runner.run` spawns a task holding an inner scope of N worker
  loops, waits on a stop channel, then `task.with_timeout(grace, workers.await)`
  and `s.cancel()` on `TimedOut`. Each handler runs in `scope h` with a
  background heartbeat every `lease_duration / 3`. A shielded `defer` nacks
  the job if the handler was canceled. `shutdown` closes the stop channel and
  blocks on a `done` channel that `run` closes in a `defer`.
- `api.xo`: four routes; `ApiErr` implements `status()` for error mapping.
- `queue_test.xo`: the eight required tests plus grace period ack, handler
  error nack, and an HTTP API test.

## Spec gaps and guesses

1. **Closure declared `-> Result[(), QueueErr]` without `!`.** Core 5.4 shows
   this form, but 2.6 says "if `T` is itself a `Result`, success must be written
   `Ok(x)`". I relied on the closure having error type `Never`, so a tail of
   type `Result[(), QueueErr]` (returned by a State method) is just the `T`
   value. The checker accepted `st.ack(job_id, token)` as the tail without
   `Ok(...)`. The spec should say explicitly whether XO0203 applies only when
   there is a non `Never` error type.
2. **Closures given a return type but no `uses`.** Core 3.3 says closure
   effects are inferred "unless declared". I wrote
   `fn(req) -> http.Response ! ApiErr { ... q.enqueue(...) ... }`, which
   performs `clock`. It is unclear whether writing the return type counts as
   declaring the signature (making it pure). The checker inferred `clock`, so
   only an explicit `uses` clause counts as a declaration. Worth one sentence
   in 3.3.
3. **Effect calls as cancellation points.** Core 7.2 lists "effect calls" as
   cancellation points, which would include `clock.now()`. So any queue call
   after cancellation unwinds, and the nack for a canceled handler only works
   inside a shielded `defer`. I built the design around that, but the spec does
   not say whether a non blocking effect like `clock.now()` is really a
   cancellation point, or only blocking ones are.
4. **Scope body as a value with an unconsumed `Result`.** In `handle_one`,
   `let outcome = scope h { h.spawn_background(...); handler(l.job) }` yields
   the handler's `Result` as the scope value. I guessed this is not a "child
   failure" (the body is not a task). The checker agreed, but 7.1 only talks
   about tasks.
5. **`is` combined with `&&` and with `|` patterns.** The grammar line
   `Binary = Unary { BinOp Unary } | Unary "is" Pattern` does not literally
   allow `j.state is Leased && j.lease_until <= now`; the prose example
   `if resp is Ok(_) && retries < 3` does. I followed the prose.
   `expect(j.state is Queued | Leased)` was also accepted (or pattern on the
   right of `is`).
6. **Waiting for `run` to finish.** "No worker may still be running after
   shutdown returns" needs `shutdown` to block until `run` exits. There is no
   join primitive for a task started elsewhere, so I used a `done: Chan[()]`
   closed in a `defer`, and `recv()` returning `None` on a closed channel as a
   broadcast. Calling `shutdown` without `run` would block forever; I
   documented it rather than guarding it.
7. **Expiry timing.** The task does not say when an expired lease's backoff
   starts. I used `lease_until + backoff(attempts)` so results do not depend on
   when the next queue call happens to observe the expiry.
8. **`let _ = x.ignore_err(...)` vs. bare expression.** `ignore_err` yields
   `T?`, a non `Result`, so both `let _ = ...` and a bare expression statement
   are allowed. I used `let _ =` in the defer (matches the spec example) and a
   bare match arm elsewhere. Both checked.

## Suspected checker bugs

None found. The two probes behaved as specified (XO0401 inside a test block,
XO0701 on a pub function whose registered handler closure uses `clock`, with a
call chain note).

Possible leniency, not confirmed as a bug: the 1000 job test handler is
`fn(j) { handled.add(1); seen.with(...) }` where `handled.add(1)` returns an
`Int` that is discarded as a statement. That is allowed by 2.6 (non `Result`
values may be discarded), so it is correct, just easy to miss.

## Go or Rust habits caught while writing

- Go habit: reaching for a `sync.WaitGroup` or `ctx.Done()` for shutdown. Xo
  has no context; I used scope cancellation plus `task.with_timeout` on an
  outer task handle (explicitly allowed in 7.1).
- Go habit: `nack` after `<-ctx.Done()` in the normal path. In Xo the nack has
  to live in a `defer` because the canceled task cannot do more effect calls.
- Rust habit: wanting `match` arms `Ok(x) => x, Err(e) => return Err(e)` style
  joins. Xo does not convert `Err(...)` in a joined branch (2.10), so I used
  early `return Err(...)` and `.map_err(...)?` instead.
- Rust habit: `while let Some(l) = q.lease(...)`. Xo has no `while let`; I
  used `for _ in 0..n` in tests and `if let ... else sleep` in the worker loop.
- Go habit: `2 << (n-1)` for backoff. Bitwise shift on `Duration` is not
  defined, so `backoff` doubles in a loop and clamps with `min`.
- Habit of mutating a copied map entry (`j := m[id]; j.state = ...`). Xo would
  flag this as XO0520, so every transition is `put(id, j with {...})`.

## Friction (rules that made the task harder)

1. **No mutation through a `Result` returning lock closure without the
   keep and fail pattern.** Every fallible queue operation needs the same
   boilerplate because returning `Err` from `with` discards the expiry work
   done inside the closure:
   ```
   let outcome = q.state.with(fn(var st) -> Result[(), QueueErr] {
     st.expire_leases(now, q.max_attempts)   // must be kept even on failure
     st.ack(job_id, token)
   })
   outcome
   ```
   Five methods repeat this exactly. A helper would need to be generic over the
   closure's result type and error, which is doable but verbose.
2. **Cancellation forces cleanup into `defer` with a flag.** To nack only the
   canceled jobs, the worker needs a `var settled` flag read in a shielded
   defer, since an ordinary `nack` after cancellation would unwind at
   `clock.now()`:
   ```
   var settled = false
   defer {
     if !settled {
       let _ = r.queue.nack(l.job.id, l.token, "canceled during shutdown")
         .ignore_err("the lease may already have expired")
     }
   }
   ...
   settled = true
   ```
   Easy to get wrong (setting the flag too early loses the nack).
3. **"Shutdown returns only after workers stop" has no direct primitive.** A
   scope owns its tasks, but a separate caller (`shutdown`) cannot join them, so
   the runner carries three synchronization handles (`Atomic[Bool]` stopping,
   `Chan[()]` stop, `Chan[()]` done) plus an `Atomic[Int]` for the test to
   observe:
   ```
   pub fn (r: Runner) shutdown() {
     if r.stopping.compare_and_swap(false, true) { r.stop.close() }
     r.done.recv()
   }
   ```
4. **Every public queue method has to read the clock outside the lock.** Pure
   `State` methods keep effects out of the closure, which is clean, but each
   method begins with `let now = q.clock.now()` and passes `now` and
   `max_attempts` down manually.
5. **`pub` requires effect clauses everywhere.** Every `pub fn (q: Queue)`
   needs `uses clock`, and the runner needs `uses (clock, fx)` with an effect
   variable threaded through `run`, `worker_loop`, and `handle_one`. Correct,
   but it is a lot of signature text for a small runner.
