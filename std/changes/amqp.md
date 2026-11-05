# std/amqp changes

## 2026-11-05: feature: std/amqp stage 1, the AMQP 1.0 type system and encoding

- Kind: feature (port)
- Commit: this push
- Found by: the Azure SDK's Service Bus and Event Hubs needs (decision 0521, accepted by the owner 2026-11-05)
- Go reference: github.com/Azure/go-amqp v1.7.0 internal/encoding (MIT, Copyright 2017 Kale Blankenship and Microsoft): types.go, encode.go, decode.go
- Test: the amqp corpus (scripts/stdcorpus/amqporacle: go-amqp's encoder and decoder on typed cases, on native, LLVM and the Go backend) and std/native/amqp_test.xo
- Timing: Mac arm64, decode plus encode of the corpus values (147 cases x 2000, Xo on Values, go-amqp on Message.UnmarshalBinary and MarshalBinary into `any`): Go 963 and 985 ms, Xo 294 and 352 ms (0.31 to 0.36 of Go; Go builds interface values and goes through the message wrapper, so this is the cost of the whole path, not of the codec alone)
- Shipped in: not yet released

`amqp.encode(Value)` and `amqp.decode(Bytes)` over a `Value` enum of every AMQP 1.0 type (spec/std.md 64). Stages 2 to 4 (connection and sessions, links, CBS) follow.
