# 0468: std/cbor, RFC 8949 CBOR encode and decode

Status: Proposed (Xo Lang 3, 2026-11-03; roadmap priority 1 after std/jpeg, from the Xo Lead)

## Context

CBOR (RFC 8949) is the binary JSON of constrained devices, COSE/WebAuthn/CWT and several
protocols a backend meets (passkey attestation, CWT tokens, IoT). Go has no CBOR in its
standard library; the community package is fxamacker/cbor. The roadmap (std-coverage) lists
CBOR as the first gap.

## Decision (proposed)

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

To be filled in with the implementation.
