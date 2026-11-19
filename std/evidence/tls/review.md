kind: review
module: tls
part: the native TLS client and server (std/tls.xo, std/native/tls.xo, std/native/llrt/tls12.xo, tls12srv.xo, tls13.xo, tls13srv.xo, tlsrec.xo, tlsneg.xo, tlsticket.xo, tlscache.xo, tlsech.xo, tlsio.xo); certificate parsing and chain checks are std/evidence/x509/review.md's
result: pass
commit: d7520dcad
source: 9bb26b8910051376b305e1f8a1b9e0643c8861e403275db348975fa588ba41af
date: 2026-11-19
host: darwin/arm64 (module tests on arm64 and LLVM), linux/arm64 host 1 (dudect, exclusive), linux/arm64 host 2 (gates)
command: read against knowledgebase/infrastructure/std-security-checklist.md; Go vulnerability database sweep of the crypto/tls entries, each compared with Go's crypto/tls source and checked by a module test; `XO_DUDECT=1000000 bin/xo test --real --backend=B --std-module=native/llrt/tls12` for B in arm64, llvm on host 1
measure: 15 Go advisories checked: 3 affected the native backends (GO-2025-4008, GO-2026-4340, GO-2026-4337), fixed with Go's behaviour and a test that fails without the fix; 3 more defects found by the review and fixed (a Lucky13 timing leak in TLS 1.2 CBC records, the first record's size limit, no Certificate message limit in the TLS 1.2 client); dudect at 10^6 measurements on host 1, every same-outcome pair under 4.5 (arm64 max |t| 2.8 over two runs, LLVM 3.7), the early exit control 30892 to 85739; 3 fault sites, none reachable from input; 1000000 fuzz cases per backend with no fault (fuzz.md)

