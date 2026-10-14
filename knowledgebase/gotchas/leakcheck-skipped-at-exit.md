# XO_LEAKCHECK reports nothing for a program that ends with os.proc.exit

Status: Observed (2026-10-14)

Symptom: `XO_LEAKCHECK=1` printed no `xo leakcheck:` line for a native
program that ends with `os.proc.exit` (or whose main returns an error,
which ends through the same exit), so its leaks went unreported;
`XO_LEAKCHECK=1 xo build ...` reported nothing for xo itself.

Cause: the report runs only when main returns (the end of the C
runtime's main in std/llrt/xo_rt.c, no atexit handler).
std/native/llrt/process.xo `exit` ends the process at once (exit_group,
libSystem's exit, ExitProcess), as the C xo_exit did before work item
B03. That is deliberate: frames still on the stack hold values, so a
report at exit would count them as leaks (Go's os.Exit skips defers the
same way).

Fix: since 2026-10-14 `exit` prints `xo leakcheck: skipped
(os.proc.exit)` to standard error when XO_LEAKCHECK is set (also for a
main that returned an error), so the gap shows
(internal/llvm TestLeakCheckExit, arm64 and LLVM). The test filters
that drop `xo leakcheck:` lines drop it too; tests that require a count
run only programs whose main returns. For a count from xo itself, its
main would have to return instead of calling os.proc.exit at its end
(selfhost/xo, not changed).
