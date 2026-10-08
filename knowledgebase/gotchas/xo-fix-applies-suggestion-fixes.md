# `xo fix` applies "did you mean" fixes too

Status: Anticipated (2026-10-08, `internal/fix`)

Symptom: after `xo fix`, a misspelled name has been replaced by the
nearest real name, which compiles but may not be what the author meant
(XO0601 `replace with ...`, `qualify as testing.X`).

Cause: the fix loop applies the first fix of every diagnostic, and the
checker attaches suggestion fixes to unresolved names. The loop cannot tell
a mechanical fix (XO0206, XO0410, XO0110) from a guess.

Fix: run `xo fix --dry-run` first and read the `XO0601` lines of the
report, or review `git diff` after a run. If guesses turn out to cause
wrong rewrites in practice, add a `Mechanical bool` to `diag.Fix` and let
the loop skip the others by default.
