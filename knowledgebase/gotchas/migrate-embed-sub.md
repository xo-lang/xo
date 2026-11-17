# std/migrate applies nothing when given the embed itself

Status: Observed (2026-11-15)

## Symptom

`migrate.up(db, MIGRATIONS, clock)` with
`const MIGRATIONS: embed.Dir = embed("migrations")` answered `[]` (no
error) on an empty database: xw's first `xw migrate` printed
"applied 0".

## Cause

An `embed("migrations")` directory keys its files by their path from the
module, `migrations/00001_init.sql`. `collect` takes only top level
`.sql` files with a version (goose's rule), so it found none, and no
pending migration is not an error.

## Fix

Pass the directory's contents: `migrate.up(db, MIGRATIONS.sub("migrations"),
clock)` (spec/std.md 85). A test that migrates an empty database and
checks the versions applied (xw's test/pg_test.sh expects
"applied N: 1 ... N") catches it.

## Since 2026-11-16

`up`, `status` and `has_pending` answer `MigrateErr.NoMigrations`
("migrate: no migrations found", goose's ErrNoMigrations) when `collect`
finds no migration, so the mistake fails loudly instead of answering `[]`.
