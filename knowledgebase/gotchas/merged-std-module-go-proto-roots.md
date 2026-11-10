# A merged std module's Go code faults "XoEq not generated"

Observed (2026-11-10, Lang 3, merging std/jwt under decision 0588).

## Symptom

A std module merged with its implementation passes on arm64 and LLVM
but its corpus fails on the Go backend:

```
fault: xo internal: XoEq not generated for x_std_jwt.Alg (decision 0542; XO_GO_PROTO=all writes every one) at std/jwt.xo:194
```

## Cause

Decision 0542 writes a type's XoEq and XoHash only when a whole program
walk from their roots reaches it. compiler/gen/proto.xo
`proto_call_roots` skipped every std file, because std's Go used to be
pregenerated in std/rt with every method. A merged module is generated
with the program as its own package, so a call in its own body (here
`List.contains` over `Alg`) is a root nobody recorded.

## Fix

`proto_call_roots` scans merged modules too (`is_std && !merged` is
skipped). When a merged module's Go run faults on a missing protocol
method, `XO_GO_PROTO=all` confirms the cause; look for another walk
that skips `is_std` without the `merged` exception.
