package rt

import (
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha3"
	"crypto/sha512"
	"encoding"
	"encoding/binary"
	"hash"
	"strings"
)

// std/crypto's Hash, digest, hmac, and Hasher (std.md 11, decision 0279)
// on Go's hashes. A Hasher is a value: it keeps the hash's marshaled
// state (encoding.BinaryMarshaler) and every write starts from a copy,
// so copies of it never share a write. An HMAC hasher keeps the inner
// hash's state and the outer padded key, as crypto/hmac computes it.

var cryptoHashes = [...]func() hash.Hash{nil, md5.New, sha1.New, sha256.New224, sha256.New, sha512.New384, sha512.New,
	func() hash.Hash { return sha3.New256() }, func() hash.Hash { return sha3.New512() },
	func() hash.Hash { return sha3.New224() }, func() hash.Hash { return sha3.New384() }, sha512.New512_224, sha512.New512_256}

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
	start string // the state hasher or hmac_hasher made, for reset
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
	s := hasherState(cryptoHashes[h.Tag]())
	return Crypto_Hasher{hash: h, state: s, start: s}
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
	s := hasherState(inner)
	return Crypto_Hasher{hash: h, state: s, opad: string(opad), start: s}
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

func (x Crypto_Hasher) M_block_size(*Task) int64 {
	return int64(cryptoHashes[x.hash.Tag]().BlockSize())
}

func (x Crypto_Hasher) M_size(*Task) int64 { return int64(cryptoHashes[x.hash.Tag]().Size()) }

func (x *Crypto_Hasher) M_reset(*Task) {
	x.state = x.start
	x.total = 0
}

func (e Crypto_HashErr) M_display(*Task) string { return e.F_msg }

const hmacNoState = "crypto: an HMAC Hasher has no binary state"

// M_marshal_binary is Go's MarshalBinary of the hash (the state kept).
func (x Crypto_Hasher) M_marshal_binary(*Task) Result[Bytes, Crypto_HashErr] {
	if x.opad != "" {
		return Err[Bytes, Crypto_HashErr](Crypto_HashErr{F_msg: hmacNoState})
	}
	return Ok[Bytes, Crypto_HashErr](Bytes(x.state))
}

// M_unmarshal_binary is Go's UnmarshalBinary into a new hash of x's; the
// length is the state's last 8 bytes, but for SHA-3, which keeps none.
func (x Crypto_Hasher) M_unmarshal_binary(_ *Task, state Bytes) Result[Crypto_Hasher, Crypto_HashErr] {
	if x.opad != "" {
		return Err[Crypto_Hasher, Crypto_HashErr](Crypto_HashErr{F_msg: hmacNoState})
	}
	h := cryptoHashes[x.hash.Tag]()
	if err := h.(encoding.BinaryUnmarshaler).UnmarshalBinary([]byte(state)); err != nil {
		return Err[Crypto_Hasher, Crypto_HashErr](Crypto_HashErr{F_msg: err.Error()})
	}
	var total int64
	if _, sha3 := h.(*sha3.SHA3); !sha3 {
		total = int64(binary.BigEndian.Uint64([]byte(state[len(state)-8:])))
	}
	return Ok[Crypto_Hasher, Crypto_HashErr](Crypto_Hasher{hash: x.hash, state: hasherState(h), total: total, start: x.start})
}

// Crypto_Shake is `crypto.Shake`: a Go value copy of sha3.SHAKE copies its
// whole state (the cSHAKE init block is shared, and never written).
type Crypto_Shake struct{ s sha3.SHAKE }

func (Crypto_Shake) XoDebug(b *strings.Builder) { b.WriteString("<shake>") }
func (x Crypto_Shake) M_debug(*Task) string     { return Debug(x) }

func Crypto_shake128(*Task) Crypto_Shake { return Crypto_Shake{*sha3.NewSHAKE128()} }
func Crypto_shake256(*Task) Crypto_Shake { return Crypto_Shake{*sha3.NewSHAKE256()} }
func Crypto_cshake128(_ *Task, n, s Bytes) Crypto_Shake {
	return Crypto_Shake{*sha3.NewCSHAKE128([]byte(n), []byte(s))}
}
func Crypto_cshake256(_ *Task, n, s Bytes) Crypto_Shake {
	return Crypto_Shake{*sha3.NewCSHAKE256([]byte(n), []byte(s))}
}

func (x Crypto_Shake) M_marshal_binary(*Task) Bytes {
	b, err := x.s.MarshalBinary()
	if err != nil {
		panic(err)
	}
	return Bytes(b)
}

func (x Crypto_Shake) M_unmarshal_binary(_ *Task, state Bytes) Result[Crypto_Shake, Crypto_HashErr] {
	if err := x.s.UnmarshalBinary([]byte(state)); err != nil {
		return Err[Crypto_Shake, Crypto_HashErr](Crypto_HashErr{F_msg: err.Error()})
	}
	return Ok[Crypto_Shake, Crypto_HashErr](x)
}

func (x *Crypto_Shake) M_write(_ *Task, data Bytes) {
	defer func() {
		if r := recover(); r != nil {
			FaultHere("%v", r)
		}
	}()
	x.s.Write([]byte(data))
}

func (x *Crypto_Shake) M_read(_ *Task, n int64) Bytes {
	if n < 0 {
		FaultHere("sha3: a read of %d bytes", n)
	}
	out := make([]byte, n)
	x.s.Read(out)
	return Bytes(out)
}

func (x *Crypto_Shake) M_reset(*Task)           { x.s.Reset() }
func (x Crypto_Shake) M_block_size(*Task) int64 { return int64(x.s.BlockSize()) }

func Crypto_sum_shake128(_ *Task, data Bytes, n int64) Bytes {
	if n < 0 {
		FaultHere("sha3: a read of %d bytes", n)
	}
	return Bytes(sha3.SumSHAKE128([]byte(data), int(n)))
}

func Crypto_sum_shake256(_ *Task, data Bytes, n int64) Bytes {
	if n < 0 {
		FaultHere("sha3: a read of %d bytes", n)
	}
	return Bytes(sha3.SumSHAKE256([]byte(data), int(n)))
}
