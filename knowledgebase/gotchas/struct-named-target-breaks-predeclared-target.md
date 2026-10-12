# A struct named `Target` anywhere in the program breaks `target` natively

Status: Observed (2026-10-12, decision 0182)

Symptom: `xo build --backend=arm64 selfhost/xo` (and `xo check --no-gc`)
failed with XO1201 "not available without a garbage collector: the name
`target` here" at `match target.os` in selfhost/ssa/expr.xo, code that
had built for days. The only change was a new package, selfhost/macho,
imported by selfhost/xo.

Cause: selfhost/macho declared its own `struct Target` (a relocation
target). The native SSA builder (`internal/ssa` build_target.go, `(*fn)
target`) checks the type of the predeclared `target` by shape, and with
a second struct named `Target` in the program the type it resolved was
not the prelude's two enum struct, so it returned nil and reported the
name as unsupported in an unrelated package. The Go backend is
unaffected.

Fix: renamed the struct (`RelocTarget`). The builder should resolve the
prelude's `Target` by identity rather than by name; until then, do not
name a type `Target` in a program that uses (or imports a package that
uses) `target`.
