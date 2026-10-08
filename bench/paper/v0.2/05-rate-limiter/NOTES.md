# Task 05 rate limiter: paper test notes (spec v0.2)

Files: `limiter.xo` (Bucket, Limit, Limiter, middleware, evictor), `main.xo`
(router and `main`), `limiter_test.xo` (tests and the `hello_req` helper).

## 1. Spec gaps

1. **Is locking a Mutex an effect?** (core 6.1, 6.2). `pub fn` without `uses`
   is "pure: deterministic given its inputs", but `Limiter.check` reads and
   writes shared state through `Mutex.with`, so its result depends on other
   tasks. The exception list (allocation, faults, cancellation, logging) does
   not mention synchronized state. Guessed: no effect needed. Marked GUESS in
   `limiter.xo`.
2. **Float and `Ord`** (std 1). "Builtin scalar types ... are `Ord`" probably
   covers `Float`, but NaN makes a total order false. I used `.min` on Float.
   Marked GUESS.
3. **How the server stops** (std 4.4, core 7.1). `serve` "runs until its scope
   is cancelled", but nothing says what cancels the scope in a real process
   (SIGTERM, SIGINT?). The task says the evictor must stop when the server
   stops. Guessed: the runtime cancels the root scope on a signal. Marked
   GUESS in `main.xo`.
4. **`http.Next` is pure** (std 4.1). `type Next = fn(Request) -> Response`
   has no effects, yet `next(req)` runs handlers that may use `net` or
   `clock`. It works for this task because "registering counts as performing
   its effects", but the typing of `next` looks inconsistent with effect
   tracking. `Router.call` is likewise pure even though middleware reads the
   clock. No guess needed, but surprising.
5. **Lost update check and closure `var` parameters** (core 5.2). XO0520
   exempts "`var` parameters and receivers". I assumed this includes closure
   parameters such as `fn(var m) { m.update(...) }` and `fn(var b) { ... }`
   in `Map.update`, where the mutation is written back by the callee. Not
   stated explicitly.
6. **Reassigning a whole `var` closure parameter** (core 5.2, 5.4). I wrote
   `m = m.filter(...)` inside `Mutex.with`. Field assignment and mutating
   methods are described; plain reassignment of a `var` parameter is implied
   by "can be mutated" but never shown.
7. **`invariant` on a `const`** (core 9.1, 3.1). `DEFAULT_LIMIT` is a const
   struct literal on a type with an `invariant`. Is the invariant checked at
   compile time? Assumed yes (it is pure and on constants).
8. **Test blocks and effects** (core 9.2). Tests call `run_evictor`, which is
   `uses clock`, and `os.clock.sleep`. The spec says real capabilities need
   `test "x" uses net`, implying fakes need nothing, but it never says a test
   body may perform any effect.
9. **Virtual clock tie breaking** (core 9.2, std 8.2). When two timers are due
   at the same virtual instant (body sleep and an evictor tick), the order is
   not specified. I sleep to `11m + 30s` to avoid the tie.
10. **Does `os.clock.advance` count as a sleep or wake background tasks?**
    (std 8.2). Not stated. My eviction unit test calls `evict_idle` directly so
    it does not depend on this.
11. **Empty `X-Api-Key` header** (task, std 4.3). `header` returns `Str?`;
    whether an empty header value is `Some("")` or `None` is unspecified. I
    treat only a missing header as missing.
12. **`Decision` discard** (core 2.6). `l.check("old", now)` as a statement
    discards a non Result value, which is allowed, but a reviewer might
    expect an XO warning. No guess.
13. **Implicit interface conversion of fakes** (std 1, std 8). Passing
    `os.clock` (`FakeClock`) where `Clock` is expected relies on the "converted
    to that interface implicitly" sentence, which is under prelude interfaces;
    I assumed it applies to capability interfaces too.

## 2. Habits

- Started to write the 429 response as a chained call split across lines
  (`http.json(...)` newline `.with_header(...)`), Go/Rust builder style. The Go
  newline rule ends the statement after `)`, so I changed it to a block with a
  `let resp` binding.
- Wanted `if b.tokens >= 1.0 { b.tokens -= 1.0; true }` on one line. No
  semicolons in xo, so it became a multi line block.
- Reached for a Go style `*Bucket` pointer in the map (`b := m[k]; b.take()`).
  Changed to `m.update(key, init, fn(var b) { ... })`, the spec's write back
  idiom.
- Wanted Go `time.Ticker` / `for range ticker.C` for the evictor. Changed to
  `while { clock.sleep(EVICT_EVERY) ... }` relying on cancellation unwinding.
- Wanted a `ctx context.Context` / `done` channel to stop the evictor. Not
  needed: `spawn_background` in the server's scope stops it.
- Wanted `math.Ceil` and `int(...)`; used `wait.ceil()` and `Int(...)`.
- Wanted Rust `Option::filter` to treat an empty header as missing; there is
  no `filter` on `T?` in std 2.5, so I dropped that refinement.

## 3. Friction

- **Lost update check in tests.** A local `var b` whose last call is a
  mutating method is error XO0520, even though the test only cares about the
  returned Bool:
  ```
  var b = new_bucket(Limit{capacity: 2, refill_per_sec: 1.0}, os.clock.now())
  ...
  expect(b.take(later)) == false
  expect(b.tokens) == 0.0   // added only so b is read after the last mutation
  ```
- **Nested write back closures.** Atomically take a token and compute
  Retry-After needs two levels of `var` closures, and I had to introduce a
  `Decision` enum to carry the result out:
  ```
  l.buckets.with(fn(var m) {
    m.update(key, new_bucket(limit, now), fn(var b) {
      if b.take(now) { Decision.Allowed } else { Decision.Denied(retry_after: b.retry_after_secs()) }
    })
  })
  ```
- **Newline termination with builders.** `http.json(...).with_header(...)`
  cannot be split across lines with a leading dot, which forced a temp
  binding inside a match arm block.
- **Effects on a returned closure type.** `rate_limit` has to spell the full
  type `fn(http.Request, http.Next) -> http.Response uses clock`, and I had to
  decide whether `rate_limit` itself needs `uses clock` (it only captures the
  clock, it does not call it, so I left it pure).
- **No Float/Duration bridge for rates.** Elapsed time to tokens is
  `(now - b.last).seconds() * b.refill_per_sec`; fine, but the Retry-After
  path is Float math, then `.ceil()`, then `Int(...)`, then `.max(1)`.
  `Duration.ceil_seconds()` exists but I had a Float, not a Duration.

## 4. Confidence

- `limiter.xo`: about 1 to 2 remaining violations (most likely: purity of
  `Limiter.check` under 6.2, and `m = m.filter(...)` on a closure `var`
  parameter).
- `main.xo`: about 0 to 1 (most likely: effect clause on `routes` or how a
  map literal with a trailing comma spanning lines is typed as
  `Map[Str, Limit]`).
- `limiter_test.xo`: about 1 to 2 (most likely: effects inside test bodies,
  virtual clock behavior with background tasks, and the
  `decode_json[Map[Str, Str]]()` explicit type argument form on a method).
