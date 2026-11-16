# std/prototext changes

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
