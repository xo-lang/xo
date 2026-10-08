# Task 03 paper test notes (spec v0.3)

Files: `retry.xo` (policy, errors, `get_with_retry`), `profile.xo`
(`Profile`, `ProfileErr`, `fetch_profile`), `retry_test.xo` (tests).

## 1. Spec gaps

1. **Qualified type in a `from` variant** (core 2.6, grammar 15 `VField`,
   `TypeName = ident`). The grammar allows only a bare identifier as a type
   name, yet `http.Response`, `json.DecodeErr`, `task.Timed` appear as types all
   over the spec. I wrote `Decode(from json.DecodeErr)` and guessed it is legal.
   The grammar should say `TypeName = [ ident "." ] ident`.
2. **Doc comments on fields and variants** (core 1.2). "Attaches to the next
   declaration" does not say whether a struct field or enum variant is a
   declaration. Guessed yes.
3. **`FakeNet.fail` lifetime** (std 8.1). `MemFs.fail_next` says "once", but
   `FakeNet.fail` says nothing. Guessed it fails every matching request, which
   the "connection errors are retried" test relies on. Also unclear: if both
   `handle` and `fail` match a pattern, which wins.
4. **What `http.parse_url` rejects** (std 4.5). No URL grammar, so the
   invalid URL test input `"::not a url::"` is a guess. Also unclear whether
   `parse_url` needs a scheme, and whether `client.get` re validates (it has
   its own `ClientErr.InvalidUrl`, so I handle both).
5. **Enum prefix on other module variants in patterns** (core 2.3 vs 3.5).
   2.3 says enums from other modules "are qualified with the module", 3.5 says
   a unit variant pattern may be "bare or qualified" and shows
   `http.ClientErr.Canceled`. Not clear if bare `Canceled` works when the
   scrutinee is an `http.ClientErr`. I qualified every one, not marked as a
   guess since qualified is clearly legal.
6. **Prefix omission and name collisions** (core 2.3). `AttemptErr.Timeout`
   and `http.ClientErr.Timeout` both exist. Inside `Again(err: ..., )` the
   expected type is known so bare `Timeout` should resolve, but I wrote
   `AttemptErr.Timeout` because the spec does not say what happens when a
   bare name matches variants of several enums in scope.
7. **Result and Option `Eq` / comparison in `expect`** (core 9.2). `expect(r)
   == Err(FetchErr.Status(404))` assumes `Result[Bytes, FetchErr]` is `Eq` and
   that `Err(...)` infers its `T` from the left side. 2.9 implies it ("Eq when
   all fields do") but `Result` is not a struct, and std 1 does not list `Eq`
   for `Result`/`Option`.
8. **Does `client.get` follow redirects?** (std 4.5). Not stated. I treat any
   non 2xx status that is not retryable (including 3xx) as
   `FetchErr.Status(code)`.
9. **Shift operator semantics** (core 15 lists `<<`). Not specified on
   overflow or negative counts. I cap the shift at 30 so it does not matter.
10. **`invariant` on a struct with defaults used in `const`** (core 9.1,
    3.1). `const EXACT = RetryPolicy{jitter: 0.0}`: is the invariant checked
    at compile time for const values? Assumed yes or at first use.
11. **No `matches` helper.** There is no boolean "does this value match this
    pattern" expression, so tests use a `match` returning `true`/`false`. Not
    a gap exactly, but the spec has no `if let` negative form either.
12. **Cancellation observability in a retry loop** (core 7.2). `clock.sleep`
    unwinds on cancel instead of returning, so a "cancelled" `FetchErr` can
    only come from `ClientErr.Canceled` or an explicit `task.is_canceled()`
    check. It is unclear whether returning `Err(Canceled)` from a cancelled
    task is useful at all, since nobody awaits it (a cancelled task's failure
    "does not count"). I added the check anyway to satisfy the task.

## 2. Habits

1. Started to write `for attempt in 1..=policy.max_attempts { ... }` with a
   `last_err` variable after the loop (Go style). That needs a sentinel
   initial value because there is no nil. Changed to `while { }` with the
   exhaustion check inside the `Again` arm, so the loop type is `Never` and
   every exit is a `return`.
2. Wanted `errors.Is` / `err == ErrX` style sentinel errors. Changed to an
   enum with fields (`Exhausted(attempts, last)`).
3. Wanted Rust's `matches!(r, Err(ProfileErr::Decode(_)))` in tests. Wrote a
   `match` that yields `Bool`.
4. Started writing `ctx context.Context` as the first parameter for
   cancellation. Removed: the scope tree carries it.
5. Started to type `math.Min(...)` / `std::cmp::min`. Changed to the `Ord`
   method `d.min(policy.max_delay)`.
6. Almost wrote `time.Duration(float64(d) * f)` for jitter. Changed to
   `d.scale(f)`.
7. Wanted a Go style `if err != nil` after `http.parse_url`. Changed to
   `.map_err(...)?` as an expression statement, discarding the `Url`.
8. Reached for `fmt.Sprintf("%d", code)`. Changed to `"${code}"`.

## 3. Friction

1. **Three effectful capability parameters on every call.** The pub
   signature must list `uses net.dial, clock, rand` and every caller passes
   three capabilities. `fetch_profile` repeats all of it:
   ```
   pub fn fetch_profile(net: Net, clock: Clock, rand: Rand, base_url: Str, user_id: Int, ...)
     uses net.dial, clock, rand
   ```
   Every test call is `get_with_retry(os.net, os.clock, os.rand, URL, EXACT)`.
   A small struct bundling them would help but then the struct type cannot
   carry effects.
2. **Jitter breaks exact delay assertions.** Because `os.rand` is seeded but
   its sequence is unknown to the test author, every exact `sleeps()` test
   needs `jitter: 0.0`, and a separate test checks the bounds. I made a
   `const EXACT = RetryPolicy{jitter: 0.0}` to cut noise.
3. **Two enum names for the same idea.** `http.ClientErr.Timeout` and my
   `AttemptErr.Timeout` both exist, so the pattern and the construction in
   one arm need different spellings:
   ```
   Err(http.ClientErr.Timeout) => Again(err: AttemptErr.Timeout, retry_after: None)
   ```
4. **Asserting an error shape without `Eq` on the whole value.** For the
   decode test I cannot build the expected `json.DecodeErr` (unknown `path`
   and `msg`), so a 4 line `match` to `Bool` replaces a one line assertion:
   ```
   let is_decode = match r {
     Err(ProfileErr.Decode(_)) => true
     _ => false
   }
   expect(is_decode)
   ```
5. **Exhaustive match over `ClientErr`.** Every transport variant has to be
   placed in a category, including `Protocol`, which the task does not
   mention; I added `FetchErr.Protocol` to keep the error model honest
   instead of folding it into another variant.
6. **Mixing `Never` and `()` arms.** The loop body is a `match` whose arms
   are `return body`, `return Err(err)`, and a block ending in
   `clock.sleep(wait)`. It type checks only through the `Never` converts to
   every type rule, which is easy to forget when reading.

## 4. Confidence

| File | Estimated remaining spec violations |
|---|---|
| `retry.xo` | 0 to 1 (most likely: doc comment on a field, or bare `BadStatus(...)` resolution inside a variant field) |
| `profile.xo` | 0 to 1 (qualified type in `from`) |
| `retry_test.xo` | 1 to 2 (`Eq` on `Result` in `expect`, `FakeNet.fail` semantics, invariant in `const`) |
