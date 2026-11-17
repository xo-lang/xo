# 0690: a capability field written without its effect argument is the OS's

Status: Accepted (owner, 2026-11-16; proposed by Xo Lang 1, task #161; amends 0652)

## Context

Decision 0652 step 2a (c4873ddcc) made the prelude `interface
Rand[fx: Effects]`. A `Rand` written without the argument takes it from
the value passed (core 6.2), which is right for a parameter: `fn f(r:
Rand)` performs what its caller's value performs. In a struct field the
same rule gives the struct an implicit effect parameter (`Client.Rand.fx`)
that no caller can name. A closure checked against a function type with a
fixed `uses` list that calls through such a field fails: the AWS SDK's
clients (`rand: Rand` in transport, config, imds and every generated
client) stopped building on main with "XO0701: the expected function type
allows only `clock, net.dial, rand, sync`, but performs `Client.Rand.fx`"
(found by xo-4c, 2026-11-16). The SDK cannot pin `rand: Rand[rand]`:
released xo (0.51 to 0.53) does not accept that type, and the SDK builds
with released xo.

## Decision

A capability type written without its effect argument in a struct field
or an enum variant's field is the OS value's type: `Rand` is `Rand[rand]`
(`Os.rand`'s), and `Clock` will be `Clock[clock]` when Clock takes its
parameter (0652 step 3). Parameters, results, locals and function types
are unchanged (the value's effect, core 6.2). A struct that wants its
field to carry its value's effect says so: `struct C[fx: Effects] { r:
Rand[fx] }`. Covariance (0582) lets a pure source (`Rand[()]`,
random.seeded) fill a `Rand[rand]` field.

## Consequences

- Code written before 0652 with `rand: Rand` fields builds again with the
  meaning it had: calls through the field perform `rand`.
- A struct holding a pure Rand in a bare field performs `rand` where it
  calls it; to keep such code pure the struct names the parameter.
- Implemented in compiler/check decl.xo `elided_effect` (TypeEnv
  `type_decl`, `os_effect_of`); test golden rand_field_os (a closure
  against `fn() -> Int uses rand` calling through a bare field: XO0701
  before; an enum variant and a pure source too).
- Spec core 6.3 states the rule.

## Alternatives

- An error at the field with a `Rand[rand]` hint: clear, but breaks the
  SDK and every library that must also build with released xo, which
  cannot spell `Rand[rand]`.
- A warning plus the implicit parameter: the calls still fail.
- Keep 0652 as is: every library with a Rand field pins its type, after a
  release that accepts `Rand[rand]`.
