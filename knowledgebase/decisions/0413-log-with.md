# 0413: log.with: a Logger with fields (log/slog Logger.With)

Status: Accepted (lead, 2026-10-29: 0.35 std survey rank 14; Xo Lang 2)

## Context

std/log has module functions only (`log.info(msg, fields)`). A handler
that wants a request id on every record passes the same map to every
call, or builds a map per call. Go's log/slog answers this with
`Logger.With(args...)`, a logger value that carries attributes onto every
record; zap (`With`) and zerolog (`With().Logger()`) do the same. The
0.35 std survey ranked it 14th.

## Decision

- `log.with(fields: Map[Str, Debug]) -> log.Logger` returns a logger
  value; `l.with(more)` derives one with the fields of both;
  `l.debug/info/warn/error(msg, fields = {})` write a record with the
  logger's fields and the record's, a record's field replacing the
  logger's of the same name (slog keeps both; one value per name matches
  how std/log's `Map` fields already work and what a JSON object can
  hold).
- A `Logger` is a value, not a handle: copies are independent, deriving
  never changes the parent, and sharing one between tasks needs nothing.
  It has no sink, level or format of its own: records go where the module
  functions' go, so `configure` and the per-test sinks (decision 0057)
  apply unchanged. A logger with its own handler (slog's `New(h)`) is
  left for a later decision.
- Attribution (decision 0173): the compilers' list of std/log calls that
  take the calling module (compiler/ssa build.xo and compiler/gen
  budget.xo `log_from`) gains `with`, so a logger made in a dependency's
  code records that dependency, and its records name it wherever its
  methods are called. A logger the main module makes and hands to a
  dependency logs as the main module, as a closure does today.
- The methods perform `log`, like the module functions.

## Consequences

- Spec std 6; std/changes/log.md; the golden log_with (arm64, LLVM, Go
  backend).
- compiler/ change: the next fixed point covers it; it calls no new
  intrinsic, so the current seed builds it.

## Alternatives

- Context-carried fields (a task-local field set that every log call
  reads): rejected for now; implicit, and core 11.6's request id is the
  planned answer for the one field everybody wants.
- slog's `With(args ...any)` key/value list: Xo has map literals, which
  the module functions already take.
