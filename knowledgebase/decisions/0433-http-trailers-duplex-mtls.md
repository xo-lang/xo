# 0433: HTTP trailers, full duplex streams, and mutual TLS

Status: Accepted (owner, 2026-10-30, through the lead). Was: Proposed (Xo lang 5 with Xo Lang 3, 2026-10-30; for the owner's
review). Closes gaps 1 and 2 of the gRPC readiness survey. Gap 10
(h2c with prior knowledge, the PRI preface sniff, h2 settings) is a
separate ADR.

## Context

gRPC and other streaming protocols need three things std/http lacks
(spec/std.md on main, 2026-10-30):

- Trailers. HTTP/1.1 receive (http1.xo Chunked) parses the trailer
  section and drops it; h2srv.xo processTrailerHeaders validates and
  drops it; neither writer sends one. gRPC reports `grpc-status` and
  `grpc-message` as trailers, and an immediate error as a trailers-only
  response (one HEADERS frame with END_STREAM, no DATA).
- Full duplex. A `handle_stream` handler (0310) reads its request body
  before `http.stream` writes; a client `Client.open` (0404) sends the
  whole request body before it returns the response head. A bidi gRPC
  stream needs both directions at once.
- Mutual TLS. `Server.with_tls` never sends a CertificateRequest
  (std.md 4.4, "Client certificates are not requested"),
  `TlsState.peer_certificates` is always empty on a server, and neither
  `tls.client` nor `Client` can present a certificate.

Go has all three (Request.Trailer, http.TrailerPrefix, h2 is duplex,
ResponseController.EnableFullDuplex since Go 1.21, tls.Config
ClientAuth, ClientCAs and Certificates), so the Go backend is mapping
only, and the native backend follows Go's behavior and texts.

## Decision

### 1. Trailers

```
fn (w: BodyWriter) set_trailer(name: Str, value: Str)        // sent after the body
fn (r: Response) with_trailer_names(names: List[Str]) -> Response   // declared up front (HTTP/1.1 `Trailer` header)
fn (b: BodyReader) trailers() -> Map[Str, Str]               // a request's, once read returned None
fn (s: ClientStream) trailers() -> Map[Str, Str]             // a response's, once read returned None
fn (r: Request) with_trailers(f: fn() -> Map[Str, Str]) -> Request   // asked when a streamed request body ends
```

- Both declared and undeclared trailers are written, as Go (declared
  names through the `Trailer` header, undeclared ones as Go's
  TrailerPrefix). `with_trailer_names` only affects HTTP/1.1's header.
- Limits: trailers count against the header size limit
  (`max_header_bytes`) and a field count cap; over it the stream fails
  with the same error as an oversized header.
- Forbidden names are rejected at `set_trailer` (an `Err` would make
  every call fallible, so it is dropped and logged once, as Go drops
  them): Content-Length, Transfer-Encoding, Host, Trailer, Content-Type,
  Content-Encoding, Content-Range, Cache-Control, Expect, TE,
  Authorization, Set-Cookie, the hop by hop fields (RFC 9110 6.5.1, Go's
  badTrailer list). On receive the same names are dropped.
- HTTP/1.1: trailer fields follow the `0\r\n` chunk; a response that is
  not chunked (a Content-Length) drops its trailers, as Go.
- HTTP/2: the decision is made when the stream function returns. With
  trailers, the last DATA has no END_STREAM and a final HEADERS frame
  carries END_STREAM; without, the last DATA carries it. Trailers with
  no body written is trailers-only: the response HEADERS with
  END_STREAM, the trailer fields merged into it (gRPC's immediate
  error), as grpc-go expects. So h2srv holds the response HEADERS until
  the handler returns or writes its first DATA; `BodyWriter.flush()`
  still sends the head at once, for a handler that writes nothing for a
  long time (a long poll).

### 2. Full duplex

- Server, HTTP/2: inside a `handle_stream` route the BodyReader stays
  readable inside the `http.stream` function, always.
- Server, HTTP/1.1: only after `http.stream(...).with_full_duplex()`,
  as Go, since many HTTP/1 clients read nothing until they finish
  writing. 0430 already split the reader and writer state (plain and
  TLS). With it:
  - `100 Continue` is sent before the first body read when the request
    asked for it (`Expect: 100-continue`);
  - the response head is flushed eagerly, before the first write;
  - after the handler returns, unread request bytes are drained up to
    the existing drain limit, else the connection closes.
