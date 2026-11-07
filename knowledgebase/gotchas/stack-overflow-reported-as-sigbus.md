# A stack overflow was reported as SIGBUS (macOS) or SIGSEGV (Linux)

Status: Observed (2026-11-07)

Symptom: unbounded recursion in a native darwin/arm64 program (xo 0.43.0)
ended with "fatal error: SIGBUS: bus error ... at address 0x7000003fa0"
and a good traceback, but nothing said it was a stack overflow. Found by
Xo Azure 1: the Azure generator before azure's ref-decode fix recursed in
type_of on an unresolved $ref (azure-http-specs
client-generator-core/exact-name). Linux reports the same hit as SIGSEGV.

Cause: the crash handler (std/native/llrt/crash.xo, decision 0343) named
every SIGSEGV and SIGBUS by its signal; only Windows had a stack overflow
message (entry.xo on_overflow, decision 0197). On macOS a store to a
PROT_NONE page, such as a fiber stack's guard page, is a SIGBUS, not a
SIGSEGV, so the report looked like a misaligned or unmapped access.

Fix: crash.xo on_guard: a SIGSEGV or SIGBUS whose address is within 64
KiB of the current fiber's mapping start (pool.xo F_MEM; the guard page is
the mapping's first page, and main runs on a fiber too) writes "fatal
error: stack overflow" before the signal's line. The signal's default
action still ends the program (status and core file as before).
internal/arm64 TestCrashReport checks C recursion, Xo recursion on main's
stack and on a task's, and that a plain SIGSEGV is not called one.

Avoid: on macOS read a SIGBUS at an address just below a stack as a stack
overflow; `XO_CRASH_SLEEP=N` and `vmmap -v PID` show which mapping the
address is in.
