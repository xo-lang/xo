# rt.str_find searches to the end of the string, not to a bound

Status: Observed (2026-10-14, audit item B10).

**Symptom.** An Xo parser in std/native/llrt is slower than its C
original by far more than the bar allows on large inputs, and the cost
grows with the square of the input: the native client's chunked bodies
measured 1.123 (arm64) and 1.104 (LLVM) times the C's instructions on
4 KiB in 32 chunks (`bench/results/http-client-2026-10-14.md`).

**Cause.** `rt.str_find(s, sub, from)` has no end bound. A helper like
http1.xo's `find(s, c, from, to)` calls it and then compares the result
with `to`, so when the byte is not in `[from, to)` (a chunk size line
without `;`, the usual case) it scans the whole rest of the input. Done
once per line of a body, that is quadratic. The C's `memchr` took the
line's length.

**Fix.** For a search inside a short range (a line, a field), scan the
bytes in the range (`bfind(b, c, from, to)` in http1.xo). Keep
`rt.str_find` for searches whose answer is usually near (the next
`\n`) or that must look to the end anyway.
