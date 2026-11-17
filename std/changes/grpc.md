# std/grpc changes



## 2026-11-17: feature: ServerKeepalive max_connection_age and max_connection_age_grace; a stream cut by a closed connection is Unavailable

- Kind: feature
- Commit: this commit
- Found by: task #236 (grpc-go's MaxConnectionAge and MaxConnectionAgeGrace were missing from ServerKeepalive)
- Go reference: google.golang.org/grpc v1.79.2 keepalive.ServerParameters; a stream on a connection closed after GOAWAY is ErrConnClosing, Unavailable
- Test: std/grpc_test.xo "keepalive_server ends a connection at max_connection_age after its grace, as grpc-go's" (the cut stream read Internal before)
- Timing: none
- Shipped in: not yet released
## 2026-11-17: feature: balancing over several addresses (with_addrs, with_dns, with_resolver, Balancer)

- Kind: feature
- Commit: this commit
- Found by: task #236 (map.txt grpc.behavior.name_resolution, grpc.WithResolvers, grpc.PickFirstBalancerName; decision 0707)
- Go reference: google.golang.org/grpc v1.79.2 balancer/pickfirst, balancer/roundrobin, resolver/manual, internal/resolver/dns (minResolutionInterval 30 s), serviceconfig loadBalancingConfig
- Test: std/grpc_test.xo "with_addrs spreads calls as grpc-go's pick_first and round_robin", "a service config's balancer policy, as grpc-go's parser"
- Timing: none (a client without addresses takes the path it took before)
- Shipped in: not yet released
## 2026-11-17: feature: unix: targets

