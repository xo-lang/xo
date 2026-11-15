package rt

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/binary"
	"fmt"
	"math/bits"
	"strings"
)

// std/crypto's Aead, AeadKey, and HKDF (std.md 11) on Go's crypto/cipher
// (AES-GCM), this package's ChaCha20-Poly1305 (internal_crypto.go, the
// one internal.crypto_op uses: Go's standard library exports none) and
// XChaCha20-Poly1305 over it, and crypto/hkdf. The error texts are native's (std/native/crypto.xo).

const aeadFail = "cipher: message authentication failed"

func (a Crypto_Aead) M_nonce_size(*Task) int64 {
	switch a.Tag {
	case aeadTagCbc:
		return 16
	case aeadTagXChaCha:
		return 24
	}
	return 12
}
func (a Crypto_Aead) M_overhead(*Task) int64 {
	if a.Tag == aeadTagCbc {
		return 32
	}
	return 16
}

// aeadTagCbc is Aes128CbcHmacSha256's variant tag (after AesGcm and
// ChaCha20Poly1305), aeadTagXChaCha XChaCha20Poly1305's.
const (
	aeadTagCbc     = 3
	aeadTagXChaCha = 4
)

// hChaCha20 is x/crypto v0.58.0's chacha20.HChaCha20 (the
// draft-irtf-cfrg-xchacha subkey) of a 32 byte key and a 16 byte nonce.
func hChaCha20(key, nonce []byte) []byte {
	var x [16]uint32
	x[0], x[1], x[2], x[3] = 0x61707865, 0x3320646e, 0x79622d32, 0x6b206574
	for i := 0; i < 8; i++ {
		x[4+i] = binary.LittleEndian.Uint32(key[4*i:])
	}
	for i := 0; i < 4; i++ {
		x[12+i] = binary.LittleEndian.Uint32(nonce[4*i:])
	}
	qr := func(a, b, c, d int) {
		x[a] += x[b]
		x[d] = bits.RotateLeft32(x[d]^x[a], 16)
		x[c] += x[d]
		x[b] = bits.RotateLeft32(x[b]^x[c], 12)
		x[a] += x[b]
		x[d] = bits.RotateLeft32(x[d]^x[a], 8)
		x[c] += x[d]
		x[b] = bits.RotateLeft32(x[b]^x[c], 7)
	}
	for i := 0; i < 10; i++ {
		qr(0, 4, 8, 12)
		qr(1, 5, 9, 13)
		qr(2, 6, 10, 14)
		qr(3, 7, 11, 15)
		qr(0, 5, 10, 15)
		qr(1, 6, 11, 12)
		qr(2, 7, 8, 13)
		qr(3, 4, 9, 14)
	}
	out := make([]byte, 32)
	for i, j := range []int{0, 1, 2, 3, 12, 13, 14, 15} {
		binary.LittleEndian.PutUint32(out[4*i:], x[j])
	}
	return out
}

// chachaKeyNonce is the ChaCha20-Poly1305 key and 12 byte nonce of a
// key's message: XChaCha20-Poly1305's HChaCha20 subkey and 4 zero bytes
// then the nonce's last 8 (x/crypto's xchacha20poly1305), else as given.
func (k Crypto_AeadKey) chachaKeyNonce(nonce []byte) ([]byte, []byte) {
	if k.alg.Tag != aeadTagXChaCha {
		return k.key, nonce
	}
	n := make([]byte, 12)
	copy(n[4:], nonce[16:24])
	return hChaCha20(k.key, nonce[:16]), n
}

func (e Crypto_AeadErr) M_display(*Task) string { return e.F_msg }

// Crypto_AeadKey is `crypto.AeadKey`: AES-GCM's cipher.AEAD, or
// ChaCha20-Poly1305's 32 byte key. It never prints its key.
type Crypto_AeadKey struct {
	alg Crypto_Aead
	// ns and ts: an AES-GCM key's nonce and tag sizes (aes_gcm_key); 0 is
	// the algorithm's
	ns, ts int
	gcm    cipher.AEAD
	key    []byte
	// cbc is the AES-128 block of Aes128CbcHmacSha256, key[:16] its MAC key
	cbc cipher.Block
}

