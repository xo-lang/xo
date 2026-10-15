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
- 2026-10-15 (B31 server checks): TestTLSServerReplayDiff
  (internal/llvm, not -short) is the server half of the differential
  fuzzing. It mutates the recorded clients of the tls13_server and
  tls12_server goldens: bytes flipped, cut, inserted; records dropped,
  repeated, swapped, moved; flights dropped or repeated; the RFC 8448
  client's Finished altered under its valid keys; the input chunked. It
  diffs tls13srv.xo and tls12srv.xo on LLVM and arm64 against the Go
  references. It found Go's alert record version (0x0301 before the
  version is chosen) missing in both servers, and tls12srv.xo's stream
  and EOF handling (fixed by xo-fa). The TLS 1.3 reference is our Go
  port, so the fuzz checks Xo against it. Agreement with crypto/tls
  itself comes from the recorded Go client traces, TestTLSDiff, and
  TestTLSInteropMatrix, which runs the same Xo program on the Go backend
  (crypto/tls on both ends) and the native backends: the Xo client
  against Go servers and Go clients against the Xo servers, for each TLS
  version, suite, group, certificate key, ALPN, and resumption.
  X25519MLKEM768, Go's default hybrid group, has no Xo counterpart: the
  matrix reads it as X25519. The ticket key derivation was wrong in
  both tlsticket.xo and its Go copy, which agreed with each other; xo-fa
  found it against crypto/tls's own TLS 1.2 tickets.
  std/rt's TestTicketsAsGo now opens Go's tickets and has Go open ours.
- 2026-10-15 (B31 post-quantum, mlkem.xo and sha3.xo): ML-KEM-768
  (FIPS 203) as Go's crypto/internal/fips140/mlkem, over a Keccak of 64
  bit lanes (sha3.xo: SHA3-256 and SHA3-512, SHAKE128 and SHAKE256). The
  decapsulation's implicit rejection picks between the two keys in
  constant time. TLS's X25519MLKEM768 hybrid (0x11ec) is Go's
  hybridKeyExchange: the client share is the encapsulation key then
  X25519, the server share the ciphertext then X25519, the secret
  ML-KEM's then X25519's. tls13srv.xo prefers it as Go 1.26's server
  does (less SecP256r1MLKEM768, SecP384r1MLKEM1024 and P-521, which Xo
  does not have). The client hello change is Xo Lang 2's, in tls13.xo.
  Checks: C2SP CCTV's accumulated test (100 in golden crypto_mlkem,
  10,000 in TestMLKEMAccumulated) and a diff against crypto/mlkem and
  crypto/sha3; a recorded Go 1.26 default client negotiating the hybrid
  in tls13_server; TestTLSInteropMatrix with Go's clients taking the
  hybrid against the Xo servers. Wycheproof's ML-KEM vectors are not in
  the repository yet.

## Alternatives

- (b) Keep Mbed TLS as the one C exception: rejected by the owner; it
  contradicts decision 0195.
- (c) Platform TLS: rejected; Linux has no platform TLS and would need
  OpenSSL as a system dependency, and behavior would differ per OS.

- 2026-10-15 (B31 part 18, xo-4b): the default path at zero Mbed TLS.
  apple_trust calls the frameworks through sys_darwin.xo declarations;
  the driver links Security and CoreFoundation when generated code names
  SecTrustEvaluateWithError, independently of needs_tls; the system's
  roots are an Xo record; needs_tls matches only xo_tls_ and xo_cl_tls_
  (the XO_LLRT_C C paths). Proof: nm of a default http_tls binary has no
  mbedtls, fiat or psa symbols (arm64 and LLVM, 3.9 MB; the C path's
  build has 790 mbedtls symbols). XO_TLS_MLKEM=0 drops the hybrid
  (the lead: an Xo name, not GODEBUG, as XO_HTTP_MAX_BODY; mirrors Go's
  GODEBUG=tlsmlkem=0). Observed while checking: rsa4096 and untrusted-root
  .badssl.com reset some connections whose ClientHello carries the
  1,216 byte hybrid share, Go 1.26's client too (1 in 16 for Go, 4 in
  16 for Xo, 2026-10-15); not an Xo protocol difference (the hellos match
  Go's, extension for extension, but for Go's extra groups).

- 2026-10-15 (B31 part 19, xo-4b): Go 1.26's full group list on the
  client (X25519MLKEM768, SecP256r1MLKEM768, SecP384r1MLKEM1024, X25519,
  P-256, P-384, P-521), ML-KEM-1024, P-521 ECDH and ECDSA, P-224 ECDSA
  verification. P-521's fixed base table was measured and rejected: a
  compiled in table (35,370 Ints) added 27 s to every arm64 TLS build
  (29.6 s against 3.0 s) and 2.5 MB to the binary; Go builds its P-521
  table lazily, which needs a process wide slot (all 64 static words are
  claimed). The lead chose the windowed pt_mul for P-521 and P-384; a
  lazy table is the follow-up once a slot registry exists.
- 2026-10-15 (B31 part 20, xo-4, the server side of part 19):
  - The Xo server (tls13srv.xo) takes Go 1.26's groups: X25519MLKEM768,
    SecP256r1MLKEM768, SecP384r1MLKEM1024, X25519, P-256, P-384, P-521.
    It chooses as Go's server does: a post-quantum hybrid first, then a
    group the client sent a share for, then the server's order. The
    hybrid key exchange is mlkem.hybrid_server_for.
  - ECDSA P-521 certificates sign with 0x0603 in TLS 1.3. P-224 has no
    TLS 1.3 scheme, so Go's server refuses it.
  - tls12srv signs with ECDSA on any curve (P-224 and P-521 too) and
    takes CurveP521 for ECDHE (xo-fa's file, done at their request).
  - tlssrv's key parsing reads P-224 and P-521 SEC 1 and PKCS #8 keys,
    as Go's ParseECPrivateKey does.
  - The Go reference port (internal_tls13srv.go, internal_tls12.go)
    does the same.
  - The interop matrix now has the P-521 certificate on the Xo server,
    P-521 on TLS 1.3 and 1.2, and both new hybrids on TLS 1.3 (alone and
    next to X25519MLKEM768) for Go's clients against the Xo server: 0
    cells differ on LLVM and arm64.
