# Notes: 09-inventory (strict Go)

## Task ambiguities and decisions
- Idempotency key scope: global (not per item). Reusing a key with a different item or qty returns 409 (ErrConflict) instead of silently returning the old reservation. An empty key disables idempotency. The key may come from the JSON body or an `Idempotency-Key` header.
- Idempotent replay returns 200 with the original reservation (first call 201). The replay returns the reservation in its current state, even if it was since confirmed, cancelled, or expired.
- Error codes: 400 invalid input or bad JSON; 404 unknown item or reservation; 409 not enough stock, already confirmed (confirm twice, cancel after confirm), confirm after cancel, duplicate item id, idempotency mismatch; 410 confirm or cancel of an expired hold.
- Cancel of an already cancelled reservation is a no op success (200). Cancel of an expired hold is 410.
- Expiry boundary: a hold expires at exactly created + 15 minutes (`!now.Before(expiresAt)`).
- Lazy release: every read (GET item, GET reservation) and every reserve sweeps all active holds under the lock, not only those for the requested item. The sweeper loop takes a tick channel so tests drive it without sleeping; main feeds it a 30 s ticker.
- Graceful shutdown: SIGINT/SIGTERM triggers `http.Server.Shutdown`, then the sweeper context is cancelled and `run` waits for the sweeper goroutine to exit.
- Added `GET /reservations/{id}` (not required) for observability.

## Bugs caught by each tool
- vet, staticcheck, errcheck, exhaustive, race: nothing found on any run.
- nilaway: 3 findings on run 1, all in test code, all false positives (see below). No real bug was caught.
- Tests: passed on the first run.

## False positives
- nilaway flagged `resp` from `http.Post` / `http.Client.Do` as possibly nil even though `err` was checked first. net/http guarantees a non nil response when err is nil, but nilaway follows an internal path in client.go (`unassigned variable resp returned from do()`). Fixed by wrapping the client in `send()` that returns an explicit error if `resp == nil`. No nolint comments and no `_ =` were used.