func (Crypto_AeadKey) XoDebug(b *strings.Builder) { b.WriteString("<aeadkey>") }
func (k Crypto_AeadKey) M_debug(*Task) string     { return Debug(k) }

func aeadErr(m string) Crypto_AeadErr { return Crypto_AeadErr{F_msg: m} }

func Crypto_aead_key(_ *Task, alg Crypto_Aead, key Bytes) Result[Crypto_AeadKey, Crypto_AeadErr] {
	if alg.Tag == aeadTagCbc {
		if len(key) != 32 {
			return Err[Crypto_AeadKey](aeadErr(fmt.Sprintf("crypto: AES-128-CBC-HMAC-SHA-256 wants a 32 byte key, got %d", len(key))))
		}
		b, err := aes.NewCipher([]byte(key[16:]))
		if err != nil {
			return Err[Crypto_AeadKey](aeadErr(err.Error()))
		}
		return Ok[Crypto_AeadKey, Crypto_AeadErr](Crypto_AeadKey{alg: alg, key: []byte(key[:16]), cbc: b})
	}
	if alg.Tag == 2 || alg.Tag == aeadTagXChaCha { // ChaCha20Poly1305, XChaCha20Poly1305
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

func (k Crypto_AeadKey) M_nonce_size(*Task) int64 {
	if k.ns != 0 {
		return int64(k.ns)
	}
	return k.alg.M_nonce_size(nil)
}

func (k Crypto_AeadKey) M_overhead(*Task) int64 {
	if k.ts != 0 {
		return int64(k.ts)
	}
	return k.alg.M_overhead(nil)
}

// Crypto_aes_gcm_key_with_nonce_size is Go's NewGCMWithNonceSize.
func Crypto_aes_gcm_key_with_nonce_size(_ *Task, key Bytes, size int64) Result[Crypto_AeadKey, Crypto_AeadErr] {
	return aesGCMSized(key, size, 16)
}

// Crypto_aes_gcm_key_with_tag_size is Go's NewGCMWithTagSize.
func Crypto_aes_gcm_key_with_tag_size(_ *Task, key Bytes, size int64) Result[Crypto_AeadKey, Crypto_AeadErr] {
	return aesGCMSized(key, 12, size)
}

func aesGCMSized(key Bytes, nonceSize, tagSize int64) Result[Crypto_AeadKey, Crypto_AeadErr] {
	b, err := aes.NewCipher([]byte(key))
	if err != nil {
		return Err[Crypto_AeadKey](aeadErr(err.Error()))
	}
	var g cipher.AEAD
	if tagSize != 16 {
		g, err = cipher.NewGCMWithTagSize(b, int(tagSize))
	} else {
		g, err = cipher.NewGCMWithNonceSize(b, int(nonceSize))
	}
	if err != nil {
		return Err[Crypto_AeadKey](aeadErr(err.Error()))
	}
	return Ok[Crypto_AeadKey, Crypto_AeadErr](Crypto_AeadKey{alg: Crypto_Aead{Tag: 1}, ns: int(nonceSize), ts: int(tagSize), gcm: g})
}

// minSealed is the shortest sealed message open reads: the tag.
func (k Crypto_AeadKey) minSealed() int {
	if k.ts != 0 {
		return k.ts
	}
	return 16
}

func aeadNonce(k Crypto_AeadKey, nonce Bytes) {
	want := int(k.M_nonce_size(nil))
	if len(nonce) != want {
		FaultHere("crypto: AEAD nonce of %d bytes, want %d", len(nonce), want)
	}
}

// cbcTag is HMAC-SHA-256(aad || iv || ct || AL)[:16], AL the aad's length
// in bits as a big endian 64 bit integer (RFC 7518 5.2.2.1).
func (k Crypto_AeadKey) cbcTag(iv, ct, aad []byte) []byte {
	m := hmac.New(sha256.New, k.key)
	m.Write(aad)
	m.Write(iv)
	m.Write(ct)
	var al [8]byte
	binary.BigEndian.PutUint64(al[:], uint64(len(aad))*8)
	m.Write(al[:])
	return m.Sum(nil)[:16]
}

func (k Crypto_AeadKey) M_seal(_ *Task, nonce, plaintext, aad Bytes) Bytes {
	aeadNonce(k, nonce)
	if k.cbc != nil {
		pad := 16 - len(plaintext)%16
		buf := make([]byte, len(plaintext)+pad)
		copy(buf, plaintext)
		for i := len(plaintext); i < len(buf); i++ {
			buf[i] = byte(pad)
		}
		cipher.NewCBCEncrypter(k.cbc, []byte(nonce)).CryptBlocks(buf, buf)
		return Bytes(buf) + Bytes(k.cbcTag([]byte(nonce), buf, []byte(aad)))
	}
	if k.gcm != nil {
		return Bytes(k.gcm.Seal(nil, []byte(nonce), []byte(plaintext), []byte(aad)))
	}
	key, n := k.chachaKeyNonce([]byte(nonce))
	ct := chachaXor(key, 1, n, []byte(plaintext))
	return Bytes(ct) + Bytes(chachaTag(key, n, ct, []byte(aad)))
}

func (k Crypto_AeadKey) M_open(_ *Task, nonce, sealed, aad Bytes) Result[Bytes, Crypto_AeadErr] {
	aeadNonce(k, nonce)
	if k.cbc != nil {
		n := len(sealed) - 16
		if n < 16 || n%16 != 0 {
			return Err[Bytes](aeadErr(aeadFail))
		}
		ct := []byte(sealed[:n])
		if subtle.ConstantTimeCompare(k.cbcTag([]byte(nonce), ct, []byte(aad)), []byte(sealed[n:])) != 1 {
			return Err[Bytes](aeadErr(aeadFail))
		}
		pt := make([]byte, n)
		cipher.NewCBCDecrypter(k.cbc, []byte(nonce)).CryptBlocks(pt, ct)
		pad := int(pt[n-1])
		if pad < 1 || pad > 16 {
			return Err[Bytes](aeadErr(aeadFail))
		}
		for _, c := range pt[n-pad:] {
			if int(c) != pad {
				return Err[Bytes](aeadErr(aeadFail))
			}
		}
		return Ok[Bytes, Crypto_AeadErr](Bytes(pt[:n-pad]))
	}
	if len(sealed) < k.minSealed() {
		return Err[Bytes](aeadErr(aeadFail))
	}
	if k.gcm != nil {
		pt, err := k.gcm.Open(nil, []byte(nonce), []byte(sealed), []byte(aad))
		if err != nil {
			return Err[Bytes](aeadErr(aeadFail))
		}
		return Ok[Bytes, Crypto_AeadErr](Bytes(pt))
	}
	key, cn := k.chachaKeyNonce([]byte(nonce))
	n := len(sealed) - 16
	ct := []byte(sealed[:n])
	if subtle.ConstantTimeCompare(chachaTag(key, cn, ct, []byte(aad)), []byte(sealed[n:])) != 1 {
		return Err[Bytes](aeadErr(aeadFail))
	}
	return Ok[Bytes, Crypto_AeadErr](Bytes(chachaXor(key, 1, cn, ct)))
}

func (k Crypto_AeadKey) M_seal_random(tk *Task, r Rand, plaintext, aad Bytes) Bytes {
	nonce := r.M_bytes(tk, k.M_nonce_size(tk))
	return nonce + k.M_seal(tk, nonce, plaintext, aad)
}

func (k Crypto_AeadKey) M_open_random(tk *Task, sealed, aad Bytes) Result[Bytes, Crypto_AeadErr] {
	nn := int(k.M_nonce_size(tk))
	if len(sealed) < nn+k.minSealed() {
		return Err[Bytes](aeadErr(aeadFail))
	}
	return k.M_open(tk, sealed[:nn], sealed[nn:], aad)
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
