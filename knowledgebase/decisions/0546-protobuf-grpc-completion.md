# 0546: Protobuf and gRPC to full coverage: ProtoJSON, reflection and health, gRPC-Web, conformance and interop, speed against Go

Status: Accepted (owner, 2026-11-07, "I'm good with your recommendations"; Xo Lang 3 drafted; requested by the owner through the Lead)

Owner's answers to the open questions (2026-11-07):

1. Conformance runner: approved. The protobuf release is downloaded and `conformance_test_runner` built on an offload host as a plain or bulk job, in a directory the builder owns (not /work/in); the release version and its sha256 are recorded in this decision when the build is done.
2. gRPC-Web (item 6): dropped for now; recorded as deferred.
3. Text format (item 3): out for now.
4. Timing targets (item 9): unary within 1.5x and streaming within 1.2x of grpc-go as the first bar, parity as the later goal.

## Context

Decision 0530 (protobuf: wire codec, `derive Proto`, `.proto` parser and generator) and 0535 (std/grpc over std/http's HTTP/2)
are Accepted and partly built. Google Cloud's newer client libraries are gRPC first (aws-19 starts the Discovery based GCP SDK
after Azure), and gRPC is a standard backend protocol, so "Xo speaks protobuf and gRPC" has to hold up against the reference
implementations on more than the happy path. This decision lists what is still missing, how each part is proven, and the
order, so the owner can accept or trim it as one piece. It amends 0530 and 0535; it supersedes neither.

What exists on 2026-11-07 (std/changes/protobuf.md, grpc.md, protoparse.md; std-coverage.md rows):

- std/protobuf: wire layer and `derive Proto` (scalars with zigzag and fixed, Option, List packed, Map, nested messages,
  Int enums, oneofs, unknown fields kept), about 8,900 differential cases against protowire and dynamicpb
  (internal/stdtest/protoref), a fuzz test against protowire and dynamicpb, marshal 1.06x and unmarshal 0.46x of Go's time.
- std/protoparse: proto3 and proto2 parser and a Xo generator (branch protoparse-wip, `xo proto gen` drafted there).
- std/grpc: Status and the 17 codes, the message frame, `grpc-timeout` and `grpc-message` coding, Server and Client for unary
  and the three streaming shapes, deadlines, gzip, `-bin` metadata; interop with grpc-go both ways (TestGrpcInterop); no
  timing row, no reflection, no health, no trailer fuzzing, response headers and trailers beyond the status are partial.
- bench/survey-2026-10-30/grpcsvc: the hand written service of the backend gap survey (686 lines: Inventory GetItem and
  ListItems, grpc.health.v1 Check and Watch, grpc.reflection.v1 ServerReflectionInfo bidi, a client for Pricing/Quote, TLS
  with ALPN h2, every message hand encoded). Its GAP comments are this decision's checklist: no protobuf runtime (done),
  no gRPC library (partly done), no descriptors, no health and reflection services, no interceptors.

Offline oracles in the Go module cache (checked 2026-11-07): google.golang.org/protobuf v1.36.12 (proto, protowire,
protojson, prototext, dynamicpb, protodesc, and internal/conformance with the testee harness and failing_tests.txt),
google.golang.org/grpc v1.79.2 (the interop client and server, `health`, `reflection`, `grpc_testing`), genproto. Not present:
`protoc`, `conformance_test_runner` (a C++ binary built from the protobuf release with bazel or cmake), `conformance.proto`,
and any gRPC-Web implementation.

## Decision (proposed)

1. **Descriptors** (`std/protodesc`, small): Xo values for FileDescriptorProto, DescriptorProto, FieldDescriptorProto,
   EnumDescriptorProto, ServiceDescriptorProto, encoded and decoded with `derive Proto` itself; `protoparse` can produce them
   from a `.proto`, and `xo proto gen` embeds the serialized descriptor of each file in the generated code next to the
   structs. They are what reflection serves and what ProtoJSON and `Any` resolve names through. Oracle: protodesc and
   dynamicpb build the same descriptors from the same files.
