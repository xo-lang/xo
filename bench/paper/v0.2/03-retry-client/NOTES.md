# Task 03 retry client: paper test notes (spec v0.2)

Files: `retry.xo` (policy, errors, `get_with_retry`), `profile.xo`
(`Profile`, `ProfileErr`, `fetch_profile`), `retry_test.xo` (tests).

## 1. Spec gaps

1. **`uses` clause with parentheses on a declaration (core 6.2 vs core 15).**
   The `retry` example in 6.2 writes `uses (clock, fx)` on a function
   declaration, but the grammar has `FnClause = "uses" EffectList` with no
   parentheses, and 15 says parentheses are needed only in function *types*.
   I wrote `uses net.dial, clock, rand` without parentheses. Not marked as a
   guess because the grammar is explicit, but the example contradicts it.
2. **Infinite loop as the tail of a function body (core 2.1, 3.2, 15).** 2.1
   says infinite loops have type `Never`, but the grammar lists `while` under
   `Stmt`, not `Expr`, and 3.2 says "the last expression of a block is its
   value". It is unclear whether a body ending in `while { ... }` (with all
   exits via `return`) type checks as `Bytes ! FetchErr`. Marked GUESS in
   `retry.xo`.
3. **Negative `Duration` (core 2.1, std 2.6).** Jitter needs
   `base + base.scale(f)` with `f` in `[-jitter, jitter)`. The spec never says
   whether `Duration` can be negative or whether `scale` accepts a negative
   factor. Marked GUESS. (`d1 - d2` could go negative too, also unspecified.)
4. **Unit variant patterns vs bindings (core 3.5, 15).** `Err(Canceled)` in a
   pattern is grammatically the `ident` binding form. The spec's own examples
   (`None =>`, `Timeout =>`) imply resolution by name, and naming rules make
   `UpperCamel` bindings illegal, but the rule is never stated.
5. **Qualified variant from another module in a pattern (core 3.5, 15).**
   `PatAlt` allows `[TypeName "."] ident "(" ... ")"`. Unclear whether
   `TypeName` can be `http.ClientErr`, and a qualified *unit* variant pattern
   (`http.ClientErr.Canceled`) is not in the grammar at all. I relied on
   prefix omission from the known scrutinee type.
6. **Prefix omission for variants of generic std enums (core 2.3, std 7).**
   In the timeout test I match `Err(TimedOut)` on `Result[Bytes,
   task.Timed[FetchErr]]`. Whether prefix omission works through a module
   qualified generic enum is not stated. Marked GUESS.
7. **Raw strings cannot contain `"` (core 1.6).** There is no way to write a
   JSON literal like `{"id": 7}`: plain strings interpolate braces, raw strings
   end at `"`, and `"""` strings interpolate. Escaping every brace with
   `{{ }}` works but is unreadable. I avoided the problem by serving fixtures
   with `http.json(200, ada)` and the invalid body with `r"{not json"`. No
   guess needed, but the gap is real.
8. **Module identity from a directory that is not an identifier (core 1.1,
   1.5).** The folder is `03-retry-client`; modules must be `snake_case`
   identifiers. The spec does not say what happens if the directory name is
   not a valid identifier (error, or only an error when imported).
9. **`?` on `Option` (core 2.5, 2.6).** `?` is defined only on `Result`. In a
   function returning `T?` there is no early exit for `None`, and `Option` has
   no `and_then` / `flat_map` in std 2.5. Forced nested `if let` in
   `retry_after_of`. Not a contradiction, but a missing piece.
10. **Cancellation observability vs `FetchErr.Canceled` (core 7.2, std 3.3).**
    `clock.sleep` cannot fail, so cancellation during the backoff sleep unwinds
    the task instead of returning. `FetchErr.Canceled` is reachable only when
    cancellation hits the client call or the explicit `task.is_canceled()`
    check. The task asks for a "cancelled" error; the spec makes it mostly
    unobservable to the caller (the caller sees `TimedOut` or nothing). I kept
    the variant and the check.
11. **Does `FakeNet.requests()` include requests failed by `fail(...)`
    (std 8.1)?** Unspecified, so the "connection errors are retried" test only
    checks sleeps and the final error.
12. **Do methods need `pub` to satisfy an interface from another module
    (core 2.4, 3.4)?** I made `display` methods `pub` so `FetchErr` converts to
    `Error` in callers outside the module. The spec example in Appendix A uses
    non pub `display` but within one module.
13. **Spelling.** The task says "cancelled"; std uses `Canceled` everywhere and
    core 7.2 shows a user `FetchErr.Cancelled`. I used `Canceled` to match std.
14. **Does a struct `invariant` hold for a struct used only as a parameter
    value?** I used `invariant max_attempts >= 1 && ...` on `RetryPolicy` and
    rely on it instead of `requires` on `get_with_retry`. Checked "after
    construction (including from defaults)", so it should hold, but whether
    contract fuzzing generates only invariant satisfying `RetryPolicy` values
    is not stated.

