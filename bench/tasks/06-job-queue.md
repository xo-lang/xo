# Task 06: Job queue with leases

Build an in memory job queue with leases, retries, and a worker runner, plus a
small HTTP API.

## Queue

- `enqueue(payload: bytes, priority: int, idempotency_key: optional string) -> job id`.
  If the same idempotency key was enqueued in the last 24 hours, return the
  existing job id and do not create a new job.
- `lease(worker_id, lease_duration) -> optional (job, lease_token)`: returns the
  visible job with the highest priority (FIFO within a priority) and hides it
  until the lease expires. Each lease gets a new token.
- `ack(job_id, token)`: marks the job done. Fails if the token is not the
  current lease (for example the lease expired and another worker took it).
- `nack(job_id, token, reason)`: records the failure. If the job has used
  `max_attempts` (default 5) it moves to the dead letter list; otherwise it
  becomes visible again after a backoff of `min(30s, 1s * 2^(attempts-1))`.
- `extend(job_id, token, lease_duration)`: heartbeat; fails on a stale token.
- An expired lease counts as a failed attempt and the job becomes visible
  again (or dead letters if attempts are used up).
- `retry_dead(job_id)`: moves a dead job back to the queue with attempts reset.
- `stats()`: counts of queued, leased, done, dead.
- Safe for concurrent use by many workers.

## Worker runner

- Runs N workers that lease jobs and call a handler.
- While a handler runs, the worker extends the lease every `lease_duration / 3`.
- A handler error nacks the job; success acks it.
- Graceful shutdown: stop leasing, wait for running handlers up to a grace
  period, then cancel them; cancelled jobs are nacked. No worker may still be
  running after shutdown returns.

## HTTP API

- `POST /jobs` with `{"payload": "<base64>", "priority": 0, "idempotency_key": "..."}` returns `{"id": "..."}` (201, or 200 for a duplicate key).
- `GET /jobs/{id}` returns state, attempts, last error.
- `GET /stats`.
- `POST /dead/{id}/retry` (404 if not dead).

## Tests to write

- Lease expiry redelivers the job to another worker, and the first worker's ack then fails.
- A job dead letters after max attempts; `retry_dead` brings it back.
- Backoff delays are correct (use a fake clock).
- Heartbeats keep a long running job from being redelivered.
- Shutdown nacks in flight jobs after the grace period and leaves no running workers.
- Idempotency key deduplicates.
- Priority ordering, FIFO within a priority.
- 50 workers, 1000 jobs, no failures: every job handled exactly once.
