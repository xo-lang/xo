# 0434: HTTP/2 without TLS (h2c prior knowledge) and HTTP/2 settings

Status: Accepted (owner, 2026-10-30, through the lead). Was: Proposed (Xo lang 5, 2026-10-30; Xo Lang 3 reviewed it the same
day, and the sniff, config, keepalive and linking notes are theirs; for
the owner's review). Closes gap 10 of the gRPC readiness survey, after 0433
(trailers, full duplex, mutual TLS).

## Context

Plain `http` is always HTTP/1.1 (decision 0121, std.md 4.4: "no h2c").
gRPC inside a service mesh speaks HTTP/2 over plain TCP to a sidecar
that terminates TLS, with prior knowledge: the client opens with the
HTTP/2 preface (`PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n`), with no Upgrade.
Go 1.24 added this to net/http: `Server.Protocols` and
`Transport.Protocols` (an `http.Protocols` set of HTTP1, HTTP2 and
UnencryptedHTTP2) and `http.HTTP2Config` for the settings that were
only reachable through x/net/http2 before. The HTTP/1.1 `Upgrade: h2c`
dance (RFC 7540 3.2) is deprecated by RFC 9113 and net/http does not
do it; grpc-go never used it.

h2srv.xo's limits are fixed constants today (250 streams, 1 MiB frames
and windows, a 4096 byte HPACK table, a 1048896 byte header list).

## Decision

Follow Go 1.24's shape, so the Go backend maps one to one.

```
enum Protocol { Http1, Http2, UnencryptedHttp2 }
fn (s: Server) with_protocols(p: Set[Protocol]) -> Server
fn (c: Client) with_protocols(p: Set[Protocol]) -> Client
struct H2Config {
  max_concurrent_streams: Int = 0       // 0: 250
  max_read_frame_size: Int = 0          // 0: 1 MiB; 16384 to 16 MiB
  max_receive_buffer_per_connection: Int = 0   // 0: 1 MiB
  max_receive_buffer_per_stream: Int = 0       // 0: 1 MiB
  max_decoder_header_table_size: Int = 0       // 0: 4096
  max_encoder_header_table_size: Int = 0       // 0: 4096
  send_ping_timeout: Duration = 0       // idle time before a PING; 0: none
  ping_timeout: Duration = 0            // the wait for its ack; 0: 15 s
  write_byte_timeout: Duration = 0      // no write progress closes the connection; 0: none
}
fn (s: Server) with_h2_config(c: H2Config) -> Server
fn (c: Client) with_h2_config(c: H2Config) -> Client
```

- Default (no `with_protocols`): `#{Http1, Http2}`, today's behavior.
- Server with `UnencryptedHttp2`: on a plain connection the server
  reads the first bytes; the 24 byte preface hands the connection to
  the HTTP/2 server, anything else to HTTP/1.1 (if `Http1` is in the
  set; otherwise the connection closes, as Go). The sniff runs only
  when `UnencryptedHttp2` is in the set, so the default HTTP/1.1 path
  pays nothing; it reads at most 24 bytes, keeps reading only while the
  bytes so far are a prefix of the preface, and otherwise hands them,
  unconsumed, to the HTTP/1.1 parser, under the read header timeout (a
  client that sends only `PRI` times out like a slow request line and
  delays no other connection).
- Client with `UnencryptedHttp2` and without `Http1`: an `http://` URL
  opens HTTP/2 with prior knowledge (Go's rule); with both, HTTP/1.1 is
  used, as Go, since the client cannot know the server speaks h2c.
- Over TLS, `Http2` absent removes `h2` from ALPN; `Http1` absent
  removes `http/1.1`.
- An empty set, or a TLS server with neither `Http1` nor `Http2`, is a
  programming error reported when `serve` starts (Go panics).
- `H2Config` zero fields take today's values; out of range values are
  clamped as Go clamps them. `send_ping_timeout` sends a PING after that
  much read silence and closes the connection when no ack arrives within
  `ping_timeout` (gRPC keepalive's role); the keepalive uses the
  connection's existing park and deadline path, not a timer task per
  connection.
- The config is copied into the connection record once, at connection
  setup; the hot paths read the record, never the Server, and no frame
  does a lookup. grpc-go's MaxConnectionAge is
  not part of net/http and stays out.
- No `Upgrade: h2c` support (RFC 9113, Go).
- Decision 0244: h2c needs the HTTP/2 server without TLS, so h2srv is
  linked when a program uses `with_protocols` with `UnencryptedHttp2`
  or TLS, and the TLS code stays unlinked for an h2c only program. This
  is a file move: the h2 exports (the `srvtls_h2_*` functions and their
  TO_H2* table entries) leave cabi_tls.xo, which imports tlssrv, for a
  module that imports h2srv only, installed by its own small installer
  (`install_h2c()`) beside the TLS one.

### Split of the work

- Xo Lang 3: the preface sniff in the connection accept path, h2srv's
  limits read from a config record instead of constants, the PING timer,
  the h2 exports moved out of cabi_tls.xo,
  the client's prior knowledge dial, ALPN lists from the set.
- Xo lang 5: the API, spec/std.md (4.4's "no h2c" sentence changes), the
  Go backend mapping (`http.Protocols`, `http.HTTP2Config`), goldens with
  Go parity, the std change log.

### Tests

- Goldens on every backend with Go parity: an h2c server answering both
  an h2c client and an HTTP/1.1 client on one port; an h2c only server
  closing an HTTP/1.1 connection; a client with only
  `UnencryptedHttp2`; ALPN with one protocol removed;
  `max_concurrent_streams` enforced (REFUSED_STREAM past the limit, as
  Go); a PING timeout closing a silent connection.
- Go differential: grpc-go's interop client against an Xo h2c server,
  in Docker on the offload hosts, with 0433's TLS run.

## Consequences

- gRPC behind a sidecar works; with 0433, grpc-go interop is complete
  for unary and streaming calls.
- A Server and a Client carry a protocol set and a config record; the
  defaults keep every program's behavior.
- h2srv's constants become per connection values, a load per use on a
  record read; Xo Lang 3 measures it with `--ab` on the linux/arm64
  host (ab-host.sh) with an h2 echo and a many streams program.

## Alternatives

- `Server.with_h2c()` and `Client.with_h2c()` booleans: shorter, but do
  not express "HTTP/2 only" or removing a protocol from ALPN, and do not
  map to Go's Protocols.
- Supporting `Upgrade: h2c`: deprecated, no gRPC client needs it, and it
  needs a second request path.
- Separate setters for each limit (`with_max_concurrent_streams`, ...):
  rejected for one record mirroring Go's HTTP2Config.
