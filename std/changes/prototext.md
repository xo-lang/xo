# std/prototext changes

## 2026-11-17: feature: proto2 extensions

- Kind: feature
- Commit: this push
- Found by: task #228
- Go reference: google.golang.org/protobuf v1.36.12 encoding/prototext (encode.go, decode.go unmarshalMessage's text.TypeName case)
- Test: testdata/corpus/prototext, generated from protobuf-go: marshal and Format of pjt2.Ext wires with extensions (one with a wrong wire type, kept as an unknown field), and unmarshal of `[full.name]` fields, brackets with spaces, a duplicate, a repeated extension in two forms, by number, discarded, one the message cannot take
- Timing: none

Extensions are written as `[full.name]: value` after the fields, sorted by full name, and read from that form, with
protobuf-go's errors ("message M cannot be extended by X", "unknown field: [x]").
## 2026-11-17: feature: the text format reader (Unmarshal)

- Kind: feature
- Commit: this push
- Found by: task #153 (prototext.Unmarshal was a map.txt gap)
- Go reference: google.golang.org/protobuf v1.36.12 encoding/prototext/decode.go and
  internal/encoding/text (decode.go, decode_number.go, decode_string.go, decode_token.go), ported with attribution
- Test: corpus prototext UNMARSHAL (703 rows: Go's own text of the seeded messages, that text changed at random,
  and 206 written edges of the lexer and decoder; 404 of them errors), each the wire of Go's deterministic Marshal of
  what Go's Unmarshal made, or Go's error; seeds 7 and 8 with 2,500 messages each also matched
- Timing: none yet (task #153 measures marshal and unmarshal against Go on host 1)

`unmarshal(r, name, text, o)` and `unmarshal_bytes` return the wire bytes of the message the text sets, as Go's
deterministic proto.Marshal writes them, with Go's error texts, lines and columns. Not yet: extension fields
(`[pkg.ext]` outside an Any), which std/protojson's registry does not hold; they are unknown fields.

## 2026-11-16: perf: marshal 0.93 and format 0.97 of Go's prototext

- Kind: perf
- Commit: this push
- Found by: task #153, XO_PROF=cpu of bench/perf/prototext marshal on host 1 (scalar_out 45%, quote_to and
  hex_to under w_string, the occurrence list of field_out)
- Go reference: none (same output)
- Test: std/prototext_test.xo (every byte's escape against hex_to); corpus prototext unchanged
- Timing: bench/perf/prototext, linux/arm64 host 1, xo bench --ab 15 rounds. New/old: marshal wall 0.80 (0.791
  to 0.811), format 0.811 (0.801 to 0.822), max RSS 1.0. Against Go's prototext (Xo/Go): marshal wall 0.929 (0.909
  to 0.953), max RSS 0.108; format 0.972 (0.955 to 0.974)

A string is scanned once for bytes that need escaping and written in one write when none do; an escape is
written into a BytesBuilder beside the plain runs. A field's occurrences on a sorted wire are an index range of
the wire's field list, not a copied list. Only string fields check UTF-8. This supersedes the escape table of the
entry below (the table is gone).

## 2026-11-16: perf: a byte's \xNN escape is one write from a table

- Kind: perf
- Commit: this push
- Found by: XO_PROF=cpu of bench/perf/prototext marshal (quote_to and hex_to under w_string)
- Go reference: none (same output)
- Test: prototext_test.xo "each control byte and invalid UTF-8 byte is escaped as \\xNN" (all 256 bytes against hex_to)
- Timing: bench/perf/prototext marshal, linux/arm64 host 1, xo bench --ab 15 rounds against the previous build: wall 0.939 (0.93 to 0.953), max RSS 1.0

A control byte or a byte that is not UTF-8 is written as its four bytes from a 1,024 byte constant, not as a
backslash, an x and two hex digits.

## 2026-11-16: perf: a plain decimal double is written without a rewrite

- Kind: perf
- Commit: this push
- Found by: task #153, XO_PROF=cpu of bench/perf/prototext marshal on host 1 after d9e68db69: float_text 13%
- Go reference: none (same output)
- Test: std/prototext_test.xo (the fast path agrees with the full rewrite on 1876 of 8021 random and decimal
  values, and declines the rest); corpus prototext unchanged on arm64, llvm and go
- Timing: bench/perf/prototext marshal, linux/arm64 host 1, xo bench --ab 15 rounds, new/old wall 0.977 (0.968 to
  0.986); max RSS 1.0

Xo's text of a double with no exponent, an integer part of at most 6 digits and a value of at least 1e-4 is
already Go's `%g` text but for a trailing `.0`, which is dropped; other values still go through the full rewrite.

## 2026-11-16: perf: marshal 0.949 of the previous build (about 1.26 of Go)

- Kind: perf
- Commit: this push
- Found by: task #153 (prototext to Proven, performance 1.33 of Go), XO_PROF=cpu of bench/perf/prototext marshal on host 1: field_out counting each field's occurrences over the whole wire (13% flat), an empty Map per message for the oneof winners
- Go reference: none (same output)
- Test: corpus prototext unchanged (660 rows, arm64, llvm and go)
- Timing: bench/perf/prototext marshal with 6000 passes, linux/arm64 host 1, xo bench --ab 15 rounds, new/old wall 0.949 (0.944 to 0.955); max RSS 1.087 (1248 to 1356 KB)

When the wire's field numbers never go down (as Go and Xo write them), a field's occurrences are one run found by
binary search, not a scan of every field per field; the oneof winners are two Lists, not a Map per message; a run of
runes written as they are is one write; hex escapes write digits without a builder.

## 2026-11-16: perf: marshal 1.56 to 1.33 of Go

- Kind: perf
- Commit: this push
- Found by: XO_PROF=cpu of bench/perf/prototext marshal (msg_fields and field_out flat, Str drops from FieldDesc copies)
- Go reference: none (same output)
- Test: corpus prototext unchanged (660 rows, arm64, llvm and go)
- Timing: bench/perf/prototext, linux/arm64 host 1, xo bench --ab 15 rounds, Xo/Go wall: marshal 1.562 to 1.332 (1.303 to 1.344), format 1.572 to 1.335 (1.313 to 1.37); max RSS 0.11 to 0.099

A field's name is written, not returned (returning fd.name copied each FieldDesc), the wire is scanned for oneof
winners only when the message has a real oneof, and a field's occurrences are counted before they are copied.

## 2026-11-16: feature: the text format writer, protobuf-go's prototext Marshal and Format

- Kind: feature
- Commit: this push
- Found by: task #102 (std/protobuf features; decision 0683 item 2): stdaudit's proto.String row was a gap (no text format)
- Go reference: google.golang.org/protobuf v1.36.12 encoding/prototext encode.go and internal/encoding/text encode.go
- Test: corpus prototext (scripts/stdcorpus/prototextoracle): 660 rows of seeded random messages over std/protojson's schema (proto2, proto3 and editions; every scalar kind, maps of every key kind, oneofs, groups, a closed enum, the well known types, Any of known and unknown type URLs), their wire with unknown fields or a second copy of the message appended, each marshalled with two option sets and some with Format; arm64, llvm and go
- Timing: bench/perf/prototext against protobuf-go v1.36.12 with protoc-gen-go messages, linux/arm64 host 1 (xo bench --ab, 15 rounds, Xo/Go wall): marshal 1.562 (1.546 to 1.589), format 1.572 (1.553 to 1.601), none 0.789; max RSS 0.11. Go writes from generated code's fast path, Xo walks the wire through the descriptor (protojson's marshal was 1.64 on the same document before task #87)

`prototext.marshal(r, name, wire, o)` writes a message's text from its wire bytes and its descriptor in a
`protojson.Registry`, as Go's Marshal writes the message Go's Unmarshal makes of those bytes; `prototext.format` is
Go's Format. Opts has Multiline, Indent, EmitASCII, AllowPartial and EmitUnknown. Go puts one or two spaces at random
between fields (detrand); this writes one. The reader (Unmarshal) is not ported yet. protojson's MsgInfo records which
string fields must hold UTF-8 (proto3 and editions' utf8_validation), which the writer checks.
