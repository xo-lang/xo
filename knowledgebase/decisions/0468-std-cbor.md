# 0468: std/cbor, RFC 8949 CBOR encode and decode

Status: Proposed (implemented 2026-11-03 for owner review; roadmap priority 1, assigned by the lead)

## Context

CBOR (RFC 8949) is the binary JSON of constrained devices, COSE/WebAuthn/CWT and several
protocols a backend meets (passkey attestation, CWT tokens, IoT). Go has no CBOR in its
standard library; the community package is fxamacker/cbor. The roadmap (std-coverage) lists
CBOR as the first gap.

## Decision

A new module `std/cbor`, written once in Xo (std/native/cbor.xo, go_compiled like std/png),
over a `Value` tree that holds the whole CBOR data model: unsigned and negative integers to
64 bits, byte and text strings, arrays, maps with any key, tags, simple values, and floats;
`encode` (preferred serialization, with `deterministic` for RFC 8949 4.2.1 key order),
`decode` and `decode_prefix` (definite and indefinite lengths, a depth limit, an element
limit, UTF-8 checked, duplicate keys optionally refused, trailing bytes refused),
`diagnostic` (RFC 8949 section 8 notation) and the JSON bridge of section 6.1 in both
directions. Typed encoding goes through `derive Json` (`encode_json[T: Json]`,
`decode_json[T: Json]`), so no new derive is needed.

Oracle: no Go standard library CBOR exists, so the corpus comes from RFC 8949 Appendix A (its
vectors with their diagnostic notation) and from fxamacker/cbor v2 (MIT) run in a scratch
module under internal/stdtest/cborref with its own go.mod (the main module keeps zero
dependencies); the fuzz differential compares accept or reject and the canonical re-encoding.

## Consequences

- The API is spec/std.md 50. The model is the data model, not the bytes: a decoded
  indefinite length item is a definite value, a float is a `Float` whichever width it
  came in, and `encode` writes the preferred serialization, so `encode(decode(x))` is the
  preferred form of x, not x. Programs that must keep exact bytes keep the bytes.
- Typed encoding is not a derive: `to_json` and `from_json` let `derive Json` types go
  through `json.to_value` and `json.from_value`; a CBOR specific derive (byte strings,
  integer keys, tags) is a later decision if programs need it.
- Oracle and tests: no CBOR exists in Go's standard library, so the oracle is fxamacker/cbor
  v2.9.0 (MIT) in its own module (internal/stdtest/cborref, `go run .`), keeping the main module
  free of dependencies. The committed corpora (RFC 8949 Appendix A with curated malformed
  inputs, and a 1,500 case byte level fuzz corpus) need no network; the "current" test and
  fresh seeds skip when the oracle cannot run. The differential found, and the module now
  follows, these oracle behaviours: Diagnose prints a bignum as its number; a NaN map key is
  never a repeated key; text is checked as UTF-8 per chunk. Tags 0 to 3 (time and bignum
  content checks), 55799, undefined and NaN are left out of the oracle's re-encoding
  comparison because its interface decode changes them.
- Timing (host 1, 20 KB document of 150 records, 100 passes, against fxamacker's decode into
  `any` and its marshal): decode 0.43, encode 0.26, deterministic encode 0.62 of its wall.
  Proven (decision 0461), though the twin builds a generic tree in both, not typed structs.
- Limits: nothing is allocated from a length (a count is checked against the bytes left),
  nesting is bounded by `max_depth` (128), and `encode` stops at 1,000 levels.

## Alternatives

- A typed codec with a `derive Cbor`: needs checker work and field attributes (byte string,
  key number, tag); the Value tree plus the JSON bridge serves the protocols named above and
  does not block a derive later.
- Keeping indefinite lengths and float widths in the model, for exact round trips: rejected,
  it makes every consumer handle forms that mean the same thing; `well_formed` and
  `decode_prefix` cover validation and sequences.
