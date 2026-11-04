# 0495: std/json token reader and writer

Status: Accepted (owner, 2026-11-04)

## Context

std/json reads a whole value (`parse_value`, `decode`) or a sequence of whole
values (`Decoder`, decision 0309), and writes a whole value (`encode`,
`Value.encode`). Two things in the AWS SDK port (aws-19, Smithy's JSON
protocols: awsJson1_0, awsJson1_1, restJson1 and the response bodies of
S3-style listings) cannot be done without building the whole `Value` tree:

- reading a large or untrusted response one token at a time (Go's
  `Decoder.Token`: delimiters, strings, numbers, booleans, null), and
  deserializing straight into the SDK's own types, skipping what it does not
  know;
- writing a request body field by field in encoding/json's exact forms
  (string escapes, number forms), without a struct to `derive Json` on.

Go's reference is encoding/json: `Decoder.Token` and `Decoder.More`,
`Decoder.InputOffset`, `Number`, and the encoder's string and float rules
(`encodeState.string`, `floatEncoder`).

## Decision

### Reader

```xo
enum Token {
  ObjectStart            // {
  ObjectEnd              // }
  ArrayStart             // [
  ArrayEnd               // ]
  Key(s: Str)            // an object member's name (Go returns it as a string token)
  Str(s: Str)
  Number(text: Str)      // the number as written (Go's json.Number with UseNumber)
  Bool(b: Bool)
  Null
}

/// @move_only
struct TokenReader {}

fn token_reader(max_token: Int = 67108864) -> TokenReader
/// Pull style over bytes already in memory (a whole response body):
fn tokens(data: Bytes) -> TokenReader
/// Push style, as Decoder (decision 0309): the tokens the input so far completes.
fn (var r: TokenReader) write(data: Bytes) -> List[Token] ! DecodeErr
fn (var r: TokenReader) finish() -> List[Token] ! DecodeErr
/// Pull style: the next token, None at the end of the value; the iteration
/// protocol of decision 0494 makes `for t? in json.tokens(body)` work.
fn (var r: TokenReader) next() -> Token? ! DecodeErr
/// Go's Decoder.More: another element or member follows in the current array or object.
fn (r: TokenReader) more() -> Bool
/// Go's Decoder.InputOffset: the byte offset after the last token.
fn (r: TokenReader) offset() -> Int
/// Skips the value that starts at the next token (a scalar, or through its matching end).
fn (var r: TokenReader) skip() -> () ! DecodeErr
```

Differences from Go, on purpose: object keys are `Key` (Go returns a string
token and the caller tracks position), numbers are always text (the caller
chooses `Int.parse` or `Float.parse`; no `float64` rounding of a large
integer), and syntax errors are `DecodeErr` with the decoder's texts (decision
0309), not `*SyntaxError`. A token's string is validated and unescaped as
`decode` does (invalid UTF-8 becomes U+FFFD; a lone surrogate escape too).

### Writer

```xo
/// @move_only
struct JsonWriter {}

fn json_writer(escape_html: Bool = true) -> JsonWriter
fn (var w: JsonWriter) object_start()
fn (var w: JsonWriter) object_end()
fn (var w: JsonWriter) array_start()
fn (var w: JsonWriter) array_end()
fn (var w: JsonWriter) key(name: Str)
fn (var w: JsonWriter) str(s: Str)            // encoding/json's string form
fn (var w: JsonWriter) int(n: Int)
fn (var w: JsonWriter) float(f: Float) -> () ! DecodeErr   // NaN and Inf are refused as Go's UnsupportedValueError
fn (var w: JsonWriter) number(text: Str) -> () ! DecodeErr // validated against the JSON grammar
fn (var w: JsonWriter) bool(b: Bool)
fn (var w: JsonWriter) null()
fn (var w: JsonWriter) raw(text: Str) -> () ! DecodeErr    // Go's RawMessage: valid JSON, compacted
@moves
fn (var w: JsonWriter) finish() -> Bytes
```

