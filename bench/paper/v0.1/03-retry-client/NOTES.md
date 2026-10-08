# Task 03 paper test notes

Files: `retry.xo` (policy, error types, `get_with_retry`), `profile.xo`
(`Profile`, `ProfileErr`, `fetch_profile`), `retry_test.xo` (tests).

## 1. Spec gaps

| # | Spec section | Gap | What I guessed |
|---|---|---|---|
| 1 | 2.6, 3.2 | How a `T ! E` function fails explicitly. Examples only show success returning a plain `T` and failure via `?` or `requires ... else`. | `return err(e)` for early failure, and a tail `err(e)` as the block's failing value (end of `get_with_retry`). Unclear whether the tail must be `T` only. |
| 2 | 14 (`std/http`) | No client API at all. | `http.parse_url(Str) -> http.Url ! http.UrlErr`, `http.client(net)`, `Client.get(url) -> http.Response ! http.ClientErr uses net`, `Response{status: Int, headers: Map[Str, Str], body: Bytes}`. |
| 3 | 14 (`std/http`) | Variants of the client error type. This matters a lot because `match` must be exhaustive. | `Connect(msg)`, `Protocol(msg)`, `Timeout`, `Canceled`. |
| 4 | 14 | HTTP header case insensitivity. | Exact match on `"Retry-After"` via `Map.get`. |
| 5 | 6.3 | Capability type names (`Net`, `Clock`, `Rand`) are named as `Os` field types but never said to be predeclared or whether they are interfaces or structs. | Predeclared, and interfaces, so test fakes can be passed where `Clock` is expected. |
| 6 | 6.1 | `Clock` and `Rand` method surface. | `clock.sleep(Duration) -> ()` with no error; `rand.float() -> Float` in [0, 1). |
| 7 | 2.1 | Duration arithmetic and conversions. Only `Int32(x)` style conversion is shown. | `Duration * Int` (used as `d * 2` and `1s * secs`); `d.nanos()`, `Duration.from_nanos(Int)`, `Float(Int)`, `Int(Float)`, `Float.round()`. |
| 8 | 2.1 | No `min` for Durations; only a generic `max[T: Ord]` example. | Wrote a local `min_duration`. |
| 9 | 2.1 | String to Int parsing. | `Int.parse(Str) -> Int?` (modelled on `Int32.try`) and `Str.trim()`. |
| 10 | 7.2 vs task | Task requires a `Cancelled` error, but 7.2 says code cannot observe cancellation; the task just unwinds with `err(Canceled)`. `Canceled` is also not in the predeclared list. | Kept `FetchErr.Cancelled`, produced only when the http client reports a cancelled request. Cancellation during `clock.sleep` cannot be turned into a `FetchErr`. |
| 11 | 2.7 | Explicit type argument syntax at a call site. | `json.decode[Profile](body)`, mirroring the declaration syntax. |
| 12 | 14 (`std/json`) | Decode function name and error type. | `json.decode[T](Bytes) -> T ! json.DecodeErr`, error implements `Display`. |
| 13 | 2.6 | Construction syntax for a `from` variant. | Positional: `ProfileErr.Fetch(FetchErr.Status(...))`. |
| 14 | 3.4, 15 | Wildcard `_` pattern, nested patterns (`err(FetchErr.Exhausted(a, l))`), and Int literal patterns are never shown. | Assumed all are allowed. |
| 15 | 15 | Grammar says match arms are `Pattern => Expr`; assignment is a statement. | Wrapped assigning arms in a block: `Timeout => { last = some(...) }`. |
| 16 | 8 | Type of `fault(msg)`. | Bottom type, so `last ?? fault("...")` type checks. |
| 17 | 4.1, 1.4 | `use std/test` makes the local name `test`, which is a keyword. | `use std/test as t`. |
| 18 | 6.4, 14 | How tests get fake capabilities, and how to inspect a fake clock. Tests have no `os` parameter in the examples. | `t.fake_net()`, `t.fake_clock()`, `t.fake_rand(seed:)`, `fake_net.handle(pattern, fn)`, `fake_clock.sleeps() -> List[Duration]`. |
| 19 | 9.2 | Whether `expect` needs an import. | Used bare, as in the spec examples. |
| 20 | 3.1 | Whether unused closure parameters trigger XO0110. | Named them `_req`. |
| 21 | 3.1 | Whether a `const` can hold a struct literal. | Yes: `const NO_JITTER = RetryPolicy{jitter: 0.0}`. |
| 22 | 1.6 | Whether `b"..."` interpolates `{}` and which escapes it accepts. | No interpolation, `\"` accepted. |
| 23 | 5.4 | Return value of `Atomic.add`. | Returns the new value. |
| 24 | 9.1 | `xo test` fuzzes every function with contracts. `backoff_delay` has a `requires` and takes a `Rand` capability; no generator for capabilities is defined. | Left the contract in; assumed the fuzzer supplies a seeded fake. |
| 25 | 2.2 | Whether `RetryPolicy{}` (all fields defaulted) is valid, and whether an `invariant` can reference several fields joined with `&&`. | Both allowed. |
| 26 | Task | Status codes outside 2xx that are not 4xx and not in the retry list (3xx, 501). | Treated like non retryable 4xx: return `FetchErr.Status`. |

