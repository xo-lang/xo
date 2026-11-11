package rt

import (
	"crypto/ecdh"
	"strings"
)

// ECDH (decision 0599): Go's crypto/ecdh, with keys generated from r as
// the native code reads it (X25519: 32 bytes; a NIST curve: Go's
// randomPoint sampling as Crypto_generate_key's ECDSA path), so the same r
// gives the same key on every backend.

// Crypto_EcdhPrivateKey is `crypto.EcdhPrivateKey`. Debug never shows key
// material.
type Crypto_EcdhPrivateKey struct {
	k   *ecdh.PrivateKey
	crv Crypto_EcdhCurve
}

// Crypto_EcdhPublicKey is `crypto.EcdhPublicKey`.
type Crypto_EcdhPublicKey struct {
	k   *ecdh.PublicKey
	crv Crypto_EcdhCurve
}

func ecdhCurve(c Crypto_EcdhCurve) ecdh.Curve {
	switch c.Tag {
	case 1:
		return ecdh.X25519()
	case 3:
		return ecdh.P384()
	case 4:
		return ecdh.P521()
	}
	return ecdh.P256()
}

func (v Crypto_EcdhPrivateKey) XoEq(x any) bool {
	o, ok := x.(Crypto_EcdhPrivateKey)
	return ok && v.k.Equal(o.k)
}

func (v Crypto_EcdhPrivateKey) XoHash() uint64 { return Hash(string(v.k.PublicKey().Bytes())) }

func (Crypto_EcdhPrivateKey) XoDebug(b *strings.Builder) { b.WriteString("<ecdhprivatekey>") }

func (v Crypto_EcdhPrivateKey) M_debug(*Task) string { return Debug(v) }

func (v Crypto_EcdhPublicKey) XoEq(x any) bool {
	o, ok := x.(Crypto_EcdhPublicKey)
	return ok && v.k.Equal(o.k)
}

func (v Crypto_EcdhPublicKey) XoHash() uint64 { return Hash(string(v.k.Bytes())) }

func (Crypto_EcdhPublicKey) XoDebug(b *strings.Builder) { b.WriteString("<ecdhpublickey>") }

func (v Crypto_EcdhPublicKey) M_debug(*Task) string { return Debug(v) }

// Crypto_ecdh_generate_key is Curve.GenerateKey reading r as the native
// code does.
func Crypto_ecdh_generate_key(tk *Task, r Rand, c Crypto_EcdhCurve) Crypto_EcdhPrivateKey {
	cv := ecdhCurve(c)
	if c.Tag == 1 {
		k, err := cv.NewPrivateKey([]byte(r.M_bytes(tk, 32)))
		if err != nil {
			panic("crypto.ecdh_generate_key: " + err.Error())
		}
		return Crypto_EcdhPrivateKey{k: k, crv: c}
	}
	size := map[int]int{2: 32, 3: 48, 4: 66}[c.Tag]
	for {
		b := []byte(r.M_bytes(tk, int64(size)))
		if c.Tag == 4 {
			for i := len(b) - 1; i >= 0; i-- {
				b[i] >>= 7
				if i > 0 {
					b[i] |= b[i-1] << 1
				}
			}
		}
		if k, err := cv.NewPrivateKey(b); err == nil {
			return Crypto_EcdhPrivateKey{k: k, crv: c}
		}
	}
}

// Crypto_ecdh_private_key is Curve.NewPrivateKey.
func Crypto_ecdh_private_key(_ *Task, c Crypto_EcdhCurve, key Bytes) Result[Crypto_EcdhPrivateKey, Crypto_KeyErr] {
	k, err := ecdhCurve(c).NewPrivateKey([]byte(key))
	if err != nil {
		return Err[Crypto_EcdhPrivateKey](keyErr(err.Error()))
	}
	return Ok[Crypto_EcdhPrivateKey, Crypto_KeyErr](Crypto_EcdhPrivateKey{k: k, crv: c})
}

// Crypto_ecdh_public_key is Curve.NewPublicKey.
func Crypto_ecdh_public_key(_ *Task, c Crypto_EcdhCurve, key Bytes) Result[Crypto_EcdhPublicKey, Crypto_KeyErr] {
	k, err := ecdhCurve(c).NewPublicKey([]byte(key))
	if err != nil {
		return Err[Crypto_EcdhPublicKey](keyErr(err.Error()))
	}
	return Ok[Crypto_EcdhPublicKey, Crypto_KeyErr](Crypto_EcdhPublicKey{k: k, crv: c})
}

func (v Crypto_EcdhPrivateKey) M_curve(*Task) Crypto_EcdhCurve { return v.crv }

func (v Crypto_EcdhPrivateKey) M_bytes(*Task) Bytes { return Bytes(v.k.Bytes()) }

func (v Crypto_EcdhPrivateKey) M_public_key(*Task) Crypto_EcdhPublicKey {
	return Crypto_EcdhPublicKey{k: v.k.PublicKey(), crv: v.crv}
}

func (v Crypto_EcdhPrivateKey) M_ecdh(_ *Task, peer Crypto_EcdhPublicKey) Result[Bytes, Crypto_KeyErr] {
	s, err := v.k.ECDH(peer.k)
	if err != nil {
		return Err[Bytes](keyErr(err.Error()))
	}
	return Ok[Bytes, Crypto_KeyErr](Bytes(s))
}

func (v Crypto_EcdhPublicKey) M_curve(*Task) Crypto_EcdhCurve { return v.crv }

func (v Crypto_EcdhPublicKey) M_bytes(*Task) Bytes { return Bytes(v.k.Bytes()) }
