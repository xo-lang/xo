# A user struct named like a prelude type fails native builds

Status: Observed (2026-10-12, decision 0178 switch point B gaps).

## Symptom

`xo build --backend=arm64 compiler/xo` failed with 138 XO1201 errors
("not available without a garbage collector (`--no-gc`) yet: the
selector `.def_file`", "this field", "assigning to this expression")
at every use of a struct named `Target` that compiler/cli/servers
declared. `xo check` of the same tree passed, and the Go backend builds
it.

## Cause

The prelude declares `struct Target` (core 12.2, the type of the
predeclared `target`). The SSA builders (internal/ssa and compiler/ssa)
name a lowered type by its declared name, and treat the prelude's
`Target` as an opaque std handle (the std_opaque list in named_type,
build.go typ); a user struct of the same name in another module is
looked up as the prelude's, so its fields have no lowering.

## Fix

The struct is `LspTarget`. Until the builders qualify type names by
module, do not name a type of a natively built program after a prelude
type (`Target`, `FileInfo`, `ProcOutput`, `DurationErr`, ...); `xo check`
does not report it. A real fix: key the std_opaque and builtin lookups
by module as well as name (compiler/ssa build.xo named_type and
internal/ssa build.go typ), with a golden that declares `struct Target`
in a user module.
