# internal/gen's inspectNode does not walk whole files

Status: Observed (2026-10-11, decision 0173)

Symptom: a whole-program scan in internal/gen (does the program call
`task.with_budget` anywhere?) missed a call inside a `test` block, so a
dependency's loop compiled without budget polls and the consumer's test
hung until the 1 minute test limit.

Cause: `inspectNode` (internal/gen/support.go) is written for function
bodies and expressions; it does not descend into every declaration kind
of a file, test declarations among them.

Fix: scan files with `check.Inspect(file.AST, ...)`, as `xo audit` does;
keep `inspectNode` for bodies.
