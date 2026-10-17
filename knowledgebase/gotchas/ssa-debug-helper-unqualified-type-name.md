# The SSA builder names Debug helpers by the unqualified type name

Status: Observed (2026-10-12).

**Symptom.** `xo build --backend=arm64 compiler/xo` failed in the SSA
builder after compiler/xo gained `build_wasm(os, m: ssa.Module, ...) ->
() ! FsErr` (decision 0179):

    ssa: parser.kind_text: b120: v362: Str = call xo.debug[Kind], v361, v0:
    argument 1 has type github.com/xo-lang/xo/compiler/lexer.Kind, want
    github.com/xo-lang/xo/compiler/ssa.Kind

**Cause.** A function with an error result records its arguments for
the error trace (decision 0104, `xo.frameargs`), which needs the Debug
text of every argument: here of `ssa.Module`, so of `ssa.Kind`. The
generated helper is named `xo.debug[<type>]` with the type's short name
(`Kind`), so `xo.debug[Kind]` of compiler/ssa and of compiler/lexer
are the same function to the builder; the first one built wins and the
other's call does not verify. Any program with two types of the same
short name in different modules whose Debug text is needed hits it.

**Fix.** Workaround in compiler/xo/wasm.xo: `build_wasm` has no error
result (it reports through `fail`). The real fix is to name generated
helpers by the qualified type (internal/ssa build_show.go `debugFunc`,
and its mirror in compiler/ssa), which changes the SSA dump and so both
builders at once.
