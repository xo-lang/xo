# Values with no words crash code that reads word 0

Status: Observed (2026-10-08, internal/arm64, twice)

Symptom: `index out of range [0] with length 0` panics in instruction
selection: first for the `Os` parameter of `main` (when giving List block
parameters a length register), then for the `()` result of an inlined
function (the continuation block's unit parameter).

Cause: in the backend's layout, `()` and capabilities (`Os`, `Stdio`)
have zero words. Code that checks `p.Type.Kind == KList` after reading
`s.vals[p][0]` reads first and checks second.

Fix: test the type (or `len(words)`) before touching a word. Any new code
that walks block parameters, call results, or edge arguments must expect
zero word values; `TestInlineVerifies` and the golden runs cover the
cases that broke.
