# Proto files for the protoparse corpus

proto3 files sampled from the Go module cache, kept at their
`module@version/path` names: grpc-go, protobuf-go, genproto, pprof and
other projects, each under the Apache License 2.0 (or the license its
module states), with the copyright headers they carry. The protoparse
generator reads them from here, not from the local module cache, so
`stdcorpus -check` gives the same answer on any machine.
`XO_STDCORPUS_PROTO_RESAMPLE=1 go run ./scripts/stdcorpus protoparse`
takes a new sample from the cache.

Last verified: 2026-11-15
