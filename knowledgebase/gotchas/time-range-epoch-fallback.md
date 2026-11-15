# Xo's Time cannot hold year 1, and `?? Time.UNIX_EPOCH` hides it

Status: Observed (2026-11-15, std/sql's MySQL driver, decision 0648)

## Symptom

scripts/mysql-diff.sh, with `parseTime=true`, read MySQL's zero date
(`0000-00-00 00:00:00`) from the Xo driver as `At(1970-01-01T00:00:00Z)`.
go-sql-driver gives Go's zero time (`0001-01-01T00:00:00Z`). The driver's
unit tests passed: nothing failed, the value was just wrong.

## Cause

`Time` is int64 nanoseconds since 1970, so it covers about 1678 to 2262
(spec/std.md 2, decision 0364). Go's zero time is year 1, which is
outside that range, so `Time.parse_rfc3339("0001-01-01T00:00:00Z")` is
`None`. The first draft wrote
`Time.parse_rfc3339("0001-01-01T00:00:00Z") ?? Time.UNIX_EPOCH`, and the
fallback turned the failed parse into a plausible time with no error.
MySQL's DATETIME allows years 1000 to 9999, so `1000-01-01` has the same
problem.

## Fix

The driver keeps a zero date, or a year outside `Time`'s range, as the
server's `Text`, as the Postgres driver keeps `infinity`
(std/native/llrt/mysql.xo time_value; spec/std.md 10.11; mysql_test.xo
covers it). Use `Civil` (decision 0614) when a format really carries wide
years. Do not use `?? Time.UNIX_EPOCH` (or any constant time) as the
fallback of a parse whose input comes from outside: return the error,
fault, or keep the text.

## Other places with the pattern (for Lang 3's review)

Found by `grep -rnE '\?\? *Time\.UNIX_EPOCH' std scripts` and a wider
grep for `??` after `parse_rfc3339`, `to_time`, `instant` and
`Time.parse*` on 2026-11-15 (xo main 46f76126c plus this branch):

- scripts/pgdiff/xo/main.xo:34 and scripts/mysqldiff/xo/main.xo:36 (fixed
  in this commit: they fault now): the `D:` argument of a case line. The
  input is the repository's own cases.txt, but a typo there sent 1970.
- scripts/stdcorpus/jsonencode.go:313-315: corpus rows on constant,
  in-range literals. Low risk.
- std/native/llrt/pg_test.xo:39-40, mysql_test.xo:51-52 and 72-74: the
  expected values of tests on constant, in-range literals. A failed
  parse would still show as a mismatch unless both sides fell back.
- No fallback of this kind in std's library code. Reviewed and loud:
  std/cron.xo:377 (faults), std/mail.xo:189 (range checked, then
  `Err`), std/evidence/time/stress (returns `Err`). std/jwt.xo:451
  date_time clamps to about 2255 on purpose, only for an error's `at`.

A lint for `?? Time.UNIX_EPOCH` after a parse, or `Time.MIN` and
`Time.MAX` constants so a fallback is visibly a sentinel, would have
caught this at once (docs/reports/xo-feedback.md in the aws repository,
2026-11-15).
