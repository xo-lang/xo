# Task 05 paper test notes

Files: `bucket.xo` (Limit, Bucket), `limiter.xo` (Limiter, Decision, middleware,
evictor), `main.xo` (server), `limiter_test.xo` (tests).

## 1. Spec gaps

| Spec section | Gap | What I guessed |
|---|---|---|
| 2.1 Duration, Time | No operators or methods for `Time`/`Duration`. | `Time - Time -> Duration`, `Time + Duration -> Time`, Duration compares with literals, `d.seconds() -> Float`. |
| 2.1 numeric conversions | Only `Int32(x)` and `Int32.try(x)` shown. No Int to Float or Float to Int. | `Float(i)` and `Int(f)` (truncating, faults out of range). |
| 2.1 Float | No math methods listed. | `f.min(g)`, `f.ceil()`, `i.max(j)`. |
| 3.1 const | Only `const MAX_USERS = 1000`. | A struct literal and a Duration literal are valid const initializers (`DEFAULT_LIMIT`, `IDLE_TTL`). |
| 3.1 XO0110 | Says unused bindings are errors; silent on unused parameters and loop counters. | Parameters exempt (`hello(req)`); `for _ in 0..20` is a valid discard pattern. |
| 3.4 / 2.3 match patterns | `NotFound(id)` binds a field, but is it positional or by name? | By field name: `Limited(retry_after_secs) => ...`. |
| 5.4 Mutex.with | Shown returning a value only implicitly (Appendix A). Not stated whether the `var m` write back happens, or whether the closure may be multi statement. | Returns the closure value; `var m` writes back on return. |
| 5.2 Map mutation | Only `List.push` and field assignment are described. | `m.put(k, v)` and `m.remove(k)` on a `var` map rebind it. Iterating a `let snapshot = m` while removing from `m` is safe. |
| 6.2 pub + uses | "pub functions must declare uses explicitly" vs "no uses clause is pure". | A pure pub function writes no clause (no `uses ()` form exists). |
| 6.2 function types | Can a pure function be passed where `fn(...) uses clock` is expected? Spec only says the reverse is rejected. | Yes, subsumption (`limiter.wrap(os.clock, hello)` with pure `hello`). |
| 6.2 closures | Grammar has no closure literal production. | `fn(req: http.Request) -> http.Response ! Error uses clock { ... }`, untyped `fn(var m) { ... }` when inferable. |
| 6.3 Clock | Capability type `Clock` exists but no methods. Struct or interface? | `clock.now() -> Time` (infallible), `clock.sleep(d)` (cancellation point, no Result). |
| 6.4 tests | Test `Os` fakes are promised, but a `test` block has no `os` parameter, and no way to advance the fake clock is given. | `test.fake_clock(start:)` returning `test.FakeClock` with `.advance(d)`, usable as a `Clock`. `test.yield()` to let spawned tasks run. |
| 7.1 scope exit | Does an early `?` out of a scope body cancel children, or block waiting for them? Does a child ended by `s.cancel()` make the scope fail? | Bound `let served = srv.serve()`, then `s.cancel()`, then `served?`. Cancelled children are not failures. |
| 7.1 spawn | Is discarding the task handle from `s.spawn(...)` allowed? | Yes, as an expression statement. |
| 9.2 tests | Can tests use `?`? | Yes, test bodies are `() ! Error`. |
| 14 std/http | No API beyond `http.server`, `srv.handle`, `srv.serve`, `http.json`, `req.param_int`. "middleware" is listed but undefined. | Wrap handlers manually (`Handler` fn type). `req.header(name) -> Str?`, `req.remote_ip() -> Str`, `resp.status`, `resp.header(name) -> Str?`, `resp.with_header(k, v)`, `http.fake_request(...)`. |
| 14 std/task | Lists "rate limiters" in std. | Ignored, task asks to build one. |
| 2.4 / 6.3 | Is `Clock` passable as a value stored in a closure? | Yes, captured by value in the returned handler and the spawned task. |
| 1.6 Map literal | Empty `{}` as Map vs block, and `{}` in an `if let` arm. | Typed by context (`let headers = if let k = ... { {"X-Api-Key": k} } else { {} }`). This one I am least sure parses. |

