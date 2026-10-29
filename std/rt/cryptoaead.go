package rt

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/subtle"
	"fmt"
	"strings"
)

// std/crypto's Aead, AeadKey, and HKDF (std.md 11) on Go's crypto/cipher
// (AES-GCM), this package's ChaCha20-Poly1305 (internal_crypto.go, the
// one internal.crypto_op uses: Go's standard library exports none), and
// crypto/hkdf. The error texts are native's (std/native/crypto.xo).

const aeadFail = "cipher: message authentication failed"

func (a Crypto_Aead) M_nonce_size(*Task) int64 { return 12 }
func (a Crypto_Aead) M_overhead(*Task) int64   { return 16 }
func (e Crypto_AeadErr) M_display(*Task) string { return e.F_msg }

// Crypto_AeadKey is `crypto.AeadKey`: AES-GCM's cipher.AEAD, or
// ChaCha20-Poly1305's 32 byte key. It never prints its key.
type Crypto_AeadKey struct {
	alg Crypto_Aead
	gcm cipher.AEAD
	key []byte
}

func (Crypto_AeadKey) XoDebug(b *strings.Builder) { b.WriteString("<aeadkey>") }
func (k Crypto_AeadKey) M_debug(*Task) string     { return Debug(k) }

func aeadErr(m string) Crypto_AeadErr { return Crypto_AeadErr{F_msg: m} }

func Crypto_aead_key(_ *Task, alg Crypto_Aead, key Bytes) Result[Crypto_AeadKey, Crypto_AeadErr] {
	if alg.Tag == 2 { // ChaCha20Poly1305
		if len(key) != 32 {
			return Err[Crypto_AeadKey](aeadErr("chacha20poly1305: bad key length"))
		}
		return Ok[Crypto_AeadKey, Crypto_AeadErr](Crypto_AeadKey{alg: alg, key: []byte(key)})
	}
	switch len(key) {
	case 16, 24, 32:
	default:
		return Err[Crypto_AeadKey](aeadErr(fmt.Sprintf("crypto/aes: invalid key size %d", len(key))))
	}
	b, err := aes.NewCipher([]byte(key))
	if err != nil {
		return Err[Crypto_AeadKey](aeadErr(err.Error()))
	}
	g, err := cipher.NewGCM(b)
	if err != nil {
		return Err[Crypto_AeadKey](aeadErr(err.Error()))
	}
	return Ok[Crypto_AeadKey, Crypto_AeadErr](Crypto_AeadKey{alg: alg, gcm: g})
}

func (k Crypto_AeadKey) M_alg(*Task) Crypto_Aead { return k.alg }

func aeadNonce(nonce Bytes) {
	if len(nonce) != 12 {
		FaultHere("crypto: AEAD nonce of %d bytes, want 12", len(nonce))
	}
}

func (k Crypto_AeadKey) M_seal(_ *Task, nonce, plaintext, aad Bytes) Bytes {
	aeadNonce(nonce)
	if k.gcm != nil {
		return Bytes(k.gcm.Seal(nil, []byte(nonce), []byte(plaintext), []byte(aad)))
	}
	ct := chachaXor(k.key, 1, []byte(nonce), []byte(plaintext))
	return Bytes(ct) + Bytes(chachaTag(k.key, []byte(nonce), ct, []byte(aad)))
}

func (k Crypto_AeadKey) M_open(_ *Task, nonce, sealed, aad Bytes) Result[Bytes, Crypto_AeadErr] {
	aeadNonce(nonce)
	if len(sealed) < 16 {
		return Err[Bytes](aeadErr(aeadFail))
	}
	if k.gcm != nil {
		pt, err := k.gcm.Open(nil, []byte(nonce), []byte(sealed), []byte(aad))
		if err != nil {
			return Err[Bytes](aeadErr(aeadFail))
		}
		return Ok[Bytes, Crypto_AeadErr](Bytes(pt))
	}
	n := len(sealed) - 16
	ct := []byte(sealed[:n])
	if subtle.ConstantTimeCompare(chachaTag(k.key, []byte(nonce), ct, []byte(aad)), []byte(sealed[n:])) != 1 {
		return Err[Bytes](aeadErr(aeadFail))
	}
	return Ok[Bytes, Crypto_AeadErr](Bytes(chachaXor(k.key, 1, []byte(nonce), ct)))
}

func (k Crypto_AeadKey) M_seal_random(tk *Task, r Rand, plaintext, aad Bytes) Bytes {
	nonce := r.M_bytes(tk, 12)
	return nonce + k.M_seal(tk, nonce, plaintext, aad)
}

func (k Crypto_AeadKey) M_open_random(tk *Task, sealed, aad Bytes) Result[Bytes, Crypto_AeadErr] {
	if len(sealed) < 12+16 {
		return Err[Bytes](aeadErr(aeadFail))
	}
	return k.M_open(tk, sealed[:12], sealed[12:], aad)
}

func Crypto_hkdf_extract(_ *Task, h Crypto_Hash, secret, salt Bytes) Bytes {
	prk, err := hkdf.Extract(cryptoHashes[h.Tag], []byte(secret), []byte(salt))
	if err != nil {
		FaultHere("crypto: hkdf_extract: %v", err)
	}
	return Bytes(prk)
}

func Crypto_hkdf_expand(_ *Task, h Crypto_Hash, prk, info Bytes, n int64) Bytes {
	size := int64(cryptoHashes[h.Tag]().Size())
	if n < 0 || n > 255*size {
		FaultHere("crypto: hkdf_expand of %d bytes, at most %d for this hash", n, 255*size)
	}
	if n == 0 {
		return ""
	}
	out, err := hkdf.Expand(cryptoHashes[h.Tag], []byte(prk), string(info), int(n))
	if err != nil {
		FaultHere("crypto: hkdf_expand: %v", err)
	}
	return Bytes(out)
}

func Crypto_hkdf(tk *Task, h Crypto_Hash, secret, salt, info Bytes, n int64) Bytes {
	return Crypto_hkdf_expand(tk, h, Crypto_hkdf_extract(tk, h, secret, salt), info, n)
}
