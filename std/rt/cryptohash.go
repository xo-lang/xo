package rt

import (
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha3"
	"crypto/sha512"
	"encoding"
	"hash"
	"strings"
)

// std/crypto's Hash, digest, hmac, and Hasher (std.md 11, decision 0279)
// on Go's hashes. A Hasher is a value: it keeps the hash's marshaled
// state (encoding.BinaryMarshaler) and every write starts from a copy,
// so copies of it never share a write. An HMAC hasher keeps the inner
// hash's state and the outer padded key, as crypto/hmac computes it.

var cryptoHashes = [...]func() hash.Hash{nil, md5.New, sha1.New, sha256.New224, sha256.New, sha512.New384, sha512.New,
	func() hash.Hash { return sha3.New256() }, func() hash.Hash { return sha3.New512() }}

func (h Crypto_Hash) M_size(*Task) int64 { return int64(cryptoHashes[h.Tag]().Size()) }

func Crypto_digest(_ *Task, h Crypto_Hash, data Bytes) Bytes {
	x := cryptoHashes[h.Tag]()
	x.Write([]byte(data))
	return Bytes(x.Sum(nil))
}

func Crypto_hmac(_ *Task, h Crypto_Hash, key, msg Bytes) Bytes {
	m := hmac.New(cryptoHashes[h.Tag], []byte(key))
	m.Write([]byte(msg))
	return Bytes(m.Sum(nil))
}

// Crypto_Hasher is `crypto.Hasher`.
type Crypto_Hasher struct {
	hash  Crypto_Hash
	state string // the inner hash's MarshalBinary
	opad  string // the outer padded key of an HMAC, "" for a plain hash
	total int64
}

func (Crypto_Hasher) XoDebug(b *strings.Builder) { b.WriteString("<hasher>") }
func (x Crypto_Hasher) M_debug(*Task) string     { return Debug(x) }

func hasherState(h hash.Hash) string {
	b, err := h.(encoding.BinaryMarshaler).MarshalBinary()
	if err != nil {
		panic(err)
	}
	return string(b)
}

func (x Crypto_Hasher) restore() hash.Hash {
	h := cryptoHashes[x.hash.Tag]()
	if err := h.(encoding.BinaryUnmarshaler).UnmarshalBinary([]byte(x.state)); err != nil {
		panic(err)
	}
	return h
}

func Crypto_hasher(_ *Task, h Crypto_Hash) Crypto_Hasher {
	return Crypto_Hasher{hash: h, state: hasherState(cryptoHashes[h.Tag]())}
}

func Crypto_hmac_hasher(_ *Task, h Crypto_Hash, key Bytes) Crypto_Hasher {
	inner := cryptoHashes[h.Tag]()
	bs := inner.BlockSize()
	k := []byte(key)
	if len(k) > bs {
		inner.Write(k)
		k = inner.Sum(nil)
		inner.Reset()
	}
	ipad, opad := make([]byte, bs), make([]byte, bs)
	copy(ipad, k)
	copy(opad, k)
	for i := range ipad {
		ipad[i] ^= 0x36
		opad[i] ^= 0x5c
	}
	inner.Write(ipad)
	return Crypto_Hasher{hash: h, state: hasherState(inner), opad: string(opad)}
}

func (x *Crypto_Hasher) M_write(_ *Task, data Bytes) {
	h := x.restore()
	h.Write([]byte(data))
	x.state = hasherState(h)
	x.total += int64(len(data))
}

func (x Crypto_Hasher) M_sum(*Task) Bytes {
	sum := x.restore().Sum(nil)
	if x.opad == "" {
		return Bytes(sum)
	}
	outer := cryptoHashes[x.hash.Tag]()
	outer.Write([]byte(x.opad))
	outer.Write(sum)
	return Bytes(outer.Sum(nil))
}

func (x Crypto_Hasher) M_written(*Task) int64 { return x.total }
