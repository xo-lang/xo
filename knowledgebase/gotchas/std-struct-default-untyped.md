# A struct literal default in a std declaration was XO1201 natively

Status: Observed and fixed (2026-10-30)

**Symptom.** After ProcOutput gained `usage: ProcUsage = ProcUsage{user:
0s, system: 0s, max_rss: 0}` (decision 0426's addendum), every native
build of user code that wrote a ProcOutput without `usage` (a fake Proc:
the user_caps golden, cmd/xo TestAttenuationFakes, TestNoGCTestLeaks)
failed with `std/prelude.xo:1126:22: error XO1201: ... this struct
literal`. The Go backend built them.

**Cause.** A std declaration file is not checked as bodies, so the
checker records no type for a default expression there. compiler/ssa
foreign_value lowered such a default with the type the checker recorded,
and struct_lit, finding none, reported the literal as unsupported. The
`None` and `[]` defaults already took the field's type in that case
(std/http's Cookie, Proc.cmd's args); a struct literal did not.

**Fix.** foreign_value lowers a struct literal default as a value of the
field's declared type (struct_lit_as), when the checker recorded none.
A new std default of another kind (a call, a map literal) can hit the
same gap: build a user program that omits the field on arm64 before
pushing.
