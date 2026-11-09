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

## The seed trap (2026-11-08, the same day)

The two push plan above (compiler padding first, then the std field) assumed an older xo that already has the padding. The Linux hosts and the amd64 goldens build xo from the 0.44 seed, whose decoder still builds the old two operand struct: on main at ddc6bc54d their builds failed with `the inlined arm64 module does not verify (219 errors; ... json.DecodeErr = struct v1092, v1093 lands v924: struct operands do not match the fields)` under XO_INLINE_STRICT=1, while the Mac passed (it builds from a newer cached xo). The Mac is not the proof for a struct layout change of a type the compiler's own decoders build: build compiler/xo from the seed (on host 2 with XO_SEED set to the seed's xo and XO_INLINE_STRICT=1). The std field was removed again (decision 0553: `go_text` is computed from `msg`); put it back in the first push after the next seed cut. Same class as seed-older-than-intrinsics.md.
