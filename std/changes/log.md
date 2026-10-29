# std/log changes

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
