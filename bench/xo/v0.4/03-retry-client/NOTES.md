# Notes: 03-retry-client (Xo 0.4, with checker)

## Result

The complete solution passed `xo check` on the first run with 0 errors and 0
warnings. A probe run (BUILD_LOG run 3) confirmed that `_test.xo` files are
type checked (XO0110 and XO0401 were reported inside a test block), so the
clean result is not just the checker skipping tests. The checker cannot run
programs, so none of the test assertions are verified at runtime.

## Layout

- `retry.xo`: `RetryPolicy`, `AttemptErr`, `FetchErr`, `get_with_retry`,
  classification, `Retry-After` parsing, backoff, jitter.
- `profile.xo`: `Profile`, `ProfileErr` (`Fetch(from FetchErr)`,
  `Decode(from json.DecodeErr)`), `fetch_profile`.
- `retry_test.xo`: 14 tests (the 5 required plus jitter bounds, Retry-After
  cap, connection retry, transport exhaustion, invalid URL, cancellation,
  backoff unit test, profile success, profile fetch error wrapping).

## Spec gaps and guesses

1. **Which `ClientErr` variants are "connection errors".** The task says retry
   on connection errors and timeouts. I retry `Connect`, `Timeout`, and
   `Protocol`; `Canceled` maps to `FetchErr.Canceled`; `InvalidUrl` maps to
   `FetchErr.InvalidUrl`. Whether `Protocol` should be retried is a judgment
   call, not something the task or spec settles.
2. **Statuses outside 2xx/4xx that are not in the retry list** (3xx after
   redirects, 501, 505, 1xx). The task only says "do not retry other 4xx". I
   treat every non 2xx status not in the retry list as `NonRetryable(status)`.
3. **Jitter and the max_delay cap.** The task formula applies jitter to the
   already capped value, so the actual sleep can exceed `max_delay` by up to
   `jitter`. I followed the formula literally. `Retry-After` delays are capped
   and not jittered.
4. **How cancellation becomes observable.** Core 7.2 says `clock.sleep`
   unwinds a cancelled task instead of returning, so `FetchErr.Canceled` can
   only come from `client.get` returning `ClientErr.Canceled`. I did not add a
   `task.is_canceled()` check after the sleep because it can never observe
   `true` there. The cancellation test uses `os.net.fail(..., Canceled)`
   because the spec gives no way to cancel a call from inside a test other
   than `task.with_timeout`, which would test a timeout, not the mapping.
5. **Whether `FakeNet.fail_next` takes priority over a registered `handle` for
   the same pattern.** std 8.1 does not say. The "connection errors are
   retried" test assumes the failure wins for the next request.
6. **`os.net.requests()` counts failed requests** ("including failed ones" in
   std 8.1), and I assumed it does not count a URL rejected before any request
   (`parse_url` fails first, so no request is made anyway).
7. **Default parameter value that is a struct literal**
   (`policy: RetryPolicy = RetryPolicy{}`). Core 3.2 allows defaults but does
   not say which expressions qualify. The checker accepted it.
8. **`const` struct literal at top level of a `_test.xo` file**
   (`const NO_JITTER = RetryPolicy{jitter: 0.0}`), relying on field defaults
   plus an `invariant`. Core 3.1 says struct construction is a compile time
   value; it does not say whether an `invariant` is checked at compile time
   for a const. Accepted.
9. **Effect labels in a pub `uses` clause.** I declared
   `uses (net.dial, clock, rand)`. `http.Client.get` is `uses net.dial`, so the
   narrower label is enough. Accepted.
10. **`?` as a statement whose value is discarded.**
    `http.parse_url(url).map_err(...)?` is used only for validation; the `Url`
    is a non Result value discarded as an expression statement (core 2.6, last
    bullet). Accepted.
11. **`expect(x) >= y` on `Duration`.** Core 9.2 says "any comparison";
    I relied on it for the jitter bounds test. Accepted.

## Suspected checker bugs

None found. Everything the checker accepted matches my reading of the spec,
and the only diagnostics I saw (in the probe) were correct.

Caveat: a zero error first run with no runtime means I cannot tell whether the
checker is lenient in areas I relied on (for example, whether it verifies that
`Retry(cause, hint)` positional binding matches declared field order, or the
`Exhausted(last: AttemptErr.Status(code), ..)` named pattern). Those would be
worth a targeted planted bug test.

## Go or Rust habits caught

- **Go:** wanted `for attempt := 1; attempt <= max; attempt++` with a `ctx`
  check. Xo has no C style `for` and no `ctx`; I used `var attempt` plus
  `while { }` (type `Never`, so it can end the function body) and relied on
  scope cancellation.
- **Go:** wanted `math.Pow` or `1 << n` for the backoff. Avoided both: a
  doubling loop that stops once it reaches `max_delay` cannot overflow
  (`Duration` overflow faults in Xo). Also avoided `2.pow(n)` because I was not
  sure whether `2.` lexes as a float start.
- **Go:** wanted `time.Duration(float64(d) * f)`. Xo has `d.scale(f)`; there
  is no implicit numeric conversion.
- **Rust:** wanted `impl Display for FetchErr`. Xo uses a plain method
  `fn (e: FetchErr) display() -> Str` and structural satisfaction.
- **Rust:** wanted `#[from]` style error wrapping; Xo's `Decode(from
  json.DecodeErr)` gives the same `?` conversion directly.
- **Rust/Go:** wrote `let _ = ...` for the URL validation at first in my head;
  0.4 makes that XO0206 on a `Result`, and here `?` is applied first so the
  value is a `Url`, which is fine as a bare statement.

## Friction

1. **Cannot observe cancellation after a sleep.** The task wants a
   `Canceled` error, but `clock.sleep` unwinds the task (core 7.2), so the
   retry loop can only report cancellation if the HTTP call itself returns
   `ClientErr.Canceled`. Testing it needed a fake failure instead of real
   cancellation:
   ```
   os.net.fail("GET https://api.test/cancel", http.ClientErr.Canceled)
   ```
2. **Testing jitter against a fake clock.** `os.clock.sleeps()` returns exact
   durations, but jitter makes them random, so exact assertions need a
   policy with `jitter: 0.0` and a separate bounds test:
   ```
   for (i, d) in sleeps.indexed() {
     expect(d) >= bases[i].scale(0.8)
     expect(d) <= bases[i].scale(1.2)
   }
   ```
   The spec does not say whether `os.rand` is seeded identically on every
   run, so a pinned expected list was not an option.
3. **Getting an error value out of a `Result` in a test.** There is no
   `unwrap_err`; I used `r.err().or_fault("...")`, which reads well but took a
   moment to find in std 2.5:
   ```
   let e = r.err().or_fault("expected the fetch to fail")
   expect(e.last_status()) == Some(502)
   ```
4. **Display boilerplate.** Each error enum needs a hand written `display`
   method to be usable with `?` into `Error` and with `${e}`. `derive Display`
   would work but prints the debug shape, which is not a good user facing
   message for an error that "includes the status code".
5. **Retry-After parsing composes nicely** but only because the helper returns
   `Duration?`; inside `get_with_retry` (which returns a `Result`) the same
   `?` on an Option would be XO0204. Splitting it into a helper was required,
   not just stylistic:
   ```
   fn retry_after(resp: http.Response) -> Duration? {
     let secs = Int.parse(resp.header("Retry-After")?.trim())?
     ...
   }
   ```
