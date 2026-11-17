# std/protowire changes

## 2026-11-17: feature: fields, walk as proto.Unmarshal reads a message

- Kind: feature
- Commit: this push
- Found by: task #292 (xo-4c's grpcreflect fuzz: walk reads a field numbered 1,168,186,489, proto.Unmarshal fails it)
- Go reference: protobuf-go internal/impl/decode.go (num > protowire.MaxValidNumber is errDecode)
- Test: std/protowire_test.xo ("fields fails a field number over 536870911, as proto.Unmarshal (walk reads it, as
  ConsumeField)")
- Shipped in: not yet released

`fields(b)` is `walk(b)` with every field number at most 536870911, else BadTag. walk keeps
protowire.ConsumeField's numbers (up to 2^31-1). protodesc, protojson, prototext, protowkt and grpchealth read
messages with fields.

## 2026-11-17: change: the unused bool writer wb is removed

- Kind: change
- Commit: this commit
- Found by: TestStdAPICovered on main (protowire.wb had no caller)
- Go reference: none (internal wire writer shared by protodesc, protowkt and protojson)
- Test: internal/repotest TestStdAPICovered
- Shipped in: not yet released

`protowire.wb` wrote a bool only when true. Its last caller moved to
the seen-aware `wbp` in 6f818c531 (a false that was on the wire is
written back), so the helper is removed. std/protowire is internal:
no user code could call it.
