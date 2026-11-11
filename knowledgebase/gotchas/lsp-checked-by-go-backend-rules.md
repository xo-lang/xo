# The language server checked by the Go backend's rules

Status: Observed and fixed (2026-11-11)

## Symptom

VS Code showed XO1201 ("`use c "vad.h"` is impossible on the Go backend:
C code is called from native builds only") on every `use c` line of a
program that `xo check`, `xo build` and `xo run` accepted with no flag.

## Cause

`check.Checker.native` defaults to false, the Go backend's rules. `xo
check` sets it through load_mode (native has been the default backend
since decision 0248), but compiler/cli/servers/lsp.xo's new_lsp_checker
built its own Checker and never set it. The LSP also used `xo check`'s
module check cache key (mod_flags "xo check\n...") while checking by
other rules. No LSP test opened a `use c` file, so nothing caught it.

## Fix

new_lsp_checker sets `c.native = true`. cmd/xo TestLSPUseCNative opens
a `use c` program that `xo check` passes and expects no diagnostics.
A new path that builds a Checker by hand (not through
compiler/cli/core's load functions) must set `native` the way `xo check`
does, or it reports what the build accepts.
