# 0734: Workstream stores its data in SQLite, outside Docker

Status: Accepted (owner, xw decision #246, 2026-11-21; written by xw-1,
task #598). Part of
[0732](0732-one-scheduler.md), and step 1 of its migration. Scope: xw
decision #244, the owner's ruling of 2026-11-21: "Workstream uses SQLite,
not Postgres; the owner must be able to stop Docker at any time."

## Context

xw (ADR 0658) keeps its data in Postgres, in the Docker container
xw-postgres, with its data directory at ~/xo-lead/xw/pgdata (426 MB on
2026-11-21). When Docker stops, xw stops with it: every hook, alert,
queue and push record. That happened on 2026-11-20 at 23:16.

The schema has 35 migrations (the workstream repo's migrations/). It
uses these Postgres features, counted in the repo's .xo files on
2026-11-21:

- `->>` JSON field reads: 86 uses;
- `DISTINCT ON`: 18;
- `NOTIFY`: 10, in config.xo and notify.xo;
- a `jsonb` column: 1;
- full text search: `to_tsvector`, `websearch_to_tsquery` and `ts_rank`
  over the docs table.

## Decision

### Storage

- One SQLite file, `~/xo-lead/xw/xw.db`, on the Mac's local disk (never
  /tmp, which a reboot clears).
- WAL mode, `synchronous=NORMAL`, `busy_timeout` 5 s and `foreign_keys`
  on.
- **One writer.** `xw serve` (under launchd, 0735) owns every write.
  Clients (`xw ...`, the hooks, the runner) send writes to it over its
  unix socket. A client may open the file read only for queries while
  `xw serve` is down, so `xw status`, `xw jobs` and `xw alerts` still
  answer, marked stale.
- SQLite comes from Xo's std or a vendored amalgamation built with xo's
  C toolchain, never from Docker.

### Mapping the Postgres features

- **`->>` JSON reads** become `json_extract(fields, '$.k')` (or SQLite's
  `->>`, which has the same meaning in 3.38 and later). The `jsonb`
  column becomes TEXT holding JSON.
- **`DISTINCT ON (k) ... ORDER BY k, t DESC`** becomes `ROW_NUMBER()
  OVER (PARTITION BY k ORDER BY t DESC) = 1`. Each of the 18 queries is
  rewritten and keeps its test.
- **`NOTIFY`**. With one writer, notification moves into the process:
  `xw serve` wakes its own waiters and pushes to its socket
  subscribers. Nothing else used LISTEN across processes.
- **Full text search.** An FTS5 virtual table over docs, ranked with
  `bm25()`. websearch syntax becomes FTS5 query syntax, through a small
  translator for quoted phrases, `-word` and OR.
- **Timestamps** are stored as ISO 8601 UTC text with 6 fraction digits,
  compared as text.

### Backup and restore

- `xw backup` runs `VACUUM INTO` to
  `~/xo-lead/xw/backup/xw-YYYYMMDD-HHMM.db`. It runs online, hourly from
  the xw schedule, keeps the newest 24, and keeps one a day for 7 days
  (at most 31 files, about 5.4 GB at 173 MB each; lead 2026-11-21,
  workstream 9ea5b98; it was 48 and 30 days, up to 78 files).
- `xw restore FILE` stops `xw serve`, moves the current file aside to
  `xw.db.before-restore`, copies FILE in, runs `PRAGMA
  integrity_check`, and starts `xw serve`.

### Tests

The workstream suites run against SQLite. pg_test.sh becomes db_test.sh
and keeps every case. A new case, **Docker stopped**, runs `xw serve`,
the hooks, `xw event`, `xw alerts` and `xw task` with no Docker daemon
(the test runs with DOCKER_HOST pointing at a dead socket and no
`docker` on PATH) and requires each one to succeed.

### Migration: data move, cutover, rollback (#245: in place, no parallel run)

1. Build `xw migrate-to-sqlite`. It reads every table from Postgres in
   one repeatable-read snapshot and writes each one into a new xw.db,
   preserving ids. It then checks row counts and a checksum per table
   (each column's text hashed in id order), and fails on any
   difference.
2. **Backup before cutover**: `pg_dump -Fc` to
   `~/xo-lead/xw/backup/pg-final-YYYYMMDD.dump`, plus a copy of pgdata
   taken while Postgres is stopped.
3. **Cutover**, one command run by the lead, in a quiet window of under
   10 minutes:
   - stop xw serve;
   - run the migration;
   - install the SQLite xw;
   - start xw serve.
4. **Smoke check**: `xw status`, `xw event smoke.cutover`, `xw alerts`,
   a task ack and a hook round. Then the Docker stopped case is run for
   real once: Docker Desktop is quit by the owner, or the case is left
   to the next time the owner stops Docker.
5. **One command back**: `install.sh --restore sqlite` reinstalls the
   previous xw binary and starts xw-postgres from the copy of pgdata.
   Anything written after the cutover is lost, so the window for using
   it is the first hour.
6. When the smoke check passes, the Postgres code, pg_test.sh and the
   xw-postgres container are deleted in the same change (#245). The dump
   is kept for 30 days.

### As built (xw-1, 2026-11-21)

Where the port differs from the plan above:

- `xw migrate-to-sqlite [--out FILE]` writes FILE.tmp (the schema by
  migrations/, then the copy) and renames it to FILE only when every
  table checked out; FILE must not exist. All tables are copied in one
  SQLite transaction with `defer_foreign_keys`, since a row may
  reference a table copied after it (the first dry run, one transaction
  per table, failed at decision_agents). The checksum is per row
  (every value as typed text, FNV-1a 64, summed so the order of rows
  does not matter), computed from the values written and again from the
  rows SQLite reads back; Postgres booleans become 0 or 1, times xw's
  UTC text, arrays and JSON JSON text. A Postgres table or column the
  SQLite schema lacks fails the copy before any row moves.
  The dry run on the live store (2026-11-21, xw-1's
  xwsq-unitdry.87373 log) copied 40 tables, 672895 rows, counts and
  checksums equal, into a 190 MB file.
- `xw restore FILE` does not stop `xw serve`: it refuses while the
  server answers, so the operator boots out org.xolang.xw first (xw
  never stops a process it did not start). It checks FILE with
  `PRAGMA integrity_check` before moving xw.db (and its -wal and -shm)
  aside to xw.db.before-restore, then checks the copy.
- `xw backup` writes to XW_BACKUP_DIR (default ~/xo-lead/xw/backup), one
  file a minute at most; the hourly run is the schedule line
  `xwbackup 1h xw/bin/xw backup` in xw-schedule.conf.
- The Docker stopped case is the whole of db_test.sh and work_test.sh:
  both export a DOCKER_HOST pointing at a dead socket and run serve with
  PATH=/usr/bin:/bin (no docker), and db_test fails when serve logs
  anything about Docker.

## Consequences

- Docker can be stopped at any time without taking xw down.
- The single writer is a bottleneck in theory. xw writes a few events a
  second at its peak, and SQLite in WAL mode with one writer handles
  thousands.
- The rewrite touches every DISTINCT ON and the search query. Each one
  keeps its unit test, and the migration's checksum proves the data
  moved.
- The data directory shrinks; 426 MB of pgdata is mostly Postgres
  overhead.

## Alternatives

- **Postgres outside Docker** (Homebrew or Postgres.app). Docker can
  then stop, but xw keeps a server process to install, upgrade and back
  up. Ruled out by #244.
- **Leave Postgres in Docker and run Docker in a separate VM that the
  owner never stops.** That moves the same risk elsewhere.
- **A key-value store** (bbolt, LMDB). It has no SQL, so all 35
  migrations' worth of queries would be rewritten as code.
