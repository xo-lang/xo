# 0281: A TLS client on an existing connection (std/tls)

Status: Accepted (owner, 2026-10-20), as drafted, with the `xo vet`
warning on `Verify.None` in the same change (XO0902; its message names
`ChainOnly` and `roots:` as the safer choices). Implemented 2026-10-20
(Xo Lang 3): spec/std.md 27, std/tls.xo, std/native/tls.xo,
std/rt/tls.go, golden tls_client. Until decision 0280 lands, a TlsConn's
read and write are `uses net`, as Conn's are; they narrow to `net.dial`
with it.

## Context

The native Postgres driver (decision 0278) dials TCP, sends SSLRequest
in plaintext, reads one byte (`S`), then runs the TLS client handshake
on the same socket. SMTP STARTTLS and Redis TLS have the same shape. Xo
has TLS only inside std/http (4.6: `with_tls`, `with_roots`), so a
driver cannot upgrade a connection it dialed itself.

libpq has three verification levels the driver must map `sslmode` onto:
`verify-full` (chain and host name), `verify-ca` (chain only), and
`require` (encryption without verification, common with managed
databases whose CA is not in the system roots).

The prelude cannot hold it: the TLS types (`TlsErr`, roots) are std's,
and a prelude method on `Conn` would pull them into every program. The
native implementation exists: llrt `tlsio.client(pd, host, roots, dl)`
runs the client handshake over a poll descriptor (the http client's
path), and tls12.Conf has `insecure`.

## Decision (proposed): the spec section

A new section of spec/std.md, `std/tls` (section 27):

```
use std/tls

enum Verify { Full, ChainOnly, None }
struct Roots {}                                    // handle: trusted certificates
struct TlsErr { msg: Str }
pub fn roots(pem: Bytes) -> Roots ! TlsErr
struct TlsConn {}                                  // handle
pub fn client(c: Conn, host: Str, roots: Roots? = None, verify: Verify = Verify.Full, alpn: List[Str] = []) -> TlsConn ! NetErr uses net.dial
fn (t: TlsConn) read(max: Int) -> Bytes? ! NetErr  uses net.dial
fn (t: TlsConn) write(data: Bytes) -> () ! NetErr  uses net.dial
fn (t: TlsConn) close()
fn (t: TlsConn) alpn() -> Str                      // the protocol the server chose, "" for none
fn (t: TlsConn) version() -> Str                   // "TLS 1.2" or "TLS 1.3"
```

```
fn pg_tls(os: Os, addr: Str, host: Str) -> tls.TlsConn ! Error uses net.dial {
  let c = os.net.dial(addr)?
  c.write(ssl_request())?
  match c.read(1)? {
    Some(b) if b == b"S" => {}
    _ => return Err(Error.msg("server refused TLS"))
  }
  tls.client(c, host)?
}
```

- `client` runs a TLS 1.2 or 1.3 client handshake (Go's default cipher
  suites, curves, and ALPN behavior) on `c` and returns the encrypted
  connection. `c` belongs to the TlsConn from then on: a later `read`
  or `write` on `c` fails with `Io("use of a connection upgraded to
  TLS")`; closing the TlsConn sends close_notify and closes `c`. `host`
  is the server name sent by SNI (not for an IP address) and the name
  verified.
- `verify`: `Full` checks the chain against the roots and the host name
  (an IP address against the certificate's IP addresses), as the http
  client does (4.6); `ChainOnly` checks the chain but not the name
  (libpq's `verify-ca`); `None` checks nothing and only encrypts
  (`require`): it accepts any certificate, so it does not stop an active
  attacker. `xo vet` reports each use (`XO0902`).
- `roots`: `None` is the system's roots, as the http client's
  (macOS's trust settings, Linux's CA bundle and directories with
  `SSL_CERT_FILE` and `SSL_CERT_DIR`); `tls.roots(pem)` trusts exactly
  the certificates of pem, with http's `with_roots` parser and its
  errors (`TlsErr("tls: no certificates found in PEM input")`).
- Errors are `NetErr`s with Go's texts: a certificate that does not
  verify is `Io("tls: failed to verify certificate: x509: ...")`; a
  server that does not speak TLS is `Io` with Go's handshake text. The
  handshake must finish within 10 seconds, else `Timeout`; it is a
  cancellation point (`Canceled`, core 7.2).
- `alpn` lists the protocols offered (none by default); `alpn()` is the
  one the server chose.
- Tests: on FakeNet `client` does no TLS: the TlsConn reads and writes
  the connection's plain bytes, `alpn()` is "" and `version()` is
  "TLS 1.3", so a driver's tests can run an in-process fake server,
  SSLRequest included. Real TLS is covered by Docker differential tests
  against Go.
- Backends: native (llrt tlsio over the connection's descriptor) and Go
  (crypto/tls.Client on the net.Conn; `ChainOnly` as
  InsecureSkipVerify with a VerifyPeerCertificate that checks the chain;
  `None` as InsecureSkipVerify). Wasm has no Net.
- Effects (decision 0280): `client` and the TlsConn's read and write are
  `net.dial`, since only a dialed `Conn` is upgraded; a server side
  (`ServerConn`) is not part of this decision.

## Consequences

- The Postgres driver (0278) maps sslmode `verify-full`, `verify-ca`,
  `require` to `Full`, `ChainOnly`, `None` by name; `disable` does not
  call `client`; `prefer` and `allow` are the driver's (try TLS, fall
  back).
- std/http keeps its own `TlsErr` and `with_roots`; a later change may
  make `http.TlsErr` the same type as `tls.TlsErr`.
- Record and replay (core 11.3): open question. Recording the
  connection's ciphertext cannot be replayed (the handshake's
  randomness); recording the TlsConn's plaintext reads and writes, as a
  replayed `Conn` is, is the likely answer.

## Alternatives

- **A prelude method `Conn.tls_client`** (the first sketch): the TLS
  types would join the prelude, and every program would carry them.
- **TLS inside each driver**: every driver would reimplement the
  verification levels.
- **Only `Full`**: breaks existing `sslmode=require` DSNs for managed
  databases, the common case Lib 1 reported.
