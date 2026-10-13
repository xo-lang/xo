# Parsing a subnormal through the big decimal cost 270,000 instructions

Status: Observed (2026-10-12, decision 0193 step 3)

Symptom: the first Xo Float.parse (std/native/llrt/num.xo) took 54.8
billion instructions for 200,000 shortest texts of subnormal Floats on
arm64, against 0.59 billion for the C path (strtod); short and 17 digit
normal texts were 0.19x and 0.30x of the C.

Cause: Go's eiselLemire64 gives up on every subnormal result, so
ParseFloat takes its 800 digit big decimal path (decimal.go: dozens of
shifts by 2^27 over hundreds of digits). Ported to Xo with the digits
in a List[Int] field of a struct, every digit access is a bounds checked
load or store through the struct, about five times Go's cost.

Fix: num.xo's eisel_lemire rounds a subnormal result itself (`subnormal`:
shift the 54 bit mantissa right by the missing binades and round half
up; fall back only when every product bit below the round bit is one).
An exact tie cannot occur for a mantissa of at most 19 digits, so half
up is right. Subnormal texts then took 195 million instructions (0.33x
of the C); TestNumDiff checks the answers against Go on random
subnormals, their halfway points, and 19 digit texts near 1e-330. The
big decimal remains for texts of more than 19 digits that Eisel-Lemire
cannot decide.
