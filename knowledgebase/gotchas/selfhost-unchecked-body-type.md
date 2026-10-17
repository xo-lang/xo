# A body the checker never typed still reaches later passes

Status: Observed (2026-10-10, stage 8 mutations of the self hosted checker)

Symptom: the Xo checker faulted with `index -1 out of range` in
`named_obj` (compiler/check/types.xo) on a module where a real-time
function was declared twice; the Go checker reported XO0603 and an
XO1401 inside the duplicate's body.

Cause: a duplicate declaration is not checked, so its expressions have
no type, but the real-time pass walks every function body, duplicates
included. The Go checker's type lookup returns nil there and every
`isPrelude(nil, ...)` is false; the Xo port keeps types as Int indexes,
and -1 (no type) was passed to `resolve` and used as a table index.

Fix: `named_obj` returns -1 for a negative type. A helper of the port
that takes a type index must accept -1 wherever the Go helper accepts
nil; a pass that walks bodies (real-time, flow, time layouts) can see
untyped expressions. Name mutations that duplicate declarations find
these (TestSelfhostCheckerMutations).
