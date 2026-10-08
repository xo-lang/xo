# `snprintf` dominated Int interpolation in the LLVM runtime

Status: Observed (2026-10-08, `std/llrt/xo_rt.c`, bench `strings`)

Symptom: the `strings` workload (two million interpolated strings) ran
in 147 ms under the reference counted LLVM backend, 1.6x of Go. Cutting
its allocations 3x (appending an Int's text into the string being built,
`str.concat_int`) gained only 10 ms.

Cause: `fmt_int` formatted with `snprintf(buf, 32, "%lld", v)`. The libc
call parses the format and goes through the locale machinery on every
Int; with four million conversions it was most of the run.

Fix: a digit loop (divide by 10 into a small buffer, reverse; unsigned
negation handles the minimum Int). The workload dropped to 36 ms, 0.40x
of Go. Floats still use `snprintf("%.*e")` plus `strtod` to find the
shortest round trip digits, which is slow too; a Ryu or Grisu port is
the fix when a Float formatting workload matters.
