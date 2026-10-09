# What an effect ceiling does and does not see

Status: Anticipated (2026-10-09)

Symptom (expected): `require github.com/acme/tick v1.0.0 uses none` and a
call `tick.apply(fn() -> Int { os.stdio.println("x"); 1 })` checks clean
although the call performs `stdio`; or a dependency's value reached
through an interface the caller defines performs an effect the ceiling
excludes, with no XO1309.

Cause: a ceiling (decision 0086 step 3, `internal/check/ceiling.go`)
counts the effect labels the dependency's function declares. Effect
variables (`fx: Effects`) carry the caller's own effects, so a callback's
effects are the caller's, bounded by the caller's `uses`. A call through
an interface is a call of the interface's method, whose declared effects
bound every implementation; when the caller owns the interface, the
caller already allowed those effects.

Fix: none needed; this is the intended scope. To keep a dependency's code
away from an effect entirely, do not hand it the capability (core 6.3):
ceilings and capabilities together close the gap. A closure with an
explicit `uses` clause that calls the dependency gets XO0701, not XO1309,
when it lacks the effect.
