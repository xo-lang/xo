# Several match arms on one line gave a false XO0301 with a hole fix

Status: Observed (2026-10-09, Codex r2 rerun, task 01)

Symptom: `match r { Ok(v) => v Err(_) => return e }` gave XO0175 with no
fix, plus XO0301 "not exhaustive: missing `Err(_)`" whose fix inserted
an `Err(_) => ???` hole. `xo fix` would apply the hole, which hides the
real arm.

Cause: parser recovery after XO0175 skipped to the end of the line, so
the second arm was dropped and the checker saw a one arm match.

Fix: when the token after an arm starts an operand and a `=>` follows on
the same line outside brackets, the parser reports XO0175 softly with a
fix inserting a newline, then parses the rest as arms. `xo fix` and the
formatter put each arm on its own line
(`internal/fix/testdata/habits/codex-arms-one-line`).
