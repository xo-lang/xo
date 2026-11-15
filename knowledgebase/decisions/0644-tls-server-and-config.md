# 0644: std/tls gains a public server, Config values for client and server, and Go's crypto/tls surface (T1 to T14)

Status: Accepted for B1 (lang3, 2026-11-15; scope approved by the owner through the
lead: the whole crypto/tls gap list of ~/xo-lead/notes/lang3/crypto-tls-gap.md,
ECH and QUIC included)

## Context

std/tls is a client on a dialed Conn (`tls.client`, named parameters).
The server code exists in std/native/llrt (tlssrv.xo, tls13srv.xo,
tls12srv.xo, session tickets in tlsticket.xo) but only std/http reaches
it (`Server.with_tls`, `with_client_auth`). Go's crypto/tls has a
Config shared by both sides, tls.Server and tls.Listen, ConnectionState,
session resumption, callbacks (GetCertificate, GetConfigForClient,
VerifyPeerCertificate, VerifyConnection), version, suite and curve
preferences, KeyLogWriter, exporters, OCSP and SCTs, a Dialer, typed
errors, ECH and QUIC.

## Decision

Batch B1 (T2, T11, T1) is built as follows; the later batches fill in the
rest in the same shape.

- **Two connection types.** A dialed connection is `TlsConn` (`uses
  net.dial`, from `client` or `dial`); an accepted one is `TlsServerConn`
  (`uses net.listen`, from `server` or `TlsListener.accept`), matching
  the prelude's Conn and ServerConn, whose effects already differ
  (decision 0280). Both satisfy Stream and NetConn. The lead approved
  this over one TlsConn type with a widened effect (2026-11-15).
- **ServerConfig** is a struct value with Go's defaults as field
  defaults: `certificates`, `next_protos`, `client_auth`, `client_cas`.
  Later pieces add fields (T4 ticket keys, T5 callbacks, T7
  preferences, T8 key log, T10 OCSP and SCTs, T13 ECH keys). The client
  keeps named parameters on `client` and `dial`; a `ClientConfig` came
  with T6 (2026-11-15), when the hooks took the parameter list past a
  dozen: `client_config(c, cfg)` and `dial_config(net, addr, cfg)` take
  every client setting, and `client` and `dial` keep their parameters
  for the common case.
- **Hooks (B2, 2026-11-15)** run at Go's call points, with Go's alerts:
  the client's `verify_peer_certificate` after chain verification (and
  with `Verify.None`, given no chains) and `verify_connection` after it,
  both bad_certificate; `get_client_certificate` at the
  CertificateRequest (TLS 1.3 fails with no alert, TLS 1.2 with
  internal_error); the server's `verify_peer_certificate` on the client's
  certificates and `verify_connection` before its Finished (1.3) or
  before the ClientKeyExchange or the resumed ServerHello (1.2);
  `get_config_for_client` then `get_certificate` at the ClientHello
  (unrecognized_name when neither gives a certificate). A hook's `Err`
  is the handshake's error text. `supports_certificate` on
  `ClientHelloInfo` and `CertificateRequestInfo` is Go's
  SupportsCertificate over the hello's lists, with Go's error texts.
