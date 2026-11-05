# Types that differ only in effects lowered to different native types

Status: Observed (Xo Lang 1, 2026-11-05)

## Symptom

`xo check` passes, a native build fails with XO1201 "converting
`Request[env]` to `Request[fx]`" (or `List[xo.iface[Provider[env]]]` to
`List[xo.iface[Provider[fx]]]`, then "an interface value's type number
given after skipped code"): a value passed to a function whose parameter
names an effect variable (`send[fx: Effects](req: Request[fx])`).

## Cause

The SSA builder interns a named type under its checker text
(Builder.type_name), which printed effect arguments and a function type's
`uses`. A generic function is not instantiated per effect argument, so
inside it the parameter stays `Request[fx]` while the caller's value is
`Request[env]`: two lowered types, and coerce had no conversion between
them.

## Fix

Effects have no run time form: type_name and type_text leave effect
arguments and `uses` out (has_effects decides when to take the
type_text path). Golden internal/llvm/testdata/golden/effect_threaded.
