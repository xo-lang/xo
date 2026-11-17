# std/prototext changes

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
