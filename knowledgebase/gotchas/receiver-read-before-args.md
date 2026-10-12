# A by value receiver is read before the call's arguments

Status: Observed (2026-10-10, porting internal/ssa to selfhost/ssa)

Symptom: the Xo SSA builder faulted with `index 3442 out of range for
List of length 3442` in the checker's `resolve`, called from
`b.type_name(b.impl_named(t))`: `impl_named` had just added the type
3442 to `b.c.tys`, but `type_name` looked it up in a table without it.

Cause: Xo evaluates a call left to right, the receiver first. A by value
receiver (`fn (b: Builder) type_name(...)`) is copied before the
arguments run, so an argument that changes the receiver's variable
(`impl_named` is a `var` method of the same `b`) is not seen by the
call. A `var` receiver behaves the other way (decision 0142: its place
is read after every argument). Go code ported as is (`b.typeName(
b.implNamed(r))` on a pointer receiver) hits this whenever the argument
grows a table the receiver indexes. All three backends agree (`t.size(
t.add(7))` with a by value `size` sees `t` before `add`).

Fix: bind such an argument first (`let it = b.impl_named(t); let name =
b.type_name(it)`), or make the method a `var` receiver. selfhost/ssa
makes every Builder method `var`, which also avoids copying the builder
(and the checker it holds) on every call. A regular expression over
calls of by value methods whose arguments call `var` methods of the same
value finds the pattern in a port. spec/core.md 5.2 states the order.

Observed again 2026-10-11 (SSA stage 2): `b.c.show(b.impl_named(ct))`,
the text of an interface value's concrete type, faulted the same way
(`show` is a by value method of the checker, `impl_named` adds to its
type table). selfhost/ssa iface.xo binds it first (`impl_text`).

Observed again 2026-10-11 (the Go code generator port, decision 0167):
`g.c.resolve(g.mono_type(t))` in selfhost/gen ranges.xo faulted the same
way on bench/xo/v0.4/02-worker-pool (`mono_type` adds the substituted
type to the checker's table). A scan for the pattern (calls of by value
methods of `g` or `g.c` whose arguments call `var` methods of `g`) found
two more in the port before they faulted; all three bind the argument
first.