## 2. Habits

- Started writing `type Limiter struct { mu sync.Mutex; buckets map[string]*Bucket }` (Go). Changed to `buckets: Mutex[Map[Str, Bucket]]` and do all bucket updates inside `with`.
- Wanted `*Bucket` pointers stored in the map so `take` mutates in place. xo has no pointers, so: `var b = m.get(key) ?? new_bucket(...)`, `b.take(now)`, `m.put(key, b)`.
- Wanted `time.Since(b.last)` / `elapsed.Seconds()` (Go). Wrote `now - b.last_refill` and `.seconds()` as guesses, and passed `now` explicitly instead of reading a global clock.
- Reached for `math.Ceil` and `int(...)`. Changed to method style `wait.ceil()` and `Int(...)`.
- Wrote `go func() { ticker := time.NewTicker(...) }()` mentally, plus a `ctx.Done()` check. Changed to `scope s { s.spawn(...) }` with `clock.sleep` as the cancellation point and `s.cancel()` after serve returns.
- Started a `defer cancel()` inside main (Go habit). Dropped it because `defer` is block scoped and the scope already waits; explicit `s.cancel()` reads clearer.
- Almost wrote Rust style `Some(x)` / `None` and `if let Some(k) = api_key`. Changed to `some("vip")`, `none`, and `if let k = api_key`.
- Wanted `for i := 0; i < 20; i++` and then `for _ in 0..20` (Rust). Kept the Rust form but it is a guess.
- Wanted to return `(Bool, Int)` from take. The task fixes `take(now) -> Bool`, so I added a `Decision` enum at the limiter level instead of a tuple.

## 3. Friction

- **No in place mutation of map values.** Every bucket update is a read, copy, mutate, write back:
  ```
  var b = m.get(key) ?? new_bucket(limit, now)
  let allowed = b.take(now)
  m.put(key, b)
  ```
  Forgetting `m.put` would silently lose the update, and nothing in the type system catches it.
- **Mutation while iterating a map.** Unclear if `for (k, b) in m { m.remove(k) }` is legal, so I take a snapshot first (`let snapshot = m`). Value semantics make it probably fine, but the spec does not say.
- **Effects on function types leak into the middleware signature.** The handler type must name `uses clock`, so `Handler = fn(http.Request) -> http.Response ! Error uses clock`. If a wrapped handler ever needs `net`, every Handler mention must change. Generic effect variables would help.
- **Scope and early exit.** The natural `srv.serve()?` inside the scope might hang forever on the infinite evictor, so I had to write:
  ```
  let served = srv.serve()
  s.cancel()
  served?
  ```
- **Testing time.** The task depends on a fake clock, but the spec never shows how a test gets one or advances it, so roughly half the test file rests on `test.fake_clock` / `advance` / `yield` guesses.
- **std/http surface is unspecified,** so the middleware and every request/response accessor is guessed. This is the single largest source of risk.
- **Explicit numeric conversions** add noise to simple math: `Float(limit.capacity)`, `Int(wait.ceil()).max(1)`.

## 4. Confidence (estimated remaining spec violations)

- `bucket.xo`: 0 to 1. Most risk is in the guessed Float/Duration methods, not the syntax. Possibly the trailing comma in the multi line struct literal.
- `limiter.xo`: 1 to 3. Risky spots: pattern binding `Limited(retry_after_secs)`, closure literal with a `uses` clause, `pub type` alias of a function type with effects, `with` closure mutating `var m`.
- `main.xo`: 1 to 2. Unused `req` parameter, passing a pure `hello` as an effectful `Handler`, scope cancellation semantics.
- `limiter_test.xo`: 2 to 4. `{}` map literal inside `if let` arms, `for _ in`, `?` in tests, `let _ = h(r)?` (discarding a non Result value with `let _`), `test.FakeClock` as a `Clock`, calling a non pub `client_key` and `run_evictor` from tests (assumed fine, same module).
