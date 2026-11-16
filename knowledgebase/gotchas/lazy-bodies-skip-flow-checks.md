# A lost update in a std body builds natively and fails only with --backend=go

Status: Observed (2026-11-15, Xo Lang 3's std/brotli.xo; dependency part fixed the same day, task #75)

## Symptom

`var j = bounds[i]; j += 1` (an element copied out of a List, changed,
never stored) in std/brotli.xo built and ran with the native backends
and `xo check` of a program; `xo build --backend=go` reported XO0520.
The same held for a dependency module of a program (decision 0517)
until task #75: its reached body built natively with a lost update.

## Cause

A native build checks runtime bodies lazily (decision 0247) and
dependency bodies lazily (decision 0517): check_lazy
(compiler/check/stmt.xo) checks a body when the SSA builder reaches it,
after the flow passes (moves, lost and stored updates XO0520, split
critical sections XO0804, constant time sections, undone writes) ran
over the eagerly checked bodies. A lazy body skipped them. The go
backend checks every body eagerly, so it reports them.

## Fix

- Dependencies: check_lazy runs the flow passes (flow_body) on a
  dependency's body it checks (cmd/xo TestDepLazyFlow), as 0517 says a
  reached body's diagnostics stop the build.
- std: still skipped in builds (0247: about 11% of the playground's
  build was the flow passes over runtime bodies). While editing a std
  module, run `xo test --std-module=M` (the module under test is checked
  in full, flow passes included) or `xo check --no-gc`; TestStdChecks
  (the stdchecks gate) reports it for every std module.
