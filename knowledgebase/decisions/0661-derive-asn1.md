# 0661: derive Asn1, typed DER over std/asn1 (decision 0601 stage 2)

Status: Proposed (Xo Lang 3, 2026-11-15). Owner, 2026-11-15: direction approved (finishes 0601's "derive Asn1
later", on the derive Xml/Cbor pattern); the std side lands first, and the compiler part lands after this ADR is
filled in and the lead has reviewed it, which accepts it.

## Context

std/asn1 (decision 0601 stage 1) has a DER `Value` tree with Go's encoding/asn1 rules, checked by a 1305 case corpus.
It has no typed API: a program reading a PKCS#8 key, a CMS or timestamp structure, an LDAP or SNMP message or an
X.509 extension std/x509 does not decode walks a `Value` by hand, keeping track of optional fields, explicit and
implicit tags and defaults itself. Go's answer is `Unmarshal` and `Marshal` over structs with `asn1:"..."` tags,
by reflection. Xo has no reflection; its typed formats are derives (Json 0277, Xml 0315, Yaml 0512, Cbor 0527) that
generate direct code. The std audit (decision 0655) lists asn1.RawContent, MarshalWithParams and UnmarshalWithParams
as gaps of this stage; 0601 deferred it until stage 1's corpus was green and the tree settled (both done).

## Decision

1. **std side (lands first, usable by hand).** std/asn1 gains what the generated code calls, each one Go's
   function in miniature:
   - `Field` (a field's kind and options) and `field(params, kind)`, Go's parseFieldParameters over the same tag
     text (`optional`, `explicit`, `application`, `private`, `tag:N`, `default:N`, `set`, `omitempty`, `utf8`, `ia5`,
     `printable`, `numeric`, `utc`, `generalized`; unknown parts ignored, as Go).
   - `FieldReader` with `next(f) -> Value? ! Asn1Err`, Go's parseField for one field of a SEQUENCE's contents
     (asn1.go:710): its error texts and its order of checks, including the quirks (an explicit field's header is
     read before its tag is compared; the inner header of an explicit tag is read up to the end of the enclosing
     contents, not the tag's own length; a zero length explicit tag is an error since Go's `Flag` is not ported);
     `open` and `open_list` (Go's parseSequenceOf with its first pass tag check); `last` (RawContent); `rest`.
   - Conversions `to_bool`, `to_int` (`integer too large`), `to_big`, `to_bytes`, `to_oid`, `to_bit_string`,
     `to_str`, `to_time`; writers `int_value`, `bit_string_value`, `str_value(s, string_type)` (Printable when
     every byte is, else UTF8, as makeField), `time_value(t, time_type)` (UTCTime unless `generalized` or outside
     1950 to 2049), `raw_struct(full)` (Marshal of a struct from its RawContent) and `Value.with_field(f)` (makeField:
     `set`, then an explicit or implicit tag in the field's class).
   - `BitString { bytes, bit_len }` with `at` and `right_align`, the field type for a BIT STRING.
2. **The derive (after review).** A derivable `Asn1` (kind Marked, marker, format `asn1`, like `Cbor`), with
   `interface Asn1 {}` in the prelude. A field's options are the string tag `asn1 "optional,explicit,tag:0"`, Go's
   tag text unchanged; an unknown option is a compile error (Go ignores it; a typo would otherwise change the
   encoding silently). Generated per struct: a reader (`FieldReader.next` per field in order, then the conversion)
   and a writer (a `Value.Sequence` of each field's value under `with_field`). API in std/asn1, compiled as calls of
   the generated functions, as `cbor.marshal` is:
   ```
   fn marshal[T: Asn1](v: T, params: Str = "") -> Bytes ! Asn1Err          // Marshal and MarshalWithParams
   fn unmarshal[T: Asn1](der: Bytes, params: Str = "") -> T ! Asn1Err     // the whole input: `trailing data`
   fn unmarshal_prefix[T: Asn1](der: Bytes, params: Str = "") -> (T, Bytes) ! Asn1Err   // Unmarshal and its rest
   ```
   The trailing data check runs after the struct's fields are read, so a field's error comes first, as in Go (the
   corpus caught the other order).
3. **The mapping** (Go's getUniversalType): `Bool` BOOLEAN; `Int` INTEGER (64 bit, `integer too large`);
   `big.BigInt` INTEGER of any size; `asn1.BitString` BIT STRING; `Bytes` OCTET STRING; `asn1.Oid` OBJECT
   IDENTIFIER; `Str` any string type read, written Printable or UTF8 or as the option says; `Time` UTCTime or
   GeneralizedTime (a time outside 1678 to 2261 is an error, where Go reads it: decision 0614's WideTime has no
   place in a `Time` field); a struct deriving Asn1 a SEQUENCE (`set`: SET, in field order as Go); `List[T]`
   SEQUENCE OF (`set`: SET OF, sorted as DER); `asn1.Value` any value as written (Go's RawValue: `Raw` with its
   class, tag and contents); a first field of type `asn1.RawContent` (a struct holding `Bytes`) receives the
   struct's whole encoding and, when not empty, is what Marshal writes.
4. **Optional fields.** `Option[T]` is the Xo form: None is omitted and an absent element reads as None. A plain
   field marked `optional` keeps Go's rule for the types with an obvious zero (`Bool`, `Int`, `Str`, `Bytes`,
   `List`): the zero value (or the `default:N` integer) is omitted on Marshal and an absent element reads as it.
   An `optional` field of another type must be an `Option` (a compile error otherwise), since Go's DeepEqual to a
   zero struct or a zero time has no Xo counterpart.
5. **Out:** `asn1.Flag` (a marker for reflection; a zero length explicit tag stays an error), `Enumerated` as a
   field type (read it as `asn1.Value`; KIND_ENUM exists in the helpers for a later newtype), slice types named
   `...SET` (Xo uses the `set` option), Go's unexported field error (Xo derives see every field), and `tags don't
   match`'s parameter dump (Go prints its %+v parameters, pointers included: the text ends after the tag and length).
6. **Spec:** spec/core.md 2.9 (the derivable table) and spec/std.md 78 gain the derive and the three functions;
   `TestDerivableTable` and the checker's tables take the entry; an `asn1 "..."` tag on a struct without `derive
   Asn1` is a warning (the 0277 rule).

## Consequences

- Typed DER without a hand written walk; PKCS, CMS, LDAP and SNMP structures port from Go with their tags copied.
- Verification is the oracle corpus `testdata/corpus/asn1fields` (in place with the std side: 430 cases over four
  struct types using every tag option, each Unmarshal result and Go's Marshal of it compared on go, arm64 and llvm).
  Its four structs are read and written by hand-written code shaped as the generated code; when the derive lands
  the corpus switches those structs to `derive Asn1` with the same expectations, so both forms stay proven.
- The std helpers are public API even if the derive were never accepted; they stand alone as Go's field rules for
  hand-written readers, which is why the std side lands first.
- Cost: a derive in compiler/check (derive.xo, derivables.xo, call.xo's std path list) about the size of Cbor's,
  a closure push (selfbuild, seedbuild) and a prelude interface (stdsync).

## Alternatives

- **Only the helpers, no derive.** Lands today and needs no language change; every user then writes the reader and
  writer by hand (about 30 lines a struct in the corpus), the boilerplate derives exist to remove.
- **Field options as separate attributes** (`optional`, `tag(0)`, `explicit` keywords on the field) instead of Go's
  tag text: easier to check, but every port retypes Go's tags; the Xml and Cbor derives took the tag text (0315,
  0527) and this follows them.
- **A schema interpreted at run time** (a `List[Field]` per struct walked by one function): a smaller compiler
  part, but every field read goes through an interpreter and each struct still needs a constructor; derives
  generate direct code for that reason (0559).
