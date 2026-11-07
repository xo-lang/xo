# 0544: FakeNet TLS: handle_tls runs the client's certificate checks in memory

Status: Proposed (Xo Lang 3, 2026-11-07; requested by Xo Azure 1 to test azidentity's Service Fabric thumbprint pin, whose wrong-pin case needs a client check that FakeNet never ran)

## Context

A test's FakeNet answers `https` URLs without TLS: `Client.with_roots`, `with_server_name`, `with_verify` and
`with_verify_conn` (decision 0541) are never exercised, so a wrong pin, a wrong name or an untrusted certificate cannot be
tested without a real listener. A FakeNet handler also sees `req.host()` as "" unless the client called `with_host`, where
Go's `Request.Host` and the native server give the URL's authority.

## Decision

`FakeNet.handle_tls(pattern, cert: tls.TlsCert, h)` registers a handler like `handle` with a certificate. For an `https`
request it matches, FakeNet first runs the client's checks against `cert` and only then calls `h`:

- the chain of `cert` against the client's roots (the system's, or `with_roots`), skipped by `Verify.None`;
- the name (`with_server_name`, else the URL's host) against the leaf's names, skipped by `Verify.None` and `ChainOnly`;
- `with_verify_conn`, called with the chain, with its text on failure;
- `Verify.None` without a `verify_conn` is refused, as for a real connection (decision 0541).

A failed check fails the request with the text of the real handshake (`ClientErr.Connect`) before the handler runs, and the
request is not recorded as delivered. The handler's `req.tls_state()` reports `server_name` (the SNI the client sent),
`version` "TLS 1.3", the cipher suite and the peer certificates (`cert`'s chain). A plain `handle` on an `https` URL keeps
today's behavior (no checks). FakeNet handlers' `req.host()` defaults to the URL's host and port.

## Consequences

- Pinning, name, roots and mode tests need no listener and no ports.
- The check code is the real handshake's (tlscert verification on the native backends, crypto/x509 on the Go backend), so
  the texts match what a real server gives.
- `host()` changes for existing FakeNet tests that print it (from "" to host:port).

## Alternatives

- Tell tests to serve a real in memory listener with `with_tls` (works today, but FakeNet's `fail`, `requests` and pattern
  routing are not available there).
- Run a whole in memory handshake: more faithful, much more code, and the verification outcome is the same.
