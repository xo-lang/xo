# Text that mentions `${` is an interpolation unless written `\${`

Status: Observed (2026-10-08, porting the lexer's messages to Xo; caught in
review before the first check)

Symptom: the Go message "`${` is not closed by `}`" copied into an Xo
string literal starts an interpolation at `${` and ends it at the next `}`,
so the line no longer parses (or interpolates a variable when the text in
between happens to be an expression).

Cause: `${` starts an interpolation in every non raw Xo string (core 1.6);
in Go strings it is plain text.

Fix: write `\${`, or use a raw string `r"..."` when the text has no
escapes. When porting messages, grep them for `${` before the first check.
