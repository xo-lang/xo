# 0540: A parameter's implicit effect variable takes the interface's argument

Status: Accepted (owner, 2026-11-06, through the lead). Proposed by Xo
Lang 1 the same day for azure decision 0009, from Xo Azure 1's azcore
Policy/Next shape; built 2026-11-06: compiler/check/infer.xo
bind_param_effs; golden iface_param_effects,
test/errorcheck/iface_param_effects.

## Context

Core 6.2: an effect argument left out of a parameter type (`next: Next`
for `struct Next[fx: Effects]`) becomes an implicit effect variable of the
function, which the function may perform without naming it in `uses`.
At a call, the variable is instantiated from the argument.

When a type is checked against an interface, the checker
(compiler/check/infer.xo, the method match before the XO0701 obligation)
substitutes the receiver's type and effect parameters (`ms` from
find_method, which is why `Bearer[fx]` with `fn (b: Bearer) apply(...)`
satisfies `Policy[fx]`; native support in 3448b607b) and the method's own
type parameters. It never binds an implicit effect variable that came
from a parameter type. That variable stays abstract, so:

```
pub interface Policy[fx: Effects] {
  fn send(self, req: Str, next: Next[fx]) -> Str ! Str uses (net.dial, clock, rand, fx)
}
pub struct Next[fx: Effects] { policies: List[Policy[fx]], at: Int = 0 }
pub struct Tag { name: Str }
pub fn (t: Tag) send(req: Str, next: Next) -> Str ! Str uses (net.dial, clock, rand) {
  next.send("${req} ${t.name}")
}
let ps: List[Policy[fx]] = [Tag{name: "a"}, extra]
```

is XO0701: "`Tag` is used as `Policy[fx]`, whose method `send` allows
only `clock, fx, net.dial, rand`, but `Tag.send` performs `Next.fx`". The
fix it offers, `Next[()]`, is wrong here: `next` carries the rest of the
pipeline, whose effects are the caller's `fx`.

The workaround is a phantom parameter on the implementing type: `struct
Tag[fx: Effects]` with `fn (t: Tag[fx]) send(req: Str, next: Next[fx]) ...
uses (..., fx)`. azcore has 24 hand-written policy types (azure 0009), and
each would carry an `fx` it never stores.

## Decision

When method `m` of a type is matched against interface method `im`, each
implicit effect variable of `m` that comes from an effect argument left
out of one of `m`'s parameter types is bound to the effect argument at the
same place in `im`'s parameter type (after the interface's own
substitution). The parameter types are then compared, and `m`'s effects
are checked against `im`'s `uses`, with that binding: in the example,
`Next.fx` is `fx` and `Tag` satisfies `Policy[fx]`.

This is the instantiation a call already does (core 6.2: the variable is
instantiated at each call from the argument), applied where the interface
fixes the argument's type. It is sound for the same reason: `m`'s body
was checked with the variable abstract, so it holds for any effect set,
including the interface's.

- Only implicit effect variables from parameter types are bound this way.
  The receiver's variables keep their current rule; result and error
  types keep core 6.2's exact rule (their omitted arguments are inferred
  from the body).
- A variable that the interface's parameter leaves abstract too (an
  interface method whose own parameter omits the argument) binds to that
  abstract variable; nothing new is allowed.
- No code generation change: effect arguments take no type argument of an
  instance (3448b607b's recv_targs), on every backend.

Spec: core 2.4 gains a bullet ("a method whose parameter type leaves an
effect argument out satisfies an interface method whose parameter fixes
it, with the variable taking the interface's argument"), and core 6.2's
"In parameter and receiver types" bullet points at it.

## Consequences

- azcore's Policy implementations stay plain structs (`struct Tag`); no
  phantom `fx` on 24 types.
- Programs that pass today keep passing: the change only turns an XO0701
  into an accepted program, and only for this shape.
- XO0701's fix note (`Next[()]`) is no longer offered for this case,
  since the program is accepted.
- Size: compiler/check only. A walk over `m`'s and `im`'s parameter types
  in infer.xo that adds the bindings to `ms2` before the parameter
  comparison, about 40 lines. Tests: a checker test, test/errorcheck for
  the shapes that stay XO0701 (a different effect set fixed by the method,
  a result type), and a golden running the Policy/Next pipeline on arm64,
  LLVM, Go, wasm and the amd64 sample.

## Alternatives

- **Keep the rule; use the phantom parameter.** It works today on every
  backend. Cost: an `fx` parameter on each implementing type that only
  exists to name the interface's argument, and every construction site
  writes or infers it.
- **Let the method declare its own effect parameter** (`fn (t: Tag)
  send[fx: Effects](req: Str, next: Next[fx]) ... uses (..., fx)`). Today a
  method with own type parameters satisfies only an interface method with
  as many (infer.xo's own_n check), so this needs a rule too, and it is
  more to write than the omitted form that core 6.2 already gives
  parameters.
- **Bind only when the interface's argument is an effect parameter of the
  interface** (not a concrete set like `net.dial`). Narrower, but a
  concrete set is no different for soundness, and the narrower rule would
  be one more thing to explain.