Security review of std/tls for decision 0616, by Xo Lang 3. Every
advisory was compared with Go's crypto/tls (go1.26, the source under
`go env GOROOT`) and checked with a module test in
std/native/llrt/*_test.xo run on arm64 and LLVM. Every fix has a test
that fails without it.

The Go backend uses its Go toolchain's crypto/tls, whose advisories are
the toolchain's; this review covers the native backends (arm64, x86-64,
LLVM, wasm's TLS through the same llrt code).

## 1. Go's CVE history

| Advisory | Defect | Xo before | Fix and test |
|---|---|---|---|
| GO-2025-4008 (CVE-2025-58189) | ALPN error text holds the client's raw protocol names | raw bytes in the error | 8c4d09e8c (quoted as Go's %q); tls12srv_test.xo and tls13srv_test.xo "an ALPN mismatch quotes the client's protocols" |
| GO-2026-4340 (CVE-2025-61730) | handshake messages after the ClientHello in its record read under the next keys | read on | eeacd32c9 (refused before the read keys change); tls13srv_test.xo "a handshake message in the ClientHello's record past the ClientHello is refused" |
| GO-2026-4337 (CVE-2025-68121) | a session ticket resumes where a client certificate is required | resumed | this review's stack (Go's checkForResumption needClientCerts: AUTH_REQUIRE_ANY and AUTH_REQUIRE_AND_VERIFY never resume); tls12srv_test.xo and tls13srv_test.xo "a ticket resumes only where no client certificate is required" |
| GO-2026-5856 (CVE-2026-42505) | ECH outer hello leaks the PSK identity | not affected (the outer hello offers none) | tls13srv_test.xo "with ECH the outer hello offers no PSK identity" |
| GO-2026-6607 (CVE-2026-97031) | repeated ECH outer extension references amplify memory | not affected | tlsech_test.xo "outer_extensions: one reference list, in the outer's order, never rewinding or naming ECH" |
| GO-2026-4870 (CVE-2026-32283) | unauthenticated KeyUpdate records keep a connection | not affected (MAX_USELESS 16, tls13.xo:622) | tls13srv_test.xo "16 KeyUpdates are read, the 17th is one too many", "a client's KeyUpdates past 16" |
| GO-2026-6090 (CVE-2026-56862) | post-handshake messages unlimited | not affected (the same count, and MAX_HANDSHAKE) | the same tests, and "17 empty application data records are one too many" |
| GO-2023-1570 (CVE-2022-41724) | large handshake records panic | not affected (MAX_HANDSHAKE 65536) | tls13srv_test.xo "a client Certificate may be 262144 bytes, other messages 65536" |
| GO-2023-2044 (CVE-2023-39321) | QUIC post-handshake message panic | not affected | the same test, QUIC part |
| GO-2023-2045 (CVE-2023-39322) | QUIC post-handshake messages unbounded | not affected (65536) | the same test |
| GO-2023-1987 (CVE-2023-29409) | peer RSA keys of any size cost CPU | not affected (MAX_TLS_RSA_BITS 8192, x509cert.xo:2266) | tls12_test.xo "a peer RSA key over 8192 bits is too large for TLS, 8192 is not" |
| GO-2022-0531 (CVE-2022-30629) | ticket_age_add not random | not affected (rand.next_bytes(4) per ticket) | read |
| GO-2021-0154 | disabled session tickets kept a zero key a client could seal with | not affected (no ticket_keys: no ticket opens) | tls12srv_test.xo and tls13srv_test.xo "with session tickets off (no ticket_keys) no ticket resumes" |
| GO-2021-0243 (CVE-2021-34558) | a certificate key of the wrong type panics the client | not affected (the key type is checked against the suite's signature before use) | read |
| GO-2023-2375 (CVE-2023-45287) | RSA key exchange timing | not applicable: Xo implements no RSA key exchange | |

## 2. Limits on hostile input

| Limit | Where | At the limit | Past it |
|---|---|---|---|
| record before the version is known, 18432 (Go's maxCiphertext) | tls13.xo MAX_CIPHER_ANY, tls13srv.xo MAX_CIPHER_ANY | waits for the body | record_overflow, "tls: oversized record received with length 18433" (fixed by this review: it was 16640, so a large ClientHello Go takes was refused) |
| TLS 1.3 record after it, 16640 (maxCiphertextTLS13) | tlsrec.xo:30, checks tls13srv.xo:457, tls13.xo:502 | waits | record_overflow |
| TLS 1.2 record, 18432 | tls12.xo:59, checks tls12.xo:1065, tls12srv.xo:255 | waits | record_overflow |
| plaintext, 16384 | tls12.xo:58, tlsrec.xo:27 | opens | record_overflow |
| handshake message, 65536 | tls12.xo:107, tls13.xo:112, tls13srv.xo:105 | read | refused |
| client and server Certificate message, 262144 (maxHandshakeCertificateMsg) | tls12.xo:108, tls13.xo:115, tls13srv.xo:109 | read | refused (the TLS 1.2 one added by this review) |
| non-advancing records (KeyUpdate, empty records), 16 | tls13.xo:622, checks :627 and :636 | read | "tls: too many non-advancing records", "tls: too many ignored records" |
| peer RSA key, 8192 bits | x509cert.xo:2266 | used | refused |
| QUIC post-handshake message, 65536 | tls13srv.xo | read | refused |

Tests: tls13srv_test.xo "a record may be 18432 bytes before the version
is known and 16640 after it" (both sides, both limits, fails before the
fix), "a client Certificate may be 262144 bytes, other messages 65536",
the KeyUpdate and empty record tests, tls12_test.xo's RSA size test.

## 3. Constant time

Secret compares all go through ct.bytes_equal: Finished verify data
(tls12srv.xo:1516, tls12.xo:1925, tls13.xo:1854, tls13srv.xo:1766), the
PSK binder (tls13srv.xo:1212), the ECH accept confirmations (tls13.xo:929
and 1258), the CBC MAC (tls12.xo:665), the ticket's MAC (tlsticket.xo:185). The AEAD
tags are checked by the AEAD code (std/evidence/crypto/review.md).

TLS 1.2 CBC records (the ECDHE CBC-SHA suites, enabled by default as in
Go): the review found a Lucky13 leak. open_cbc removed the padding, then
hashed the record with a plain HMAC, so the number of SHA-1 blocks
hashed told the padding length, and the MAC read and compare ran on a
length that depended on it. Measured before the fix: arm64, 10^5
measurements, ok256 vs badpad |t| 4.80; LLVM -O0, 2 x 10^4, 11.06. Go's
own CBC code has the same partial protection (it hashes extra blocks
but not a constant number). Fixed as BoringSSL's
EVP_tls_cbc_digest_record: tls12.xo mac_cbc_ct hashes every block a
valid padding allows, the length block chosen by masks
(sha1.compress_block over a masked block, ct.pick of the state), and
mac_at reads the MAC from the decrypted record by a mask over every
offset the padding allows. Tests: tls12_test.xo "the constant time CBC
MAC and its read match mac_cbc and a slice for every length the
padding allows" (10 record sizes, every padding), "the CBC timing
classes are what they say", and the dudect test.

dudect (tls12_test.xo "dudect: a CBC record's check takes the same time
whatever its padding and MAC"): classes ok1 (1 byte of padding), ok256
(256), badmac, badpad, in one pool, 7 pairs including 2 A/A pairs, and
an early exit control that must exceed 4.5.

The pairs compare only records with the same outcome: ok1 with ok256
(the padding length of a record that opens), badmac with badpad (why a
record fails), and two A/A pairs. Whether a record opens is public, as
in every TLS stack: a record that fails ends the connection with an
alert the peer sees, so success against failure is not a secret and is
not measured.

The mask based code alone was not enough on host 1, an Apple M4 Pro core
running a Linux VM: its first 10^6 run still read ok1 vs ok256 13.3 on
arm64 though no branch or address depended on the padding. Arm's DIT bit
(Data Independent Timing, decision 0619) makes the core's data
processing instructions take the same time whatever their operands; the
Apple cores time some without it. open_cbc now runs under ct.dit_on and
ct.dit_off (tls12.xo open_cbc_at), as the AEADs and the other constant
time code do; tls12_test.xo "a CBC record is opened with DIT set where
the CPU has it, and DIT is as before after" fails without it
(gotchas/dudect-needs-dit-on-apple.md).

Results at 10^6 measurements on host 1 (exclusive), after the fix:

| Backend | Run | ok1 vs ok256 | badmac vs badpad | ok1 vs ok1 | badpad vs badpad | control |
|---|---|---|---|---|---|---|
| LLVM | 1 | 3.72 | 3.07 | 0.59 | 0.78 | 30892 |
| arm64 | 1 | 1.88 | 1.69 | 1.06 | 1.95 | 85056 |
| arm64 | 2 | 2.13 | 2.37 | 0.97 | 2.79 | 85739 |

Before DIT, the same pairs on arm64 read 13.3 and 13.6 (ok1 vs ok256)
in one run; an A/A pair read 4.8 once in that series, the host's noise
at this size, which is why the result is two runs on arm64. The "|t| up
to 278 between paddings" in open_cbc_at's doc comment is a different
measurement: the decryption and MAC read stage timed alone (the stage
isolating experiment that found the cause), without DIT; with DIT that
stage read 0.7 to 1.6.

These results are linux/arm64 only (arm64 and LLVM backends). x86-64 has
no DIT and was not measured; the CBC path there rests on the mask based
code alone, and a dudect run on the linux/amd64 host is still to do.

## 4. Fault behaviour

Three fault sites in the TLS code, none reachable from input:

- tls12srv.xo:1495 and tls13srv.xo:1560, `s.peer_leaf ?? fault(...)` in
  client_verify: the state is WAIT_CVERIFY only after the client's
  Certificate set peer_leaf (tls12srv.xo:1366 goes to WAIT_CCS when the
  client sent none; tls13srv.xo:1515 sets peer_leaf before 1520 sets the
  state).
- tlsech.xo:540, `seal ... or_fault`: hpke's Sender.seal fails only for
  an export-only AEAD, and tlsech.xo:530 takes only AEADs 1 to 3 from the
  server's ECH config before the sender is made.

Everything else a peer sends ends in an alert and an error text. The
fuzz evidence (fuzz.md: TestTlsFuzz and TestPemFuzz, 10^6 cases each on
arm64 and LLVM) found no fault; stress.md and race.md hold the
concurrent runs.

## 5. Defaults

- Versions: TLS 1.2 and 1.3 (min_version 0); nothing older is
  implemented.
- Cipher suites (tls12.xo:112, tls12srv.xo:935): Go's defaults with
  ECDHE, the AEADs first, then the ECDHE CBC-SHA suites, which Go also
  enables (now constant time, section 3). No RSA key exchange, 3DES or
  RC4 is implemented, so insecure_cipher_suites() is empty.
- Verification: Verify.Full by default. Verify.ChainOnly and
  Verify.None are marked INSECURE in their doc comments (added by this
  review, std/tls.xo); Verify.None is refused without a verify_conn
  that pins the certificate ("tls: Verify.None needs a verify_conn",
  tlsio.xo:395).
- Renegotiation is refused (Go's RenegotiateNever, tls12.xo:1211).
- Session tickets need ticket_keys; a server without them resumes
  nothing (GO-2021-0154 row).

## Sign-off

xo-lang-1, 2026-11-19: read against d7520dcad std/native/llrt/tls*.xo,
x509cert.xo and tlsio.xo: every limit constant and check line, the
bytes_equal sites, the 3 fault sites, the renegotiation refusal
(no_renegotiation alert), the Verify.None refusal, and open_cbc's DIT wrap
and masked mac_cbc_ct/mac_at (loop bounds from dec.len() only) match the
text. dudect was not rerun. Its notes (the compare sites named, the 278
figure placed, x86-64 not measured) are folded in above.
