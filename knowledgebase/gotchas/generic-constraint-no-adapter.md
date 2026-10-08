# A constraint method that can fail, on a type whose method cannot, faults

Status: Observed (2026-10-08, while writing testdata/golden/mono_generics);
fixed 2026-10-08 (golden program `iface_adapters`)

Symptom: with `interface Named { fn name(self) -> Str ! Error }`,
`fn names[T: Named](xs: List[T])` calling `x.name()?`, and
`fn (q: Sq) name() -> Str` (cannot fail), `names([Sq{...}])` type checked
but faulted at run time: `rt: ...Sq does not implement the interface`.
The same happened for `let n: Named = x` with `x: T`, for a constraint with
more methods converted to one with fewer, and (without generics) for an
interface value converted to an interface whose method can fail
(`let n: B = a` with `a: A`, `A.name() -> Str`, `B.name() -> Str ! Error`).

Cause: a conversion of a type parameter to an interface was lowered to
`rt.Iface[Named](x)`. `Sq`'s Go method returns `string`, the interface
method `rt.Result[string, rt.IError]`, so the Go type does not implement the
Go interface. Conversions of concrete values build an adapter
(`ifaceAdapter` in internal/gen/support.go), but generic Go code does not
know the concrete type, and interface to interface conversions skipped the
adapter.

Fix (internal/gen/adapt.go, std/rt/core.go):

- In a specialized copy (`--mono`, the default) the type is concrete:
  `toIface` and constraint method receivers apply the copy's substitution
  and build the adapter directly.
- In generic Go code (`--no-mono`, and the fallback for recursion through
  type parameters) `rt.Iface` consults a registry: package main's
  `xo_adapt.go` registers, with `rt.RegAdapter`, an adapter for every pair
  of a concrete user type named anywhere in generated code and an interface
  generic code converts a type parameter to, when the type implements the
  interface and needs an adapter.
- An interface value converted to an interface with different Go methods is
  wrapped in an adapter around the source interface.

Remaining gap: a concrete type that exists only inside generic code (built
from a type parameter, never named concretely) is not registered; under
`--mono` such code is specialized and does not need the registry.
