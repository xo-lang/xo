# A type over a function of an ambiguous struct took another module's type

Observed 2026-11-04.

Symptom: a native build (arm64, amd64, LLVM) failed with XO1201 "not
available on the native backends yet: converting `b.Change` to
`a.Change`" (or between `fn(b.Change) -> b.Change` and
`fn(a.Change) -> a.Change`), in a program where two modules each declare
their own struct `Change` and each makes an Option of a function over it
(`(fn(Change) -> Change)?`) or a generic struct holding one. The Go
backend ran it. xo's own build hit it when List.map in place (decision
0508) made `fn(T) -> T` Options for compiler/cli's core.Change and
modules.Change.

Cause: the SSA builder names a type by its text and interns it by that
name. type_text (compiler/ssa build.xo) puts the module path on a struct
name that more than one module declares, for Named and Tuple types, but a
function type fell through to the checker's show, which prints the bare
name. Both modules' `(fn(Change) -> Change)?` got one name and so one
type.

Fix: type_text has a Func case that prints each parameter, result, error
and effect through type_text (2026-11-04). Regression golden
internal/llvm/testdata/golden/fn_type_same_name_structs. Any new kind of
type text that can hold a named type must go through type_text, not
show, for the same reason.
