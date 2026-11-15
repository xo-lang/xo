# std/asn1 changes

## 2026-11-15: feature: derive Asn1, marshal and unmarshal (decision 0661)

- Kind: feature
- Commit: this commit
- Found by: the std audit (decision 0655 features line): Marshal, Unmarshal, MarshalWithParams, UnmarshalWithParams and RawContent were gaps
- Go reference: encoding/asn1 Marshal, Unmarshal, getUniversalType, parseField, makeField (go1.26)
- Test: testdata/corpus/asn1fields (1035 cases over five structs, hand-written and derived, go, arm64, llvm); test/errorcheck/derive_asn1

`asn1.marshal[T]`, `asn1.unmarshal[T]` and `asn1.unmarshal_prefix[T]`
over a struct that derives Asn1, each with Go's params text. New field
types: `Enumerated` (Go's asn1.Enumerated, 32 bits as Go reads it),
`RawContent`, and `time.Civil` for times of any year (`to_civil`,
`civil_value`); a `Time` field's out of range error now names
`time.Civil`.

## 2026-11-15: feature: struct fields: FieldReader, Field and with_field (decision 0661)

- Kind: feature
- Commit: this commit
- Found by: the std audit (decision 0655 features line): asn1.RawContent, MarshalWithParams and UnmarshalWithParams were gaps
- Go reference: encoding/asn1 parseField, parseSequenceOf, parseFieldParameters, makeField and makeBody (go1.26)
- Test: testdata/corpus/asn1fields (430 cases, go, arm64, llvm); std/asn1_test.xo

`field(params, kind)` reads Go's struct tag options; `FieldReader.next`
reads one field of a SEQUENCE as Go's parseField does (explicit and
implicit tags, application and private classes, optional fields and
defaults, every string type into a string field, both time types into a
time field, SET OF and SEQUENCE OF through `open_list`, RawValue as
KIND_ANY, RawContent through `last`), and `Value.with_field` writes a
field as makeField does. `BitString` is the field type for a BIT STRING.
These are what `derive Asn1` will generate calls to (decision 0661,
Proposed); they work by hand today.

## 2026-11-15: fix: a PrintableString with `*` encodes, as Go's Marshal

- Kind: fix
- Commit: this commit
- Found by: decision 0661's corpus work (makePrintableString allows `*`)
- Go reference: encoding/asn1 makePrintableString (allowAsterisk, rejectAmpersand)
- Test: std/asn1_test.xo "printable allows * when asked"

`Value.Printable(s).der()` refused `*`; Go's Marshal with `printable`
writes it (certificates use `*.example.com`) and refuses only `&`.

## 2026-11-15: feature: `Asn1Err` is an open enum (decision 0649)

- Kind: feature
- Commit: this commit
- Found by: decision 0649 (a new variant of a std error enum broke two SDKs, 0644's `ClientErr.Tls`)
- Go reference: none (Go's errors are values compared with errors.Is; Rust's `#[non_exhaustive]`)
- Test: compiler/check's open enum test; the open_enum golden; xo check over the programs that use the module

`Asn1Err` is declared `open`: a `match` on it outside std needs a `_` (or
binding) arm, warning XO0306 in 0.52 and an error after, so that std can
add a variant in a minor release without breaking a build. Matches that
already have `_` are unchanged.

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
