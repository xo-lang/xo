# Go rejects methods that grow their receiver's type arguments

Status: Observed (2026-10-07, P1 runtime)

Symptom: `List[T].M_indexed() List[Tuple2[int64, T]]` does not compile:
"instantiation cycle". Go instantiates every method of every instantiated
generic type, so List[Tuple2[int64, T]] needs List[Tuple2[int64,
Tuple2[...]]], forever.

Fix: such methods are top level functions (`rt.List_indexed[T](tk, xs)`,
`rt.Map_entries`). The generator applies the same rule to user methods
(`growsSelf` in internal/gen/decl.go): a method whose signature
instantiates its own receiver type with other arguments is lowered to
`<Type>_M_<name>(tk, recv, ...)`. Such a method cannot satisfy a Go
interface, so an Xo interface implemented through it needs an adapter.

Polymorphic recursion is the same error for functions (2026-10-08): `fn
depth[T](x: T, n: Int)` calling `depth([x], n - 1)` is an instantiation
cycle in Go, with and without `--mono` (the `--mono` depth cap falls back
to the generic function, which Go rejects). Unsupported in P1.
