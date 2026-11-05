# Interface instances keyed by a type's short name

Status: Observed 2026-11-05 (aws xo-issues 15)

## Symptom

Two program modules each define a type with the same name (`a.Provider`,
`b.Provider`, enums `K`, the aws SDK's `credentials.Cache` beside an app's
`struct Cache`), and values of both are boxed into interfaces (one interface or
two different ones). The native backend fails with XO1510 ("closure
xo.show[Provider] ... capture 0 has the wrong type"), also with
XO_BACKEND_CACHE=off; LLVM builds and runs wrong: `b.Provider{s: "ab"}` printed
as `Provider{n: 4331386432}`, `get()` returned another type's field, and the
SDK program crashed with SIGSEGV. Renaming one of the types fixed it.

## Cause

compiler/ssa keyed an interface value's per-type pieces by `b.c.show(t)`, the
checker's display text, which names a program type by its bare name:
`impl_text` (the type number `type_id`, `xo.show[T]`, the boxing thunks), the
instance key of a generic function's type arguments, and `xo.frameargs`. Both
`Provider`s got one type number and one show thunk, built for whichever came
first. Lowered type names already used the module path for such names
(`type_name`, `ambiguous_type_names`, commit 532252574's shared names); these
keys did not. Error boxing under `!` was unaffected (its display goes through
the type's lowered name).

## Fix

The three keys use `b.type_name(t)`, which names a type by its module path
when another program module's type shares its name. Golden
internal/llvm/testdata/golden/iface_same_name_types (arm64, LLVM, Go parity,
wasm).

## Avoiding it

Any SSA key or symbol derived from a type must come from `b.type_name`, never
`b.c.show`; `b.c.show` is for messages only.
