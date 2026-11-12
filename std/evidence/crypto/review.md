kind: review
module: crypto
part: RSA encryption and decryption (decision 0618): crypto.encrypt_oaep, decrypt_oaep, encrypt_pkcs1v15, decrypt_pkcs1v15; std/native/llrt/rsa.xo encrypt_oaep_fn, decrypt_oaep_fn, encrypt_pkcs1v15, decrypt_pkcs1v15, ciphertext_in, ct_bytes_mask
result: pass
commit: bfaaaed19
source: 9e051a0f0598a605b3327c865a0c1bc46d5f34a615526fb66287a69d836eea15
date: 2026-11-12
host: linux/arm64 host 1 (timing)
command: go run ./scripts/stdproven timing -n 1000000 -backends arm64,llvm -pairs oaep_first:oaep_lhash,oaep_lhash:oaep_sep,v15_head:v15_nozero,v15_nozero:v15_short,oaep_sep:oaep_sep,v15_nozero:v15_nozero -host "linux/arm64 host 1" crypto rsa_decrypt
measure: 4 Go vulnerability entries checked (1 shared, in std/tls, fixed); 3 limits; 2 constant time operations, dudect max |t| 3.357 over 12 pairs (A/A included) of 10^6 measurements; 0 fault sites reachable from input; 1 legacy scheme documented

Security review of std/crypto's RSA encryption against
knowledgebase/infrastructure/std-security-checklist.md (decision 0616),
by Xo Lang 3. The RSA private operation itself (bigmod's constant time
exponentiation, decision 0222) is outside this part; it has its own
codegen and LLVM constant time tests (test/codegen/consttime,
internal/llvm TestConstantTimeLLVM).

## 1. Go's CVE history

The Go vulnerability database (vuln.go.dev, downloaded 2026-11-12, 4801
entries) has no entry whose affected package is crypto/rsa. Four stdlib
entries concern RSA:

- GO-2021-0160 (CVE-2015-8618), math/big: Int.Exp's Montgomery carry
  propagation gave wrong results, risking RSA private keys. Not shared:
  Xo's RSA uses std/native/llrt/bigmod, not a port of math/big's Exp. The
  private operation checks its result with the public operation
  (rsa.xo private_op), so a wrong exponentiation is refused, not
  released. Test: Go's OAEP known answer vectors decrypt
  (std/native/crypto_test.xo "go rsa: OAEP known answers"), and golden
  crypto_rsa_encrypt's 140 Go ciphertexts.
- GO-2023-1987 (CVE-2023-29409), crypto/tls: huge RSA keys in a peer's
  certificate chain made verification expensive. SHARED: Xo's native TLS
  had no upper bound (rsa.xo public_key checks only the lower one). Fixed
  in e2a58a0fd: x509cert.rsa_too_large, Go's 8192 bit cap and text at the
  four places a peer's chain is parsed. Test: std/native/llrt/tls12_test.xo
  (8192 bits allowed, 8193 refused, certificates from Go).
- GO-2023-2375 (CVE-2023-45287), crypto/tls before Go 1.20: removing
  PKCS #1 v1.5 padding after math/big's variable time arithmetic leaked
  timing. Not shared: Xo's TLS offers no RSA key exchange (no
  DecryptPKCS1v15SessionKey, decision 0618), and decrypt_pkcs1v15's
  padding removal is constant time (item 3, timing test v15_*).
- GO-2025-3421 (CVE-2025-22865), crypto/x509: ParsePKCS1PrivateKey
  panicked on a key missing its CRT values. Not shared: Xo computes absent
  CRT values only when they are absent (spec/std.md 5), and a key whose
  CRT values are present but zero is Go's error, "x509: private key
  contains zero or negative value", a KeyErr. Test:
  std/native/crypto_test.xo "CVE-2025-22865: a PKCS #1 key with zero CRT
  values is Go's error, not a fault" (the key from Go's test).

## 2. Limits on hostile input

A decrypting party gets the ciphertext and, for OAEP, the label from a
peer; an encrypting party gets the peer's public key.

