# A Go oracle run on arm64 fuses a*b+c, so its float answers are not portable

Status: Observed (2026-11-16, task #116)

## Symptom

TestRandomFuzz seed 3: std/random's Zipf draw 35 for s=1.08, v=11.39
gave 37364334090267 where the corpus (Go's math/rand/v2 Zipf, generated on
the arm64 Mac) wanted 37364334090268. Every input to the step was
bit-identical; `ur := hxm + r*hx0minusHxm` differed by one ulp, and
`floor(x + 0.5)` fell on the other side.

## Cause

The Go spec lets a compiler fuse `x*y + z` into one fused multiply-add,
and Go does on arm64 (also ppc64, riscv64, s390x), not on amd64. A
generator that calls Go's library on arm64 records the fused result. Xo's
floats are strict (decision 0287), like Go on amd64, so Xo is right and
the reference is platform-dependent. It shows only where a result sits
on a rounding boundary, so it can pass thousands of cases first.

## Fix

Compute the reference with each product rounded on its own:
`float64(a*b)` blocks fusion (Go spec, Floating-point operators). For a
library function, copy its code into the generator with those
conversions (scripts/stdcorpus/random.go zipfStrict) and keep the
committed corpus byte-identical to prove the copy is faithful. For a
whole library (a module oracle such as scripts/stdcorpus/brotlioracle),
build it with fusion off: `go run -gcflags=all=-d=fmahash=n .` (the
compiler's FMA bisect switch; "n" matches no site, so nothing fuses, and
it prints "fmahash triggered [DISABLED]" lines on stderr). TestBrotliFuzz
seed 182 (2026-11-16): a quality 10 stream was 5203 bytes from Go on
arm64 and 5202 from Go on amd64, from Go on arm64 with fusion off and
from Xo, all three identical; the encoder's float cost model chose other
blocks. Guarding the generator's own products is not enough when it calls
Go's math: on arm64 math.Exp is assembly and math.Log is pure Go that
fuses, so random fuzz seed 17 (2026-11-16) drew one more than Xo even
after zipfStrict; the generator now calls strictExp and strictLog, the
guarded copies std/rt/gomath_gen.go has. Seed 87 then found the same in
math/rand/v2's NormFloat64 (Go's own code, its tables unexported), so
since 2026-11-16 scripts/stdcorpus runs itself again with fusion off
(noFMA in main.go: GOFLAGS gets `-gcflags=all=-d=fmahash=n`, so the flag
reaches every oracle module a generator starts with its own `go run`;
the compiler's notices are filtered): every generator and the Go code
it calls compute unfused, and a run built with fusion anyway (a fused
x*y+z detected) writes no corpus. Every committed corpus was unchanged
under the flag. scripts/stdcorpus/nofma_test.go keeps the rule
(agent-rules.md, #147). A Go oracle outside scripts/stdcorpus needs the
same flag. See also
go-backend-fma-contraction.md and go-backend-fused-float.md (the same
fusion in the Go backend's output).
