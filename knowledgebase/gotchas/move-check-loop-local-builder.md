# A move only builder declared in a loop body was reported as moved

Status: Observed (2026-10-09)

Symptom: XO0510 "use of moved value `sb`" for
`while ... { var sb = StrBuilder.new(); sb.write(..); out = out + sb.freeze() }`,
pointing at the `freeze` of the previous iteration.

Cause: the move checker (internal/check/flow.go) walks a loop body twice
so a move in one iteration is seen by the next, but a `let` or `var` in
the body did not clear the moved state of the binding it declares, so
the second walk saw the first walk's `freeze`.

Fix: `let` (a plain binding) and `var` clear the moved state of their
binding. Tests in rules_test.go ("builder declared in loop body").
