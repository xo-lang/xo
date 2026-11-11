# 0604: std/crypto ML-KEM, crypto/mlkem as a public API

Status: Accepted (owner, 2026-11-11; Xo Lang 3)

## Context

The std audit of 2026-11-11 (bench/results/std-audit-2026-11-11.md)
ranks "ML-KEM is inside the TLS stack only (std/native/llrt/mlkem.xo),
no public API" eighth (score 24, all from Go's standard library:
`crypto/mlkem.EncapsulationKey768`, the size constants, GenerateKey768,
Decapsulate, and the 1024 forms). Programs reach for it for
post-quantum key agreement outside TLS: HPKE (gap 13, which needs a KEM
and a public AEAD and ECDH, decision 0599), age style file encryption,
Noise and Signal style handshakes, and hybrid X25519 + ML-KEM schemes.

std/native/llrt/mlkem.xo already implements FIPS 203 ML-KEM-768 and
ML-KEM-1024 (key expansion from a 64 byte seed, encapsulation from a
32 byte message, decapsulation with implicit rejection), checked
against Go for the TLS hybrids X25519MLKEM768, SecP256r1MLKEM768 and
SecP384r1MLKEM1024; its NTT has the SSE2 path of decision 0336. It is
internal (`llrt`), answers None instead of Go's errors, and the Go
backend reaches it only through TLS.

## Decision (proposed)

1. std/crypto gains crypto/mlkem's surface, in the shape of the ECDH
   API (decision 0599), with Go's error texts in KeyErr:

   ```xo
   enum MlkemParams { Mlkem768, Mlkem1024 }

   /// A decapsulation key: Debug text `<decapsulationkey>`, never key material.
   struct MlkemDecapsulationKey {}
   /// An encapsulation key: ==.
   struct MlkemEncapsulationKey {}

   const MLKEM_SHARED_KEY_SIZE: Int = 32
   const MLKEM_SEED_SIZE: Int = 64
   fn (p: MlkemParams) encapsulation_key_size() -> Int    // 1184, 1568
   fn (p: MlkemParams) ciphertext_size() -> Int           // 1088, 1568

   fn mlkem_generate_key(r: Rand, p: MlkemParams) -> MlkemDecapsulationKey uses rand // GenerateKey768/1024
   fn mlkem_decapsulation_key(p: MlkemParams, seed: Bytes) -> MlkemDecapsulationKey ! KeyErr // NewDecapsulationKey768
   fn mlkem_encapsulation_key(p: MlkemParams, key: Bytes) -> MlkemEncapsulationKey ! KeyErr  // NewEncapsulationKey768
   fn (k: MlkemDecapsulationKey) params() -> MlkemParams
   fn (k: MlkemDecapsulationKey) bytes() -> Bytes                    // the 64 byte seed (d || z)
   fn (k: MlkemDecapsulationKey) encapsulation_key() -> MlkemEncapsulationKey
   fn (k: MlkemDecapsulationKey) decapsulate(ciphertext: Bytes) -> Bytes ! KeyErr
   fn (k: MlkemEncapsulationKey) params() -> MlkemParams
   fn (k: MlkemEncapsulationKey) bytes() -> Bytes
   fn (k: MlkemEncapsulationKey) encapsulate(r: Rand) -> (Bytes, Bytes) uses rand  // (shared key, ciphertext)
   ```

   Errors are Go's: `mlkem: invalid seed length`, `mlkem: invalid
   encapsulation key length`, `mlkem: invalid polynomial encoding` (an
   encapsulation key whose coefficients are not reduced, FIPS 203's
   modulus check), `mlkem: invalid ciphertext length`. A ciphertext of
   the right length never fails: decapsulation rejects implicitly, as
   FIPS 203 says (a pseudorandom key from z).
2. Randomness comes from the Rand capability, as ECDH and generate_key
   take it: key generation reads the 64 byte seed (d, then z) and
   encapsulation the 32 byte message m, so one seeded r gives the same
   keys and ciphertexts on every backend, and Go's
   crypto/mlkem/mlkemtest.Encapsulate768(ek, m) is the oracle for a
   given m. (Go's Encapsulate reads crypto/rand itself; Xo has no
   ambient randomness.)
3. The native bodies call llrt/mlkem.xo (no new arithmetic: expand,
   encaps_k, decaps_k, plus the encapsulation key check of FIPS 203
   7.2 if llrt does not already make it); the Go backend's twins in
   std/rt call Go's crypto/mlkem and mlkemtest with the same reads.
   Checked against Go: a corpus from Go's crypto/mlkem (seeded seeds and
   messages for both parameter sets, keys, ciphertexts and shared keys;
   wrong lengths; encapsulation keys with an unreduced coefficient;
   tampered ciphertexts giving the implicit rejection key) on every
   backend, and the accumulated vectors of Go's mlkem tests (the
   10,000 iteration SHAKE128 chain hashes of FIPS 203's test suite).
4. Not in this stage: the expanded NIST decapsulation key format (Go's
   NewDecapsulationKey takes only the seed, so this matches Go), and a
   crypto.Encapsulator style interface (HPKE, gap 13, would add one when
   it lands, over this and ECDH).

## Consequences

- Programs get post-quantum KEMs without TLS; HPKE becomes possible
  over 0599's ECDH, this, and the public AEAD.
- std/crypto grows by two key types and nine functions; spec/std.md's
  crypto section, std/changes/crypto.md and scripts/stdaudit/map.txt
  (crypto/mlkem) change with it.
- The llrt implementation gains a second caller; its speed is already
  measured for TLS (decision 0336), and a timing row against Go's
  crypto/mlkem goes beside the corpus (owner rule for ports).

## Alternatives

- A separate std/mlkem module: crypto's ECDH, signatures and AEADs live
  in std/crypto, so keys and KeyErr stay together there.
- Go's ambient-randomness Encapsulate(): Xo has no ambient randomness;
  the Rand capability keeps it testable and deterministic, as ECDH does.
- Bytes in, Bytes out functions with no key types: Go validates the
  encapsulation key once at construction; key types keep that and the
  Debug redaction of the seed.
