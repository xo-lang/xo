# `use c` in a dependency tried to write its binding into the module cache

Status: Observed (2026-10-11, decision 0156 vouch tests)

**Symptom.** `xo check --no-gc` of a module that requires a dependency
with a `use c` line reported `XO1104 cannot bind C header ... mkdir
<module cache>/github.com/acme/cz@v1.0.0/.xo: permission denied`, then
`XO0601 name not found` for the binding's module name.

**Cause.** `loadCImport` put the binding cache at the module root of the
importing module (`<root>/.xo/cbind`). For a required module that root is
in the read only module cache. `use go` already avoided this with
`goBindRoot` (the main module's cache for dependencies); `use c` did not,
because no test had a C binding in a dependency.

**Fix.** `loadCImport` uses `goBindRoot` too (2026-10-11), so a
dependency's binding lands in the main module's `.xo/cbind`. The self
hosted checker (selfhost/check/cimport.xo) still looks in the
dependency's root; it finds no cached binding there and reports the run
unsupported, which TestSelfhostChecker counts but does not compare.
