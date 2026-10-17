# `x[...]` is type arguments or an index, decided by shape

Status: Observed (2026-10-10, `KIND_NAMES[k]` in compiler/check)

The grammar uses square brackets for both explicit type arguments
(`json.decode[User](b)`, `Mutex[Int].new(0)`) and indexing (`xs[i]`). The
parser treats `[...]` after an expression as type arguments when every element
looks like a type (starts with an UpperCamel name, `module.UpperCamel`, `(`,
or `fn`, and contains only type tokens) and either a call follows or the base
is itself an UpperCamel type name. Everything else is an index.

Trap: indexing a map keyed by enum variants and calling the result,
`handlers[Red](x)`, parses as a call with type argument `Red`. Write
`let h = handlers[Red]` then `h(x)`. The checker should report a precise error
for this case (a non generic callee with type arguments) rather than a generic
type mismatch.

Update (spec 0.4, 2026-10-07): effect arguments (core 15: TypeArg) may also
appear: an element that is a whole `name` or `name.sub` counts. With an
UpperCamel base everything in the brackets is type arguments
(`Cache[Int, Item, net.dial].new()`); with a value base a call must follow and
at least one element must look like a type (`f[Int, net](x)`), so
`fns[i](x)` stays an index. `f[net](x)` is therefore an index too; effect
arguments of functions are inferred anyway.

Observed (2026-10-10): with effect arguments counting as type elements,
`KIND_NAMES[k]` (a constant list indexed by a local) parsed as type
arguments and was rejected with XO0407 "type arguments are only allowed
on generic functions and types". Fixed by decision 0143: a base spelled
like a constant (`UPPER_SNAKE`) with only lowercase names in the
brackets is an index. `handlers[Red](x)` above is still a call with type
arguments.
