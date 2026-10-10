# 0106: HTTPS in std/http: tls_cert, Server.with_tls, Client.with_roots

Status: Accepted (2026-10-09); point 2 (HTTP/1.1 only) superseded by
0121 (2026-10-10: HTTP/2 over TLS on every backend). Implemented on the Go backend
(`std/rt/http.go`, `http_net.go`) and the native backends
(`std/native/http.xo`, `std/llrt/xo_tls.c`, decision 0105). Spec: std.md
4.6. Tests: the `http_tls` golden (`internal/llvm/testdata/golden`, run
by TestGolden, TestGoldenShared, TestGoldenASan, TestGoldenThreads,
TestGoldenTasks, and internal/arm64's TestGoldenRun), TestTLSDiff
(`internal/llvm/tls_diff_test.go`).

## Context

std.md 4.4 and 4.5 had no TLS surface: the Go backend's client called
`https` URLs through `http.DefaultTransport` (system roots, HTTP/2 when
the server offered it), its server could not serve HTTPS at all, and the
native backends rejected an `https` literal with XO1201 (decision 0083).
Decision 0089 made native TLS optional for 1.0 but wanted it; a native
implementation needs a surface both backends share, and the Go backend
had no server API to copy.

## Decision

1. **API** (std.md 4.6):
   - `http.tls_cert(cert_pem: Bytes, key_pem: Bytes) -> TlsCert ! TlsErr`:
     a PEM chain and key, checked as `tls.X509KeyPair` checks them, with
     its error texts. Bytes, not paths: reading files needs `Fs`, which
     the caller already holds, so no capability hides in the call.
   - `Server.with_tls(certs: List[TlsCert]) -> Server`: a copy that serves
     HTTPS; `certs[0]` unless SNI names a DNS name of another certificate
     (Go's `getCertificate`). An empty list is an `Io` error of `serve`.
   - `Client.with_roots(pem: Bytes) -> Client ! TlsErr`: the certificates
     of `pem` replace the system's roots (Go's `RootCAs`), for private CAs
     and tests. No way to skip verification.
   - `struct TlsErr { msg: Str }`, like `UrlErr`.
   The `with_` style matches `Request.with_header`; no new effect (the
   `serve` and `send` effects already cover the network).
2. **HTTP/1.1 only, also over TLS, on both backends.** The Go backend's
   server and client set `http.Protocols` to HTTP/1 (ALPN `http/1.1`), so
   a program behaves the same on every backend and an Xo server sees the
   same request (`User-Agent: Go-http-client/1.1`) from either client.
   HTTP/2 comes back for both once the native backends have it (ALPN is
   in place).
3. **Failed handshakes are not logged.** net/http logs `http: TLS
   handshake error from ...` on standard error for every client that gives
   up (scanners, a browser rejecting a test certificate); std/rt drops
   that line (its other server log lines stay) and the native server logs
   nothing either.
4. **Errors are Go's.** The native client reproduces crypto/tls's and
   net/http's texts and `ClientErr` variants: verification failures are
   `Protocol("<Op> \"<url>\": tls: failed to verify certificate: x509:
   ...")` in the order of `x509.Certificate.Verify` (the leaf's validity,
   the host name, the chain), a plain HTTP answer is `http: server gave
   HTTP response to HTTPS client`, alerts are `Connect("remote error: tls:
   ...")` (a `net.OpError` in Go), and the handshake has
   `http.DefaultTransport`'s 10 s timeout. The native server answers
   plain HTTP on a TLS port with net/http's `400 Client sent an HTTP
   request to an HTTPS server.` On macOS the system's roots are checked
   through the Security framework calls Go's `systemVerify` makes, so even
   those texts (`x509: “name” certificate is not trusted`) match.
5. **Tests stay in process.** In memory listeners and `FakeNet` ignore
   TLS: a test of a `with_tls` server or an `https` call needs no
   certificate.

## Consequences

- The Go backend's client no longer uses HTTP/2 (std.md 4.6 says so;
  `spec/core.md` "Changes in 0.9"). Servers that only speak HTTP/2 over
  TLS are rare; HTTP/1.1 remains universal.
- Programs that serve HTTPS read their certificate files themselves and
  declare `fs.read`; certificates do not rotate while `serve` runs (a new
  server is needed). Reloading is future work.
- TestTLSDiff found three behaviors to copy: alerts are Connect errors,
  a TLS 1.3 server's certificate request failure arrives after the
  handshake (read once more after a failed exchange), and macOS's own
  verifier texts.
- No client certificates (mutual TLS), no `InsecureSkipVerify`, no
  session resumption: each is an addition, not a change, when needed.

## Alternatives

- **File paths** (`serve_tls(cert_file, key_file)`, Go's `ListenAndServeTLS`):
  hides a file read without `fs.read`.
- **A `TlsConfig` struct** with versions, suites, client auth: more
  surface than any current program needs; the defaults are Go's.
- **Keeping HTTP/2 on the Go backend**: the backends would differ in what
  a server sees (`Go-http-client/2.0`, no `Connection` header), which
  breaks the golden comparison that keeps them honest.
- **Logging handshake failures natively too**: noise on standard error
  that goldens would have to filter; dropped on both sides instead.

## Owner confirmation (2026-10-09)

HTTP/1.1 only on every backend is confirmed until HTTP/2 lands on both
backends (queued with the native leftovers).

Note (2026-10-10): HTTP/2 landed on both backends (decision 0121): with
TLS, servers and clients negotiate `h2` by ALPN on every backend, and
cleartext stays HTTP/1.1. The rest of this decision (the API, Go's error
texts, quiet handshake failures, in process tests) holds.
