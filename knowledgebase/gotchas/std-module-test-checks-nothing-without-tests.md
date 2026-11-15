# `xo test --std-module=M` checks nothing when M has no tests

Status: Observed (2026-11-15)

## Symptom

A compiler change (decision 0662, a closure capturing shared state
performs `sync`) was measured by running `xo test --std-module=M` over
every std module and counting errors: all 94 modules reported 0. The
gate then failed in TestStdGenCurrent: `go run ./scripts/stdgen` stopped
on errors in std/metrics.xo, and TestStdChecks found four more in
std/jsonrpc.xo.

## Cause

`xo test --std-module=M` for a module with no `_test.xo` prints "the std
module M has no tests" and exits without type checking M. metrics,
jsonrpc and testing are such modules. A per-module scan through
`xo test` is therefore blind to every untested module, and its "0
errors" counts only the tested ones.

## Fix

Check all of std with the tools that check every module:

- `go test ./cmd/xo -run '^TestStdChecks$' -count=1` (every std module,
  with every error listed);
- `go run ./scripts/stdgen -check` (also catches stale generated Go after
  a std signature change; regenerate with `go run ./scripts/stdgen`).

Both under heavy.sh. Run them before gating a change that alters what
the checker accepts.
