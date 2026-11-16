# Postgres CREATE TABLE IF NOT EXISTS races with itself

Observed 2026-11-16 (std/ui sql_store, decision 0686).

Symptom: several processes starting at once against one Postgres, each
running `CREATE TABLE IF NOT EXISTS t (...)`: all but one may fail with
`duplicate key value violates unique constraint
"pg_type_typname_nsp_index"` (SQLSTATE 23505, std/sql's
`SqlErr.Constraint`), although the statement says IF NOT EXISTS.

Cause: Postgres checks for the table before it inserts the catalog rows,
without a lock between the two; two sessions both pass the check and the
second hits the unique index on pg_type.

Fix: treat `Constraint` from a CREATE ... IF NOT EXISTS as "another
session created it" (std/ui `sql_create`), or serialize the creation
(an advisory lock, or migrations run once before the instances start).
Test: std/ui "sql_store opens when another instance created its tables
at the same moment" (a driver that answers CREATE as Postgres did).
