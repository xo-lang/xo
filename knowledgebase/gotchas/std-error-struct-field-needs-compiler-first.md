# A new field on json.DecodeErr must land after the compiler that fills it

Status: Observed (2026-11-08, decision 0553).

## Symptom

Adding `go: Str = ""` to `json.DecodeErr` (std/json.xo) made every older xo fail to build compiler/xo:
`xo: the inlined arm64 module does not verify (219 errors; the first: repl.run: ...: json.DecodeErr = struct v1092, v1093 lands v924: struct operands do not match the fields)`.
Writing `go: ""` at every literal in std did not help.

## Cause

compiler/ssa/json.xo `dec_err` builds the decoder's error struct itself, with exactly two operands (path, msg). An xo built before the change
compiles the new std with that two operand struct against a three field type.

## Fix

Two pushes. First compiler/ssa/json.xo `dec_err` pads every field past path and msg with an empty Str (it works with the two field struct, so any
xo can build it). Then, once an xo built from that push exists (main's cached build), the std field. Test the second step with
`XO_SEED=<xo built from the first> go run ./scripts/xobuild`. The Go backend (compiler/gen/json.xo) names its fields and needs no change.
