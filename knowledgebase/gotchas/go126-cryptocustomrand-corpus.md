# Go 1.26 ignores a custom reader in crypto signing and key making

Status: Observed (2026-11-15)

## Symptom

`go run ./scripts/stdcorpus -check` reported x509create stale on every
run (Lang 3), and ecdsaraw and hpke did the same: two regenerations gave
different ECDSA signatures and HPKE messages, though the generators
passed a seeded math/rand reader to Go.

## Cause

Go 1.26 (GODEBUG cryptocustomrand, changed in 26) replaces any reader
other than crypto/rand.Reader with secure randomness in crypto/ecdsa,
ecdh, rsa and the rest (crypto/internal/rand.CustomReader). With
cryptocustomrand=1 the custom reader is used, but randutil.MaybeReadByte
first reads one byte from it with probability one half, so a seeded
stream still gives one of two outputs. crypto/hpke and crypto/mlkem take
no reader at all: NewSender, Seal and Encapsulate draw from Go's DRBG.

## Fix

- In the generator's main package: `//go:debug cryptocustomrand=1`
  (scripts/stdcorpus/main.go).
- Pass a reader of one repeated byte (x509create.go ctRand, a zero
  reader): skipping a byte then changes nothing. ECDSA's hedged nonce
  still mixes in the key and the digest; PSS's salt is zeros. A zero
  stream cannot make a NIST key (randomPoint rejects 0), so generate keys
  from fixed scalars instead.
- For APIs with no reader (HPKE, ML-KEM), pin Go's output in a committed
  file and have Go check each pinned value on every run (hpke.go
  hpkePins: Open gives the plaintext, NewRecipient takes the enc).
- Check every new Go-generated corpus with two regenerations and
  `stdcorpus -check` before committing it.
