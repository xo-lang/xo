# 0244: TLS and HTTP/2 linked only when a program configures TLS

Status: Proposed (2026-10-16, Xo lang 3; for the owner, through the lead)

## Context

Every native program that runs an `http.server` links the whole TLS 1.2
and 1.3 server, X.509, the cipher suites, and the HTTP/2 server, whether
or not it calls `Server.with_tls`
(bench/results/binary-size-native-2026-10-16.md):

- json_server, a plain HTTP JSON server, is 2,664 KiB stripped on
  darwin/arm64, of which 1,105 KiB (52 percent of its code) is TLS and
  crypto code and about 104 KiB is h2srv and hpack. Rust's tiny_http
  version is 755 KiB, Go's `-s -w` 5,564 KiB.
- The reason is the data path, not the linker: the arm64 and LLVM
  backends emit only what the SSA builders reach from main and the C ABI
  roots. hconn.xo reads a connection's `HC_TLS` word at run time and
  calls `tlssrv.hc_read`, `tlssrv.write`, `tlssrv.nb_read`; hserve.xo
  calls `tlssrv.handshake`, `tlssrv.alpn_h2`, `tlssrv.conn`,
  `tlssrv.free`, `tlssrv.close_notify` and `h2srv.conn_new` and its
  friends; std/native/http.xo calls `h2srv.conn` for HTTP/2 requests.
  All of those are reachable from `Server.serve`, so all of TLS is.

Measured (2026-10-16, main at d764faf9 plus the gzip fix): in a
throwaway std copy the 13 `tlssrv` and 10 `h2srv` entry points those
modules call were made `fault("measurement stub")`, which is what the
linker sees when they are unreachable. json_server went from 2,727,600
to 1,291,000 bytes stripped (minus 53 percent, 2,664 to 1,261 KiB); TLS
and crypto code from 1,131,248 to 6,220 bytes; HTTP and net from 422,904
to 383,804 (h2srv, hpack). The program's behavior over plain HTTP is
unchanged by construction (those calls only run when `HC_TLS` is set).

## Decision

TLS server code is reachable only from `Server.with_tls`:

1. hserve.xo keeps, per server record (`SR_TLS` today), a pointer to a
   small table of TLS operations: handshake, alpn_h2, conn, hc_read,
   nb_read, write, close_notify, free, srv_free, bad_cipher, and the
   HTTP/2 connection entry points (conn_new, conn_stop, conn_free,
   stream_release, emit, respond_at, finish, end, and http.xo's
   `h2srv.conn`). The table is a static record of function addresses
   built in tlssrv.xo (`tlssrv.ops()`), the way pool.xo already calls
   `TW_KICK` through an address the test world installs.
2. `Server.with_tls` (std/native/http.xo) is the only caller of
   `tlssrv.ops()` and stores the table in the server record with the
   certificates. hconn and hserve call through the table, and never name
   tlssrv or h2srv directly; a plain server's table is 0 and the TLS
   branches are never taken (they are not today either).
3. HTTP/2 stays TLS only (no h2c, decision 0121), so it rides on the
   same table.
4. A regression test: bench/size/native/measure.sh's json_server, or a
   repotest that builds a plain `http.server` and checks that no
   `_xo_f_tls13srv` or `_xo_f_h2srv` symbol is in it.

## Consequences

- A plain HTTP server is about half its size: json_server about 1,261
  KiB instead of 2,664 (measured upper bound), notes-service about 1.7
  MB instead of 3.1 (it keeps SHA-256 and Argon2id, which it uses).
  Go's `-s -w` build of the same server is 5,564 KiB.
- An HTTPS server is unchanged in size; each TLS read, write, and
  handshake call goes through one indirect call, noise next to the
  record cryptography.
- No API or behavior change; spec std.md 4.4 and 4.6 unchanged.
- The client side is not covered: `http.client` decides http or https
  per URL at run time, so a client links its TLS client (about 900 KiB
  in https_get) even for plain URLs. Making that lazy would need the
  same table set by the first https request, which still keeps it
  reachable; only an API change (an opt in per client) would drop it.
  Not proposed.

## Alternatives

- Linker dead stripping: does not help; the code is reachable, only not
  reached at run time.
- A separate `std/https` module or a build flag: an API change for a
  linking problem; rejected.
- Compiler analysis that proves `with_tls` is never called and folds
  `HC_TLS` to 0: a whole program constant propagation through memory
  words; far more work and fragile next to the function table.
