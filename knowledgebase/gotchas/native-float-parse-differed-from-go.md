# Native Float.parse rejected text Go's ParseFloat accepts

Status: Observed (2026-10-12, decision 0193 step 3)

Symptom: the first run of TestNumDiff (internal/llvm/num_test.go)
failed one line on both native backends and both paths: `Float.parse("1_000")`
was None where the Go backend (`strconv.ParseFloat`) gives 1000.0.
Hex floats (`0x1p-2`) and texts over 4096 bytes were None too.

Cause: the C runtime's Float.parse (std/llrt/xo_std.c `n_parse_float`,
now only the `XO_LLRT_C=num` path) validates a decimal grammar of its own
and then calls strtod: no underscores, no hex floats, a length limit.
Go's ParseFloat takes Go's float literal syntax, underscores between
digits (strconv.underscoreOK, also without a base prefix), and any
length.

Fix: std/native/llrt/num.xo ports Go 1.26's atof.go whole (special
values, readFloat with hex and underscores, the exact path,
Eisel-Lemire, the big decimal fallback, atofHex), so the native
backends now parse what the Go backend parses. The C path keeps its
grammar; TestNumDiff skips those lines for it (`numCSkips`). Compare a
native text routine against Go's own function on a generated corpus,
not against the C it replaces.
