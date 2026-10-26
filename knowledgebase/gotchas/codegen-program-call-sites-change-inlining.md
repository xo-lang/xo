# A codegen pattern program's second call site changes what is inlined

Status: Observed (2026-10-26, Xo Lang 1)

## Symptom

Adding a function to test/codegen/enums that called `Duration.parse` made
an existing pattern there fail: `ok_nanos: arm64 code does not match "bl
_xo_f_text_2eparse__duration"`. Nothing in the compiler had changed.

## Cause

Each test/codegen directory is one program, and the inliner decides per
program: `prelude.duration_parse` was inlined with one call site and
called with two (the few sites rule, compiler/arm64 inline.xo), so both
functions' code changed shape. A pattern that depends on inlining holds
only for the program's call sites as they are.

## Fix

Put a pattern that depends on a callee being inlined in its own program
directory (test/codegen/retains holds only ok_sum), or call the callee
from one place. Check a new absent pattern against the parent's xo too:
it must fail there (`xo dev codegen --backend=arm64 DIR` with the
parent's bin/xo); a pattern for an inline retain first said `tbz`, the
parent's retain was `tbnz`, and the parent passed it.
