# `xo fix` applied edit distance guesses that changed the program

Status: Observed (2026-10-09), fixed the same day (decision 0087)

Symptom: on a Python style draft, `xo fix` turned `t += x` into
`it += x` and `map(f, xs)` into `has(f, xs)`; on Codex's cache task it
replaced the effect variable `fx` with the type `Fs`. Each step "fixed"
an XO0601 and produced a different, wrong program.

Cause: every XO0601 carried the closest name by edit distance as a fix,
and `xo fix` applies the first fix of every diagnostic without judgment.
For short names almost anything is within distance one or two.

Fix: the guess stays a note ("did you mean"), and becomes a fix only when
it is clear: a unique closest candidate, at most half the name's length
away, for names of three or more characters (`confidentGuess` in
internal/check/check.go). Habit names (`nil`, `len`, `Vec`, `this`) get
their Xo form instead of a guess (internal/check/habits.go). When adding
a fix anywhere, ask whether applying it blindly can change what the
program means; if so, make it a note.
