# std/sqlite changes

## 2026-11-06: feature: std/sqlite, a SQLite driver for sql.open_driver

- Kind: feature
- Commit: this commit
- Found by: owner-approved roadmap item (Xo Lead, 2026-11-06), decision 0534
- Go reference: none in Go's standard library; github.com/mattn/go-sqlite3 (MIT) for the DSN, the type
  mapping and the differential corpus
- Test: std/native/sqlite_test.xo (DSN keys, an unknown key, a memory database's binds, blob round trip,
  constraint and missing table errors, transactions)
- Timing: pending (bench/perf/sqlite against mattn and modernc on linux/arm64)

New module over the vendored SQLite 3.54.1 amalgamation (std/native/sqlite, public domain, SHA3-256 of the
release zip in its VERSION) through `use c`. Native backends only.
