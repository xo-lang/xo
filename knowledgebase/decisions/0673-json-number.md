# 0673: json.Number, a JSON number kept as its text, and BigInt in JSON

Status: Accepted (lead for the owner, 2026-11-16 overnight; proposed by Xo Lang 3, 2026-11-16, task #89)

## Context

Go's encoding/json writes a `*big.Int` as a bare JSON number of any
length (`{"n":123456789012345678901234567890}`, Int.MarshalJSON) and reads
one back exactly (UnmarshalJSON; `null` leaves the value unchanged).
APIs that carry large integers (blockchain amounts, 128-bit IDs, Stripe
and similar money in minor units past 2^63) send them that way.

Xo has no JSON form for big.BigInt. `derive Json(via: P)` (decision 0478)
cannot express it: every `Json` type that can be P writes a string, an
Int (64 bits), a Float (53 bits of mantissa), a list or an object. Go
has json.Number (a string type written as a raw number); Xo's JsonWriter
has `number(text)` but no type an encoder can use. This is the last open
features gap of std/big (scripts/stdaudit/map.txt big.Int.MarshalJSON,
UnmarshalJSON); BigRat and BigFloat already travel as Go's MarshalText
strings (2026-11-16).

## Decision

1. std/json gains `struct Number { text: Str }` (Go's json.Number), a
   `Json` type the compiler knows, as it knows `json.Value`:
   - encoding writes `text` as it is, after checking it against the JSON
     number grammar (JsonWriter.number; an invalid text is the encode
     error Go's `json: invalid number literal` is);
   - decoding accepts only a JSON number token and keeps its text
     exactly as written (no Int or Float conversion); a string, `null`
     or anything else is `DecodeErr` "expected number" (`Number?` takes
     `null`);
   - methods: `int() -> Int ! DecodeErr` and `float() -> Float !
     DecodeErr` (Go's Int64 and Float64), `display()` the text;
   - `json.to_value(n)` is `Value.Int` when the text is an integer that
     fits, else `Value.Num` (lossy, as Go's decoding into `any` without
     UseNumber).
2. std/big's BigInt derives `Json(via: json.Number, to: int_marshal,
   from: int_unmarshal)`: base 10 digits, and Go's
   `math/big: cannot unmarshal "x" into a *big.Int` for a number with a
   fraction or exponent.
3. Compiler: check/infer.xo (Number is Json), ssa/json.xo and
   ssa/jsonval.xo (encode and decode of the token), gen/json.xo and
   std/rt (the Go backend's Json_Number with XoJsonEnc/XoJsonDec),
   wasm through the same ssa. Goldens on every backend against Go's
   output, and a corpus of Go's Int.MarshalJSON/UnmarshalJSON results.

## Consequences

- Any user type can travel as a raw JSON number through `via:
  json.Number` (fixed-point decimals, IDs), Go's json.Number use.
- One more std type the compiler special-cases (with Value and Patch).
- A decoded `null` for a BigInt field is an error, not Go's no-op: Xo
  has no "leave the field as it was"; `BigInt?` reads `null` as None.

## Alternatives

- A `Value.Number(text)` variant instead of a type: breaks every
  exhaustive match on `json.Value` (a breaking change for users), and
  still needs a typed proxy for `via`.
- BigInt `via: Str` (`"123"`): works today with no compiler change, but
  is not the JSON Go and other services write or accept.
- Special-case BigInt in the compiler: no reuse for other number-shaped
  types, and couples the compiler to std/big.
- Leave BigInt without JSON: math/big stays features Solid.

## Implementation notes (Xo Lang 3, 2026-11-16)

- Encoding cannot fail in Xo (`json.encode` returns Bytes), so a Number
  whose text is not a JSON number encodes as `null`, as a NaN Float does;
  Go's Marshal fails with `json: invalid number literal`. An empty text
  encodes as `0` (Go's rule).
- A string is refused as point 1 says, although Go's json.Number field
  reads a string holding a valid number: that keeps BigInt's via decode
  equal to big.Int's UnmarshalJSON, which refuses one.
- `float()` accepts `_` between digits as strconv.ParseFloat does; `int()`
  refuses it as strconv.ParseInt with base 10 does.
