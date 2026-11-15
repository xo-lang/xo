# 0644: std/tls gains a public server, Config values for client and server, and Go's crypto/tls surface (T1 to T14)

Status: Proposed (lang3, 2026-11-15; scope approved by the owner through the
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

## Decision (draft; filled in as batch B1 lands)

- `tls.ServerConfig` and `tls.ClientConfig` are plain struct values with
  Go's defaults as field defaults (Xo has no shared mutable Config);
  `tls.client` keeps its named parameters and becomes `client_with` of a
  ClientConfig built from them.
- `tls.server(c: Conn, config: ServerConfig) -> TlsConn ! NetErr` runs the
  server handshake on an accepted Conn; `tls.listen` wraps a Listener so
  `accept` returns a TlsConn after its handshake. std/http's with_tls
  builds a ServerConfig and calls the same code.
- `TlsConn.state() -> ConnState` is Go's ConnectionState (version, cipher
  suite, curve, server name, ALPN, resumed, peer certificates, verified
  chains, OCSP, SCTs, exporters); the existing accessors stay.
- Callbacks are function fields of the configs, each with the effects
  of the code that calls it declared on the field's type.
- The remaining pieces (resumption, ticket keys, preferences, key log,
  OCSP and SCTs, Dialer, typed errors, ECH, QUIC) are fields and
  functions of the same two configs and of TlsConn, in Go's names.

## Consequences

To be written with B1.

## Alternatives

- One Config for both sides as Go has: most fields apply to one side
  only, and Xo values cannot share state the way a Go *Config does.
- Named parameters on `tls.server` as on `tls.client`: past a dozen
  options, and the callbacks, a struct reads better and composes.
