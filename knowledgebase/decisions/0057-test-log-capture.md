# 0057: Logs in tests are buffered per test and assertable

Status: Accepted (2026-10-08)

## Context
std/log is ambient (decision 0018): no capability, no effect. Under
`xo test` records went to the real standard error, so passing tests
interleaved `INFO ...` lines with the results, and a test could not check
that something was logged without calling `log.configure(os.stdio)` and
parsing `os.stdio.errors()` (gap 5 in
`bench/results/real-app-2026-10-08.md`).

## Decision
- Every test world gets its own log sink (`rt.newTestLogSink`), `Info`
  level, no output configured. Records that would go to standard error are
  buffered and printed under the test's result only when it fails, or
  with `-v` (Go's `t.Log` behavior). `--json` lines carry them as `log`.
- Every record that passes the sink's level is also kept as a
  `log.Record { level, msg, fields: Map[Str, Str] }` and returned by
  `testing.logs()`, oldest first. This holds whether or not the test
  configured an output.
- `log.configure` inside a test (or inside a `main` run by
  `testing.run_main`) still redirects output to the given `Stdio` (the
  fake one), as before, and changes the level of what is captured.
- `testing.logs()` is a plain function (no effect), like logging itself.
  Outside a test it returns an empty list.

## Consequences
- `xo test` output contains only results unless a test fails.
- Debug records are not captured unless the test lowers the level with
  `log.configure(os.stdio, level: log.Level.Debug)`; this keeps
  `testing.logs()` equal to what a program would emit.
- Field values are captured as text (their `Display` or `debug` form), so
  assertions compare strings: `fields.get("n") == Some("3")`.
- A test that hits the real time limit still reports the logs written so
  far.

## Alternatives
- Default the sink to `os.stdio` of the test: logs would be assertable
  through `os.stdio.errors()` but only as text, and mixed with what the
  code under test prints. Rejected in favor of structured records.
- A method on `TestOs` (`os.logs()`) or on `FakeStdio`: the sink belongs to
  the test's world, not to a capability, and `run_main` gets an `Os`, not
  the `TestOs`. A function in std/testing reads the world directly.
- Capture every level regardless of configuration: makes captured records
  differ from what the program would log. Rejected.
