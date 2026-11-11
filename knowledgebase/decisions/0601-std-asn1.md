# 0601: std/asn1, ASN.1 DER as a public module

Status: Accepted (owner, 2026-11-11 morning, via the lead; proposed by Xo Lang 3, 2026-11-11). Stage 1 implemented 2026-11-11 (std/asn1.xo, spec/std.md 78).

## Context

The std audit of 2026-11-11 (bench/results/std-audit-2026-11-11.md) ranks
"ASN.1 DER is inside std/native/llrt/asn1.xo (X.509), no public API"
fifth (score 36): `encoding/asn1.ObjectIdentifier` (6 stdlib files),
`Marshal` (5), `Unmarshal` (4), `RawValue` (3), `BitString` (4) and ten
more. Programs reach for it to read or write PKCS#8, PKCS#1, SEC 1 keys
by hand, certificate extensions std/x509 does not decode, CMS and
timestamp structures, SNMP, LDAP and Kerberos messages, and ECDSA
signatures as (r, s).

std/native/llrt/asn1.xo already has a TLV reader (`read`, `children`,
`uint_bytes`, `oid`, `text`, `time`) and DER writers (`enc_*`) for
X.509 and TLS; they are internal (`llrt`), keyed by byte offsets, and
answer None or a sentinel on a bad encoding rather than Go's errors.

Go's Marshal and Unmarshal walk a struct by reflection with field tags
(`asn1:"optional,explicit,tag:0"`). Xo has no reflection; it has derive
(Json, Cbor) and value trees (json.Value, cbor.Value, decision 0468).

## Decision (proposed; stages)

### Stage 1: a value tree with Go's DER rules

A pure module std/asn1 (merged, decision 0588):

```
enum Value {
  Bool(v: Bool), Int(v: BigInt), BitString(bytes: Bytes, bit_len: Int),
  OctetString(v: Bytes), Null, Oid(v: Oid), Enumerated(v: Int),
  Utf8(s: Str), Printable(s: Str), Ia5(s: Str), Numeric(s: Str), T61(s: Str), Bmp(s: Str),
  UtcTime(t: Time), GeneralizedTime(t: Time),
  Sequence(items: List[Value]), Set(items: List[Value]),
  Tagged(class: Class, tag: Int, explicit: Bool, v: Value),
  Raw(class: Class, tag: Int, constructed: Bool, bytes: Bytes),
}
enum Class { Universal, Application, ContextSpecific, Private }
struct Oid { arcs: List[Int] } derive Eq, Ord, Hash   // display: dotted, as Go's String
enum Asn1Err { Syntax(msg: Str), Structure(msg: Str) }  // display: Go's "asn1: syntax error: ..." texts

fn parse(der: Bytes) -> Value ! Asn1Err            // the whole input, one value (Go's trailing data error)
fn parse_prefix(der: Bytes) -> (Value, Bytes) ! Asn1Err   // Unmarshal's rest
fn (v: Value) der() -> Bytes ! Asn1Err             // Marshal's encoding (Set sorted as DER)
fn oid(dotted: Str) -> Oid ! Asn1Err
fn (b: Value) bit_at(i: Int) -> Int                // BitString.At
fn right_align(bytes: Bytes, bit_len: Int) -> Bytes   // BitString.RightAlign
```

- Parsing follows Go's strictness: minimal length and integer encodings,
  no indefinite lengths, valid UTF-8 and PrintableString characters,
  Go's time formats, a 64 bit arc limit in OIDs, and Go's error texts
  (checked by a corpus against Go's Unmarshal into RawValue and the
  typed targets).
- A context specific tag stays `Raw` on parse (DER does not say whether
  it is explicit); `Value.tagged_as(...)` reads it as explicit or
  implicit when the schema says, as Go's `explicit` field tag.
- The X.509, TLS and PKCS code keeps llrt/asn1.xo (its offset reader is
  faster and allocation free); std/asn1's parser is its own, sharing the
  encoders where the output is the same.

### Stage 2 (later, its own decision): `derive Asn1`

Struct encoding by derive, with field attributes for optional, explicit
and implicit tags, as Go's struct tags. Only after stage 1's corpus is
green and the tree's shape has settled.

## Consequences

- Hand-written PKCS, CMS, LDAP and SNMP parsers port without copying
  llrt's internals.
- One more std module (pure, every backend); spec/std.md gains a section;
  scripts/stdaudit/map.txt maps encoding/asn1.
- Two ASN.1 parsers in std (llrt's for X.509 and TLS, std/asn1's public
  one); the corpus keeps both against Go.

## Alternatives

- Export llrt/asn1.xo as is: offsets and sentinels are not an API, and
  its leniencies (where X.509 needs them) differ from Unmarshal's.
- derive Asn1 first: most uses in the audit are RawValue, OIDs and
  BitStrings read by hand, which a tree serves; derive needs the tree's
  rules anyway.
- A cryptobyte style builder and reader (golang.org/x/crypto/cryptobyte):
  closer to how Go's own crypto code reads DER now; a later addition over
  the same rules if the tree is too slow for a hot path.

## Stage 1 as built (2026-11-11)

- std/asn1.xo, one merged file (decision 0588), pure; spec/std.md 78.
- `Asn1Err` gained `Invalid(msg)` beside Syntax and Structure: Go makes
  some errors with errors.New or fmt.Errorf (`asn1: invalid UTF-8
  string`, `invalid BMPString`, its time texts), and Invalid shows its
  text as is.
- OID arcs are limited to 2^31 - 1, not 64 bits: Go's
  parseObjectIdentifier reads arcs with parseBase128Int, which refuses
  larger ones (`base 128 integer too large`); the proposal's "64 bit arc
  limit" was wrong.
- Time errors: a time Go refuses is `Invalid` with Xo's own text
  (`asn1: invalid UTCTime "..."`); Go's time.Parse texts are not ported,
  and the corpus compares them as one class. A `Time` holds 1678 to
  2261 (decision 0364), so a valid DER time written in another year is
  `asn1: time out of range`.
- ENUMERATED is read as an INTEGER of up to 64 bits (Go's Enumerated
  target is 32 bits; the tree's `Int` holds 64).
- `tagged_as(explicit, tag, constructed)` is the read the proposal named
  `tagged_as(...)`.
- Checked against Go's Unmarshal and Marshal: scripts/stdcorpus asn1.go,
  1305 cases (random trees from Marshal, their bit flips, truncations
  and byte swaps, and hand-made cases for each rule), on the Go, arm64
  and LLVM backends; std/asn1_test.xo for oid, bit_at, right_align,
  tagged_as and the encode errors.
- The X.509 and TLS code keeps llrt/asn1.xo (no shared encoders yet: its
  writers are internal and offset based).