- **`server(c, config) -> TlsServerConn`** does not fail and does not
  handshake: as Go's `tls.Server`, the handshake runs on the first
  `handshake`, `read` or `write`, so a slow or silent client never holds
  up `accept`. The handshake is bounded by the connection's deadlines
  (the earlier of the read and write deadlines; Go's Handshake under
  SetDeadline) and is a cancellation point. A missing certificate is an
  error at the handshake ("tls: no certificates configured"), and
  `listen` refuses it up front (Go's Listen text).
- **`listen`, `new_listener`, `TlsListener.accept`** are Go's Listen,
  NewListener and listener.Accept: `accept` returns the connection
  before its handshake.
- **`state() -> ConnState`** (T2) on both types is Go's
  ConnectionState with Go's numbers; `cipher_suite_name`,
  `version_name` and `curve_name` give Go's texts. Fields for later
  pieces (SCTs, OCSP, tls_unique, ECH) exist with empty defaults now, so
  adding them later breaks no struct literal.
- **T11:** `dial` (Go's Dial and DialWithDialer's timeout over both the
  dial and the handshake), `load_cert` (LoadX509KeyPair), `net_conn`
  (NetConn), `verify_hostname`, `close_write` (close_notify, then the
  connection's CloseWrite) and `local_addr`/`remote_addr`. The last two
  and `close_write` need the same methods on the prelude's Conn and
  ServerConn, so the prelude gains them (lead approval 2026-11-15; spec
  std 3.2), on every backend: native (getsockname, getpeername,
  shutdown), wasip2 (new intrinsics `rt.wasi_sock_conn_local` and
  `rt.wasi_sock_shutdown` over `tcp-socket.local-address` and
  `shutdown`), Go (net.Conn).
- **Native code:** the server is std/http's existing engine
  (std/native/llrt/tlssrv.xo), given a configuration of its own
  (`tlssrv.conn_cfg`: no default ALPN, its own ticket keys, no plain
  HTTP 400 answer). A new module tlsserve.xo holds the handle; it is
  linked only when a program reaches `tls.server` (a cabi hook frees
  the engine's block). Session tickets are per connection until T4
  gives ticket keys, so cross-connection resumption waits for T3 and T4.
- **Tests** are Go's unit tests ported as goldens (tls_server,
  tls_dial, tls_conn_state, tls_close_write; go, arm64, llvm with Go's
  output as want). Go's handshake recordings (testdata/Client-* and
  Server-*) replay bytes made with fixed randomness and OpenSSL peers,
  which Xo's handshake cannot reproduce; each becomes an interop case
  in the evidence pass at the end of 0644 instead: the same
  configuration with Go's crypto/tls on the other end, checking the
  outcome on both ends (the state fields, the alert and its text). The
  table below maps every recording (go1.26.11's 120) to its case and the
  piece that makes it possible; "none" rows test what Xo leaves out.

| Recording | Interop case | Piece |
| --- | --- | --- |
| Client-TLSv10-ClientCert-ECDSA-ECDSA | none: TLS 1.0 and 1.1 (Xo implements 1.2 and 1.3; Go's default MinVersion is 1.2) | - |
| Client-TLSv10-ClientCert-ECDSA-RSA | none: TLS 1.0 and 1.1 (Xo implements 1.2 and 1.3; Go's default MinVersion is 1.2) | - |
| Client-TLSv10-ClientCert-Ed25519 | none: TLS 1.0 and 1.1 (Xo implements 1.2 and 1.3; Go's default MinVersion is 1.2) | - |
| Client-TLSv10-ClientCert-RSA-ECDSA | none: TLS 1.0 and 1.1 (Xo implements 1.2 and 1.3; Go's default MinVersion is 1.2) | - |
| Client-TLSv10-ClientCert-RSA-RSA | none: TLS 1.0 and 1.1 (Xo implements 1.2 and 1.3; Go's default MinVersion is 1.2) | - |
| Client-TLSv10-ECDHE-ECDSA-AES | none: TLS 1.0 and 1.1 (Xo implements 1.2 and 1.3; Go's default MinVersion is 1.2) | - |
| Client-TLSv10-ECDHE-RSA-AES | none: TLS 1.0 and 1.1 (Xo implements 1.2 and 1.3; Go's default MinVersion is 1.2) | - |
| Client-TLSv10-Ed25519 | none: TLS 1.0 and 1.1 (Xo implements 1.2 and 1.3; Go's default MinVersion is 1.2) | - |
| Client-TLSv10-ExportKeyingMaterial | none: TLS 1.0 and 1.1 (Xo implements 1.2 and 1.3; Go's default MinVersion is 1.2) | - |
| Client-TLSv10-RSA-RC4 | none: TLS 1.0 and 1.1 (Xo implements 1.2 and 1.3; Go's default MinVersion is 1.2) | - |
| Client-TLSv11-ECDHE-ECDSA-AES | none: TLS 1.0 and 1.1 (Xo implements 1.2 and 1.3; Go's default MinVersion is 1.2) | - |
| Client-TLSv11-ECDHE-RSA-AES | none: TLS 1.0 and 1.1 (Xo implements 1.2 and 1.3; Go's default MinVersion is 1.2) | - |
| Client-TLSv11-Ed25519 | none: TLS 1.0 and 1.1 (Xo implements 1.2 and 1.3; Go's default MinVersion is 1.2) | - |
| Client-TLSv11-RSA-RC4 | none: TLS 1.0 and 1.1 (Xo implements 1.2 and 1.3; Go's default MinVersion is 1.2) | - |
| Client-TLSv12-AES128-GCM-SHA256 | interop: Xo tls.dial against a Go server with the recording's suite, curve or key (set on the Go end); version, suite and curve in state equal on both ends | T1 |
| Client-TLSv12-AES128-SHA256 | interop: Xo tls.dial against a Go server with the recording's suite, curve or key (set on the Go end); version, suite and curve in state equal on both ends | T1 |
| Client-TLSv12-AES256-GCM-SHA384 | interop: Xo tls.dial against a Go server with the recording's suite, curve or key (set on the Go end); version, suite and curve in state equal on both ends | T1 |
| Client-TLSv12-ALPN | interop: Xo tls.dial against a Go server; the negotiated protocol on both ends equal | T1 |
| Client-TLSv12-ALPN-NoMatch | interop: Xo tls.dial against a Go server; no common protocol: Go's no_application_protocol alert | T1 |
| Client-TLSv12-ClientCert-ECDSA-ECDSA | interop: Xo tls.dial against a Go server with client auth; the client certificate and its verified chain in the server's state | T1 |
| Client-TLSv12-ClientCert-ECDSA-RSA | interop: Xo tls.dial against a Go server with client auth; the client certificate and its verified chain in the server's state | T1 |
| Client-TLSv12-ClientCert-Ed25519 | interop: Xo tls.dial against a Go server with client auth; the client certificate and its verified chain in the server's state | T1 |
| Client-TLSv12-ClientCert-RSA-AES256-GCM-SHA384 | interop: Xo tls.dial against a Go server with client auth; the client certificate and its verified chain in the server's state | T1 |
| Client-TLSv12-ClientCert-RSA-ECDSA | interop: Xo tls.dial against a Go server with client auth; the client certificate and its verified chain in the server's state | T1 |
| Client-TLSv12-ClientCert-RSA-RSA | interop: Xo tls.dial against a Go server with client auth; the client certificate and its verified chain in the server's state | T1 |
| Client-TLSv12-ClientCert-RSA-RSAPKCS1v15 | interop: Xo tls.dial against a Go server with client auth; the client certificate and its verified chain in the server's state | T1 |
| Client-TLSv12-ClientCert-RSA-RSAPSS | interop: Xo tls.dial against a Go server with client auth; the client certificate and its verified chain in the server's state | T1 |
| Client-TLSv12-ECDHE-ECDSA-AES | interop: Xo tls.dial against a Go server with the recording's suite, curve or key (set on the Go end); version, suite and curve in state equal on both ends | T1 |
| Client-TLSv12-ECDHE-ECDSA-AES-GCM | interop: Xo tls.dial against a Go server with the recording's suite, curve or key (set on the Go end); version, suite and curve in state equal on both ends | T1 |
| Client-TLSv12-ECDHE-ECDSA-AES128-SHA256 | interop: Xo tls.dial against a Go server with the recording's suite, curve or key (set on the Go end); version, suite and curve in state equal on both ends | T1 |
| Client-TLSv12-ECDHE-ECDSA-AES256-GCM-SHA384 | interop: Xo tls.dial against a Go server with the recording's suite, curve or key (set on the Go end); version, suite and curve in state equal on both ends | T1 |
| Client-TLSv12-ECDHE-ECDSA-CHACHA20-POLY1305 | interop: Xo tls.dial against a Go server with the recording's suite, curve or key (set on the Go end); version, suite and curve in state equal on both ends | T1 |
| Client-TLSv12-ECDHE-RSA-AES | interop: Xo tls.dial against a Go server with the recording's suite, curve or key (set on the Go end); version, suite and curve in state equal on both ends | T1 |
| Client-TLSv12-ECDHE-RSA-AES128-SHA256 | interop: Xo tls.dial against a Go server with the recording's suite, curve or key (set on the Go end); version, suite and curve in state equal on both ends | T1 |
| Client-TLSv12-ECDHE-RSA-CHACHA20-POLY1305 | interop: Xo tls.dial against a Go server with the recording's suite, curve or key (set on the Go end); version, suite and curve in state equal on both ends | T1 |
| Client-TLSv12-Ed25519 | interop: Xo tls.dial against a Go server with the recording's suite, curve or key (set on the Go end); version, suite and curve in state equal on both ends | T1 |
| Client-TLSv12-ExportKeyingMaterial | interop: Xo tls.dial against a Go server; both ends' exporter outputs equal | T9 |
| Client-TLSv12-P256-ECDHE | interop: Xo tls.dial against a Go server with the recording's suite, curve or key (set on the Go end); version, suite and curve in state equal on both ends | T1 |
| Client-TLSv12-RSA-RC4 | none: a suite Go's defaults leave out (InsecureCipherSuites; Xo offers none of them) | - |
| Client-TLSv12-RenegotiateOnce | none: renegotiation (off by default in Go, not in 0644) | - |
| Client-TLSv12-RenegotiateTwice | none: renegotiation (off by default in Go, not in 0644) | - |
| Client-TLSv12-RenegotiateTwiceRejected | refusal: a Go server that asks to renegotiate; the Xo client refuses (Go's RenegotiateNever) | T1 |
| Client-TLSv12-RenegotiationRejected | refusal: a Go server that asks to renegotiate; the Xo client refuses (Go's RenegotiateNever) | T1 |
| Client-TLSv12-SCT | interop: Xo tls.dial against a Go server; the SCTs in state equal Go's | T10 |
| Client-TLSv12-X25519-ECDHE | interop: Xo tls.dial against a Go server with the recording's suite, curve or key (set on the Go end); version, suite and curve in state equal on both ends | T1 |
| Client-TLSv13-AES128-SHA256 | interop: Xo tls.dial against a Go server; the TLS 1.3 suite set on the Xo client by T7's preference (Go's server cannot pick a 1.3 suite) | T7 |
| Client-TLSv13-AES256-SHA384 | interop: Xo tls.dial against a Go server; the TLS 1.3 suite set on the Xo client by T7's preference (Go's server cannot pick a 1.3 suite) | T7 |
| Client-TLSv13-ALPN | interop: Xo tls.dial against a Go server; the negotiated protocol on both ends equal | T1 |
| Client-TLSv13-CHACHA20-SHA256 | interop: Xo tls.dial against a Go server; the TLS 1.3 suite set on the Xo client by T7's preference (Go's server cannot pick a 1.3 suite) | T7 |
| Client-TLSv13-ClientCert-ECDSA-RSA | interop: Xo tls.dial against a Go server with client auth; the client certificate and its verified chain in the server's state | T1 |
| Client-TLSv13-ClientCert-Ed25519 | interop: Xo tls.dial against a Go server with client auth; the client certificate and its verified chain in the server's state | T1 |
| Client-TLSv13-ClientCert-RSA-ECDSA | interop: Xo tls.dial against a Go server with client auth; the client certificate and its verified chain in the server's state | T1 |
| Client-TLSv13-ClientCert-RSA-RSAPSS | interop: Xo tls.dial against a Go server with client auth; the client certificate and its verified chain in the server's state | T1 |
| Client-TLSv13-ECDSA | interop: Xo tls.dial against a Go server with the recording's suite, curve or key (set on the Go end); version, suite and curve in state equal on both ends | T1 |
| Client-TLSv13-Ed25519 | interop: Xo tls.dial against a Go server with the recording's suite, curve or key (set on the Go end); version, suite and curve in state equal on both ends | T1 |
| Client-TLSv13-ExportKeyingMaterial | interop: Xo tls.dial against a Go server; both ends' exporter outputs equal | T9 |
| Client-TLSv13-HelloRetryRequest | interop: Xo tls.dial against a Go server preferring P-256 while the Xo client shares X25519 first (T7 curve preferences); state shows the retry | T7 |
| Client-TLSv13-KeyUpdate | interop: Xo tls.dial against a Go server; a KeyUpdate from the Go end mid-stream | T1 |
| Client-TLSv13-P256-ECDHE | interop: Xo tls.dial against a Go server with the recording's suite, curve or key (set on the Go end); version, suite and curve in state equal on both ends | T1 |
| Client-TLSv13-X25519-ECDHE | interop: Xo tls.dial against a Go server with the recording's suite, curve or key (set on the Go end); version, suite and curve in state equal on both ends | T1 |
| Server-TLSv10-ECDHE-ECDSA-AES | none: TLS 1.0 and 1.1 (Xo implements 1.2 and 1.3; Go's default MinVersion is 1.2) | - |
| Server-TLSv10-ExportKeyingMaterial | none: TLS 1.0 and 1.1 (Xo implements 1.2 and 1.3; Go's default MinVersion is 1.2) | - |
| Server-TLSv10-RSA-3DES | none: TLS 1.0 and 1.1 (Xo implements 1.2 and 1.3; Go's default MinVersion is 1.2) | - |
| Server-TLSv10-RSA-AES | none: TLS 1.0 and 1.1 (Xo implements 1.2 and 1.3; Go's default MinVersion is 1.2) | - |
| Server-TLSv10-RSA-RC4 | none: TLS 1.0 and 1.1 (Xo implements 1.2 and 1.3; Go's default MinVersion is 1.2) | - |
| Server-TLSv11-FallbackSCSV | refusal: a Go client against Xo tls.server at MaxVersion TLS 1.1 with the fallback SCSV; refused with an alert (Go sends inappropriate_fallback; Xo, 1.2 and 1.3 only, protocol_version: to confirm in the pass) | T1 |
| Server-TLSv11-RSA-RC4 | none: TLS 1.0 and 1.1 (Xo implements 1.2 and 1.3; Go's default MinVersion is 1.2) | - |
| Server-TLSv12-ALPN | interop: a Go client against Xo tls.server; the negotiated protocol on both ends equal | T1 |
| Server-TLSv12-ALPN-Fallback | interop: a Go client against Xo tls.server; the negotiated protocol on both ends equal | T1 |
| Server-TLSv12-ALPN-NoMatch | interop: a Go client against Xo tls.server; no common protocol: Go's no_application_protocol alert | T1 |
| Server-TLSv12-ALPN-NotConfigured | interop: a Go client against Xo tls.server; the negotiated protocol on both ends equal | T1 |
| Server-TLSv12-ClientAuthRequestedAndECDSAGiven | interop: a Go client against Xo tls.server with client auth; the client certificate and its verified chain in the server's state | T1 |
| Server-TLSv12-ClientAuthRequestedAndEd25519Given | interop: a Go client against Xo tls.server with client auth; the client certificate and its verified chain in the server's state | T1 |
| Server-TLSv12-ClientAuthRequestedAndGiven | interop: a Go client against Xo tls.server with client auth; the client certificate and its verified chain in the server's state | T1 |
| Server-TLSv12-ClientAuthRequestedAndPKCS1v15Given | interop: a Go client against Xo tls.server with client auth; the client certificate and its verified chain in the server's state | T1 |
| Server-TLSv12-ClientAuthRequestedNotGiven | interop: a Go client against Xo tls.server with client auth; the client certificate and its verified chain in the server's state | T1 |
| Server-TLSv12-ECDHE-ECDSA-AES | interop: a Go client against Xo tls.server with the recording's suite, curve or key (set on the Go end); version, suite and curve in state equal on both ends | T1 |
| Server-TLSv12-Ed25519 | interop: a Go client against Xo tls.server with the recording's suite, curve or key (set on the Go end); version, suite and curve in state equal on both ends | T1 |
| Server-TLSv12-ExportKeyingMaterial | interop: a Go client against Xo tls.server; both ends' exporter outputs equal | T9 |
| Server-TLSv12-IssueTicket | interop: a Go client against Xo tls.server, a second connection resuming (or not, for Disabled and PreDisable); did_resume on both ends | T3, T4 |
| Server-TLSv12-IssueTicketPreDisable | interop: a Go client against Xo tls.server, a second connection resuming (or not, for Disabled and PreDisable); did_resume on both ends | T3, T4 |
| Server-TLSv12-P256 | interop: a Go client against Xo tls.server with the recording's suite, curve or key (set on the Go end); version, suite and curve in state equal on both ends | T1 |
| Server-TLSv12-RSA-3DES | none: a suite Go's defaults leave out (InsecureCipherSuites; Xo offers none of them) | - |
| Server-TLSv12-RSA-AES | interop: a Go client against Xo tls.server with the recording's suite, curve or key (set on the Go end); version, suite and curve in state equal on both ends | T1 |
| Server-TLSv12-RSA-AES-GCM | interop: a Go client against Xo tls.server with the recording's suite, curve or key (set on the Go end); version, suite and curve in state equal on both ends | T1 |
| Server-TLSv12-RSA-AES256-GCM-SHA384 | interop: a Go client against Xo tls.server with the recording's suite, curve or key (set on the Go end); version, suite and curve in state equal on both ends | T1 |
| Server-TLSv12-RSA-RC4 | none: a suite Go's defaults leave out (InsecureCipherSuites; Xo offers none of them) | - |
| Server-TLSv12-RSA-RSAPKCS1v15 | interop: a Go client against Xo tls.server with the recording's suite, curve or key (set on the Go end); version, suite and curve in state equal on both ends | T1 |
| Server-TLSv12-RSA-RSAPSS | interop: a Go client against Xo tls.server with the recording's suite, curve or key (set on the Go end); version, suite and curve in state equal on both ends | T1 |
| Server-TLSv12-Resume | interop: a Go client against Xo tls.server, a second connection resuming (or not, for Disabled and PreDisable); did_resume on both ends | T3, T4 |
| Server-TLSv12-ResumeDisabled | interop: a Go client against Xo tls.server, a second connection resuming (or not, for Disabled and PreDisable); did_resume on both ends | T3, T4 |
| Server-TLSv12-SNI | interop: a Go client against Xo tls.server with the recording's suite, curve or key (set on the Go end); version, suite and curve in state equal on both ends | T1 |
| Server-TLSv12-SNI-GetCertificate | interop: a Go client against Xo tls.server with a GetCertificate callback (NotFound: the callback's error) | T5 |
| Server-TLSv12-SNI-GetCertificateNotFound | interop: a Go client against Xo tls.server with a GetCertificate callback (NotFound: the callback's error) | T5 |
| Server-TLSv12-X25519 | interop: a Go client against Xo tls.server with the recording's suite, curve or key (set on the Go end); version, suite and curve in state equal on both ends | T1 |
| Server-TLSv13-AES128-SHA256 | interop: a Go client against Xo tls.server with the recording's suite, curve or key (set on the Go end); version, suite and curve in state equal on both ends | T1 |
| Server-TLSv13-AES256-SHA384 | interop: a Go client against Xo tls.server with the recording's suite, curve or key (set on the Go end); version, suite and curve in state equal on both ends | T1 |
| Server-TLSv13-ALPN | interop: a Go client against Xo tls.server; the negotiated protocol on both ends equal | T1 |
| Server-TLSv13-ALPN-Fallback | interop: a Go client against Xo tls.server; the negotiated protocol on both ends equal | T1 |
| Server-TLSv13-ALPN-NoMatch | interop: a Go client against Xo tls.server; no common protocol: Go's no_application_protocol alert | T1 |
| Server-TLSv13-ALPN-NotConfigured | interop: a Go client against Xo tls.server; the negotiated protocol on both ends equal | T1 |
| Server-TLSv13-CHACHA20-SHA256 | interop: a Go client against Xo tls.server with the recording's suite, curve or key (set on the Go end); version, suite and curve in state equal on both ends | T1 |
| Server-TLSv13-ClientAuthRequestedAndECDSAGiven | interop: a Go client against Xo tls.server with client auth; the client certificate and its verified chain in the server's state | T1 |
| Server-TLSv13-ClientAuthRequestedAndEd25519Given | interop: a Go client against Xo tls.server with client auth; the client certificate and its verified chain in the server's state | T1 |
| Server-TLSv13-ClientAuthRequestedAndGiven | interop: a Go client against Xo tls.server with client auth; the client certificate and its verified chain in the server's state | T1 |
| Server-TLSv13-ClientAuthRequestedNotGiven | interop: a Go client against Xo tls.server with client auth; the client certificate and its verified chain in the server's state | T1 |
| Server-TLSv13-ECDHE-ECDSA-AES | interop: a Go client against Xo tls.server with the recording's suite, curve or key (set on the Go end); version, suite and curve in state equal on both ends | T1 |
| Server-TLSv13-Ed25519 | interop: a Go client against Xo tls.server with the recording's suite, curve or key (set on the Go end); version, suite and curve in state equal on both ends | T1 |
| Server-TLSv13-ExportKeyingMaterial | interop: a Go client against Xo tls.server; both ends' exporter outputs equal | T9 |
| Server-TLSv13-HelloRetryRequest | interop: a Go client against Xo tls.server with the recording's suite, curve or key (set on the Go end); version, suite and curve in state equal on both ends | T1 |
| Server-TLSv13-IssueTicket | interop: a Go client against Xo tls.server, a second connection resuming (or not, for Disabled and PreDisable); did_resume on both ends | T3, T4 |
| Server-TLSv13-IssueTicketPreDisable | interop: a Go client against Xo tls.server, a second connection resuming (or not, for Disabled and PreDisable); did_resume on both ends | T3, T4 |
| Server-TLSv13-KeySharePreference | interop: a Go client against Xo tls.server with the recording's suite, curve or key (set on the Go end); version, suite and curve in state equal on both ends | T1 |
| Server-TLSv13-P256 | interop: a Go client against Xo tls.server with the recording's suite, curve or key (set on the Go end); version, suite and curve in state equal on both ends | T1 |
| Server-TLSv13-RSA-RSAPSS | interop: a Go client against Xo tls.server with the recording's suite, curve or key (set on the Go end); version, suite and curve in state equal on both ends | T1 |
| Server-TLSv13-RSA-RSAPSS-TooSmall | refusal: a Go client against Xo tls.server with an RSA key too small for PSS; Go's alert and text | T1 |
| Server-TLSv13-Resume | interop: a Go client against Xo tls.server, a second connection resuming (or not, for Disabled and PreDisable); did_resume on both ends | T3, T4 |
| Server-TLSv13-Resume-HelloRetryRequest | interop: a Go client against Xo tls.server, a second connection resuming (or not, for Disabled and PreDisable); did_resume on both ends | T3, T4 |
| Server-TLSv13-ResumeDisabled | interop: a Go client against Xo tls.server, a second connection resuming (or not, for Disabled and PreDisable); did_resume on both ends | T3, T4 |
| Server-TLSv13-X25519 | interop: a Go client against Xo tls.server with the recording's suite, curve or key (set on the Go end); version, suite and curve in state equal on both ends | T1 |

## Consequences

- Programs can serve TLS on any accepted connection (custom protocols,
  STARTTLS servers, tests that need a TLS peer) without std/http.
- The prelude's Conn and ServerConn have three more methods; user types
  implementing Stream or NetConn are unaffected (neither interface
  gained them).
- Wasm: TLS is still not available in a command (spec core's wasm
  paragraph); the prelude methods work on wasip2 sockets.
- 75 recordings are B1 interop cases, 20 wait for later pieces (T3, T4,
  T5, T7, T9, T10), and 25 test refusals or are out of scope (TLS 1.0
  and 1.1, RC4, 3DES, renegotiation).

## Alternatives

- One Config for both sides as Go has: most fields apply to one side
  only, and Xo values cannot share state the way a Go *Config does.
- Named parameters on `tls.server` as on `tls.client`: past a dozen
  options, and the callbacks, a struct reads better and composes.
- One TlsConn for both sides with `uses net`: it widens every client
  call's effect and loses the dial/listen split 0280 made.
- Handshake inside `server` or `accept` (as a first draft had it):
  one silent client would block the accept loop until its deadline; Go
  defers the handshake for the same reason.
