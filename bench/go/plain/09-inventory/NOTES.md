# Notes: 09-inventory

## Ambiguities resolved

- Item id: client may supply `id` in `POST /items`; if omitted a random id is generated. Duplicate id returns 409. Missing `stock`, negative stock, or empty name returns 400.
- Idempotency key: accepted in the JSON body (`idempotency_key`) or the `Idempotency-Key` header. Keys are global (not per item). A replay returns the original reservation with 200 (new reservation returns 201), even if it has since been confirmed, cancelled or expired (its current status is shown). Reusing a key with a different item or qty returns 400.
- Status mapping: not enough stock, already confirmed, cancelled, and duplicate item id all return 409. Expired returns 410 for both confirm and cancel. Confirming twice is 409. Cancelling a cancelled reservation is 409 (not treated as idempotent).
- A hold expires at exactly created + 15 minutes (`now >= expires_at` counts as expired).
- Lazy expiry runs on `GET /items/{id}`, on reserve (for that item), and on any read or mutation of a reservation. The background sweep runs every 30 seconds over all items.
- Concurrency: one mutex over the whole store. Simple, and makes the never oversell invariant obvious. Throughput was not a stated requirement.
- Sweeper is testable through `RunSweeper(ctx, ticks)` with an injected tick channel; production uses `StartSweeper(30s)` whose stop function cancels and waits for the goroutine. `main` stops it after `http.Server.Shutdown` on SIGINT/SIGTERM.
- Added `GET /reservations/{id}` as a convenience (not required).

## Bugs caught by the loop

None. The first run of build, vet and test passed.
