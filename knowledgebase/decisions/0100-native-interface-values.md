# 0100: Interface values on the native backends

Status: Accepted (2026-10-09). Implemented in `internal/ssa/build_iface.go`
(lowering, conversion, dispatch, Display and Debug, `==`), `OpRebind`
(`internal/ssa/ssa.go`, both backends), `internal/ssa/build_show.go`
(`ifaceCall`), `internal/ssa/build.go` (`coerce`). Goldens (Go, LLVM, and
arm64 compared, leak check 0): `internal/llvm/testdata/golden/iface_values`,
`iface_eq`, `iface_go` (internal/gen's `interfaces`), `iface_adapters`
(internal/gen's, with a library module).

## Context

The spec audit (`bench/results/spec-audit-2026-10-09.md`, row 2.4) listed
interface types used as values as the largest native gap: every program
with a `List[Store]`, a `Shape` parameter, or a struct field of interface
type was `XO1201`. The native backends had type erasure only for the
prelude's Error, Display, and Debug, which are lowered as their text
(`xo.dyn`, decision 0074), and dispatch only through monomorphized
generics. The SSA layer already has closures (decision 0048): a code
pointer and a reference counted environment, with sharing between
threads, unwinding, and leak checked drop functions in both backends.

## Decision

1. **An interface value is a struct of closures over the concrete
   value.** `xo.iface[I]` has one function value per method of `I` in
   declaration order (parameters and result lowered with `I`'s type
   arguments; a method that can fail returns its `Result`), then three
   hidden fields: `xo.eq: fn(Int, fn(Str, Bool) -> Str) -> Bool`,
   `xo.tid: Int` (a number per concrete type), and `xo.show: fn(Str,
   Bool) -> Str`, which appends the concrete value's Display (false) or
   Debug (true) text.
2. **One environment per conversion.** A conversion allocates one
   environment holding the concrete value (the `xo.show` closure); the
   method functions and `xo.eq` share it through the new op `OpRebind`
   (the code of another Indirect function with the same captures, the
   environment of an existing function value, consumed). LLVM lowers it
   to an `insertvalue` of the code pointer, arm64 to the code address and
   the same environment word. Reference counting needs nothing new: each
   field holds a counted reference to the one environment.
3. **Method functions adapt.** The code of method `m` for concrete type
   `C` (`xo.iface[I].C.m`) reads the value out of its environment, calls
   `C.m`, and converts the result to the interface's signature: `Ok(...)`
   around a method that cannot fail where `I.m` can, and the error
   converted as at a `?` (`from` variants, `Error`). Generic code calling
   a constraint's method on a type parameter uses the same adaptation
   (`iface_adapters`).
4. **Interface to interface.** Converting an interface value to another
   interface copies the function values whose types agree, adapts the
   others with a closure over the source value, and copies the hidden
   fields, so Display, Debug, and `==` still see the concrete value.
   A Display or Error value (`xo.dyn`) converts like a concrete value
   whose `display` is its text.
5. **Display and Debug.** `${x}`, `x.debug()`, and Debug text of a value
   holding an interface value call `xo.show` (the Go backend formats the
   dynamic value the same way). `x.debug()` on any value was XO1201
   natively and is now lowered too.
6. **Equality.** `a == b` on interface values calls `a.xo.eq(b.xo.tid,
   b.xo.show)`: false when the concrete types differ; otherwise the
   function reads the other value out of `b`'s show closure (whose
   environment has the same layout as its own, the one place an
   `OpCapture` reads another closure's environment; the verifier allows
   it) and compares with `==` of the concrete type (Map and Set by
   content). A concrete type holding a function value is equal to
   nothing, as std/rt's `EqAny` (reflect.DeepEqual of non nil functions).
   Structs, lists, options, and enums holding interface values compare
   through the builder's `xo.eqc` helpers (which already handled Map and
   Set).

## Consequences

- No XO1201 remains for interface types; generic interface methods
  (methods with their own type parameters) and interface methods with
  `var` parameters are still XO1201 when used through a value.
- A conversion costs one allocation (the concrete value's environment)
  and a call through an interface costs an indirect call; a conversion
  of an interface value to a narrower interface allocates only for
  methods whose signature changes. The `iface_values` golden went from
  2379 allocations (one environment per method) to 712 with `OpRebind`.
- Hello world size is unchanged (arm64 310208 bytes, LLVM 88272 bytes on
  2026-10-09): nothing is emitted for a program without interface values.
- Capability interfaces (Fs, Clock, ...) stay capabilities (`KCap`): a
  user type implementing one is not a capability value natively. Fs since
  decision 0307 (a `user` field in the native Fs).

## Alternatives

- **Vtables in the C runtime** (a static table per concrete type and
  interface, a boxed value): fewer closures, but a new value kind in both
  backends (layout, drop, share, equality helpers) and in the RC verifier.
  The closure form reuses everything that already works and is leak
  checked; `OpRebind` removes most of its cost.
- **One closure per method** (each with its own environment): what the
  first version did; k+2 allocations per conversion.
- **Equality through Debug text**: wrong for NaN, `-0.0`, and Maps with
  different insertion orders.