- Client: `http.pipe() -> (PipeWriter, BodyReader)` (Go's io.Pipe) on a
  bounded channel, so a writer blocks when the reader falls behind
  (backpressure). `PipeWriter.write`, `close()` and
  `close_with_error(e)` (Go's CloseWithError: the reader's next read
  returns that error). `c.open(req.with_body_reader(r))` returns at the
  response head, which may arrive before the request body ends; on h2
  the request DATA and response DATA then flow at once.

### 3. Mutual TLS

`TlsCert` moves to std/tls (std/tls must not import std/http); std/http
keeps `http.TlsCert` and `http.tls_cert` as aliases, so no program
changes.

```
enum ClientAuth { None, Request, RequireAny, VerifyIfGiven, RequireAndVerify }   // Go's five, Go's meaning
fn (s: Server) with_client_auth(mode: ClientAuth, roots: tls.Roots? = None) -> Server
    // roots: the CAs that verify client certificates (Go's ClientCAs); None: the system's
fn (c: Client) with_cert(cert: tls.TlsCert) -> Client   // presented when a server asks
// std/tls: client(..., cert: TlsCert? = None), a new trailing parameter with a default
```

- `TlsState` gains `verified_chains: List[List[x509.Certificate]]` (Go's
  VerifiedChains); `peer_certificates` is filled on a server.
- Handshake:
  - TLS 1.3: CertificateRequest (with signature_algorithms) after
    EncryptedExtensions; the client's Certificate and CertificateVerify
    (context string "TLS 1.3, client CertificateVerify") enter the
    transcript before the client Finished.
  - TLS 1.2: CertificateRequest after ServerKeyExchange; the client's
    Certificate before ClientKeyExchange and CertificateVerify after it.
  - Schemes offered and accepted: those std/crypto verifies today:
    ecdsa_secp256r1_sha256 (verify_raw_ecdsa), rsa_pss_rsae_sha256/384/512
    (verify_pss), rsa_pkcs1_sha256/384/512 for TLS 1.2 only (verify), and
    ed25519 (ed25519_verify).
  - Chains are checked with `x509.Certificate.verify(VerifyOptions{roots,
    intermediates: the rest of the client's chain, key_usages:
    [ClientAuth], at: now})`, as Go's ClientCAs check. Hostnames are not
    checked on a client certificate.
  - Error texts are Go's: `tls: client didn't provide a certificate`,
    `tls: failed to verify certificate: x509: ...`.
- Session resumption: the ticket carries the peer's certificates and
  verified chains, as Go's ticket does, so a resumed connection reports
  the same `TlsState`. A server with a mode above `Request` refuses a
  ticket that carries no chain (one issued before mTLS was on) and runs
  a full handshake.
- Decision 0244 (std links TLS only when used): the server reads peer
  data through srvtls.xo's function table (a TO_PEER entry in hconn.xo
  beside the existing TO_NEG, whose record type lives in the leaf module
  llrt/tlsneg.xo; 690a119fa, gotchas/tls-table-direct-call-links-tlssrv.md),
  never by calling tlssrv directly, and the
  connection keeps it in a new HC_ peer word next to HC_TLS.

### Split of the work

- Xo Lang 3: h2srv and h2w frames, the http1w writer and the http1
  reader's trailer map, hclient, the hconn peer word and table entries,
  the tls1x server and client handshake changes.
- Xo lang 5: the API, spec/std.md, the Go backend mapping, the goldens
  with Go parity, this ADR and the std change logs.

### Tests

- Goldens on every backend with Go parity: a gRPC style echo (bidi
  messages, then a `grpc-status` trailer), trailers-only on h2, HTTP/1.1
  chunked trailers both ways, forbidden trailer names dropped, a
  duplex HTTP/1.1 stream with `with_full_duplex`, `http.pipe` with
  close_with_error, mTLS in each ClientAuth mode (no certificate, a
  certificate from another CA, a certificate without the clientAuth EKU,
  a good one) on TLS 1.2 and 1.3, the verified chains, and a resumed
  mTLS connection.
- Go differential: grpc-go's interop client and server against an Xo
  server and client, in Docker on the offload hosts (not the Mac).

## Consequences

- gRPC over HTTP/2 with TLS (and mTLS) becomes writable in Xo; h2c for
  meshes waits for the gap 10 ADR.
- `tls.client` gains a defaulted parameter and `TlsState` a field:
  source compatible; a program that builds a `TlsState` literal (none in
  std or the corpus) would need the new field.
- `TlsCert` moving is invisible through the aliases.
- The TLS server grows a client certificate path and a larger ticket;
  code linked only when a server uses TLS (0244).

## Alternatives

- Trailers only through a declared list (Go before TrailerPrefix):
  rejected, gRPC servers set `grpc-message` only on error.
- Duplex always on HTTP/1.1: rejected, as Go, since clients that write
  their whole body first would deadlock against a handler that writes a
  large response before reading.
- Refusing resumption whenever client auth is on: simpler, but costs
  every mTLS reconnect a full handshake; Go keeps the chain in the
  ticket.
- `tls.client` taking the PEM pair instead of a TlsCert: parses the key
  on every connection; rejected.
