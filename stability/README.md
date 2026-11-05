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
  they differ or a step misses its `expect` status. Build it with
  `cd stability/harness && go build -o /tmp/harness .`.

An app's directory holds `app.json` (env with `{addr}` and `{data}`, the
readiness path, named tokens, the scenario file, the redacted keys) and its
scenario, one JSON step per line: `method`, `path`, `body`, `token` (a name
from app.json or `{saved}`), `expect`, `save` (`{"name": "field"}` from the
JSON response, used later as `{name}`), or `restart` (`term` or `kill`).
