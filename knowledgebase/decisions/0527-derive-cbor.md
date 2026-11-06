# 0527: derive Cbor, typed CBOR values (cbor.marshal and cbor.unmarshal)

Status: Accepted (owner, 2026-11-05)

## Context

std/cbor (decision 0468) has the RFC 8949 `Value` tree: `decode`, `encode` (preferred and deterministic), diagnostic
notation and a JSON bridge. It has no typed API: a program that receives a CBOR message (WebAuthn and COSE, IoT payloads,
CTAP, the Azure and AWS services that speak CBOR) walks a `Value` by hand. Go's reference is `fxamacker/cbor/v2`
(`Marshal` and `Unmarshal` over structs with `cbor:"name,omitempty"` tags), the library std/cbor's oracle already uses.
Xo's typed formats go through a derive: `derive Json` (0277's wire names), `derive Xml` (0315) and `derive Yaml` (0512,
field tags `yaml "name,omitempty"`). CBOR is the next format with a struct mapping.

## Decision

1. A new derivable `Cbor` (kind Marked, marker, format `cbor`, like `Yaml`): `derive Cbor` on a struct or enum whose fields
   are types the mapping below covers. `derive Cbor(case: snake | camel | pascal | kebab)` and `field as "key"` follow 0277
   (a field's own wire name wins). A field string tag `cbor "name,omitempty"` carries fxamacker's options, with its
   spelling: `omitempty`, `omitzero`, `keyasint` (the key is the decimal integer of the name), `toarray` (a struct as an
   array in field order, a tag on the struct's first field `cbor ",toarray"`), `-` to skip. The default key is the field's
   wire name (Xo's snake_case field name unless `as` or `case:`; fxamacker's Go names are capitalized, which Xo does not
   copy, as for Json).
2. API in std/cbor, compiled as calls of the functions `derive Cbor` generates, as `yaml.decode` is:
   ```
   fn marshal[T: Cbor](v: T, deterministic: Bool = true) -> Bytes ! CborErr
   fn unmarshal[T: Cbor](data: Bytes, strict: Bool = false, max_depth: Int = 128) -> T ! CborErr
   fn unmarshal_prefix[T: Cbor](data: Bytes, strict: Bool = false) -> (T, Int) ! CborErr
   ```
   `marshal` sorts map keys by the bytewise order of their encodings (fxamacker's `SortBytewiseLexical`/`CoreDeterministic`
   without the other options), because Xo's `Map` has no random order to copy and a program that wants insertion order
   builds a `Value`. `strict` is fxamacker's `ExtraDecErrorUnknownField`.
3. The mapping (fxamacker's defaults unless stated): `Bool` as true/false; `Int` as the smallest integer head (major 0 or
   1), decoded from either with an overflow error; `Float` as a float64 head (not shortened: `ShortestFloatNone`); `Float32`
   as float32; `Str` as text; `Bytes` as a byte string; `List[T]` as an array; `Map[Str, V]` and `Map[Int, V]` as a map;
   `Option[T]` as the value or null (decoding null or undefined into an `Option` gives `None`, into a plain field leaves
   the zero value); `Time` as an integer of Unix seconds with fractional seconds as a float (fxamacker's `TimeUnix` and
   `TimeUnixMicro` default is recorded against the oracle and may change this line); `Duration` as an integer of
   nanoseconds; `cbor.Value` and `cbor.Raw` (the undecoded item) as themselves; an enum as fxamacker would map a Go sum
   (a one entry map from the variant name to its fields, as `derive Json` writes it); nested structs that derive Cbor.
4. Decoding follows `Unmarshal`: the first type mismatch is remembered and decoding goes on (so the whole item is
   consumed), then `UnmarshalTypeError` text is returned as `CborErr.Type(path, got, want)`; unknown keys are skipped
   unless `strict`; duplicate map keys in a struct are the last wins unless the oracle shows otherwise; tags are
   transparent (a tagged item decodes as its content) except 0/1 for `Time` and 2/3 for bignums into `Int` when they fit.
5. A new error variant `CborErr.Type` is a new case in a public enum: callers that match every variant of `CborErr` get a
   compile error until they add it (a breaking change, recorded in std/changes/cbor.md as 0467 did for jpeg).
6. The spec change: spec/core.md 2.9 (the derivable table) and std.md 50 gain the derive and the three functions;
   `TestDerivableTable` and the checker's derive tables take the entry; a `cbor "..."` tag on a struct without `derive
   Cbor` is a warning (the 0277 rule).

## Consequences

- Typed CBOR without a hand written `Value` walk. The generated code runs on every backend; the Go backend calls the same
  generated Xo.
- Verification is the oracle corpus: `scripts/stdcorpus/cbor.go`'s fxamacker oracle gains typed cases (struct, tags,
  omitempty, keyasint, toarray, options, errors) compared byte for byte on `marshal` and by value and error text on
  `unmarshal`, plus a fuzz corpus of generated documents against fxamacker. A timing row against fxamacker's
  `Marshal`/`Unmarshal` in bench/perf/cbor (twincheck) on the same typed structs; Go's fast paths (cached struct info,
  no reflection in the loop, preallocated slices) are ported as direct generated code.
- Cost: a derive implementation in the checker (about the size of `derive Yaml`'s decoder plus an encoder) and a decision
  for every fxamacker option; options that Xo's types make meaningless (`json.Marshaler` hooks, `Unmarshaler`) are out.

## Alternatives

- Typed values through `derive Json` and the JSON bridge (`cbor.to_json`/`from_json`): no compiler work, but byte strings
  become base64 text, integers and floats blur, and map keys must be text, so the result is not what fxamacker writes. Kept
  as the quick path programs can already use; not the typed API.
- A hand written `Cbor` interface with `to_cbor`/`from_cbor` methods per type: the user writes the mapping, no derive; no
  rule is shared and nothing is checked against the oracle.
- Waiting for a service that needs it: AWS and Azure SDK work uses CBOR in a few places (the Smithy RPC v2 protocol), so the
  derive is also what those SDKs would generate against.

## Prototype findings (2026-11-05)

Pinned against fxamacker/cbor v2.9.0 by the typed corpora (1,600 cases agree): Time is written as
TimeUnixDynamic (integer seconds, a float when there is a fraction) and read from integer or float seconds or
RFC 3339 text, Duration is integer nanoseconds, both Time and Duration are supported; a repeated key reaching a
field is dropped (first wins) and a text key matches exactly before ignoring case; a keyasint field also matches
its number as text; built in tags 0 to 3 are checked on every decoded value; arrays of small integers read as
Bytes. Xo's zero Time is the Unix epoch, so an absent Time field encodes as 0 where Go's zero time encodes as
null (use `Time?`). `toarray` (a tag on the first field) is built and pinned (an array of exactly the field count). Enums and `derive Cbor(case:)` are not built yet.
