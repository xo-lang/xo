# 0541: TLS client hooks: verify_conn, server name, request Host

Status: Accepted (owner, 2026-11-07, relayed by xo-lead; proposed by Xo Lang 3, 2026-11-06; requested by Xo Azure 1 for azidentity's Service Fabric thumbprint pin and the
workload identity token proxy)

## Context

An Xo client could not see or pin the server's certificate, set the TLS server name apart from the URL's host, or send a
Host that differs from the URL's. Go's azidentity needs all three: Service Fabric managed identity pins the SHA-1 of the
leaf certificate (`InsecureSkipVerify` plus `VerifyConnection`), the Kubernetes token proxy sets `tls.Config.ServerName`,
and its transport leaves `Request.Host` as the original authority while the connection goes to the proxy.

## Decision

1. `Client.with_verify_conn(f: fn(List[x509.Certificate]) -> () ! Str)` (Go's `tls.Config.VerifyConnection`): called after
   the handshake and the verification mode's own check, with the server's chain leaf first, before any request byte; an
   `Err` text fails the connection with that text. It works with `Verify.None`.
2. `Client.with_server_name(name)` (Go's `tls.Config.ServerName`): the SNI and the name verified, "" the URL's host.
3. `Request.with_host(host)` (Go's `Request.Host`): the Host of HTTP/1.1 and the `:authority` of HTTP/2 for the first
   host's requests; the connection and the TLS name stay the URL's.
4. A client with a server name or hook is its own pool key: its TLS settings ride on the Roots record the client already
   carries (as the client certificate of decision 0433 does), so it never shares or reuses a pooled connection with a client
   without them (Go clears `TLSNextProto` for the same reason).
5. `Client.tls_options()` reports what a client has set (server name, hook, custom roots, client certificate) for tests.
6. `tls.pin_sha1(hex)` and `tls.pin_sha256(hex)` make a `verify_conn` function that compares the digest of the leaf's DER
   to the pin in constant time (colons and spaces in the pin ignored); `tls.client` takes the same hook as `verify_conn`.

## Consequences

- No new TLS state outside the Roots record; no change to a client that sets none of these.
- Amendment (2026-11-07, Azure 1's Service Fabric item): `Client.with_verify(tls.Verify)` lets a client skip chain and name checks (Go's InsecureSkipVerify) and `with_roots` keeps the name, hook and mode. `Verify.None` is refused at connect time unless a `with_verify_conn` is set, and the spec marks it unsafe alone.
- The hook runs once per new connection (a pooled connection was checked when made).

## Alternatives

- A pin option only (`with_pinned_cert`): narrower; the hook covers it and Go's own shape.
- A `Host` header honored on the client: would change requests that forward a server request's headers.
