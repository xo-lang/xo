# Notes

## Ambiguities resolved

- **Attempt counting.** An attempt is counted when a job is leased. `nack` and lease expiry dead letter the job when `attempts >= max_attempts` (default 5); otherwise backoff is `min(30s, 1s * 2^(attempts-1))` using the post lease attempt count (1s, 2s, 4s, 8s, 16s, 30s, ...).
- **Expired lease visibility.** The spec says nack waits a backoff but an expired lease simply "becomes visible again". I made expired jobs visible immediately at the moment the lease expired (the lease duration already acted as a delay), but the expiry still counts as a failed attempt with last error "lease expired".
- **Lease expiry is lazy.** Every queue operation first reaps expired leases using the injected clock, so there is no background goroutine. An ack after expiry fails even if no other worker has re-leased the job yet.
- **Idempotency.** Empty key means no key. The window is `now - first_enqueue < 24h`; at exactly 24h a new job is created. `Enqueue` returns `(id, duplicate bool)` so the HTTP layer can choose 200 vs 201. A duplicate ignores the new payload and priority.
- **retry_dead.** Resets attempts to 0, keeps the last error for diagnostics, makes the job visible now, and puts it at the back of its priority level (new FIFO sequence number).
- **Stats.** `queued` includes jobs waiting out a backoff.
- **Shutdown cancellation.** Any job whose handler was running when the grace period elapsed is nacked, even if the handler returns nil after cancellation. `Shutdown(grace)` returns whether it had to cancel. A handler that ignores its context blocks shutdown, since the spec forbids returning with workers still running.
- **Heartbeat failure.** If `extend` fails (lease lost), the handler context is cancelled and the error is reported via `OnError`; the final ack/nack will then fail as stale and is also reported.
- **Handler panics** are recovered and treated as errors (nack).
- **Non positive lease durations** default to 30s.
- **HTTP.** Invalid JSON or invalid base64 returns 400. `POST /dead/{id}/retry` returns 404 for both unknown and not dead jobs, and 200 with the job on success. `GET /jobs/{id}` returns id, state, attempts, last_error, priority.
- **Time in runner tests.** The queue uses an injected clock; the runner's heartbeat ticker uses real time. Deterministic heartbeat and backoff tests run against the queue with a fake clock; the runner heartbeat test uses short real durations (150ms lease, 600ms job).

## Known limitations

- Done jobs and idempotency entries are kept forever (no pruning), so memory grows with total jobs enqueued.
- Lease selection is a linear scan over queued jobs, fine for this scale but not for very large queues.

## Bugs the loop caught

- Run 1: the HTTP test helper JSON decoded every response body, including the plain text 405 from `ServeMux`. Test bug only; fixed by decoding only `application/json` responses. No implementation bugs were caught by the loop.
