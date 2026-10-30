# Backend gap survey, 2026-10-30

Five services written from spec/core.md and spec/std.md alone (std's
sources unread) and type checked with `xo check` to 0 errors; each
missing piece is marked where the code needed it with `// GAP:` (and
`// LANG:` for language obstacles, `// SPEC:` for spec errors). The
ranking is in knowledgebase/infrastructure/std-coverage.md, "Survey of
backend gaps (2026-10-30)".

- chathub/: a WebSocket chat and notification hub with presence.
- queueworker/: a reliable queue worker over the Redis wire protocol.
- grpcsvc/: a gRPC service and client over HTTP/2 with protobuf.
- ledger/: a double entry money ledger on Postgres.
- dataport/: a CSV, TSV and XLSX import and export batch job.

They are not run; `xo check DIR` is the test.
