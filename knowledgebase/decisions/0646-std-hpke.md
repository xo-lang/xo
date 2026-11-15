# 0646: std/hpke, Go's crypto/hpke as a pure module on std/crypto

Status: Accepted (owner, 2026-11-15, the crypto gap list C1 to C13 via the lead; aws-43)

## Context

Lang 3's crypto and TLS gap audit (~/xo-lead/notes/lang3/crypto-tls-gap.md)
lists HPKE as C13: Go 1.26's crypto/hpke (RFC 9180's base mode and the
post-quantum KEMs of draft-ietf-hpke-pq) has no Xo counterpart. TLS
Encrypted Client Hello (the audit's T13) needs it, so the lead put it
first in the crypto list. Decisions 0599 (ECDH) and 0604 (ML-KEM) made
the pieces public with this in view; HKDF and the AEADs were public
already, and SHAKE (C1) lands with this decision.

## Decision

1. A new module, std/hpke, written once in Xo over std/crypto's public
   API (a merged module, decision 0588, and in std/go_compiled.txt, so
   the Go backend compiles the same source: no std/rt twin and no new
   native code). Its surface is Go's, in Xo's names:
   `Kem`, `Kdf` and `Aead` enums with `id()` and `new_kem`, `new_kdf`,
   `new_aead`, `dhkem`; `Kem.generate_key`, `public_key`, `private_key`,
   `derive_key_pair`; `dhkem_*_key`, `mlkem_*_key`, `hybrid_*_key`;
   `PublicKey` and `PrivateKey` with `kem`, `bytes`, `public_key`;
   `new_sender`, `new_recipient`, `Sender.seal`, `Recipient.open`,
   `export`, and the one shot `seal` and `open`. Errors are an `HpkeErr`
   with Go's texts (spec/std.md 84).
2. `Sender` and `Recipient` are `@move_only` (core 5.3): Go's are
   pointers, and an Xo value copy of a Sender would seal two messages
   under one nonce. Go's NewSender returns the encapsulated key beside
   the Sender; here it is `Sender.enc()`, so the constructor returns one
   move only value.
3. Randomness comes from a `Rand`, as everywhere in std/crypto. The
   order of reads is fixed: DHKEM's ephemeral key
   (`crypto.ecdh_generate_key`), ML-KEM's 32 byte message, a hybrid's
   ML-KEM message and then its curve key. That order is what Go's
   TestVectors hooks give (ikmE as the ML-KEM message then the curve
   key), so a Rand answering fixed bytes replays the vectors on every
   backend without a test only API.
4. Checked against Go (corpus hpke, scripts/stdcorpus/hpke.go): the base
   mode vectors of Go's testdata/rfc9180.json and hpke-pq.json (the
   accumulated encryption and export checks included), DeriveKeyPair of
   seeded inputs, messages Go sealed opened as sealed, with a changed
   byte and cut short, Go's errors for identifiers, keys, encapsulated
   keys, export lengths (at the 255 block HKDF bound) and hybrids, and
   TestRoundTrip over every KEM, KDF and AEAD. Native, go and llvm.
5. Not in this stage: the PSK, Auth and AuthPSK modes (Go has only the
   base mode), DHKEM(X448), ML-KEM-512 and the TurboSHAKE KDFs (Go has
   none of them).
