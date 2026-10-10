# Branches in a constant time review that are not the code's

Status: Observed (2026-10-10, decision 0132).

## Symptom

Listing the conditional branches of `third_party/mbedtls/xo/xo_p256.c`'s
assembly (to check that no branch depends on a secret) showed one
`b.ne` in each of the branch free helpers (`pt_add`, `pt_double`,
`fe_cneg`, `limbs_reduce`, ...), which looked like secret dependent
branches.

## Cause

Apple clang turns on the stack protector by default: each function with
a local array compares the stack canary before returning and branches to
`__stack_chk_fail`. The branch depends on the canary, not on the data.
With inlining on (the real build), most helpers disappear into their
callers, so their branches cannot be attributed by function name either.

## Fix

Review the assembly twice: once with `-O2 -fno-inline
-fno-stack-protector` to attribute every conditional branch to a function
(the helpers must have none; loops may branch on their counters and the
public window index), and once as built (`-O2`, arm64 and
`-target x86_64-apple-macos12`) to see that inlining added none. The
review's results are in decision 0132.

A related trap from the same tests: a P-256 ECDSA hash longer than 32
bytes is truncated to its leftmost 32, so changing its last byte does
not change the signature's validity. Tests that alter a hash must
change a byte among the first 32.
