# A machine fix on a file that is not .xo is dropped by `xo fix`

Status: Observed (2026-10-11, adding XO1315 on xo.mod, decision 0155)

**Symptom.** A diagnostic carries a fix whose edit targets xo.mod (or any
file that is not a `.xo` source), the LSP and `--json` show it, but
`xo fix` and `xo check --fix` report no change for it and leave the file
alone. Without the second half of the fix below, the loop would instead
apply the fix again every round, because the check rereads the file from
disk.

**Cause.** `fix.Run` keeps the files it may edit in an in memory overlay
and `applyFixes` skips any fix with an edit outside it; the overlay held
only the `.xo` files under the dirs. And `check.findModRoot` read xo.mod
with `os.ReadFile`, not through the overlay, so a rewritten xo.mod was
not seen by the next round.

**Fix.** `fix.sourceFiles` also lists every xo.mod under the dirs plus the
xo.mod of the module around each dir; `findModRoot` reads through
`c.readSource` (the overlay). Text rewriters and the formatter skip files
that are not `.xo`. A new fix on another kind of file (xo.work, xo.lock)
needs the same two steps: list the file in `sourceFiles`, and read it
through the overlay wherever the checker loads it.