## 2. Habits

- **Go three clause `for attempt := 1; attempt <= max; attempt++`.** Changed to
  `var attempt = 1` with `while { ... }` and an explicit `attempt += 1`.
- **Go `time.Duration(secs) * time.Second`.** Changed to `1s * secs` (core 2.1
  `d * n`).
- **Go `1 << (n-2)` / `math.Pow`.** Started writing
  `policy.base_delay * 2.pow(n - 2)`; changed to a capped doubling loop because
  Int and Duration overflow fault.
- **Rust `?` on `Option` in `retry_after_of`.** Wrote
  `let raw = resp.header("Retry-After")?` first; changed to nested `if let`.
- **Rust `Ok(resp.body)` as the success return.** Changed to
  `return resp.body` (2.6: a plain `T` is success).
- **Rust `matches!(r, Err(ProfileErr::Decode(_)))` / Go `errors.As` in tests.**
  Changed to a `match` that yields a `Bool`, then `expect(is_decode)`.
- **Rust `ClientErr::Canceled` paths.** Changed to `.` and prefix omission.
- **Rust `impl Display for FetchErr`.** Changed to a receiver method
  `pub fn (e: FetchErr) display() -> Str`.
- **Go `ctx context.Context` parameter for cancellation.** Removed; the scope
  tree carries cancellation and `task.is_canceled()` checks it.
- **Rust `..Default::default()` for the policy.** Changed to field defaults so
  `RetryPolicy{jitter: 0.0}` works.
- **Go `if err != nil { ... }` after `client.Get`.** Changed to a single
  `match client.get(url)` with nested `match resp.status`.
- **Writing a JSON test fixture as a string literal.** Started typing
  `r"{"id": 7, ...}"`; switched to `http.json(200, ada)`.

## 3. Friction

- **No `?` or `and_then` for `Option`.** Parsing an optional header into an
  optional duration needs three nested levels:
  ```
  if let Some(raw) = resp.header("Retry-After") {
    if let Some(secs) = Int.parse(raw.trim()) {
      if secs >= 0 { return 1s * secs.min(86_400) }
    }
  }
  None
  ```
- **Overflow faults on untrusted input.** `1s * secs` faults if a server sends
  a huge `Retry-After`, so I had to clamp first (`secs.min(86_400)`). There is
  no `saturating_mul` for `Duration` (std 2.1 lists it only for `Int`). Same
  for the backoff: `base_delay * 2.pow(n - 2)` can fault, so `backoff` is a
  guarded loop instead of one expression.
- **Braces interpolate everywhere, raw strings cannot hold quotes.** The
  invalid JSON fixture needs `r"{not json"`; a valid JSON fixture cannot be
  written readably at all.
- **Capability threading.** Every call takes three capabilities plus the
  policy: `get_with_retry(os.net, os.clock, os.rand, "https://api.test/down",
  policy)`. Tests repeat `os.net, os.clock, os.rand` eleven times.
- **Repeated `uses` on every `pub` function.** `get_with_retry` and
  `fetch_profile` both spell `uses net.dial, clock, rand`. Adding logging was
  free (ambient), but adding a new effect to `get_with_retry` forces editing
  every pub caller.
- **Asserting a variant shape in tests is verbose.** When a variant carries a
  value I do not want to pin (`InvalidUrl(msg)`), I need:
  ```
  let invalid = match r {
    Err(InvalidUrl(..)) => true
    _ => false
  }
  expect(invalid)
  ```
- **Tail loop typing.** The natural shape (loop until return) relies on the
  `Never` typing of `while { }` as the body tail, which the grammar does not
  clearly allow.
- **Cancellation semantics.** `clock.sleep` unwinds instead of returning an
  error, so the `FetchErr.Canceled` variant the task asks for is hard to make
  meaningful; the timeout test can only assert `TimedOut` from outside.
- **Lost update rule (XO0520) shaped the loop.** I first kept a
  `var hint: Duration?` across iterations, assigned in each match arm. On the
  last iteration that assignment is never read, which would trip XO0520, so I
  computed the wait immediately after the exhaustion check and dropped the var.

## 4. Confidence

| File | Estimated remaining spec violations |
|---|---|
| `retry.xo` | 1 to 3 (tail `while` typing, negative `Duration`, `uses` form on declarations, unit variant pattern resolution) |
| `profile.xo` | 0 to 1 (possibly `from json.DecodeErr` with a module qualified type, default param with a struct literal) |
| `retry_test.xo` | 2 to 3 (`TimedOut` prefix omission, `Err(InvalidUrl(..))` resolving to `FetchErr` rather than `http.ClientErr`, discarding `Bytes` from `get_with_retry(...)?` as a statement, `os.net.fail` request recording) |
