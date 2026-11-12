# std/log changes

## 2026-11-12: feature: log handlers, sinks and typed values

- Kind: feature
- Commit: this push
- Found by: the owner's std features axis, high group (decision 0617 C)
- Go reference: log/slog Handler, TextHandler, JSONHandler, Value
- Test: golden log_handlers (arm64, LLVM, Go backend parity, wasm); cmd/xo TestAuditInstalls
- Shipped in: not yet released

`configure_handler(h)` sends every record to a `Handler[fx]`; `text_handler` and `json_handler` write `configure`'s lines to a `Sink[fx]` (`stderr_sink`, `stdout_sink`, `file_sink`, `conn_sink`, `func_sink`); `multi` and `level_range` combine them. A handled `Record` gains `time` and `values` (`log.Value`, log/slog's `Value`). A sink's write error drops the line and counts it (`dropped()`). The installing call performs the handler's effects (spec core 6.3); `xo audit` lists it. Times and durations arrive as `Value.Any` for now.

## 2026-11-11: docs: the header says what the module is for

- Kind: docs
- Commit: this commit
- Found by: pkg.xolang.org and xolang.dev showed `xo doc std`'s summary of std/log as a note about the file (the Xo Lead, 2026-11-11)
- Go reference: none
- Test: cmd/xo TestDocStdSummaries
- Shipped in: not yet released

The header comment of std/log.xo opens with a sentence that says what the module is for, which `xo doc std` lists as its summary. No declaration changed.

## 2026-11-07: feature: log calls below the level do not evaluate their arguments; `log.enabled`

- Kind: feature
- Commit: this push
- Found by: Xo Lang 1's xo-vs-go survey, row 7 (log/filtered 1.79x Go)
- Go reference: log/slog Logger.Enabled (Go builds a call's arguments before the level check; Xo now skips them)
- Test: golden log_lazy (arm64, LLVM, Go backend parity, wasm)
- Timing: pending, xo bench --ab on bench/perf/log filtered
- Shipped in: not yet released

`debug`, `info`, `warn` and `error` (and a `Logger`'s) are compiled as `if log.enabled(level) { call }` on every backend (decision 0558), so a record below the configured level builds no fields map, interpolation or `Debug` boxes. `log.enabled(level)` is new for work that is not an argument. An argument with a side effect does not run when the record is dropped.

bench/perf/log `filtered` (100,000 records below the level, two fields each) on linux/arm64 host 1, `--ab` against the commit before, 6 rounds: wall 13.5 ms to 0.54 ms (B/A 0.04, 0.039 to 0.047), max RSS unchanged (1260 kB).

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
