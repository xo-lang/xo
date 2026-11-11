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

## Addendum: a module generated into rt (2026-11-10, std/multipart)

A merged module listed in std/rt_hosted.txt (decision 0597) has its Go in
std/rt, but the runtime generic types' XoEq and XoHash (decision 0551)
are still kept per program, so its comparisons must root them in every
program that uses it. `multipart.write` compares an `Option[Str]`, and
the Go corpus faulted `XoEq not generated for rt.Option[string]`. Two
walks cover it: `proto_call_roots` scans such a module (`is_std_mod`
is true for it, so the skip names `rt_hosted`), and gen.xo
`rt_hosted_roots` generates its bodies once more with the output dropped,
for the comparison roots that only generation records.

