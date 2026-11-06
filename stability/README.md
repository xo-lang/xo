# stability

The stability program's corpus (decision 0520): real Xo services, each run on
the offload hosts through a cross backend differential, load, a soak, chaos and
fuzzing. Phase 1 builds the starter app and the harness.

- [notes/](notes/): the notes-service example with its notes on disk
  (journal.xo: a snapshot and an append log, replayed at start), the starter
  app. `NOTES_DATA=DIR` keeps notes in DIR; `NOTES_FSYNC=1` fsyncs each change
  before the response. Tests: `xo test stability/notes` (also with
  `--backend=go` and `--backend=llvm`).
- [shortener/](shortener/): a URL shortener written blind from the spec
  (decision 0520 phase 2): links with hit counts, an append log with a
  snapshot past 1 MiB, batched hit flushes; NOTES.md lists the spec gaps the
  writer hit; go/ is its Go twin. Tests: `xo test stability/shortener`.
- [harness/](harness/): the checks, in Go (its own module, so it shares no
  runtime with the apps). `harness diff -xo XO -app stability/APP` builds the
  app on each backend (`-backends go,arm64,llvm`, the first the reference),
  runs DIR/app.json's scenario against each build (restarts with SIGTERM or
  SIGKILL included, on the same data directory) and compares status,
  Content-Type and body, with app.json's `redact` keys masked; exit 1 when
  they differ or a step misses its `expect` status. `harness soak -xo XO -app
  stability/APP -out OUT [-backend arm64] [-rate 500] [-d 2h] [-env K=V,...]` runs bench/soak's
  open loop mix (per app, in soak.go) against a build under XO_LEAKCHECK=1 and
  XO_CRASH_FILE=OUT/crash.txt (a report there is an alert),
  samples RSS, fds, latency and the app's `heap_live` and `timers_pending` (app.json `stats`,
  std/debug's /sched) into OUT/samples.csv (on Linux also the open sockets,
  with each fd's link target in OUT/fds-first.txt and OUT/fds-last.txt, so a
  climbing fds column shows whether sockets or files are left open, and the
  harness's own sockets, which are the load client's connections to the app:
  the client keeps a burst's connections pooled for the whole run unless
  `-client-idle DUR` closes them after DUR unused, and the AnonHugePages KiB,
  since RSS counts a transparent huge page whole), and
  writes OUT/drift.txt; exit 1
  on drift (open fds judged on the run's second half, so a connection pool
  that fills early and stays is not a leak), blocks left at exit, or a stop that takes over 20 s (its threads'
  stacks are then saved beside the run's log). `harness load -xo XO -app
  stability/APP -out OUT [-rates 1000,4000] [-sat 64] [-d 30s] [-rounds 3]
  [-drop-env K,...] [-owners N] [-idle 6s]` offers the same mix at each fixed rate (open loop) and
  at saturation (closed loop, 64 workers), to the xo build and to the app's
  Go twin (app.json `twin`) in alternating rounds, each run on a fresh
  start after a 5 s warmup, and writes OUT/load.csv (one row per run:
  req/s, p50 to p99.9, errors, drops, CPU per request, peak RSS) and
  OUT/load.txt (medians, ranges, xo/go, and "better" or "worse" only when
  the ranges do not overlap); exit 1 only when a run had errors (notes:
  `-drop-env NOTES_DATA`, since the twin keeps no journal); `-env
  XO_RTSTATS=1` adds to the xo build's environment, and each xo run keeps
  its /debug/sched text at the end of its measurement in its run
  directory's stats.txt; `-owners N` (default 1, the admin user alone)
  spreads the mix over N users that the app makes from app.json
  `owners.env` set to N, each request using user n's `owners.token`
  (notes: `NOTES_USERS`, `t0p-admin-u{n}`). Each run's mem.txt holds the
  app's resident KiB at the end of its load (`busy`) and `-idle` (default
  6 s) after it (`idle`; decision 0533's age passes give pages back only
  after 30 to 60 s, so `-idle 75s` measures them). Build it with
  `cd stability/harness && go build -o /tmp/harness .`.

An app's directory holds `app.json` (`name`, which picks the soak and load
traffic mix in soak.go, env with `{addr}` and `{data}`, the
readiness path, named tokens, the scenario file, the redacted keys,
the stats path and its token, the Go twin's directory, and `owners`
for -owners) and its
scenario, one JSON step per line: `method`, `path`, `body` (a string is
sent as its text, any other JSON value as its JSON), `token` (a name
from app.json or `{saved}`), `expect`, `save` (`{"name": "field"}` from the
JSON response, used later as `{name}`), or `restart` (`term` or `kill`).
