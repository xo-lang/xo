# std/migrate changes

## 2026-11-17: feature: goose's verbose records at Debug (SetVerbose, WithVerbose)

- Kind: feature
- Commit: this push
- Found by: decision 0655 audit (scripts/stdaudit/map.txt gap rows, task #190)
- Go reference: pressly/goose v3.27.3 (provider_run.go runMigrations and runSQL logf; Copyright (c) 2012 Liam Staskawicz, MIT license)
- Test: std/migrate_test.xo: the records of a tx, a NO TRANSACTION and an empty migration and a second run, in goose's order and fields; none at the default Info level; a code migration as type go
- Shipped in: not yet released

The functions that run migrations (`up`, `up_results`, `up_code`, `up_by_one`, `up_code_by_one`, `apply_version`, `up_unversioned` and their `_list` forms) now perform `log` and write goose Provider's verbose records (as with `WithVerbose` and `WithSlog`, `"logger": "goose"`) at `Debug`: "executing statement", "migration completed", then "successfully migrated database" or "no migrations to run". They cost nothing unless the level is `Debug`; `log.configure(out, level: log.Level.Debug)` is goose's `SetVerbose(true)`.

## 2026-11-17: feature: TiDB, Redshift, Aurora DSQL and Turso dialects

- Kind: feature
- Commit: this push
- Found by: decision 0655 audit (scripts/stdaudit/map.txt gap rows, task #190)
- Go reference: pressly/goose v3.27.3 (internal/dialects tidb.go, redshift.go, dsql.go, turso.go; Copyright (c) 2012 Liam Staskawicz, MIT license)
- Test: std/migrate_test.xo: goose's CREATE TABLE for each, Aurora DSQL's id as the highest id plus 1, and Redshift, Dsql and Turso run with no lock statement
- Shipped in: not yet released

`Dialect.Tidb` (mysql driver, GET_LOCK), `Dialect.Redshift` and `Dialect.Dsql` (postgres driver, no advisory lock) and `Dialect.Turso` (SQLite statements, no lock). With `Layout.Goose` each makes goose's table for that database; Aurora DSQL's rows get id = MAX(id) + 1 as goose writes them. Turso over HTTP still has no std/sql driver.

## 2026-11-17: feature: goose's version table (Layout.Goose)

- Kind: feature
- Commit: this push
- Found by: decision 0655 audit (scripts/stdaudit/map.txt gap rows, task #190)
- Go reference: pressly/goose v3.27.3 (internal/dialects; Copyright (c) 2012 Liam Staskawicz, MIT license)
- Test: std/migrate_test.xo: a goose-made table continues (db_version, has_pending, up, status), the table made with its version 0 row, and a version whose newest row is not applied is pending
- Shipped in: not yet released

Every function with a `dialect` takes `layout: Layout`; `Layout.Goose` reads and writes goose's goose_db_version (id, version_id, is_applied, tstamp), made as goose makes it for the dialect, so a database goose migrated is continued.

## 2026-11-17: feature: ENVSUB ON and OFF (goose environment substitution)

- Kind: feature
- Commit: this push
- Found by: decision 0655 audit (scripts/stdaudit/map.txt gap rows, task #190)
- Go reference: pressly/goose v3.27.3 (internal/sqlparser/parser.go; Copyright (c) 2012 Liam Staskawicz, MIT license), mfridman/interpolate v0.0.2 (Copyright (c) 2014 Buildkite, MIT license)
- Test: std/migrate_test.xo: goose's TestEnvsub testdata (test01, test02, test03) and TestEnvsubError, collect passing env, and interpolate's grammar (defaults, substrings, `?` errors, escapes, parse errors)
- Shipped in: not yet released

`parse` and `collect` take `env: Map[Str, Str]`; lines after `-- +goose ENVSUB ON` are substituted from it instead of being refused. `Env.vars()` gives goose's behavior.

## 2026-11-17: feature: code migrations (goose Go migrations, WithGoMigrations)

- Kind: feature
- Commit: this push
- Found by: decision 0655 audit (scripts/stdaudit/map.txt gap rows, task #190)
- Go reference: pressly/goose v3.27.3 (migration.go GoFunc, NewGoMigration; provider_options.go WithGoMigrations; provider_run.go runGo; Copyright (c) 2012 Liam Staskawicz, MIT license)
- Test: std/migrate_test.xo: provider_run_test's TestGoOnly with a transaction and without one, SQL and code migrations in one order with status, a version both SQL and code refused, a failed step rolled back
- Shipped in: not yet released

New `CodeStep` (`Tx` or `NoTx`) and `CodeMigration` make an Xo function a migration step; new `up_code` and `up_code_by_one` apply SQL and code migrations together in version order. goose's global registry (AddMigration and its variants) is not planned: the code migrations are passed as a value.

## 2026-11-17: feature: exclusions, isolated DDL, no versioning (goose WithExcludeNames, WithExcludeVersions, WithIsolateDDL, WithDisableVersioning)

- Kind: feature
- Commit: this push
- Found by: decision 0655 audit (scripts/stdaudit/map.txt gap rows, task #190)
- Go reference: pressly/goose v3.27.3 (provider_options.go WithExcludeNames, WithExcludeVersions, WithIsolateDDL, WithDisableVersioning; provider_collect.go; Copyright (c) 2012 Liam Staskawicz, MIT license)
- Test: std/migrate_test.xo: provider_collect_test's excludes case by name and by version, an excluded duplicate, version 0 refused; the isolate DDL run; the no versioning seed run applied twice with no version table
- Shipped in: not yet released

`collect` takes `exclude_names` and `exclude_versions`; new `isolate_ddl` runs every migration outside a transaction; new `up_unversioned` applies every migration each call without a version table or lock, for seed data.

## 2026-11-17: feature: create and fix (goose create, goose fix)

- Kind: feature
- Commit: this push
- Found by: decision 0655 audit (scripts/stdaudit/map.txt gap rows, task #190)
- Go reference: pressly/goose v3.27.3 (create.go Create, CreateWithTemplate and SetSequential; fix.go Fix; helpers.go camelCase, snakeCase; Copyright (c) 2012 Liam Staskawicz, MIT license)
- Test: std/migrate_test.xo: TestCamelSnake's cases, TestSequential (four sequential creates), TestFix (four timestamped files, then two more), the template's fields, a second create in the same second, the timestamp check
- Shipped in: not yet released

New `create` writes a migration file named by the clock's UTC time or, with `sequential`, the next sequential version, from `SQL_TEMPLATE` or a given template; new `fix` renames timestamped files to sequential versions. MigrateErr gains `File(err: FsErr)`.

## 2026-11-16: behavior: no migrations is an error; up_results

- Kind: behavior
- Commit: this push
- Found by: decision 0655 audit (task #190); gotchas/migrate-embed-sub.md (xw's first migrate applied 0 from a nested embed)
- Go reference: pressly/goose v3.27.3 (ErrNoMigrations, ErrNoMigrationFiles, MigrationResult and its String, truncateDuration; Copyright (c) 2012 Liam Staskawicz, MIT license)
- Test: std/migrate_test.xo: TestProvider's empty case and its nested map without fs.Sub, assertResult's fields over up_results, MigrationResult.String's examples
- Shipped in: not yet released

`up`, `status`, `has_pending` and their `_list` forms answer `MigrateErr.NoMigrations` when given no migration (an empty directory, or an `embed("dir")` passed without `.sub("dir")`), where they answered `[]`. New `up_results` answers a `MigrationResult` per migration applied, displayed as goose's log line.

## 2026-11-16: feature: up to a version, up by one, apply one version, current version, pending check

- Kind: feature
- Commit: this push
- Found by: decision 0655 audit (scripts/stdaudit/map.txt gap rows, task #190)
- Go reference: pressly/goose v3.27.3 (internal/gooseutil UpVersions, Provider.UpTo, UpByOne, ApplyVersion, GetDBVersion, HasPending, ErrNoNextVersion, ErrVersionNotFound, ErrAlreadyApplied; Copyright (c) 2012 Liam Staskawicz, MIT license)
- Test: std/migrate_test.xo: resolve_test.go's UpVersions cases with and without allow_missing, up to a target, a target of 0, up_by_one to NoNext, apply_version (unknown, each, again), has_pending with and without allow_missing
- Shipped in: not yet released

`up` and `up_list` take `to`; new `up_by_one`, `apply_version`, `db_version`, `has_pending`, `up_versions`, and MigrateErr's `NoNext`, `NotFound`, `AlreadyApplied` and `BadVersion`. Breaking: `MigrateErr.Missing` carries `newest` and `to`, and its text is goose's ("detected N missing (out-of-order) migration...").

## 2026-11-15: feature: std/migrate, schema migrations

- Kind: feature
- Commit: this push
- Found by: decision 0658 item 8 (xw, the agent work system, needs schema changes); std-coverage.md listed migrations as planned
- Go reference: pressly/goose v3.27.3 (internal/sqlparser, NumericComponent, the Provider's partial error and out-of-order rule; Copyright (c) 2012 Liam Staskawicz, MIT license)
- Test: std/migrate_test.xo: goose's parser_test.go and parse_test.go with its testdata (9 valid-up goldens, 4 invalid files, NO TRANSACTION), TestCollectFileSources cases, tx_partial_errors and TestPending on sql.memory
- Shipped in: not yet released

New module: goose's `.sql` file format, `up` applying pending migrations in version order (each in a transaction unless NO TRANSACTION) under a Postgres advisory lock or MySQL GET_LOCK held on one connection, a version table, `status`, forward only.
