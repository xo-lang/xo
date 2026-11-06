# stability

The stability program's corpus (decision 0520): real Xo services, each run on
the offload hosts through a cross backend differential, load, a soak, chaos and
fuzzing. Phase 1 builds the starter app and the harness.

- [notes/](notes/): the notes-service example with its notes on disk
  (journal.xo: a snapshot and an append log, replayed at start), the starter
  app. `NOTES_DATA=DIR` keeps notes in DIR; `NOTES_FSYNC=1` fsyncs each change
  before the response. Tests: `xo test stability/notes` (also with
  `--backend=go` and `--backend=llvm`).
- [harness/](harness/): the checks, in Go (its own module, so it shares no
  runtime with the apps). `harness diff -xo XO -app stability/APP` builds the
  app on each backend (`-backends go,arm64,llvm`, the first the reference),
  runs DIR/app.json's scenario against each build (restarts with SIGTERM or
  SIGKILL included, on the same data directory) and compares status,
  Content-Type and body, with app.json's `redact` keys masked; exit 1 when
  they differ or a step misses its `expect` status. `harness soak -xo XO -app
  stability/APP -out OUT [-backend arm64] [-rate 500] [-d 2h]` runs bench/soak's
  open loop mix (per app, in soak.go) against a build under XO_LEAKCHECK=1 and
  XO_CRASH_FILE=OUT/crash.txt (a report there is an alert),
  samples RSS, fds, latency and the app's `heap_live` and `timers_pending` (app.json `stats`,
  std/debug's /sched) into OUT/samples.csv, and writes OUT/drift.txt; exit 1
  on drift, blocks left at exit, or a stop that takes over 20 s (its threads'
  stacks are then saved beside the run's log). `harness load -xo XO -app
  stability/APP -out OUT [-rates 1000,4000] [-sat 64] [-d 30s] [-rounds 3]
  [-drop-env K,...]` offers the same mix at each fixed rate (open loop) and
  at saturation (closed loop, 64 workers), to the xo build and to the app's
  Go twin (app.json `twin`) in alternating rounds, each run on a fresh
  start after a 5 s warmup, and writes OUT/load.csv (one row per run:
  req/s, p50 to p99.9, errors, drops, CPU per request, peak RSS) and
  OUT/load.txt (medians, ranges, xo/go, and "better" or "worse" only when
  the ranges do not overlap); exit 1 only when a run had errors (notes:
  `-drop-env NOTES_DATA`, since the twin keeps no journal). Build it with
  `cd stability/harness && go build -o /tmp/harness .`.

An app's directory holds `app.json` (env with `{addr}` and `{data}`, the
readiness path, named tokens, the scenario file, the redacted keys,
the stats path and its token, the Go twin's directory) and its
scenario, one JSON step per line: `method`, `path`, `body`, `token` (a name
from app.json or `{saved}`), `expect`, `save` (`{"name": "field"}` from the
JSON response, used later as `{name}`), or `restart` (`term` or `kill`).
