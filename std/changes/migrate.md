# std/migrate changes

## 2026-11-15: feature: std/migrate, schema migrations

- Kind: feature
- Commit: this push
- Found by: decision 0658 item 8 (xw, the agent work system, needs schema changes); std-coverage.md listed migrations as planned
- Go reference: pressly/goose v3.27.3 (internal/sqlparser, NumericComponent, the Provider's partial error and out-of-order rule; Copyright (c) 2012 Liam Staskawicz, MIT license)
- Test: std/migrate_test.xo: goose's parser_test.go and parse_test.go with its testdata (9 valid-up goldens, 4 invalid files, NO TRANSACTION), TestCollectFileSources cases, tx_partial_errors and TestPending on sql.memory
- Shipped in: not yet released

New module: goose's `.sql` file format, `up` applying pending migrations in version order (each in a transaction unless NO TRANSACTION) under a Postgres advisory lock or MySQL GET_LOCK held on one connection, a version table, `status`, forward only.
