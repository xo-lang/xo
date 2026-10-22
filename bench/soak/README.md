# bench/soak: a steady-load soak of examples/notes-service

Decision 0325's second item (owner, 2026-10-21): run the service for
hours under steady mixed load and watch for drift, on the Mac natively
and in Docker linux/arm64 (Windows later, through CI with the owner's
approval).

    MODE=native DURATION=2h RATE=500 bench/soak/soak.sh XO OUT
    MODE=docker DURATION=2h RATE=500 bench/soak/soak.sh XO OUT

- `client/` (Go) offers a fixed request rate (open loop: a ticker feeds
  128 workers; a request that finds none free is counted as a drop, so a
  slow server shows as latency and drops, not as a lower offered rate):
  22% create a note (ttl 60 to 180 s, so the purge job removes some),
  28% get one, 10% list (the heavy one), 22% delete one, 18% /health:
  creates and deletes balance, so the store stays about the same size
  (a first mix with more creates than deletes grew the notes table and
  its list cost without bound, which the drift check rightly flagged).
- Every `EVERY` (10 s) it records, to `OUT/result/samples.csv`: requests,
  errors, drops, p50/p99/p99.9 of the interval, the server's RSS, open
  fds and CPU microseconds per request (from /proc on Linux, ps and lsof
  on macOS).
- At the end `drift.txt` judges, after a 10% warmup: RSS growth per
  hour (limit 10% of the warmup-end value), open fds slope (limit 1
  per hour), the p99 of the last quarter against the first quarter
  (limit +20%), and the error rate (limit 0.1%); the exit status is 1
  on any alert.
- The server runs with `XO_LEAKCHECK=1` and gets SIGTERM at the end, so
  its leak-checker report is in `OUT/result/server.log`.
- Once decision 0325's `/metrics` exists the sampler reads the runtime's
  counters from it; until then the series are the process's own.
