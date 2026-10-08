# Notes

## Ambiguities resolved

- Language mapping: `enqueue` returns `(id, created bool)`; an empty idempotency key means "no key". `lease` returns `(Leased, ok, error)`; the error is only for a non positive duration.
- Attempts are counted when a job is leased. `nack` dead letters when `attempts >= max_attempts`, otherwise the job is visible again after `min(30s, 1s * 2^(attempts-1))`, so the first failure waits 1s.
- An expired lease is treated exactly like a nack with reason `"lease expired"`, including the backoff. The backoff is measured from the lease expiry time, not from when the expiry was noticed. Expiry is detected lazily at the start of every queue operation, so an ack after expiry fails even if no other worker has taken the job yet.
- The idempotency window is 24 hours from the first enqueue with that key. After the window a new job is created and the key points to it. Old keys are not garbage collected (memory grows with distinct keys).
- FIFO uses the original enqueue sequence. A retried or redelivered job keeps its original position, so once visible it is ahead of newer jobs of the same priority.
- `retry_dead` resets attempts to 0 and makes the job visible immediately; the last error is kept for history.
- `stats` counts a job waiting out a backoff as queued.
- Shutdown: stop leasing, wait up to the grace period, then cancel handler contexts and wait for every worker and handler goroutine to return (it must, to satisfy "no worker still running"). A handler that returns an error (typically `context.Canceled`) is nacked. A handler that ignores cancellation and returns nil is acked, since its work completed. `Shutdown` returns whether the grace period was exceeded.
- If a heartbeat fails (lease lost), the handler context is cancelled and the worker neither acks nor nacks, because its token is stale.
- A handler panic is recovered and treated as a handler error (nack).
- HTTP: `payload` is required and must be standard base64; unknown JSON fields are rejected with 400. `POST /dead/{id}/retry` returns 404 for both unknown and not dead jobs, and 200 with the job on success. `GET /jobs/{id}` also returns priority and the base64 payload.
- Time in runner tests: the queue takes an injectable `Clock` (fake clock used for queue, backoff and HTTP tests). Runner tests run inside `testing/synctest` bubbles, which fake `time.Now`, timers, tickers and sleeps, so heartbeat, grace period, and the 50 worker test run in fake time.

## Bugs caught by tools

- None of the tools caught a real bug in the implementation. All tests passed with `-race` on the first run.

## False positives

- nilaway (run 1): `resp` from `http.Client.Post` is documented as non nil when `err` is nil, but nilaway cannot see that contract through the stdlib. I resolved it by adding an explicit `if resp == nil { t.Fatal(...) }` check in the test rather than suppressing it.

## Suppressions and discarded errors

- None. No `//nolint` comments and no `_ =` discarded errors. (Some tests ignore the `created bool` result of `Enqueue` with `id, _ :=`; that is not an error value.)
