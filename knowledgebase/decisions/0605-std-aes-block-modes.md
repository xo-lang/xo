# 0605: std/crypto AES blocks and the CBC and CTR modes

Status: Accepted (owner, 2026-11-11; Xo Lang 3)

## Context

The std audit of 2026-11-11 (bench/results/std-audit-2026-11-11.md)
ranks "no raw block modes (CTR, CBC); AEADs only (Aead)" ninth (score
16, all from Go's standard library: `crypto/cipher.Block`,
NewCBCEncrypter, NewCBCDecrypter, BlockMode, NewCTR, Stream). Programs
need them to interoperate with formats that predate AEADs: PKCS#8 and
PEM private keys encrypted with AES-CBC (PBES2), CMS and S/MIME,
AES-CTR in SSH and in some cloud SDKs' client side encryption, JWE's
A128CBC-HS256 (whose CBC plus HMAC std/crypto's Aead already has as
Aes128CbcHmacSha256), and AES key wrap built over the block function.

std/native/llrt/aesgcm.xo already has the AES key schedule,
encrypt_block, decrypt_block, cbc_encrypt and cbc_decrypt (the TLS 1.2
CBC suites and pkcs12 use them), constant time with the hardware AES
instructions of decision 0231 where the target has them.

## Decision (proposed)

1. std/crypto gains an AES block and two modes, with Go's texts:

   ```xo
   /// An AES key schedule (Go's aes.NewCipher Block): Debug text `<aesblock>`.
   struct AesBlock {}
   fn aes_block(key: Bytes) -> AesBlock ! KeyErr       // 16, 24 or 32 bytes, else `crypto/aes: invalid key size N`
   fn (b: AesBlock) encrypt_block(src: Bytes) -> Bytes  // one 16 byte block (Block.Encrypt)
   fn (b: AesBlock) decrypt_block(src: Bytes) -> Bytes
   fn cbc_encrypt(b: AesBlock, iv: Bytes, plaintext: Bytes) -> Bytes   // NewCBCEncrypter + CryptBlocks
   fn cbc_decrypt(b: AesBlock, iv: Bytes, ciphertext: Bytes) -> Bytes
   /// A CTR keystream (NewCTR): xor continues where the last call ended.
   struct Ctr {}
   fn ctr(b: AesBlock, iv: Bytes) -> Ctr
   fn (var c: Ctr) xor(data: Bytes) -> Bytes            // XORKeyStream
   ```

   Where Go panics, Xo faults with Go's text: a block that is not 16
   bytes (`crypto/aes: input not full block`), CBC input that is not
   whole blocks (`crypto/cipher: input not full blocks`), an IV that is
   not 16 bytes (`cipher.NewCBCEncrypter: IV length must equal block
   size`, and the decrypter's and NewCTR's texts). These are program
   errors, as in Go; a key of the wrong size is an error value
   (KeyErr), as Go's KeySizeError is. CBC has no padding, as Go's has
   none (PKCS#7 is the caller's, or a later helper if programs want it).
2. The native bodies call llrt/aesgcm.xo (CTR is new there: the counter
   block incremented as a 128 bit big endian number, as Go's, with the
   block function a block at a time; a wide path like GCM's can follow a
   measurement). The Go backend's twins call crypto/aes and
   crypto/cipher. Checked against Go: a corpus of keys, IVs and inputs of
   every size class (CTR across call boundaries that split blocks, the
   counter wrapping at 2^64 and 2^128), NIST SP 800-38A vectors, and a
   timing row against Go's crypto/cipher beside the corpus (owner rule
   for ports).
3. Not in this stage: CFB and OFB (deprecated in Go), StreamReader and
   StreamWriter, DES and 3DES (llrt has them for pkcs12; Go keeps them
   for legacy only), a generic Block interface (AES is the only block
   cipher std exposes).

## Consequences

- PBES2 encrypted keys, CMS, SSH's AES-CTR and similar formats can be
  written in Xo; AES key wrap (RFC 3394) is a short function over
  encrypt_block.
- std/crypto grows by two types and seven functions; spec/std.md's
  crypto section, std/changes/crypto.md and scripts/stdaudit/map.txt
  (crypto/cipher, crypto/aes) change with it.
- Unauthenticated modes are easy to misuse: the spec text points to
  Aead for new designs, as Go's documentation does.

## Alternatives

- BlockMode values with a stateful crypt_blocks: Go's CBC keeps the
  chaining value between calls, but its uses are whole messages; the
  whole-buffer functions are simpler, and a stateful form can be added.
  CTR keeps state (Ctr) because streams are split across reads.
- A generic `Block` interface over AES, DES and others: one cipher today;
  an interface can come with a second one.
- Leave it to Aead: formats fixed by other parties need CBC and CTR.
