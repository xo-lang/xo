# A cache codec written for one pass drops fields a later pass sets

Status: Observed (2026-10-18)

## Symptom

With the inliner's per-function cache (decision 0220 addendum stage D
2a), `XO_BACKEND_CACHE=verify` failed in internal/arm64
TestBackendCacheModuleFacts ("35 functions' inlining differs from their
cache entries, the first cabi.fiber_map_failed") after a libc build and
then a no-libc build shared one cache. The darwin binaries and the
assembly were the same with the cache on and off, so the comparison of
darwin builds alone passed.

## Cause

The inliner's entries reuse the codec of the reference counting cache
(compiler/ssa rccache.xo encode_func, decode_func), which was written
for functions after insert_rc. Two fields are set only later, by the
arm64 inliner: each value's inline site (Value.inl) and the function's
inline sites (Func.inl_sites), the DWARF inline records (decision
0252). The codec dropped both. A function read back lost them, and so
did every caller that copied its body: the code was the same, the DWARF
of a linux/arm64 ELF was not (a darwin build has no DWARF in the
binary, so its comparison could not see it).

## Fix

The codec writes and reads both (rc cache v6). TestBackendCacheEditIdentity
also builds linux/arm64 and compares the ELF files, and fails with the
old codec. When a cache's entries start holding a later stage's
functions, check the codec against every field of ssa.Func and
ssa.Value, and compare a build whose binary carries the debug
information.
