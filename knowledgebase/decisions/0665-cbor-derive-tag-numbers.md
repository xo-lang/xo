# 0665: derive Cbor(tag: N), a CBOR tag number for a derived type (fxamacker's TagSet)

Status: Accepted (owner, 2026-11-15, on the lead's recommendation; proposed by xo-lang-3)

## Context

fxamacker/cbor registers a tag number for a Go type with `TagSet.Add(TagOptions{EncTag, DecTag}, type, num)` and
an encoding or decoding mode built `WithTags`: the type's values are written inside that tag (COSE_Sign1 is tag 18,
a CWT tag 61, WebAuthn's attestation formats and many IoT schemas register their own), and decoding checks it
(`WrongTagError`: `cbor: wrong tag number for T, got 7, expected 18`). These are 11 of the stdaudit gaps of
fxamacker/cbor (decision 0655): TagSet, NewTagSet, TagOptions, the four EncModeWith(Shared)Tags and
UserBufferEncModeWith(Shared)Tags, DecModeWith(Shared)Tags, WrongTagError and its Error. derive Cbor (0527) treats
tags as transparent on decode and writes none. A program can wrap a `Value` in `Value.Tag` by hand, but not a
derived struct, whose `marshal` writes directly (0559).

Go registers tags at run time on a mode, so one type can carry different tags under different modes. Xo's derive
fixes each field's mapping at compile time (0527: no Go type mapping options).

## Decision

1. `derive Cbor(tag: N)` on a struct or enum, N an unsigned integer literal (0 to 2^64 - 1, not 0 to 3 or 55799,
   whose meanings are fixed: `XO0422` with the reason). `marshal` writes the value as `N(value)` wherever it appears
   (top level, a field, a list element), as EncTagRequired.
2. `derive Cbor(tag: N, tag_rule: optional | required | ignored)` sets decoding as DecTagMode: `optional` (the
   default, DecTagOptional) accepts the value with tag N or without a tag; `required` (DecTagRequired) refuses it
   untagged (`CborErr.Type`, Go's `cbor: cannot unmarshal <type> into Go value of type T (expect CBOR tag value)`);
   `ignored` (DecTagIgnored) skips any tags. Under `optional` and `required` another tag number is
   `CborErr.Type` with WrongTagError's text, T the Xo type name.
3. The tag goes around the value after `toarray` and enum mapping; `std/cbor.Value.Tag` stays the way to tag a
   `Value`. No run time registry (TagSet, NewTagSet, the WithTags modes and shared tags map to the derive option).
4. The checker's derive table takes `tag` and `tag_rule` (spelled as `case:` is); the generated `cbor_wr__T` writes
   the head with `cbor.put_head(out, 6, N)` and `cbor_rd__T` checks it through a new std/cbor helper
   `want_tag(v, n, rule, type_name, var st) -> (Bool, Value)` (std/native/cbor.xo, beside `untag`).

## Consequences

- A COSE or CWT structure is a struct with `derive Cbor(tag: 18)`; its bytes match fxamacker's under
  `TagSet.Add(TagOptions{EncTag: EncTagRequired, DecTag: DecTagRequired}, reflect.TypeOf(T{}), 18)`.
- One type has one tag. A program that needs a type tagged differently in two places wraps a `Value`, or declares a
  second type. Go's per mode registries have no Xo form (recorded on the 11 rows).
- compiler/check/derive.xo and the derive Cbor expansion change, so the push carries selfbuild and seedbuild, and
  the cbor goldens (cbor_tag) run on arm64 and LLVM; the oracle corpus gains tagged struct cases against
  fxamacker's TagSet.

## Alternatives

- A run time registry (`cbor.tags().add(...)` passed to `marshal_with`): the generated code would look up every
  struct's tag at run time, which 0559's direct writer avoids, and it brings back Go's per mode options 0527 left out.
- A field level tag (`cbor "x,tag=18"`): fxamacker registers types, not fields, and a type tagged in one field and
  not another is the inconsistency TagSet exists to prevent.
- Leaving the rows as gaps: COSE (RFC 9052) and CWT (RFC 8392) need tagged structures, and std/jwt's COSE work
  would build them by hand.
