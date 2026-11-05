# stability

The stability program's corpus (decision 0520): real Xo services, each run on
the offload hosts through a cross backend differential, load, a soak, chaos and
fuzzing. Phase 1 builds the starter app and the harness.

- [notes/](notes/): the notes-service example with its notes on disk
  (journal.xo: a snapshot and an append log, replayed at start), the starter
  app. `NOTES_DATA=DIR` keeps notes in DIR; `NOTES_FSYNC=1` fsyncs each change
  before the response. Tests: `xo test stability/notes` (also with
  `--backend=go` and `--backend=llvm`).
