# 0202: TLS written in Xo replaces Mbed TLS

Status: Accepted (owner, 2026-10-13); not started. Last in decision
0195's order.

## Context

Decision 0195 aims for no C that Xo compiles. The zero-C audit
(bench/results/zero-c-audit-2026-10-13.md, "Owner decisions found", A)
found Mbed TLS (third_party/mbedtls, 196,689 raw lines, much of it
configured out) and fiat-crypto (4,226 lines) are the largest C left,
all of it blocked on constant time code (prerequisite 5). It offered
three end states: (a) an Xo TLS stack after Go's design; (b) Mbed TLS as
the one permanent C exception; (c) the platform's TLS (Security.framework,
SChannel), which leaves Linux needing OpenSSL as a system library.

## Decision

(a): TLS and the crypto under it are written in Xo, following Go's
crypto/tls and crypto design, and replace Mbed TLS and fiat-crypto. It
comes last in decision 0195's order, after the constant time
prerequisite (5) exists. The constant time support stays internal to
std (decision 0074) unless the owner decides to expose a user marker.

## Consequences

- No C remains in the TLS path; one implementation on every target,
  including Linux without a system TLS library.
- New, security critical protocol code in Xo: it needs test vectors
  (Wycheproof, RFC vectors), interoperability tests against major
  servers and clients, differential tests against the current Mbed TLS
  path, and fuzzing, before the C path is removed (decision 0193's
  mechanism: the C path stays selectable until then).
- Constant time code must survive the optimizer on every backend: the
  prerequisite (5) design must make branches and memory accesses on
  secret values impossible to introduce, and be checked (for example by
  timing tests and inspecting the emitted code).
