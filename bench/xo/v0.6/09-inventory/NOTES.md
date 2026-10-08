# Notes: 09-inventory (Xo 0.6)

## Design

- All state (items, reservations, idempotency index, id counters) lives in one
  `Mutex[State]`, so "check available, then hold" is a single critical section
  (no XO0804 split). Available = stock minus the sum of `Held` reservations.
- Every public operation first calls `expire_holds(now)` inside the same `with`
  (lazy release on every read). The closure returns `Result[T, InvErr]` as its
  success value (the "keep and fail" pattern from core 5.4), so expiry is kept
  even when the operation itself fails.
- `run_sweeper` loops `clock.sleep(30s)` then `sweep()`. `main` runs it with
  `spawn_background` next to `srv.serve()`, so a signal cancels the scope,
  `serve` returns, and the sweeper is cancelled (graceful shutdown).
- Errors: 400 bad body, bad qty, missing key; 404 unknown item or
  reservation; 409 insufficient stock, already confirmed, already cancelled,
  idempotency key reused with different item or qty; 410 expired.

## Spec gaps and guesses (all guesses were accepted by the toolchain)

- Whether a closure declared `-> Result[T, E]` (no `!`) may simply end with a
  call that returns `T ! E`. Core 5.4 only shows explicit `Ok(...)`/`Err(...)`.
  I guessed yes and it checked.
- Whether a function declared `-> T ! E` may `return outcome` where
  `outcome: Result[T, E]` came out of a `with`. Core 2.6 says any `Result[T, E]`
  is returned as is; it worked.
- Whether a `var` receiver method can be called on a `var` closure parameter
  (`fn(var st) { st.expire_holds(now) }`). Core 3.4 lists var bindings, var
  parameters, and fields; closure parameters are not named explicitly. It worked.
- Whether `xo test` would fuzz functions on structs holding `Mutex` and `Clock`.
  I avoided `requires` on `Inventory` methods (explicit `if` checks instead),
  because a fuzzer generating random `State` would trip `get_or_fault` on
  inconsistent indexes. The spec does not say how generators treat handle types.
- `invariant` together with `derive Json` ordering (`} invariant ... derive Json`):
  taken from the grammar; it checked.
- Exact timing semantics of `FakeClock.advance` with a background task that
  reschedules its own timer: guessed that repeated timers inside the window all
  fire. The log shows the sweep at 00:15:00 releasing the hold.
- `testing.run_main(main, os)` with `os.proc.signal_after(2m)` to test graceful
  shutdown: passing `main` as a value is not shown in the spec, but worked.

## Suspected compiler or runtime bugs

None observed. Check was clean and all 8 tests passed on the first run.

## Go or Rust habits caught while writing

- Wanted `m[k]` reads (Go) for items; used `get(...).or_err(...)?` and
  `get_or_fault` (XO0410 in 0.6).
- Wanted to mutate a copied reservation then forget to store it (Go value copy
  bug); wrote `r with {status: ...}` and an explicit `put`.
- Wanted `_ =>` arms on my own enums; listed every variant (XO0305).
- Wanted a Go style `ctx` for the sweeper; used scope cancellation instead.
- Avoided `let _ = result` (XO0206); none needed.

## Friction

- No `Ok` count helper on lists of results, so tests use
  `filter(fn(r) { r.is_ok() }).len()` and `is` patterns.
- `task.map_concurrent` was the simplest way to fire 100 concurrent
  reservations without spawning tasks whose failing handles would fail the
  test scope (core 7.1: errors are only consumed through a `let` bound and
  awaited handle, which is awkward for 100 handles in a list).
