# `or_err` on a Result: the did you mean fix made it worse

Status: Observed (2026-10-09, rerunning bench/habits on Codex task 10)

Symptom: after `Duration.parse` was added returning a `Result`, Codex's
reconstructed first run `Duration.parse(x).or_err("invalid flush")?` got
XO0601 with the fix "replace with `is_err`"; `xo fix` applied it and the
module went from 1 error to 2.

Cause: `or_err` exists only on Option. The edit distance guess picked
`is_err` (two edits away, unique), which is a different operation with a
different type.

Fix: `or_err` with one argument on a Result is a habit
(internal/check/habits.go): the note explains that the value is already a
Result and the fix rewrites to `.map_err(fn(_) { e })`. Habit golden
`internal/fix/testdata/habits/codex-or-err-on-result`. When adding a std
function whose result shape differs from its siblings (`Int.parse`
returns an Option), rerun `python3 -I bench/habits/measure.py` on the
snaps to see what the guesses do.
