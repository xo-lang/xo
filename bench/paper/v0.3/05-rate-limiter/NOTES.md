# Paper test notes: 05-rate-limiter (spec v0.3)

Files: `limiter.xo` (Bucket, Limiter, evictor, middleware), `main.xo` (routes,
main), `limiter_test.xo` (tests).

## 1. Spec gaps

1. **Compound assignment operators** (core 3.1, 15). Only `+=` appears (`total
   += x`, `count += 1`). The grammar says `Assign` without defining it. I
   guessed `-=` exists (`b.tokens -= 1.0`, marked `// GUESS`).
2. **Effects of a function that returns a closure** (core 6.2). `rate_limit`
   is `pub` and returns `fn(http.Request, http.Next) -> http.Response uses
   clock`. It never calls `clock` itself. The spec says a function's effects
   are the union of what it *calls*, and std 4.1 says registering a handler
   counts as performing its effects, but it does not say whether creating a
   closure counts. I guessed it does not, so `rate_limit` has no `uses`
   clause (marked `// GUESS`). If creation does count, it needs `uses clock`.
3. **Effects inside test blocks** (core 9.2). Tests call `routes(...)` which is
   `uses clock`, and call `os.clock.sleep`. The spec says real capabilities
   need `test "x" uses net`, but is silent on whether a test calling an
   effectful function with a fake needs a `uses` clause. I assumed tests are
   implicitly allowed every effect when using fakes (not marked, since there
   was no place to write anything).
4. **When a freshly spawned background task starts vs `advance`** (std 8.2).
   If the test called `os.clock.advance(10m)` right after `spawn_background`,
   it is unclear whether the evictor has already reached its first `sleep`
   (registered a timer). I avoided this by making the test body sleep
   (`os.clock.sleep(10m + 30s)`) so auto advance only jumps when every task
   is blocked. The spec could state that `advance` first lets runnable tasks
   reach a blocking point.
5. **Return type of a function whose body is `while { }`** (core 3.2). The
   spec says `while {}` has type `Never` and can end a body of any type. I
   wrote `run_evictor` with no return type (so `()`), relying on that. A
   `-> Never` return type would also seem valid; the spec does not say which
   is idiomatic.
6. **Comparison against an optional in `expect`** (core 2.10, 9.2). Whether
   `expect(resp.header("Retry-After")) == "4"` auto wraps `"4"` into `Some`
   depends on whether the right side of `==` counts as a place where the type
   is "already known". I wrote `Some("4")` to be safe.
7. **Reassigning a `var` closure parameter** (core 3.3, 5.2). In `evict_idle` I
   write `m = m.filter(...)` on the `var m` given by `Mutex.with`. The spec
   shows method mutation (`m.put`) and field assignment, not plain
   reassignment of a `var` parameter. I assumed it is allowed and written
   back like any other mutation.
8. **Discarding a non Result call as a statement** (core 2.6). `l.check(...)`
   returns `Decision` and is used as a bare statement in a test. The spec says
   non Result values may be discarded; fine, but there is no `xo vet` note
   about silently discarding a value that looks important. Not a guess.
9. **`Mutex.new({})` element type inference** (core 2.2, 5.4). `Mutex.new({})`
   as a struct field initializer relies on the field type to infer
   `Mutex[Map[Str, Bucket]]`. Appendix A does the same, so this seems
   intended, but the rule for inferring a generic *static call's* type
   argument from the expected type is only stated for struct literals.
10. **Effects in an `invariant`** (core 9.1). The invariant calls
   `Float(capacity)`. Contract clauses must be pure; I assume conversions are
   pure. Spec does not list which builtin calls are pure, but core 6.1
   defines pure as no capability effects, so this is fine.

## 2. Habits

1. Started to write `func (b *Bucket) Take(now time.Time) bool` (Go). Changed
   to `pub fn (var b: Bucket) take(now: Time) -> Bool`.
2. Wanted a `sync.Mutex` field next to a `map[string]*Bucket` and lock/unlock
   with `defer mu.Unlock()`. Changed to `Mutex[Map[Str, Bucket]]` and
   `with(fn(var m) { m.update(key, init, fn(var b) { ... }) })`.
3. Wanted `b := m[key]; if b == nil { b = newBucket() }` then mutate through
   the pointer. In xo that is a lost update (XO0520); used `Map.update`.
4. Wanted `time.NewTicker(time.Minute)` plus `select { case <-ctx.Done():
   return; case <-t.C: }` for the evictor. Changed to `while { clock.sleep(1m)
   ... }` and relied on cancellation unwinding `sleep`.
5. Wanted `math.Ceil(x)` and `int(...)`: changed to `Int(x.ceil())`.
   Wanted `math.Min`: changed to `a.min(b)` from `Ord`.
6. Started typing `strconv.Itoa(secs)` for the header; changed to `"${secs}"`.
7. Wrote `"{secs}"` first by v0.1 habit; v0.3 interpolation is `${...}`.
8. For the 429 path I reached for a Rust style `Result<(), Duration>` from
   `check`. Changed to an explicit `Decision` enum, since a denial is not an
   error and `Result` would trigger the mandatory handling rule.
9. In tests I wanted `assert.Equal(t, 200, resp.StatusCode)`; changed to
   `expect(resp.status) == 200`.

## 3. Friction

1. **Returning extra data out of a nested mutation.** `take` must return
   `Bool` (task requirement), but the middleware also needs the retry time,
   computed from the bucket state *after* refill. Since the bucket only exists
   inside `Map.update` inside `Mutex.with`, the computation has to happen in
   the innermost closure and come out as a value through two layers:
   ```
   l.buckets.with(fn(var m) {
     m.update(key, new_bucket(limit, now), fn(var b) {
       if b.take(now) { Decision.Allowed } else { Decision.Limited(retry_after: b.retry_after()) }
     })
   })
   ```
   It works, but two nested closures for "get or create, then mutate" is
   heavy. Also `new_bucket(limit, now)` is evaluated on every request even
   when the key exists (the `init` argument is eager); a lazy `fn() -> V`
   variant of `update` would avoid that.
2. **Fake clock and background tasks.** Testing the evictor needed reasoning
   about virtual time ordering (timers at the same instant, when the
   background task first registers its timer). I offset the test sleeps by
   30s to avoid same instant ties:
   ```
   os.clock.sleep(10m + 30s)
   expect(l.size()) == 1
   ```
3. **Effects on higher order constructors.** `rate_limit` returns a function
   typed `uses clock`, and I had to think about whether `rate_limit` itself
   must declare `uses clock`, and then `routes` must declare `uses clock`
   because `wrap` counts as performing it. The effect shows up in three
   signatures for one `clock.now()` call.
4. **Time passed in vs clock captured.** To keep `Limiter.check` pure and easy
   to test, I pass `now: Time` instead of a `Clock`. That is good design, but
   it was driven by the effect rule (pub fns need explicit `uses`), not chosen
   freely.
5. **No `-=` in the spec**, small but it made me stop and wonder.

## 4. Confidence

| File | Estimated remaining spec violations |
|---|---|
| `limiter.xo` | 0 to 2 (most likely: `-=`, `uses` on `rate_limit`, `m = ...` on a var closure param) |
| `main.xo` | 0 to 1 (`Mutex`/map literal inference in `new_limiter` call with a multi line map) |
| `limiter_test.xo` | 1 to 2 (test effects with fakes; `l.check(...)` as a bare statement inside a test; `expect` with `Some(...)` comparison) |
