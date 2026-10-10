# 0111: One variable cannot be passed to two `var` parameters

Status: Accepted (2026-10-09, by the owner). Implemented 2026-10-09:
`XO0503` in internal/check (`checkVarAliases`), core 5.2.

## Context

A call such as `both(al, al)`, where `both` takes two `var` parameters,
behaves differently by backend: the Go backend passes `var` arguments by
reference, so each parameter sees the other's writes; the native backends
copy each argument in and back out, so the last copy-out wins. The native
backends report it as XO1201 today (decisions 0107 to 0110 batch). The
spec does not say which is right.

## Decision

Passing the same variable (or overlapping places, such as `x` and `x.f`,
or `xs[i]` and `xs[j]` when the checker cannot prove `i != j`) to two
`var` parameters of one call is a compile error on every backend, with a
new XO code and a note suggesting a temporary copy or a single `var`
parameter of a struct. With aliasing impossible, by reference and copy
in/copy out give the same result, so each backend keeps its current
calling convention.

## Consequences

- One observable meaning on every backend; native drops its XO1201 for
  this case.
- Rare legitimate uses need an explicit copy.

## Alternatives

- Copy in/copy out everywhere: changes the Go backend's behavior and
  cost.
- By reference everywhere: native must pass pointers, and the result of
  aliasing writes stays order dependent and surprising.

## Implementation notes (2026-10-09)

- The checker compares the `var` arguments of every call (a `var`
  receiver included) after the arguments are matched to parameters, so
  named arguments count. A place is a binding followed by fields and
  indexes; two places overlap when they share the binding and one path
  is a prefix of the other, unless a step differs for certain: two field
  names, or two indexes that are different integer or plain string
  literals. Any other index (a variable, an expression, a constant
  name) may be equal, so `both(g[i], g[j])` is rejected; `i != j` is not
  proved from conditions.
- The SSA builder's XO1201 for this case is gone; the internal/gen
  golden `list_value_semantics` passes two copies and is in
  `internal/llvm/testdata/gen_native.txt`.
