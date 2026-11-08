# std/protojson changes

## 2026-11-07: port: std/protojson, the canonical JSON mapping

- Kind: port
- Commit: this push
- Found by: ADR 0546 (protobuf and gRPC to full coverage), item 2
- Go reference: google.golang.org/protobuf v1.36.12 (encoding/protojson)
- Test: see below
- Timing: not measured yet
- Shipped in: not yet released

Descriptor driven marshal and unmarshal of any message (spec/std.md 74), with the well known types' special forms. Test: internal/stdtest/testdata/corpus/protojson (protojson marshal of fixed and random messages, byte identical after compaction; unmarshal of curated and random inputs, equal wire bytes, rejected inputs matched) on arm64, LLVM and the Go backend. Google's protobuf conformance runner (v29.3, test/protoconf, bench/results/protobuf-conformance-2026-11-07.md): 1411 successes and 2 listed failures against protobuf-go's 1412 and 0; the run found sint64's extremes faulting in the encoder, fixed here. Skipped as gaps: text format, jspb and proto2 message types.
