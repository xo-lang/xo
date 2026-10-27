# Poly1305's 64 bit limbs are slower on Apple arm64 than donna's 26 bit ones

Status: Observed (2026-10-27, xo-lang-4)

**Symptom.** Porting Go's 64 bit limb Poly1305 (sum_generic.go: four 64 by
64 bit products a block, umulh for the high halves) made it 0.656 of its
time on an i7-9750H, so it was made the portable code. On the M-series
Mac, `xo bench --ab` (15 rounds, the chacha twin's poly workload) read
1.174 (1.168 to 1.18) times the wall time, with 0.667 of the instructions.

**Cause.** Each 64 bit block is one chain: the products feed the carries,
which feed the reduction, which feeds the next block. Donna's 26 bit limbs
(two blocks a step, r and r squared) do more multiplies, but independent
ones, which the wide Apple cores run side by side. Fewer instructions is
not less time.

**Fix.** The 64 bit limbs run on amd64 only (std/native/llrt/chacha.xo:
seal64 and open64 with cpu.poly_blocks there; Poly26, seal26 and open26,
donna's code, on every other target). The general rule: measure a
"fewer instructions" port with `--ab` on each architecture it runs on
before making it the shared portable code.