Commas and colons are inserted by the writer. A call out of order (a value
where a key is due, `object_end` with no open object) faults, since it is a
bug in the caller, not an input error. Strings are written as
encoding/json writes them: `"` and `\` escaped, control characters as `\u00XX`
(`\n`, `\r`, `\t` short), `<`, `>`, `&` as `\u003c`, `\u003e`, `\u0026`
unless `escape_html` is false, U+2028 and U+2029 as `\u2028`, `\u2029`,
invalid UTF-8 as `\ufffd`. Floats follow Go's `floatEncoder`: shortest
representation, exponent form for exponents below -6 or at or above 21,
`e-09` cleaned to `e-9`.

## aws-19's asks and the cbor questions (2026-11-04)

Taken, as additions to the API above:

- `TokenReader.value() -> json.Value ! DecodeErr`: the value that starts at the
  next token, as `parse_value` reads it (the same Value and errors), for the
  fields the SDK keeps whole.
- `JsonWriter.value(v: json.Value) -> () ! DecodeErr`: writes a Value token by
  token (a `Num` that is NaN or infinite is refused).
- `tokens(b"")`, and input of whitespace only, give None at once: no error, no
  token (Go's `Decoder.Token` returns io.EOF there). An input that ends inside
  an array or object is `invalid JSON: unexpected EOF` (Go returns io.EOF
  there too, which callers must tell from a clean end; a DecodeErr cannot be
  mistaken for one).
- Floats keep `encode`'s shortest form; NaN and infinities are refused
  (`json: unsupported value: NaN`); aws-19 passes `escape_html: false`.

Weighed on cbor (std/cbor), for later changes, not part of this build:

- Float width: a `Value.Float32` variant is taken (a float written at 32 bits
  when the caller says so, kept on reading a half or single), over an encode
  option, because the width belongs to the value (a Smithy float shape) and a
  global option cannot say it per field. It is its own change with its own
  std/changes entry.
- `Tag(4, [exp, mantissa])` round trip and `to_int()` refusing a Uint above
  2^63-1 are checks, not designs: they get tests first, and a fix if one fails.
- A cbor token reader and writer: low priority, after this one is in use, with
  the same shape if the SDK needs it (it reads cbor whole today).

## Consequences

- Both pieces are pure and pull no new capability; the reader reuses the
  lexer of `Decoder` (decision 0309), the writer the string and float
  routines `encode` already has (they exist as std/native/llrt/jsontext.xo
  and rtjson.xo), so the output is the same as `encode`'s by construction.
- Tests: the reader against Go's `Decoder.Token` on the json corpus inputs
  (Go oracle, every token and error), the writer against `json.Marshal` of
  the same values (an Xo `Value` tree written token by token must equal
  `Value.encode()`), and a round trip of both.
- A timing row beside the json twin: reading a large document token by token
  against Go's `Decoder.Token` (known to be slow in Go, so it should not be
  hard to match).
- Built 2026-11-04 (std/native/json.xo, std/rt/jsontoken.go; spec/std.md
  5.3): the reader is Go's `Decoder.Token` state machine, so its error texts
  and `more`/`offset` are Go's (the corpus `jsontoken` checks them against
  Go on 1,255 streams); `Key` after `{` with a bad byte has no context text,
  as Go's tokenError has none for tokenObjectStart. The `for t? in` form
  waits for decision 0494.
- Two new std types and about 20 functions; the Go backend implements them in
  std/rt with encoding/json itself.

## Alternatives

- A `Value`-only API (`parse_value` and `Value.encode`): what exists; costs the
  whole tree in memory and a copy of every string, which the SDK wants to
  avoid for list responses.
- Go's exact `Token() (Token, error)` with `any` and `Delim`: Xo has no
  `any`; the `Token` enum is the typed form, and `Key` removes a state
  machine from every caller.
- A callback (SAX) reader: inverts control; the pull reader composes with
  `for t? in` (decision 0494) and with push input from a socket.
- Numbers as `Int` or `Float` tokens: loses the text (a 20 digit integer, a
  trailing zero) that the SDK must preserve (`json.Number`).
