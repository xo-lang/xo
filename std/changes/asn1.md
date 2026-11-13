# std/asn1 changes

## 2026-11-13: security: parse reads nested values by offset, linear on every backend

- Kind: security
- Commit: this commit
- Found by: the second reader of the decision 0616 x509 review (asn1's depth on wasm), then a probe
- Go reference: encoding/asn1 parseField works on offsets into one slice (Go slices share memory)
- Affected: Xo 0.50.1 and earlier on the WebAssembly and Go backends, which copy a slice (spec/core.md 2.2, decision 0330); the native backends' slices are views
- Test: std/asn1_test.xo on arm64, llvm and wasm (the depth test timed out on wasm before); corpora asn1, x509, x509create, pkcs12 on go, arm64, llvm
- Timing: on wasm (node), 20000 nested SEQUENCEs: 2201 ms before, 0 ms after; 200000 (800 KB): out of memory before, 0 ms after
- Shipped in: not yet released

parse and parse_prefix sliced the contents of each nested value before reading it, which on the WebAssembly and Go backends copies the rest of the input at each level: memory and time quadratic in the depth, up to the 10000 level cap. The reader now passes offsets into the one input and slices only a leaf's contents.

## 2026-11-13: security: nesting deeper than 10000 levels is a structure error

- Kind: security
- Commit: this commit
- Found by: the decision 0616 security review of std/x509 (Go advisory sweep: GO-2026-5972)
- Go reference: encoding/asn1 parseField's maxDecodeDepth (10000)
- Affected: Xo 0.50.1 and earlier, every backend (std/asn1 is Xo code)
- Test: std/asn1_test.xo (9999 levels parse; 10000 and 200000 are `asn1: structure error: nesting depth exceeded`; fails without the cap)
- Timing: none (one comparison per value)
- Shipped in: not yet released

`parse` and `parse_prefix` read SEQUENCE and SET values recursively with no depth limit, so a small input of deeply nested headers could exhaust the task's stack. They now stop at Go's limit of 10000 levels with `asn1: structure error: nesting depth exceeded`.

## 2026-11-11: feature (Breaking): WideTime for times outside 1678 to 2261

- Kind: feature
- Commit: this push
- Found by: decision 0614 (the lead's report: GeneralizedTime years such as 0001 and 9999 were `asn1: time out of range`)
- Go reference: encoding/asn1 Unmarshal and Marshal of time.Time for years 0 to 9999 (go1.26.1)
- Test: corpus asn1 regenerated (three random rows now WideTime, seven edge cases); golden time_civil_codecs; std/asn1_test.xo
- Timing: none (only the out of range branch changed)
- Shipped in: not yet released

`parse` returns `Value.WideTime(c, generalized)` where it returned the error; `der` writes it in UTC as Go's Marshal.
Migration: an exhaustive `match` on `asn1.Value` adds a `WideTime(c, g)` arm.

## 2026-11-11: port: std/asn1, Go's encoding/asn1 as a DER value tree (decision 0601 stage 1)

- Kind: port
- Commit: this push
- Found by: std audit 2026-11-11 (bench/results/std-audit-2026-11-11.md, rank 5: no public ASN.1 API)
- Go reference: encoding/asn1 (asn1.go, marshal.go of go1.26.1)
- Test: TestStdCorpora asn1 on go, arm64, llvm (1305 cases against Go's Unmarshal and Marshal); std/asn1_test.xo
- Shipped in: not yet released

A new module: parse, parse_prefix, Value.der, oid, bit_at, right_align
and tagged_as over a Value tree (spec/std.md 78) with Go's DER rules and
error texts. Not ported: Marshal and Unmarshal of structs by field tags
(derive Asn1 is decision 0601 stage 2).
