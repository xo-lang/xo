# macOS keeps a SA_RESETHAND handler for SIGILL and SIGTRAP

Status: Observed (2026-10-22, decision 0343 stage 1)

Symptom: the crash handler (std/native/llrt crash.xo) installs its
handlers with SA_RESETHAND and returns after the report, so the faulting
instruction runs again under the default action. For SIGSEGV that ended
the program as expected (status 139). For a `brk` (SIGTRAP) on macOS the
handler ran a second time and printed "(the crash report faulted:
SIGTRAP)" before the program ended with status 133.

Cause: POSIX leaves SA_RESETHAND for SIGILL and SIGTRAP unspecified, and
macOS does not reset those two: the handler stays installed. Linux resets
them.

Fix: the handler sets the signal's action to SIG_DFL itself before it
returns, whatever SA_RESETHAND did. Do not rely on SA_RESETHAND alone to
make a returning handler fatal.
