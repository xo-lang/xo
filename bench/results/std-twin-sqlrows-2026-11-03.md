# std/sql row cursor: memory (2026-11-03)

Mac (darwin/arm64), Postgres 17 in Docker, `SELECT g, repeat('x',40), repeat('y',40) FROM generate_series(1, N)`, max RSS of one run (`/usr/bin/time -l`), Xo at the row cursor commit (decision 0479).

| rows | `query` MB | `rows` MB |
|---|---|---|
| 100,000 | 34.2 | 3.8 |
| 1,000,000 | 320 | 3.9 |

`rows` is flat in the row count. The Go (pgx) twin and wall time on host 1 are not measured yet.
