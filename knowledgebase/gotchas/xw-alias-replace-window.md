# A table replaced by DELETE then INSERT is empty for concurrent readers

Status: Observed (2026-11-17, xw task #212)

## Symptom

xw's pg_test check "status: an alias has its own block" failed now and
then: `xw status` showed no block for an alias that the alias file
listed.

## Cause

`ingest_aliases` ran every second as `DELETE FROM aliases`, then one
INSERT per line, with no transaction around them. A status read that ran
between the DELETE and the INSERTs saw an empty aliases table.

## Fix

Replace the rows in place: upsert each name (`ON CONFLICT (name) DO
UPDATE ... WHERE aliases.agent <> EXCLUDED.agent`), then
`DELETE FROM aliases WHERE name <> ALL($1::text[])` for the names that
left the file. A kept row is never deleted. A transaction also works,
but the upsert writes nothing when the file has not changed.
workstream 443f750. pg_test has an AFTER DELETE trigger that logs each
deleted alias and fails if a kept alias was deleted; it failed before
the fix.

Any periodic "rebuild the table from a file" job in xw that readers query
at the same time needs the same treatment.