2. **ProtoJSON** (`std/protojson`, canonical proto3 JSON mapping): `protojson.marshal[T](m, opts)` and `unmarshal[T]` for any
   `derive Proto` type: lowerCamelCase names with `json_name` honored, 64 bit integers as strings, bytes as base64, enums as
   names (numbers accepted on read, unknown numbers kept), `NaN` and `Infinity` strings, maps with string keys, oneofs as the
   set member, `Timestamp` and `Duration` as strings, `Struct`, `Value`, `ListValue`, `FieldMask`, wrappers, `Empty`, and `Any`
   with its `@type` (resolved through descriptors, so an unknown type is an error as in Go), unknown fields rejected unless
   `discard_unknown`, and the `EmitDefaultValues`, `UseProtoNames` and `UseEnumNumbers` options. The well known types
   (`google.protobuf.*`) ship as generated Xo modules under std. Oracle: `protojson` (Go) over the same descriptors: a corpus
   of messages marshaled both ways, byte identical output (Go randomizes whitespace in `protojson`; the corpus uses
   `Multiline: false` and compares after `json.Compact`, as protojson's own tests do) and equal decoded messages for hand
   written and random JSON inputs, with the rejected inputs matched by error class.
3. **Text format** (`std/prototext`) is out for now (owner, 2026-11-07), except what the conformance suite requires (item 7), as an optional
   stage at the end.
4. **gRPC completion** in std/grpc: response headers and trailing metadata on both ends (`call.header()`, `call.trailer()`,
   `ctx.set_header`, `ctx.set_trailer`), `grpc-status-details-bin` as a `google.rpc.Status` with typed details (needs
   item 1 and the well known types), interceptors as a list of `fn(Call, Next)` for unary and stream (the shape of the
   router `wrap`), `max_recv_size` and `max_send_size`, keepalive pings (`GOAWAY` and `PING` follow std/http h2), name
   resolution and balancing stay out (0535).
5. **Health and reflection** (`std/grpc/health`, `std/grpc/reflection`, same module family): `grpc.health.v1.Health` Check
   and Watch with per service status, and `grpc.reflection.v1.ServerReflection` (v1, plus v1alpha for old grpcurl) serving
   the descriptors registered by the generated code, so `grpcurl list`, `describe` and calls by name work against an Xo
   server. Oracle: grpc-go's `health` and `reflection` client packages (and their tests' expectations) against the Xo
   server; grpc-go servers against Xo's clients.
6. **gRPC-Web** (deferred by the owner, 2026-11-07; kept here for when it returns): `application/grpc-web` and `application/grpc-web+proto` on the same Server (trailers as a
   body frame with the 0x80 flag, no HTTP trailers, text mode with base64), plus CORS preflight left to the app's router.
   Server side first; a client for browsers is not Xo's job. There is no offline Go implementation to differ against, so the
   proof is the published protocol document (PROTOCOL-WEB.md): byte exact vectors written by hand from it, plus a
   round trip with the Xo client in web mode. This item is dropped first if the owner wants less.
7. **Conformance** with Google's protobuf conformance suite: the runner (`conformance_test_runner`) is a C++ binary built
   from the protobuf release tarball with bazel or cmake, neither of which is installed; it speaks `conformance.proto` over
   stdin and stdout to a "testee" program. The plan is a testee `conformance_xo` (a Xo program using `std/protobuf` and
   `std/protojson`, about 300 lines, `xo run`), with the runner built once on an offload host from a downloaded release.
   That download and build need the owner's go (network, about 20 minutes of build, about 2 GB of disk): this decision
   asks for it. Until then the Go oracle corpora of items 1 and 2 are the check, and `internal/conformance`'s
   `failing_tests.txt` from protobuf-go is the list of cases Go itself skips, which Xo's expected failures list starts from.
   Required: proto3 and proto2 binary and ProtoJSON cases pass, with every failure either fixed or listed with a reason in
   `internal/stdtest/protoconf/expected_failures.txt` (text format cases are listed as not supported unless item 3 is done).