- Ciphertext length: longer than the key, or a value not below the
  modulus, is the decryption error before any private operation
  (rsa.xo ciphertext_in: the length at std/native/llrt/rsa.xo:503, below
  the modulus at rsa.xo:513); shorter is padded on the left, as Go. Tests:
  golden crypto_rsa_encrypt (one byte longer with a leading zero, a
  trailing byte added, the modulus, all 0xff) and TestRsaFuzz.
- Label: any length, hashed once (public data, linear time).
- Peer public key size for encryption: from 1024 bits (Go's floor, the
  insecure key text; std/native/crypto.xo:1480 rsa_public_err) with no upper bound, as Go's crypto/rsa (only its
  TLS caps peers at 8192); the cost is one public exponentiation with
  e below 2^31 (rsa.xo check_public), linear in e's bit length. TLS
  peers are capped at 8192 bits (item 1).

## 3. Constant time

- OAEP decoding (rsa.xo decrypt_oaep_fn): the first byte check, the label
  hash compare (ct_bytes_mask, every byte through ct.barrier), and the
  search for the 0x01 separator (a loop over every byte of DB, with
  ct.mask_eq and ct.pick, no early exit) combine into one mask; the only
  branch is on the combined answer, and the message's position is read
  from a masked index after it. As Go's crypto/internal/fips140/rsa
  decryptOAEP.
- PKCS #1 v1.5 decoding (rsa.xo decrypt_pkcs1v15): the 0x00 0x02 head,
  the first zero after it (a loop over every byte, masks only) and the
  eight byte minimum (ct.mask_lt) combine into one mask, one branch. As
  Go's crypto/rsa decryptPKCS1v15.
- The private operation's own fault check (rsa.xo private_op, the
  result run back through the public operation) compared with Bytes
  `==`, which may stop at the first differing byte; it now uses
  ct.bytes_equal, as Go's constant time Equal (fixed in bfaaaed19, the
  second reader's F7). The result was never released on a mismatch.
- Every failure, whichever check failed, returns the one KeyErr text
  (crypto.xo decryption_error), so neither the error nor its timing tells
  the padding checks' causes apart. ciphertext_in's refusals (longer
  than the key, not below the modulus) return before the private
  operation; both conditions are public, and Go refuses them the same
  way.
- Timing tests, two levels:
  - the padding checks alone (std/native/llrt/rsa_test.xo dudect, run
    by `xo test --real`, decision 0619): rsa.oaep_unpad and
    pkcs1_unpad on fixed blocks of each class, XO_DUDECT measurements a
    pair (default 10^5), with A/A pairs and a deliberately early exit control that
    must fail. The classes share one pool in alternating runs of 32
    (knowledgebase/gotchas/dudect-pool-layout.md: alternating slot by
    slot made the page crossing blocks one class, which failed even the
    A/A pairs). Results: decision 0619's Results and the gotcha
    (linux/arm64 native with DIT, the whole unpad, max |t| 2.4; the
    MGF1-only A/A pair 3.1; amd64 LLVM -O2 2.1). The 10^6 measurement
    figures of this review are the end to end run below.
  - the whole decryption, end to end (std/evidence/crypto/timing-rsa_decrypt.md):
    four pairs of ciphertext classes that fail different checks (OAEP:
    first byte vs label hash, label hash vs separator; PKCS #1 v1.5:
    head vs no zero, no zero vs short padding), every block checked by
    scripts/ctgen to fail its class's check (and, but for oaep_first,
    only that check: an oaep_first block has a random data block, so it
    also fails the label hash and the separator; the pair still sets the
    first byte check against a block that passes it), one pool in runs of 32,
    10^6 measurements each, on arm64 and LLVM, a 1024-bit key, with
    two A/A pairs (oaep_sep, v15_nozero against themselves) as the
    noise floor: every pair passes, max |t| 3.357 (arm64 oaep_first vs
    oaep_lhash; the A/A pairs reach 1.44 and 1.62), the threshold 4.5,
    on linux/arm64 host 1 at 716c0f0e7.

