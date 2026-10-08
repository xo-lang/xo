# A constraint method that can fail, on a type whose method cannot, faults

Status: Observed (2026-10-08, while writing testdata/golden/mono_generics)

Symptom: with `interface Named { fn name(self) -> Str ! Error }`,
`fn names[T: Named](xs: List[T])` calling `x.name()?`, and
`fn (q: Sq) name() -> Str` (cannot fail), `names([Sq{...}])` type checks
but faults at run time: `rt: ...Sq does not implement the interface`.

Cause: a constraint method call on a type parameter is lowered to
`rt.Iface[Named](x).M_name(tk)`. `Sq`'s Go method returns `string`, the
interface method `rt.Result[string, rt.IError]`, so the Go type does not
implement the Go interface. Conversions of concrete values build an adapter
(`ifaceAdapter` in internal/gen/support.go), but conversions from a type
parameter skip it because the type is unknown in the generic Go body.
`--mono` keeps the same behavior: devirtualization requires identical Go
signatures, so the copy falls back to `rt.Iface`.

Fix (open): in a `--mono` copy the type is known, so call the concrete
method and wrap the result (`rt.Ok`), or build the adapter. For the
generic lowering, register adapters per instantiation in rt (a table from
concrete Go type to adapter constructor) and look it up in `rt.Iface`.
Workaround: declare the concrete method with the same `! Error`.
