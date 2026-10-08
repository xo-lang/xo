# Task 09: Inventory reservations

Build an in memory inventory service that never oversells.

## Requirements

- Items have an id, a name, and a stock count.
- `reserve(item_id, qty, idempotency_key) -> reservation`: holds `qty` units for
  15 minutes. Fails if not enough unreserved stock. The same idempotency key
  returns the same reservation instead of reserving again.
- `confirm(reservation_id)`: turns the hold into a sale (stock decreases).
  Fails if the reservation expired or was cancelled.
- `cancel(reservation_id)`: releases the hold.
- Expired holds are released automatically (a background sweep every 30
  seconds, and lazily whenever stock is read).
- Available stock = stock minus active holds. It must never go below zero,
  even with many concurrent reservations for the same item.
- HTTP API: `POST /items` (create), `GET /items/{id}` (stock, held, available),
  `POST /reservations`, `POST /reservations/{id}/confirm`,
  `POST /reservations/{id}/cancel`. Errors map to 400, 404, 409 (not enough
  stock, already confirmed), and 410 (expired).
- Graceful shutdown stops the sweeper.

## Tests to write

- 100 concurrent reservations of 1 unit for an item with 10 in stock: exactly 10 succeed.
- Idempotent reserve returns the same reservation and holds stock once.
- An expired hold is released (fake clock) and confirm then returns 410.
- Confirm then cancel fails; cancel then confirm fails.
- Available stock is correct after a mix of reserve, confirm, cancel, expiry.
