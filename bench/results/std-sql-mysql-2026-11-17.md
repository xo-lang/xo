# std/sql's MySQL driver against go-sql-driver (2026-11-17)

Decision 0648 item 12's timing. bench/perf/sqlmysql (by hand: needs
MYSQL_DSN): 1,000 rows of five columns (id, name, score, flag, count) read
whole 300 times over the text protocol and summed; Xo's `query_as` against
go-sql-driver/mysql v1.10.1's `Rows.Scan` through database/sql. Checksums
equal (242129700).

darwin/arm64 (the Mac Studio, Docker, not a quiet window), mysql:8.4 in its
own container published on 127.0.0.1 only, removed afterwards by its ID.
Both sides use `tls=skip-verify` (Xo's default is `tls=true`, verified,
decision 0648 item 4, and MySQL's auto generated certificate does not
verify; go-sql-driver's default is `preferred`, which does not verify), so
both pay the same TLS cost. Wall seconds of 7 alternating runs each,
including process start and the connection:

| | runs | median |
|---|---|---|
| Go (go-sql-driver v1.10.1) | 0.880 0.769 0.882 0.863 0.749 0.827 0.870 | 0.863 |
| Xo | 0.943 0.879 0.955 0.917 0.918 0.798 0.942 | 0.918 |

Xo / Go = 1.06 (the ranges overlap). xo built at origin/main 59824eb7c.