## 2. Habits

- Wanted `for attempt := 1; attempt <= n; attempt++` (Go). Changed to
  `for attempt in 1..=policy.max_attempts`.
- Wanted `var last error` with a zero value (Go). No zero values or nil, so it
  became `var last: AttemptErr? = none` and assignments `last = some(...)`.
- Wanted `return nil, fmt.Errorf(...)` / `Err(...)` (Rust). Wrote
  `return err(FetchErr.Status(code: code, url: url))`.
- Wanted `time.Duration(float64(d) * factor)` (Go). Had to guess explicit
  conversions via nanos.
- Wanted Rust's or patterns `429 | 500 | 502 | 503 | 504 =>`. Not in spec, so
  used a `Set` constant with `.contains`.
- Wanted `use std/test` and call `test.fake_clock()`. Hit the keyword clash,
  renamed with `as t`.
- Wanted a closure counter `n++` captured by reference (Go). Closures capture
  by value, so used `Atomic[Int]`.
- Wanted `.map_err(...)` (Rust) on the `json.decode` result. Not defined, so
  used `match` with a `return err(...)` arm.
- Started to write `min(a, b)` as a builtin (Go 1.21). Wrote `min_duration`.
- Started to write `Connect(msg) => last = ...` as a match arm. Assignment is not
  an expression in the grammar, so wrapped in `{ }`.

## 3. Friction

- **Optionals need explicit `some`.** Because the only implicit conversion is
  `Display` to `Error`, every assignment to an optional var is wrapped:
  ```
  last = some(AttemptErr.BadStatus(code: code))
  ```
  and the end of the loop needs an unwrap that can never fail:
  ```
  let final = last ?? fault("max_attempts invariant violated: no attempt was made")
  ```
- **Cancellation cannot be reported as a value.** The task asks for
  `FetchErr.Cancelled`, but section 7.2 says cancellation unwinds at
  `clock.sleep`. The `Cancelled` variant is only reachable through a guessed
  http client error:
  ```
  Canceled => return err(FetchErr.Cancelled)
  ```
- **Exhaustive match over an unknown stdlib enum.** I had to invent the full
  variant list of `http.ClientErr`; a single wrong guess is a compile error.
- **Value semantics in test fakes.** A handler closure cannot mutate a
  captured counter, so each test builds an `Atomic`:
  ```
  let calls = Atomic[Int].new(0)
  net.handle("GET {USER_URL}", fn(_req) {
    let n = calls.add(1)
    if n <= 2 { reply(503) } else { reply_body(200, b"hello") }
  })
  ```
- **Capabilities in every signature.** Three capability params plus `uses net,
  clock, rand` on both `pub` functions; `fetch_profile` has six parameters and
  has to re-thread all of them just to call `get_with_retry`.
- **No explicit-failure syntax.** Unsure whether to write `err(x)` as a tail or
  `return err(x)`; mixing both in `get_with_retry` feels inconsistent.
- **Verifying delays depends on unspecified fake clock API.** The core test
  assertion `expect(clock.sleeps()) == [100ms, 200ms]` rests entirely on a guess.
- **Duration math.** Exponential backoff with jitter needs Float scaling of a
  Duration; with no implicit conversions and no documented conversions this is
  four guessed functions in one line:
  ```
  Duration.from_nanos(Int(nanos.round()))
  ```

## 4. Confidence

| File | Estimated remaining spec violations |
|---|---|
| `retry.xo` | 2 to 4. Most risk is in guessed stdlib names (not spec violations as such), the tail `err(...)`, `fault` in `??`, and the multi-field `invariant`. |
| `profile.xo` | 0 to 2. Explicit type args `json.decode[Profile]`, default param holding a struct literal. |
| `retry_test.xo` | 3 to 6. Fake capability API, `use std/test as t`, const struct literal, nested and wildcard patterns, `_req` naming, bytes literal escapes, property test over a private function. |
