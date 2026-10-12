# The SSA builder names Debug helpers by the unqualified type name

Status: Observed (2026-10-12).

**Symptom.** `xo build --backend=arm64 selfhost/xo` failed in the SSA
builder after selfhost/xo gained `build_wasm(os, m: ssa.Module, ...) ->
() ! FsErr` (decision 0179):

    ssa: parser.kind_text: b120: v362: Str = call xo.debug[Kind], v361, v0:
    argument 1 has type github.com/xo-lang/xo/selfhost/lexer.Kind, want
    github.com/xo-lang/xo/selfhost/ssa.Kind

**Cause.** A function with an error result records its arguments for
the error trace (decision 0104, `xo.frameargs`), which needs the Debug
text of every argument: here of `ssa.Module`, so of `ssa.Kind`. The
generated helper is named `xo.debug[<type>]` with the type's short name
(`Kind`), so `xo.debug[Kind]` of selfhost/ssa and of selfhost/lexer
are the same function to the builder; the first one built wins and the
other's call does not verify. Any program with two types of the same
short name in different modules whose Debug text is needed hits it.

**Fix.** Workaround in selfhost/xo/wasm.xo: `build_wasm` has no error
result (it reports through `fail`). The real fix is to name generated
helpers by the qualified type (internal/ssa build_show.go `debugFunc`,
and its mirror in selfhost/ssa), which changes the SSA dump and so both
builders at once.
