# A generic function used as a value has no recorded type arguments

Status: Observed (2026-10-08, `internal/ssa`, SSA golden `generics`)

Symptom: `let f: fn(Str) -> Str = ident` with `fn ident[T](x: T) -> T`
failed to build: the SSA builder asked `check.Info.RefSubst` for the type
arguments of `ident` and got nil. The deleted AST path of internal/llvm
rejected the same program ("using `ident` as a value").

Cause: the checker records a substitution for a reference only for a
method used as a value (`RefSubst`); for a plain generic function used
as a value it unifies the signature with the expected function type but
keeps no per reference substitution. Calls are fine: `CallOf(...).Subst`
has the type arguments.

Fix: the builder matches the function's declared signature against the
function type the value has at that point (`matchTParams` in
`internal/ssa/build_call.go`), binding each type parameter to the
corresponding part of the concrete type, then instantiates as for a
call. If the checker ever records the substitution, prefer it.
