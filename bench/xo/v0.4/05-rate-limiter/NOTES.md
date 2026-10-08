# Notes: 05-rate-limiter (Xo 0.4, with checker)

Files: `xo.mod`, `limiter.xo` (Bucket, Limiter, middleware, router, evictor),
`main.xo` (server plus background evictor in a scope), `limiter_test.xo`
(eight tests: the five required plus a direct Bucket test, a remote IP
fallback test, and a direct `evict_idle` test alongside the background
evictor test).

Result: 0 errors and 0 warnings on the first run.

## Spec gaps and guesses

1. **Fake clock vs. a freshly spawned background task.** To test the evictor
   end to end I wanted `s.spawn_background(...)` then `os.clock.advance(11m)`.
   std 8.2 says `advance` fires timers due within `d`, but nothing says whether
   a task spawned just before `advance` has reached its first `sleep` yet. If
   it has not, its timer is scheduled after the advance and never fires. I
   avoided the question by having the test body itself call
   `os.clock.sleep(11m + 1s)` so auto advance (core 9.2) drives both tasks.
   The extra 1s is there because "timers due at the same instant fire in the
   order they were scheduled" and the body's 11m timer would be scheduled
   before the evictor's 11m timer.
2. **Assigning to a `var` closure parameter of `Mutex.with`.** In
   `evict_idle` I wrote `m = m.filter(...)` inside `with(fn(var m) {...})`.
   Core 5.4 shows only method mutation (`m.put`) of the `var` parameter; plain
   reassignment being written back is my reading of "mutations to the `var`
   parameter are written back". The checker accepted it.
3. **Match guards.** `Some(k) if !k.is_empty() => k` is in the grammar
   (`MatchExpr` in core 15) but no prose section describes guards or how they
   interact with exhaustiveness. I assumed the `_` arm makes it exhaustive.
4. **Whether an empty `X-Api-Key` counts as missing.** The task says "or the
   remote IP if missing". I treated an empty header as missing.
5. **Retry-After rounding.** "Whole seconds until one token is available, at
   least 1" does not say ceil vs. round. I used ceil, computed via
   `Duration.from_secs(deficit / rate).ceil_seconds().max(1)`. Going through
   `Duration` rounds to the nanosecond first, which hides float noise such as
   `2.0000000000000004` that would otherwise ceil to 3. The spec does not say
   whether `ceil_seconds` of an exact `2s` is 2 (I assumed yes).
6. **Effects of `routes`.** std 4.1 says registering middleware counts as
   performing its effects, so `routes` declares `uses clock`. `rate_limit` is
   `pub` so it must declare `uses clock` too (core 6.2).
7. **`invariant` on `Limit` vs. `const DEFAULT_LIMIT`.** Core 9.1 says
   invariants are checked after construction; I assumed that includes const
   struct literals (it would be a compile time check there) and that the
   invariant may combine fields with `&&`.
8. **Middleware as a returned function.** I first considered
   `pub fn middleware(l: Limiter, clock: Clock) -> fn(http.Request, http.Next) -> http.Response uses clock`.
   Core 15 says the trailing `uses` then belongs to the returned function type.
   That is what I would want, but it reads ambiguously, so I wrote a plain
   `rate_limit(l, clock, req, next)` function and wrapped it in a closure at
   registration instead.
9. **Did the checker check the tests?** Neither the text nor the `--json`
   output reports per file or per test counts, so I cannot tell from the
   output whether `limiter_test.xo` and the `test` blocks were type checked.
   Next experiment: copy the folder to the scratchpad, add
   `let x: Int = "a"` inside one test block, and run `xo-0.4 check` on the
   copy to see whether XO04xx is reported.

## Suspected checker bugs

None observed. The program passed on the first run, so there was no
diagnostic to dispute. Items in the gaps list above (2, 3, 7) were accepted;
if the spec intends otherwise for any of them, that acceptance would be a
checker bug, but I have no evidence either way.

## Go or Rust habits caught

- **Pointer to a map entry (Go).** My first instinct was
  `b := m[key]; b.take(now)` with `*Bucket` values. In Xo that is the XO0520
  lost update pattern. I used `m.update(key, new_bucket(limit, now), fn(var b) {...})`
  inside `Mutex.with` from the start.
- **`mu.Lock(); defer mu.Unlock()` (Go).** Replaced by `Mutex[Map[...]].with`.
- **`math.Ceil` on a float then `int(...)` (Go).** Replaced by the
  `Duration.from_secs(...).ceil_seconds()` route described above.
- **`time.Since(last)` (Go).** There is `clock.since`, but the bucket is pure
  and takes `now`, so `now - b.last_refill` (Time minus Time is Duration).
- **Pointer receiver for a mutating method (Go), `&mut self` (Rust).** Written
  as `fn (var b: Bucket) take(now: Time) -> Bool`; calling it in a test needs
  `var b`.
- **`context.Context` for stopping the evictor (Go).** Not needed: the
  evictor is a `spawn_background` task in the same scope as `srv.serve()?`,
  so it is cancelled when `serve` returns.

## Friction

1. **Read modify write of a map entry under a lock needs two nested
   closures.** In Go this is three lines with a pointer. In Xo:
   ```
   l.buckets.with(fn(var m) {
     m.update(key, new_bucket(limit, now), fn(var b) {
       if b.take(now) { Decision.Allowed } else { Decision.Limited(retry_after_secs: b.retry_after_secs(now)) }
     })
   })
   ```
   It is correct by construction, but the `init` argument is built on every
   call even when the key exists, and the result has to be threaded through
   two closure return values. I added a `Decision` enum mostly to carry the
   Retry-After value out of the inner closure.
2. **Float, Int, and Duration conversions for the Retry-After math.** There is
   no `Duration / Float`, no `Duration * Float` operator (only `scale`), and
   `Float` to `Int` truncates. Elapsed time to tokens is
   `elapsed.seconds() * b.refill_per_sec`, and tokens back to seconds is
   `Duration.from_secs((1.0 - tokens) / rate).ceil_seconds().max(1)`. Every
   step is explicit, which is fine, but choosing between `Float.ceil()` plus
   `Int(...)` and the `Duration` route needed thought about float noise.
3. **Fake clock semantics for background tasks are underspecified for
   tests.** Testing "evicts every minute" depends on when a spawned task
   reaches its first `sleep` relative to `advance`, and on same instant timer
   order. I had to design the test around auto advance and an extra second
   rather than writing the obvious `advance(11m)` (gap 1 above).
4. **Explicit clock plumbing.** Every layer that reads time takes a `Clock`
   (`routes`, `rate_limit`, `run_evictor`) and `pub` functions must restate
   `uses clock`. This is what makes the fake clock tests easy, so the cost is
   mostly typing, but it is three signatures for one `now()` call.
