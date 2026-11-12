# 0619: Arm's DIT bit around std's constant time code

Status: Proposed (stub, 2026-11-12, Xo Lang 3; the lead approved the
plan the same day)

## Context

The decision 0616 RSA review's dudect of rsa.pkcs1_unpad and oaep_unpad
alone: native arm64 and LLVM -O2 pass (max |t| 2.3, 1.7), but the LLVM
-O0 test build leaks on Apple silicon (|t| 81) with no data dependent
branch or select in its code (knowledgebase/gotchas/dudect-o0-llvm.md).
A C replica: -O0 |t| 23.7 without DIT, 1.08 with PSTATE.DIT set; DIT
costs +25% (-O0) and +11% (-O2) in that loop.

## Decision

To be written: an rt intrinsic that sets DIT for a non-yielding constant
time section on every arm64 build (native and LLVM, every opt level), its
scheduler interaction, the amd64 limit (no user mode DOITM) with the -O2
llrt fallback, dudect rows (-O0, -O2, native, amd64 host) and --ab cost
on host 1.