- 2026-10-14 (B31 part 2, nistec.xo, bigmod.xo, rsa.xo): every secret
  dependent selection goes through ct.xo's masks (ct.mask_eq,
  ct.mask_bit, ct.pick, each behind rt.ct_barrier): the scalar and
  exponent windows' table reads, the Montgomery product's final
  subtraction, modular subtraction's add back. Carries and borrows are
  computed with bitwise operations (bits.Add64's formulas), which an
  optimizer may turn into flag arithmetic (adc, sbc) but has no reason to
  turn into a branch; the places where it could still matter are the
  carry chains in nistec.xo and bigmod.xo (mac, adc, sbb), which have no
  barrier, and are worth adding to test/codegen/consttime when its
  patterns grow. The branches that remain are on public values: the
  exponent bits of an inversion (m - 2), a public RSA exponent,
  rejection sampling of a nonce (only the accepted one is used), an
  ECDSA signature's r or s being zero, and the RSA fault check.
- 2026-10-14 (B31 part 3, x509verify.xo): chain verification follows
  the Go release the reference runs (1.26), whose Verify checks DNS name
  constraints per chain after building the chains (older releases checked
  them while building), so the outcome codes differ from older Go when a
  chain both breaks a constraint and lacks a usage. When several chains
  verify, Go reports the first one its pools list (candidates with a
  matching key identifier first), x509verify.xo the first in the order
  given (roots, then intermediates); the tls_verify golden has one valid
  chain per case. Everything chain verification reads is public, so it
  has no constant time requirement.
- 2026-10-15 (B31 part 4, tls13.xo and tlsrec.xo): the client is sans
  I/O (feed the server's bytes, take the bytes to send), so the TLS
  boundary keeps the sockets and the tests replay traces without a
  network. Its secrets go through hkdf.xo, hmac.xo and the AEADs; the
  server's Finished is compared with ct.bytes_equal; a record's padding
  is scanned from the end after the AEAD opened it, which shows only the
  padding length (RFC 8446 5.4 allows this). The test hooks (a fixed
  ClientHello and ECDHE secret) are Conf fields that the TLS path leaves
  empty. Certificate expiry texts use UTC, where Go's use the local
  zone; the integration passes the zone offset as tlscert.xo does.
- 2026-10-15 (B31 part 7, tlsio.xo): the HTTP client's TLS is the Xo
  client by default, with Mbed TLS behind decision 0193's switch
  (XO_LLRT_C=tlsio, tlsio_c.xo). The lead accepted the TLS 1.2 gap on
  the zero-c branch: zero-c does not merge into main before the TLS 1.2
  client is in. The connection's state lives in an rt.Any kept by
  address (replaced after each call, values being values), its roots in
  the C's records until the root loading moves to Xo.
- 2026-10-15 (B31 part 8): the client negotiates the version in one
  ClientHello, as Go: tls13.xo builds it (TLS 1.2's suites and
  extensions when its Conf has legacy_suites), checks the downgrade
  sentinel, and stops in HANDOFF when the server picks 1.2; tls12.xo's
  resume goes on from the ServerHello with the hello's random, session
  id and transcript. A failed 1.3 handshake is never retried as 1.2
  (that would let an attacker force the downgrade). The differential
  fuzzing compares behavior (state, bytes sent, error class) and counts
  differences only in a refused certificate's text separately.
- 2026-10-15 (B31 part 9): x509.xo accepts exactly what Go's
  crypto/x509 accepts: its parser is a port of parser.go over cbyte.xo
  (cryptobyte's strict reading), checked by TestX509ParseDiff. Where
  the Xo stack cannot do what Go does it refuses instead: P-224 and P-521
  signatures, and IP, e-mail and URI name constraints over certificates
  with such names. A verification's texts are Go's, including the
  unknown authority's hint, so the TLS fuzz's text-only differences
  must stay at zero.
- 2026-10-15 (B31 part 11): name constraints follow Go 1.26's
  constraints.go algorithm itself (sort, prune, binary search), not only
  its results, because the error text names the constraint the search
  finds. Certificate policy validation (Go's policiesValid) comes next;
  until then it is a known gap where Xo accepts what Go refuses.
- 2026-10-15 (TLS 1.2 client, tls12.xo): TLS 1.2 is in scope as Go's
  defaults (ECDHE over X25519, P-256 and P-384; ECDSA, RSA PKCS #1 v1.5,
  RSA-PSS and Ed25519 signatures; the AES-GCM and ChaCha20-Poly1305
  suites; the extended master secret; ALPN and SNI; no CBC, RSA key
  exchange, renegotiation, or resumption). Its record layer is its own
  (an explicit GCM nonce, the TLS 1.2 additional data), not tlsrec.xo.
  The golden's reference is crypto/tls itself, run over the recorded
  bytes with a repeatable random source
  (gotchas/go126-tls-ignores-config-rand.md), so the Xo client must
  produce Go's bytes and error texts exactly. The server is tls12srv.xo.
- 2026-10-15 (TLS 1.2 version negotiation, lead): as Go's client, one
  ClientHello offers TLS 1.3 and 1.2; tls13.xo sends it and, on a TLS
  1.2 ServerHello, stops in HANDOFF; tlsio.xo calls tls12.resume(conf,
  random, sid, hello, plaintext from the ServerHello on), then
  tls12.feed with the unread bytes. A failed handshake is never retried
  with a 1.2 only hello (that would be a downgrade hole). Both sides
  check the downgrade sentinels (DOWNGRD 01 and 00).
- 2026-10-15 (B31 server, tls13srv.xo): the TLS 1.3 server is sans I/O
  like the client and follows Go's handshake_server_tls13.go: Go's suite
  order (AES-GCM first unless the client lists ChaCha20-Poly1305 first),
  the server's group preference with a key share first, else a
  HelloRetryRequest, Go's ALPN rule (with the h2 to http/1.1 fallback),
  the signature scheme in the client's order among the certificate's,
  the dummy ChangeCipherSpec always, and Go's error texts. It sends no
  session tickets and asks for no client certificate (Go's defaults
  less tickets: no resumption yet). The client's Finished is compared
  with ct.bytes_equal. Test hooks (Conf: random, ECDHE key, PSS salt,
  the RFC 8448 encoding, fixed EncryptedExtensions, a ticket) are empty
  on the TLS path. Checked by golden tls13_server: RFC 8448 section 3
  byte for byte from the server's side (its PSS salt recovered from the
  RFC's signature), recorded flights of Go 1.26's crypto/tls client
  (its X25519MLKEM768 share ignored), and the in-memory pair of
  tls13.xo's client and this server against Go's client and server.
- 2026-10-15 (TLS 1.2 server, tls12srv.xo): tls13srv.xo reads the first
  ClientHello; one offering 1.2 and not 1.3 stops it in HANDOFF, and
  tlssrv.xo calls tls12srv.resume(conf, hello, rest). No client
  certificates, tickets, or renegotiation. crypto/tls's ECDSA nonces
  cannot be repeated from its random source, so the golden hands the
  server the trace's ECDSA signature and checks one the server makes
  separately; RSA signatures (PSS salts from the same source) match
  byte for byte.
- 2026-10-15 (B31 server wiring, tlssrv.xo): the HTTP server's TLS is
  Xo by default: tlssrv.xo runs tls13srv.xo over the accepted socket
  (xo_tls.c's waits, cancellation, HTTP detection, close_notify), hands
  a ClientHello without TLS 1.3 to xo-fa's tls12srv.resume
  (tls13srv.xo's HANDOFF state), and picks the certificate by the
  client's server name as Go's getCertificate does. http.tls_cert is
  tls.X509KeyPair in Xo: PKCS #1 and PKCS #8 RSA, SEC 1 and PKCS #8
  ECDSA P-256 and P-384, PKCS #8 Ed25519, with Go's error texts.
  XO_LLRT_C=tlssrv keeps Mbed TLS (tlssrv_c.xo). After the handshake,
  reads give one record each
  (gotchas/tls-server-read-one-record.md). Known gaps: no session
  tickets (Go and Mbed resume; the Xo path does a full handshake each
  time), no client certificates (Go's default asks for none), no TLS 1.2
  CBC suites (the scope of tls12srv.xo), and the client's roots are still
  Mbed's (no Ed25519 roots).
- 2026-10-15 (B31 session resumption, tlsticket.xo): the servers issue
  and accept session tickets as Go's: tlsticket.xo holds Go's
  SessionState encoding for a server's session, its ticket sealing
  (AES-128-CTR then HMAC-SHA256, the MAC compared in constant time) and
  automatic keys (a new key every 24 hours, keys kept 7 days, rotated by
  tlssrv.xo under the server's lock). tls13srv.xo sends one
  NewSessionTicket after its Finished when the client offers psk_dhe_ke,
  and resumes with Go's checkForResumption checks (at most 5
  identities, TLS 1.3 state, 7 day lifetime, same hash, the binder
  compared in constant time; no 0-RTT). TLS 1.2 tickets (RFC 5077) are
  xo-fa's, in tls12srv.xo, on the same file. The client does not resume
  (Go's client resumes only with a ClientSessionCache, which Xo does not
  expose).

- 2026-10-15 (performance, xo-fa): P-256's base point multiplications
  read a fixed base table that is a generated constant
  (std/native/llrt/nistec_g256.xo, 7,680 Int words, made by
  scripts/nistecg256 and checked by tls_nistec), since Xo has no
  process wide mutable state for a table built at run time; its entries
  are chosen under masks, so the multiplication stays constant time.

- 2026-10-15 (TLS 1.2 CBC, lead): the TLS 1.2 server also speaks the
  four ECDHE CBC-SHA suites Go's server enables by default (c009, c013,
  c00a, c014), last in Go's order: HMAC-SHA1 (sha1.xo), MAC then
  encrypt, explicit IVs, AES decryption added to aesgcm.xo (BearSSL's
  bitsliced inverse cipher). Receiving follows Go's halfConn.decrypt:
  extractPadding in constant time over the last 256 bytes, the MAC over
  a constant amount of data (tls10MAC hashes the rest after its sum),
  one bad_record_mac for a bad length, bad padding, or a bad MAC. Server
  side first; the client offers them too since the follow-up (Go's
  client default), in tls13.xo's combined hello as well.

## Alternatives

- (b) Keep Mbed TLS as the one C exception: rejected by the owner; it
  contradicts decision 0195.
- (c) Platform TLS: rejected; Linux has no platform TLS and would need
  OpenSSL as a system dependency, and behavior would differ per OS.
