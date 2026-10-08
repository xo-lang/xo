# The last statement of a scope reported the next line

Status: Observed (2026-10-08, deadlock reports named a spawn one line low)

Symptom: a task spawned by the last statement of a `scope` body was
reported as `spawned at main.xo:7` for a call on line 6 (the scope's
closing brace). Faults raised in that statement had the same offset.

Cause: codegen delivers the last statement of a scope body with a
`pre` statement (`x_done3 = true`, the flag that tells `Scope.Close` the
body completed) written after the statement's `//line` directive, so the
call itself landed one line further.

Fix (internal/gen/fn.go `deliver`, `dReturn`): after writing `pre`,
write the expression's `//line` directive again.