- Kind: feature
- Commit: this commit
- Found by: task #236 (map.txt grpc.behavior.unix_targets)
- Go reference: google.golang.org/grpc v1.79.2 internal/resolver/unix (unix:///path, unix:path, the authority localhost)
- Test: std/grpc_test.xo "unix: targets call a server on a Unix socket", arm64 and LLVM
- Timing: none
- Shipped in: not yet released

## 2026-11-17: feature: server keepalive enforcement (ServerKeepalive min_time, permit_without_stream)

- Kind: feature
- Commit: this commit
- Found by: task #236 (grpc-go's KeepaliveEnforcementPolicy)
- Go reference: google.golang.org/grpc v1.79.2 keepalive.EnforcementPolicy, internal/transport/http2_server.go handlePing
- Test: std/grpc_test.xo "keepalive_server closes a connection that pings too often, as grpc-go's enforcement policy"
- Timing: none
- Shipped in: not yet released

## 2026-11-17: feature: a handler chooses its response compressor

- Kind: feature
- Commit: this commit
- Found by: task #236 (std Proven; scripts/stdaudit/map.txt SetSendCompressor, ClientSupportedCompressors)
- Go reference: google.golang.org/grpc v1.79.2 server.go SetSendCompressor, ClientSupportedCompressors, validateSendCompressor (the texts)
- Test: std/grpc_test.xo "a handler chooses the response's compressor" (gzip for a plain request and identity for a gzip one, on the wire and through the client; an unknown compressor; a call after the head), arm64 and LLVM; golden grpc_hooks
- Timing: none
- Shipped in: not yet released

## 2026-11-17: fix: a handler that sends nothing ends the call trailers-only, as grpc-go

- Kind: fix
- Commit: this commit
- Found by: task #236, the retry test (an Xo server's Unavailable was never retried: its head came before the status, so the call read as committed)
- Go reference: google.golang.org/grpc v1.79.2 internal/transport/http2_server.go WriteStatus (headers not sent: the status goes in a trailers-only response)
- Test: std/grpc_test.xo "retries follow the service config" (the retried calls failed after one attempt before), arm64 and LLVM; TestGrpcInterop, TestGrpcHealthInterop, TestGrpcReflectInterop
- Timing: none
- Shipped in: not yet released

## 2026-11-17: feature: service config and retries (decision 0705)

- Kind: feature
- Commit: this commit
- Found by: task #236 (std Proven; scripts/stdaudit/map.txt grpc service config and retry rows)
- Go reference: google.golang.org/grpc v1.79.2 service_config.go (parsing and validation), stream.go withRetry and shouldRetry (codes, commit, pushback, backoff), clientconn.go GetMethodConfig and retryThrottler; retry_test.go and service_config_test.go
- Test: std/grpc_test.xo "service config JSON parses as grpc-go's", "method_config takes the method's entry", "retries follow the service config" (retried until success, capped, disabled, a code not listed, committed, every attempt failing, a negative pushback, the method's timeout and response limit, throttling), arm64 and LLVM
- Behavior difference: streaming calls are not retried (grpc-go retries a stream before its first response); hedging and load balancing config are read past
- Timing: none (a call without a config takes the path it took)
- Shipped in: not yet released

## 2026-11-17: feature: target names, Client.target and canonical_target

- Kind: feature
- Commit: this commit
- Found by: task #236 (std Proven; scripts/stdaudit/map.txt ClientConn.Target, CanonicalTarget, behavior.name_resolution)
- Go reference: google.golang.org/grpc v1.79.2 clientconn.go parseTargetAndFindResolver (the default scheme dns), Target and CanonicalTarget; internal/resolver/passthrough
- Test: std/grpc_test.xo "target names: dns, passthrough and a bare host:port" (parsing, and calls through each to a server), arm64 and LLVM; golden grpc_hooks
- Behavior difference: a dns:// authority (the DNS server) is not used, std/http's resolver is; target names are called over plain HTTP/2
- Timing: none (once per client)
- Shipped in: not yet released

## 2026-11-17: fix: a connection lost under a call is Unavailable, as grpc-go

- Kind: fix
- Commit: this commit
- Found by: task #236, the keepalive test (a keepalive close ended the call Internal)
- Go reference: google.golang.org/grpc v1.79.2 internal/transport (a closing transport fails its streams with codes.Unavailable)
- Test: std/grpc_test.xo "client keepalive closes a connection whose server stopped answering" (Internal before, Unavailable now), arm64 and LLVM
- Shipped in: not yet released

## 2026-11-17: feature: keepalive

- Kind: feature
- Commit: this commit
- Found by: task #236 (std Proven; scripts/stdaudit/map.txt grpc keepalive rows)
- Go reference: google.golang.org/grpc v1.79.2 keepalive/keepalive.go, dialoptions.go WithKeepaliveParams (the 10 s minimum), server.go KeepaliveParams (the 1 s minimum); keepalive_test.go TestKeepaliveClientClosesUnresponsiveServer
- Test: std/grpc_test.xo "client keepalive closes a connection whose server stopped answering", "keepalive_server sets the HTTP/2 pings and the idle timeout", arm64 and LLVM; golden grpc_hooks
- Behavior difference: std/http's client pings an idle connection too (PermitWithoutStream true); no enforcement policy and no MaxConnectionAge
- Timing: none (settings only)
## 2026-11-17: behavior: a server's response head has no Date, as grpc-go's

- Kind: behavior
- Commit: this commit
- Found by: task #236 (map.txt grpc.behavior.response_head)
- Go reference: google.golang.org/grpc v1.79.2 internal/transport/http2_server.go writeHeaderLocked (no date field)
- Test: std/grpc_test.xo "send and receive limits and response metadata" (header metadata had a date), arm64 and LLVM
- Timing: none
- Shipped in: not yet released

## 2026-11-17: feature: default call options, on_finish and the peer

- Kind: feature
- Commit: this commit
- Found by: task #236 (std Proven; scripts/stdaudit/map.txt grpc gap rows WithDefaultCallOptions, OnFinish, OnFinishCallOption, Peer, StatsHandler)
- Go reference: google.golang.org/grpc v1.79.2 dialoptions.go WithDefaultCallOptions, rpc_util.go OnFinish, peer/peer.go
- Test: std/grpc_test.xo "default call options, on_finish and the peer" (a call's own metadata winning over the defaults, the default timeout sent, on_finish for OK, an error and a stream, the peer's IP), arm64 and LLVM
- Behavior difference: Call.peer is the client's IP without the port (grpc-go's Peer.Addr has both); StatsHandler is mapped partial to the interceptors and on_finish
- Timing: none (a call copies the default metadata only when the client has some)
- Shipped in: not yet released

## 2026-11-17: feature: interceptors (decision 0703)

- Kind: feature
- Commit: this commit
- Found by: task #236 (std Proven; scripts/stdaudit/map.txt grpc interceptor rows and behavior.interceptors)
- Go reference: google.golang.org/grpc v1.79.2 server.go chainUnaryServerInterceptors and chainStreamServerInterceptors, dialoptions.go WithChainUnaryInterceptor, clientconn.go chainUnaryClientInterceptors; interceptor_test.go (chain order)
- Test: std/grpc_test.xo "server interceptors chain in order around every method" (order with unary inside stream, one registered after the methods, metadata and request changed, a refusal, a stream method's kind) and "client interceptors chain around a call" (order, metadata added, an answer without a call, a stream interceptor adding metadata and refusing), arm64 and LLVM; golden grpc_calls unchanged on the Go backend
- Behavior difference: unary interceptors run inside the stream ones (grpc-go runs one kind per method); a client stream interceptor changes the options only
- Timing: none (a call without interceptors takes the path it took)
- Shipped in: not yet released

## 2026-11-17: feature: services, unknown and a stop that waits for handlers

- Kind: feature
- Commit: this commit
- Found by: task #236 (std Proven; scripts/stdaudit/map.txt grpc gap rows GetServiceInfo, ServiceInfo, MethodInfo, UnknownServiceHandler, WaitForHandlers)
- Go reference: google.golang.org/grpc v1.79.2 server.go GetServiceInfo, ServiceInfo, MethodInfo, UnknownServiceHandler (handleStream's unknown path) and WaitForHandlers
- Test: std/grpc_test.xo "services lists the registered methods by service", "unknown serves the methods no service registered" (unknown service, unknown method, a stream, a non gRPC 404, Unimplemented without it), "a stop waits for the running handlers", arm64 and LLVM
- Timing: none (registration only; a call's path is unchanged)
- Shipped in: not yet released

## 2026-11-17: feature: the :authority of a client and of a call, and its validation

- Kind: feature
- Commit: this push
- Found by: task #236 (grpc-go's WithAuthority, CallAuthority, AuthorityValidator; stdaudit gaps)
- Go reference: grpc-go v1.79.2 dialoptions.go WithAuthority, rpc_util.go CallAuthority, credentials/tls.go ValidateAuthority, internal/transport/http2_client.go NewStream
- Test: TestGrpcInterop tls cases (xo and grpc-go, each way and with themselves: the :authority the server sees, a call authority the certificate does not cover failing Unavailable with grpc-go's text); golden grpc_creds (over cleartext, unary and streaming, the audience)
- Timing: none (a call authority over https has its own connections, checked once per connection)

`Client.with_authority(a)` sets every call's `:authority` and, over https, the server name the handshake checks.
`Client.with_call_authority(a)` sets the calls' `:authority`, checked against the server's certificates before any
request byte goes out. `AuthInfo.validate_authority` is grpc-go's AuthorityValidator; a handler's `Call.authority` is
the request's `:authority`.

## 2026-11-17: feature: per-RPC credentials, security levels and the peer's AuthInfo

- Kind: feature
- Commit: this push
- Found by: std coverage map (credentials rows, grpc.behavior.mtls)
- Go reference: grpc-go v1.79.2 credentials (SecurityLevel, CheckSecurityLevel, AuthInfo, TLSInfo, RequestInfo, PerRPCCredentials), grpc.WithPerRPCCredentials, grpc.PerRPCCredentials, http2_client.go getTrAuthData and getCallAuthData
- Test: TestGrpcInterop's tls cases (mutual TLS with credentials that require it, Xo and grpc-go crossed both ways); golden grpc_creds; module tests "security levels" and "per-RPC credentials"
- Timing: none

`Client.with_per_rpc_creds` and `with_call_creds` add a `PerRpcCreds` function's metadata to each call, with
grpc-go's audience, its Unauthenticated error for credentials that need TLS over cleartext, and its Internal
mapping of control plane codes (gRFC A54). A handler's `Call.auth_info` is "tls" (with the TLS state and client
certificates) or "insecure"; `check_security_level` gives grpc-go's texts.

## 2026-11-17: feature: response header metadata from a stream handler (SetHeader, SendHeader)

- Kind: feature
- Commit: this push
- Found by: task #89 (decision 0699)
- Go reference: grpc-go v1.79.2 grpc.SetHeader, grpc.SendHeader (ErrIllegalHeaderWrite after the head)
- Test: TestGrpcInterop's md line: a handler sets x-h, sends x-s, then both calls fail with grpc-go's Internal text, Xo and grpc-go crossed as client and server
- Timing: none

`Send.set_header(name, value)` adds header metadata until the first message, `send_header` or the status;
`Send.send_header(md)` sends the head now. After that both are `Internal` "transport: SendHeader called multiple
times". Calls are served with `http.Response.with_late_head`.

## 2026-11-17: fix: a compressed message above the receive limit is ResourceExhausted, as grpc-go

- Kind: fix
- Commit: this commit
- Found by: task #89, reading grpc-go's rpc_util.go decompress beside std/grpc's pull while adding the client limits
- Go reference: google.golang.org/grpc v1.79.2 rpc_util.go decompress ("grpc: received message after decompression
  larger than max N")
- Test: TestGrpcInterop line gziplimit (a gzip response of 2,000 bytes against a 100 byte limit: Xo said Internal
  "grpc: failed to decompress the received message" before)
- Shipped in: not yet released

gunzip stopped at the limit with an error, which pull reported as a failed decompression; it is now
ResourceExhausted with grpc-go's text.

## 2026-11-17: feature: response metadata and message size limits on the client and server

- Kind: feature (behavior change)
- Commit: this commit
- Found by: task #89 (std features line; scripts/stdaudit/map.txt grpc gaps), decision 0546 item 4
- Go reference: google.golang.org/grpc v1.79.2 (grpc.Header, grpc.Trailer, MaxCallRecvMsgSize, MaxCallSendMsgSize,
  MaxSendMsgSize; rpc_util.go, stream.go, server.go texts; internal/transport isReservedHeader)
- Test: std/grpc_test.xo "send and receive limits and response metadata, as grpc-go"; TestGrpcInterop lines md,
  recvlimit, sendlimit and big, the same both ways against grpc-go
- Shipped in: not yet released

`ClientCall.header()` and `trailer()` give the response's metadata as grpc-go's client does. `Client.with_max_recv`
and `with_max_send` and `Server.with_max_send` set the limits, with grpc-go's ResourceExhausted texts. Changes: a
unary call's response is held to the 4 MiB receive limit (it was unbounded) and is read by the stream reader, so a
cut or bad frame reports the stream's error text; a server's head is `content-type` alone, plus `grpc-encoding: gzip`
when compressing (it also sent `grpc-accept-encoding` and `grpc-encoding: identity`).

## 2026-11-17: fix: a status's details reach the client (grpc-status-details-bin)

- Kind: fix
- Commit: this commit
- Found by: task #184 (otlp's gRPC exporter could not read a RetryInfo)
- Go reference: google.golang.org/grpc v1.79.2 internal/transport (grpc-status-details-bin in the trailers, base64 without padding)
- Test: std/grpc_test.xo "a status's details reach the client as grpc-status-details-bin" (details came back empty before)
- Shipped in: not yet released

A handler's `Status.details` were dropped: the server never wrote the `grpc-status-details-bin` trailer and the client always read `details` as empty. The server now writes it when details are set (trailers only responses too) and the client decodes it from the trailers or headers.

## 2026-11-16: fix: the client reads a response as grpc-go does (task #115)

- Kind: fix (behavior change)
- Commit: this commit
- Found by: TestGrpcFuzz (internal/stdtest/grpcfuzz_test.go), a differential of `Client.call` against grpc-go v1.79.2: both clients call a raw HTTP/2 server (grpcref's `cannedserve`) that answers seeded heads, frames cut at seeded points, trailers, trailers only responses
- Go reference: grpc-go internal/transport/http2_client.go operateHeaders, rpc_util.go recvMsg and checkRecvPayload, grpcutil.ContentSubtype
- Test: std/grpc_test.xo "the client answers a response's head as grpc-go does" and "a frame's length is checked before its flag" (both fail before the fix); TestGrpcFuzz seeds 2 to 7 (5000 calls each on amd64 and llvm, all equal)

- A response whose content-type is not `application/grpc`, alone or followed by `+` or `;` (compared as written:
  `Application/GRPC` is not gRPC), ends the call at its head, before any message, with the code grpc-go gives its HTTP
  status (Unknown for one with none, 200 among them) and grpc-go's text, `unexpected HTTP status code received from
  server: 200 (OK); transport: received unexpected content-type "text/html"`. It read the grpc-status of such a
  response, or read its body as frames.
- A gRPC response's HTTP status is not looked at: a 429 with grpc-status 0 is OK (it was Unavailable).
- No grpc-status is Unknown with the grpc-message (it was Internal, "server closed the stream without sending
  trailers").
- A grpc-status that strconv.ParseInt refuses is Unknown, `transport: malformed grpc-status: strconv.ParseInt: parsing
  " 0": invalid syntax`; one in the head ends the call before its content-type is looked at; one in the trailers of a
  stream cut short replaces "unexpected EOF".
- A frame's length is checked first (ResourceExhausted), then its whole payload (Truncated), then its flag.

Departures, modeled by the fuzz: a grpc-status past the 17 codes is Unknown (grpc-go keeps the number; `Code` has no
case for it); a stream that ends inside a message's 5 byte header is Internal "unexpected EOF" (grpc-go's
Stream.ReadMessageHeader drops the bytes read before the end and takes the trailers' status).
## 2026-11-15: feature: `FrameErr` is an open enum (decision 0649)

- Kind: feature
- Commit: this commit
- Found by: decision 0649 (a new variant of a std error enum broke two SDKs, 0644's `ClientErr.Tls`)
- Go reference: none (Go's errors are values compared with errors.Is; Rust's `#[non_exhaustive]`)
- Test: compiler/check's open enum test; the open_enum golden; xo check over the programs that use the module

`FrameErr` is declared `open`: a `match` on it outside std needs a `_` (or
binding) arm, warning XO0306 in 0.52 and an error after, so that std can
add a variant in a minor release without breaking a build. Matches that
already have `_` are unchanged.

## 2026-11-10: refactor: one file with its bodies (decision 0588 stage 2)

- Kind: refactor
- Commit: this commit
- Found by: decision 0588 stage 2 (pure modules merged with their implementation)
- Go reference: none (no behavior change)
- Test: std/grpc_test.xo (`xo test --std-module=grpc` on native and llvm); internal/stdtest corpus grpc on go, arm64 and llvm

std/grpc.xo now holds the bodies of std/native/grpc.xo (removed), and the Go
backend compiles them from it as its own package; std/rt/grpc.go (the Go backend's display methods) is removed, the
module's own `display` methods serve it. The API, texts and errors
are unchanged: every item the declaration file had is `pub`, with the same
parameter names.

## 2026-11-06: feature: gzip compression (Client.with_gzip; compressed messages are read)

- Kind: feature
- Commit: this commit
- Found by: decision 0535's list of what the first version leaves out
- Go reference: PROTOCOL-HTTP2.md (Compressed-Flag, Message-Encoding), grpc-go encoding/gzip
- Test: TestGrpcInterop "gzip" line both ways (an Xo client with_gzip against grpc-go, a grpc-go client UseCompressor(gzip) against an Xo server)
- Shipped in: not yet released

Every call advertises `grpc-accept-encoding: gzip,identity`. A message with the compressed flag is inflated when the peer's `grpc-encoding` is gzip
(its size after inflating is held to the receive limit: ResourceExhausted), else the call fails Internal. `client.with_gzip()` sends its messages
compressed; a server answers with the encoding of the request, as grpc-go does.

## 2026-11-06: feature: grpc.bin_encode and grpc.bin_decode for `-bin` metadata

- Kind: feature
- Commit: this commit
- Found by: decision 0535's list of what the first version leaves out
- Go reference: PROTOCOL-HTTP2.md (Custom-Metadata: a name ending in `-bin` carries base64, written unpadded, read padded or not); grpc-go internal/metadata
- Test: std/native/grpc_test.xo "binary metadata is unpadded base64 and reads padded or not" (RFC 4648 vectors, 40 length round trips)
- Shipped in: not yet released

A metadata value of a name ending in `-bin` is bytes: `bin_encode(data)` makes the header text, `bin_decode(text)` reads one from a peer.

## 2026-11-06: feature: std/grpc, gRPC over std/http's HTTP/2 (decision 0535)

- Kind: feature
- Commit: this commit
- Found by: owner decision 2026-11-06 (gRPC after protobuf, before OTLP export)
- Go reference: google.golang.org/grpc v1.79.2 (the interop oracle, internal/stdtest/grpcref), PROTOCOL-HTTP2.md
- Test: std/native/grpc_test.xo (frame, timeout and message codecs, random byte reads), testdata/corpus/grpc (grpc-timeout headers and status messages as grpc-go sends and decodes them), golden grpc_calls (unary, status, unknown method, a server deadline, server, client and bidirectional streams), TestGrpcInterop (an Xo client against a grpc-go server and a grpc-go client against an Xo server, the same six calls)

New module: the 17 status codes, `Status`, the message frame, `grpc-timeout` and `grpc-message` coding, a `Server` that
registers unary and streaming handlers and mounts on `http.server`, and a `Client` with `unary` and `call` for the
three streaming shapes. Deadlines cancel a server handler. Unfinished against decision 0535: compression, binary metadata,
extra response metadata, `xo proto gen` stubs, fuzzing of the trailer parser beyond the random byte tests, a timing row.