8. **gRPC interop** with grpc-go's interop tests, locally: `google.golang.org/grpc/interop` has a client and a server for
   the standard cases (empty_unary, large_unary, client_streaming, server_streaming, ping_pong, empty_stream,
   cancel_after_begin, cancel_after_first_response, timeout_on_sleeping_server, custom_metadata, status_code_and_message,
   unimplemented_method, unimplemented_service, special_status_message, client_compressed and server_compressed
   streaming and unary, soak and the h2 `http2` cases). Both directions: the grpc-go interop client against an Xo server
   implementing `TestService` (`grpc_testing`, a `.proto` the generator handles), and an Xo client run of the same cases
   against the grpc-go interop server, started by a Go test in internal/stdtest/grpcref (TestGrpcInterop grows from its
   handful of lines to the list above). The cases that need other infrastructure (alts, xds, grpclb, orca) are listed as not
   applicable.
9. **Speed**: a timing row against grpc-go, recorded in bench/perf/grpc with a Go twin like the other rows, on the
   linux/arm64 host (`ab-host.sh`, exclusive): unary round trips per second at 1 and 64 concurrent calls with a 100 byte and
   a 10 KB message, server streaming throughput of 1,000 and 100,000 messages, bidirectional ping pong latency (p50, p99),
   and the cost of ProtoJSON marshal and unmarshal against `protojson` on the 12 KB document of bench/perf/protobuf. The
   expectation is stated before measuring (decision 0461's rule): unary within 1.5x of grpc-go and streaming within 1.2x;
   a result worse than that gets a profile and a fix list, not a waiver.
10. **Maturity** (decision 0461): each item lands Solid on all three axes: a Go oracle or interop run, fuzzing where there
    is a parser (the ProtoJSON reader, the grpc frame and trailer reader, the reflection request decoder), and a timing row.
    Change logs per module, coverage rows, spec sections.

## Order

1. protoparse (proto2, rebase, gate) and `xo proto gen` with services (client struct, server interface) and embedded
   descriptors: the branch protoparse-wip is nearly there (items 1 and the generator).
2. `std/protodesc` and the well known types.
3. `std/protojson` with its oracle corpus and fuzz test (the largest piece).
4. gRPC completion (item 4), health and reflection (item 5), against grpc-go.
5. Interop list (item 8), the timing row (item 9).
6. Conformance testee and runner (item 7), once the owner approves the download; gRPC-Web (item 6) last.

## Consequences

- New std modules: protodesc, protojson, grpc/health, grpc/reflection (and prototext only if item 3 is taken), each with the
  std module bar (ADR, change log, coverage row, spec section, differential, fuzz, timing row).
- `xo proto gen` grows from message structs to services and descriptors; generated files change shape once, early.
- The owner approved (2026-11-07) fetching and building Google's conformance runner on an offload host; gRPC-Web and text format are out for now.
- Cost: large (protojson alone is about the size of protobuf's wire layer); the order above makes each step usable
  (reflection and ProtoJSON unlock the GCP SDK work first).

## Alternatives

- **Skip ProtoJSON** and let apps use std/json: rejected, the canonical mapping differs (int64 strings, enum names,
  Timestamp text, Any) and Google's REST/JSON endpoints and gRPC-Gateway style APIs need it.
- **Reflection without descriptors** (names hard coded per service): rejected, grpcurl needs full file descriptors and the
  generator already has them.
- **Write the conformance testee only against Go oracles** (no Google runner): kept as the fallback in item 7; the runner is
  the independent check of the oracles' own assumptions, hence the request.
- **Port grpc-go's interop server wholesale**: rejected, its xds, alts and balancer cases are out of scope; the standard
  cases of item 8 are the contract.
- **gRPC-Web through an envoy style proxy**: rejected, it is a small framing difference on the same Server.

## Open questions for the owner (answered 2026-11-07, see the Status)

- The conformance runner download and build (item 7): approve fetching the protobuf release on an offload host?
- Keep gRPC-Web (item 6) or drop it?
- Text format (item 3): in or out?
- Whether the timing targets of item 9 (unary 1.5x, streaming 1.2x of grpc-go) are the right bar.
