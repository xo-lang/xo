# A port that stops at the first error hides simplified error paths

Status: Observed (2026-10-10, compiler/parser resynchronization)

Symptom: once the Xo parser went on after a hard error, 26 of 20,000
token mutations still differed from the Go parser, and many hard error
messages differed (`found RParen ")"` for `found `)``, `the function
body` for `the body of `f``, `malformed Go import path` without the
quoted path).

Cause: while a hard error stopped the parse, nothing after it was ever
compared, so the port had simplified what follows an error: a `while`
condition used as an expression was not parsed, a misplaced `..` pattern
was not consumed, `derive` and `invariant` were parsed after a failed
member list, a struct literal in a condition did not skip the literal,
and messages were shortened.

Fix: port the error paths line by line too (internal/parser's `p.bad`
checks are `p.bad` in the Xo parser), describe tokens with the Go
parser's `describe` (`describe_go`), and compare every dump, rejected
files included. The token mutations (`XO_SELFHOST_PARSE_FUZZ=20000`,
several `XO_SELFHOST_SEED`s, `XO_SELFHOST_FAILS=N` to list every
difference) find what the repository files do not.
