# A dudect under `xo test --backend=llvm` times -O0 code, which leaks on Apple silicon

Status: Observed 2026-11-12 (Xo Lang 3, the 0616 RSA review's F1).

## Symptom

std/native/llrt/rsa_test.xo's dudect of `pkcs1_unpad` and `oaep_unpad`
passed natively (max |t| 2.3) and failed on LLVM: v15_nozero against
v15_short |t| 81, and two sets of blocks of the same class (v15_short
against v15_short) |t| 5.7. The LLVM test binary ran `pkcs1_unpad` in
about 700 ns a call against 106 ns natively.

## Cause

`xo test` builds an LLVM test binary at -O0 (decision 0236, compiler/xo
main.xo `tests && !split.opt_set`). xo's IR for the function has no
branch on the data (only the bounds checks and the loop), but at -O0 the
loop keeps `looking` and `index` on the stack and reloads them every
iteration, and on Apple silicon that code's time followed where the
block's first zero byte was (plausibly the load value predictor: the
same class of effect as the SLAP attack). It is not the code programs
ship: `xo build` uses -O2, where the values stay in registers.

## Fix

Fixed by decision 0619 (2026-11-12): std's constant time code runs with
Arm's DIT bit set where the CPU has it, and its modules compile at -O2 in
every -O0 LLVM build (split.xo ct_split), so a timing test of std's code
no longer needs XO_LLVM_OPT=O2. What follows was the workaround, and
still applies to timing a constant time function outside those modules:

Run a timing test on LLVM with `XO_LLVM_OPT=O2` (the opt level is then
set, so the test build uses it): every pair passes (max |t| 1.7), the
early exit control is still seen (|t| 246), and the classes' mean times
are equal (62 ns). The A/A pairs (the same class on both sides) in
rsa_test.xo show a harness fault like this one: keep them. Check the
assembly the program ships (`xo build --backend=llvm`, `otool -tV`) when
a backend disagrees with another.
