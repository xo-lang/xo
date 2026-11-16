# 0687: HTTP/2 extended CONNECT (RFC 8441) and WebSockets over it

Status: Accepted (owner, 2026-11-16, as proposed; proposed by Xo Lang 3, 2026-11-16; task #89, map.txt's
`http2.SettingEnableConnectProtocol` gap). Changes decision 0121's "no
extended CONNECT" and 0313's "not now: WebSockets over HTTP/2".

## Context

RFC 8441 lets an HTTP/2 client open a tunnel on one stream: a CONNECT
request with a `:protocol` pseudo-header (`websocket`), sent only after
the server's SETTINGS carry `SETTINGS_ENABLE_CONNECT_PROTOCOL` (0x8) = 1.
The server answers 200 and both sides then exchange DATA frames on that
stream for as long as the tunnel lives.

Go 1.26's bundled HTTP/2 (net/http h2_bundle.go) implements both sides
but keeps them off by default (`http2disableExtendedConnectProtocol`,
enabled only by `GODEBUG=http2xconnect=1`, read once at init; issue
#71128: advertising it made browsers try WebSockets over HTTP/2 against
servers whose websocket package could not answer). Off, a `:protocol`
pseudo-header is a stream PROTOCOL_ERROR; on, the server advertises the
setting, accepts CONNECT with `:protocol` and `:path`/`:scheme`, and
hands the handler a request whose header holds `:protocol`; the client
waits for the server's first SETTINGS before sending such a request and
fails it when the setting is absent.

Xo today (decision 0121): the native server parses `:protocol` nowhere,
and a handler starts only once the request body is complete, so a
tunnel, whose body never completes, cannot reach a handler. std/rt (the
Go backend) runs net/http, so it inherits Go's default off.

## Decision (proposed)

1. **Off by default, as Go**: `http.Server` gains `extended_connect: Bool
   = false`. On, the server's SETTINGS carry ENABLE_CONNECT_PROTOCOL = 1;
   off, a `:protocol` is the stream error Go gives (PROTOCOL_ERROR),
   which is today's behavior made explicit.
2. **The request**: `Request.protocol: Str` (empty when absent; Go puts
   it in the header as `:protocol`, which Xo's Headers do not admit).
   Go's validation: CONNECT with `:protocol` needs `:path` and
   `:scheme`; CONNECT without it keeps today's rule.
3. **A tunnel handler**: a stream with `:protocol` starts its handler at
   the HEADERS frame, with the request body as a Stream read while it
   arrives and the response written as DATA frames as the handler writes
   (a `Tunnel` value: read, write, close, as `Conn`), flow control as
   for any stream. This is the one change to 0121's model, limited to
   extended CONNECT streams; ordinary requests keep "the handler starts
   once the body is complete".
4. **The client**: `Transport` sends a request with `protocol` set only
   after the server's first SETTINGS, and fails it with Go's error text
   (`http2: extended connect not supported by peer`) when the setting is
   absent.
5. **WebSockets over it** (std/websocket): `websocket.accept` answers an
   extended CONNECT with `:protocol` websocket (200, no
   Sec-WebSocket-Key or Accept, RFC 8441 section 5) when the server
   enabled it; `websocket.dial` over an HTTP/2 connection uses it when
   the server advertised the setting, else falls back to HTTP/1.1 as
   today.
6. **The Go backend**: std/rt cannot set GODEBUG after net/http's init;
   it runs the server process with `GODEBUG=http2xconnect=1` set in the
   environment of the built program when a Server enables it (checked
   before stage 1: whether Go 1.26's http.HTTP2Config has a field for it,
   which would be preferred).

Stages, each its own push: (1) the setting, `:protocol` parsing and the
off-by-default error, with h2 frame tests against Go's server and client
(TestH2Diff's style); (2) the tunnel handler and client on the native
backends; (3) std/websocket over it, Autobahn's cases over HTTP/2 where
the suite allows, and a browser check (Chrome speaks it); (4) the Go
backend.

## Consequences

- map.txt's `http2.SettingEnableConnectProtocol` row maps; 0313's
  websocket gap for RFC 8441 closes at stage 3.
- A server that enables it must answer WebSockets over HTTP/2 (std/
  websocket does from stage 3), the trap Go's #71128 describes; the
  option's doc says so.
- The native server gains a second handler start point (stage 2), which
  needs its own tests for flow control and stream reset during a tunnel.

## Alternatives

- **On by default**: browsers would try WebSockets over HTTP/2 against
  any Xo server; one whose handler does not use std/websocket would
  break those clients (#71128). Go chose off.
- **Only the setting and parsing, no tunnel**: maps the row in name only;
  no program could use it.
- **A general full-duplex handler for every request**: larger change to
  0121's model (every handler would start at HEADERS); not needed for
  this row.
