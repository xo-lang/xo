# A bare dbg(x) statement was XO0209

Status: Observed, fixed (2026-10-18).

## Symptom

`dbg(p)` on a line of its own, the way dbg is used most, failed the
build: "error XO0209: the `Point` that `dbg(...)` returns is discarded,
and the call does nothing else".

## Cause

XO0209 (decision 0258) reports an expression statement whose call has no
effects once they are solved. dbg prints without a capability (core
11.4, decision 0265), so its call carries no effect and looked like a
call that only computes its value.

## Fix

compiler/check stmt.xo expr_stmt skips `dbg(...)` as it skips
`.ignore_err(...)`; the XO0209 unit test (check_test.xo) has a bare dbg
in a function without Stdio. Any later output that bypasses the effect
system needs the same exemption.
