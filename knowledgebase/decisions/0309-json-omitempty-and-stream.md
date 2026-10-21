# 0309: JSON omitempty, and a streaming decoder

Status: Accepted (owner, 2026-10-21, within decision 0276)

## Context

The coverage table's encoding/json row has two gaps from Go's package
that services hit:

- **omitempty.** Go's `json:",omitempty"` leaves a field out of the
  output when it is empty. APIs that send sparse objects (PATCH bodies,
  webhook payloads, cloud SDK requests) depend on it, and Xo writes every
  field.
- **A streaming decoder.** Go's json.Decoder reads a sequence of values
  from a stream: NDJSON, concatenated objects, a long-running response.
  Xo's `decode` and `parse_value` take one whole document.

Decision 0277 put a field's wire name in `as "key"`, after the field
name. The owner approved both gaps on 2026-10-21.

## Decision

1. **omitempty.** The wire string takes Go's tag options after a comma:
   `email as "email,omitempty": Str?`. `as ",omitempty"` keeps the
   field's default key (its name, or what `case:` makes of it).
   - With `omitempty`, `derive Json`'s encoder leaves the field out when
     its value is empty in Go's sense: `false`, `0`, `0.0`, `""`, an
     empty List, Map, Set, or Bytes, and `None`. A struct or an enum
     value is never empty, as in Go.
   - Decoding is unchanged: a missing key takes the field's default, as
     before.
   - Any other option (`string`, `omitzero`, an unknown word) is a new
     checker error with a new XO code. The key part follows 0277's rules
     (XO0412 now means a key that is empty with no options).
2. **json.Decoder** (push style, like std/compress's, decision 0305):

   ```
   /// @move_only
   struct Decoder {}
   fn decoder(max_value: Int = 67108864) -> Decoder
   fn (var d: Decoder) write(data: Bytes) -> List[Value] ! DecodeErr   // the values completed so far
   fn (var d: Decoder) finish() -> List[Value] ! DecodeErr            // the rest; an unfinished value is an error
   fn from_value[T: Json](v: Value) -> T ! DecodeErr
   ```

   - The values are what Go's json.Decoder reads: any JSON values one
     after another, with optional whitespace between them (NDJSON,
     concatenated objects, numbers separated by spaces). The errors are
     Go's texts.
   - A number at the end of the input written so far is complete only
     when a delimiter follows it, or at `finish`.
   - `max_value` bounds the bytes of one value (Go has no bound): an
     error, not unbounded buffering of a value that never ends.
   - `from_value` decodes one Value into a typed T, as `decode` does from
     its text.

## Consequences

- omitempty needs work in compiler/check (wire_keys reads the options),
  compiler/ssa json.xo (native, LLVM, wasm) and compiler/gen json.xo (Go).
  It is a compiler change, covered by the hourly fixed point.
- The Decoder is std only: std/native/json.xo with a scanner that finds
  where each value ends, and std/rt on Go's json.Decoder.
- Tests:
  - Go's encoding/json omitempty tests as goldens on every backend.
  - A Go-generated corpus of value streams cut at random places
    (scripts/stdcorpus json_stream), against json.Decoder's values and
    errors.

## Alternatives

- **`derive Json(omit_empty: true)` for the whole struct:** coarser than
  Go, and a mixed struct (an optional field beside a required zero
  `count`) could not be written.
- **A field attribute syntax (`@omitempty kind: Str`):** new syntax,
  where the `as` string already carries the wire name, and Go users read
  `",omitempty"` at once.
- **A Decoder that reads from a Stream or Conn:** that waits for a
  streaming I/O design; the push API sits under one later.
