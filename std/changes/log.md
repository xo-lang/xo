# std/log changes

## 2026-11-04: perf: a record's JSON keys are ordered by insertion, not sort_by

- Kind: perf
- Commit: this commit
- Found by: the log twin row (a closure and a second list per record)
- Go reference: log/slog's JSONHandler writes attributes in order; Xo sorts the keys of a record for stable output (unchanged)
- Test: internal/llvm/testdata/golden/log_with and the log goldens (same output)
- Shipped in: not yet released

The keys of a record (a handful) are put in order by insertion into a list of indexes, with no closure and no second list for `sort_by`. The output is the same.


## 2026-10-29: feature: a Logger that carries fields (log.with)

- Kind: feature
- Commit: this commit
- Found by: the 0.35 std survey (rank 14): a request id on every record
  of a handler meant passing the same map to each call
- Go reference: log/slog Logger.With (go1.26.1); decision 0413
- Test: internal/llvm/testdata/golden/log_with (arm64, LLVM, and the Go
  backend: fields on every record, a derived logger, a record's field
  overriding the logger's, the level filter)

`log.with(fields)` returns a `log.Logger` whose `debug`, `info`, `warn`
and `error` add its fields to every record; `l.with(more)` derives one
with more fields. A record's own field replaces the logger's of the same
name. A logger made in a dependency's code keeps that dependency as its
records' module (decision 0173).
