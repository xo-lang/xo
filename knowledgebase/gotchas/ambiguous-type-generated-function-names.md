# Two packages with a type of the same name shared generated SSA functions

Status: Observed (2026-10-12, decision 0178 stage 4), fixed the same day
in internal/ssa and selfhost/ssa; regression test
internal/ssa TestAmbiguousGeneratedNames.

## Symptom

A native build (`--backend=arm64` or `llvm`) of a program whose packages
each declare a type of the same name failed in the SSA verifier:

    xo build: ssa: xo.debug[List[Ctx]]: b6: v23: Str = call xo.debug[Ctx], v21, v22:
    argument 1 has type github.com/xo-lang/xo/selfhost/check.Ctx,
    want github.com/xo-lang/xo/selfhost/cli/modules.Ctx

It appeared when selfhost/cli/modules (with a `Ctx` struct holding a
`check.Checker`) was linked with selfhost/check (whose `Checker` holds a
`List[Ctx]` of its own `Ctx`). Two packages `a` and `b` that each declare
`struct Ctx` and interpolate `xs.debug()` of a `List[Ctx]` reproduce it.
The Go backend was not affected (Go generates per Go type).

## Cause

The SSA builder interns lowered types under `builder.typeName`, which
qualifies a name another module also declares (decision 0158's
`Diag` finding), but the functions it generates per checker type were
named with the plain printed type: `xo.debug[...]`, `xo.gen[...]`,
`xo.json.enc[...]`, `xo.json.dec[...]`, `xo.sql.dec[...]`. The second
package's request found the first package's function by name and called
it with the wrong type.

## Fix

Name the generated functions with `b.typeName(ct)` (Go) and
`b.type_name(ct)` (selfhost/ssa), the same qualified text the types use.
Without an ambiguous name nothing changes (typeName is the printed
form), so goldens and dumps are unchanged.

Avoid: any new per type generated function must use typeName too.