## 4. Fault behaviour

No decryption input reaches a fault. private_op returns exactly
size(n) bytes, so the slices of oaep_unpad (rsa.xo:566, called by
decrypt_oaep_fn) (em[0], em[1..1 + hl],
em[1 + hl..kk], db[0..hl], db[index + 1..]) and of pkcs1_unpad (rsa.xo:629, called by decrypt_pkcs1v15)
(em[0], em[1], em[i] for i below kk, em[index + 1..kk]) are in range;
kk < 2 hl + 2 and kk < 11 are refused first. bigmod.from_bytes of
ciphertext_in cannot fail for kk bytes and the modulus's limb count, and
its None arm returns None (the decryption error), since below() would
read past an empty list rather than refuse it. A failed
private operation (a fault in the CRT, caught by the public check) is
the decryption error. Tests: internal/stdtest TestRsaFuzz, against Go's
crypto/rsa for a fresh 2048-bit key: one ciphertext of every length from
0 to k + 2, then random cases (Go's ciphertexts changed at random, hand
built blocks failing each check, valid ones at random message lengths):
seed 3 with 4,000 random cases (4,259 with the lengths; 1,463 decrypt in
Go) on arm64 and LLVM, and seed 2 with 600 (859; 221 decrypt) on arm64,
LLVM and Go, linux/arm64 host 1 at 716c0f0e7's code: no fault, every
outcome Go's.

encrypt_pkcs1v15 draws nonzero padding from the program's Rand until it
has enough: a Rand that only returns zeros (a test stream) never ends,
as Go's nonZeroRandomBytes. Rand is the program's own capability, not
peer input.

## 5. Defaults

OAEP is the documented scheme; PKCS #1 v1.5 encryption is named legacy
in each function's doc comment, spec/std.md 5 and std/changes, as the
owner's condition on decision 0618 and Go's Deprecated notice. Leaving
out DecryptPKCS1v15SessionKey is a mitigation, not only a gap: its
use is TLS RSA key exchange, the Bleichenbacher setting, which Xo's TLS
does not offer. No option weakens either scheme (no separate MGF1 hash).
OAEP accepts Md5 and Sha1 as its hash, as Go's does: OAEP's security
does not rest on the hash's collision resistance, and both are needed to
read data from older systems.

encrypt_pkcs1v15 refuses a key below 1024 bits before it checks the
message's length, as Go's EncryptPKCS1v15 (fixed in this review; before,
a long message with a small key gave "message too long").

## Sign-off

Signed by a second agent (Xo Lang 3 subagent), 2026-11-12: read at 716c0f0e7 with the evidence commit 3ee944a7f and the fix bfaaaed19. I checked rsa.xo's OAEP and PKCS #1 v1.5 encrypt and decrypt paths (encrypt_oaep_fn, decrypt_oaep_fn, encrypt_pkcs1v15, decrypt_pkcs1v15, ciphertext_in, oaep_unpad, pkcs1_unpad, ct_bytes_mask, private_op, check_public), ct.xo, and crypto.xo's RSA functions against Go 1.26's crypto/rsa and fips140/rsa. Before the single final branch I found no branch, early exit or index on secret data, and no fault reachable from a ciphertext or a label. I verified the four CVE entries, e2a58a0fd's four rsa_too_large call sites, the cited module tests, the 140-case golden crypto_rsa_encrypt, TestRsaFuzz's length sweep and case counts, scripts/ctgen's classes, and the timing figures against timing-rsa_decrypt.md. Findings: F1 to F6 were text errors (a stale None-arm description, slice locations, oaep_first purity, rsa_test's default of 10^5 measurements and its MGF1 A/A label, the timing claim for public refusals, missing file:line for the limits), all corrected in the text. F7 was private_op's fault check comparing with Bytes `==`, which may stop at the first differing byte; it was fixed in bfaaaed19 with ct.bytes_equal, as Go's constant time Equal. F8 was a text item: the review now names that commit and its header follows it.
