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
	func() hash.Hash { return sha3.New224() }, func() hash.Hash { return sha3.New384() }, sha512.New512_224, sha512.New512_256,
	func() hash.Hash { return newB2b(32) }, func() hash.Hash { return newB2b(48) }, func() hash.Hash { return newB2b(64) },
	newLegacyKeccak256, newLegacyKeccak512}

// The tags of Hash.Blake2b_256 to Blake2b_512 and Keccak256 and Keccak512
// (after Sha512_256's 12).
const (
	hashTagBlake2b256 = 13
	hashTagBlake2b512 = 15
	hashTagKeccak256  = 16
	hashTagKeccak512  = 17
)

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
	// a blake2b_hasher's digest size and key (b2size 0 for the others):
	// its state restores into a digest made with them
	b2size int
	b2key  string
}

// newHash is a new hash of x's: its Hash's, or blake2b_hasher's keyed
// BLAKE2b.
func (x Crypto_Hasher) newHash() hash.Hash {
	if x.b2size != 0 {
		d, err := newB2bDigest(x.b2size, []byte(x.b2key))
		if err != nil {
			panic(err)
		}
		return d
	}
	return cryptoHashes[x.hash.Tag]()
}

func (Crypto_Hasher) XoDebug(b *strings.Builder) { b.WriteString("<hasher>") }
func (x Crypto_Hasher) M_debug(*Task) string     { return Debug(x) }

func hasherState(h hash.Hash) string {
	if d, ok := h.(*b2bDigest); ok {
		// a keyed digest's too (its MarshalBinary refuses a MAC)
		return string(d.marshalState())
	}
	b, err := h.(encoding.BinaryMarshaler).MarshalBinary()
	if err != nil {
		panic(err)
	}
	return string(b)
}

func (x Crypto_Hasher) restore() hash.Hash {
	h := x.newHash()
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

func (x Crypto_Hasher) M_block_size(*Task) int64 { return int64(x.newHash().BlockSize()) }

func (x Crypto_Hasher) M_size(*Task) int64 { return int64(x.restore().Size()) }

// Crypto_blake2b_hasher is x/crypto's blake2b.New(size, key).
func Crypto_blake2b_hasher(_ *Task, size int64, key Bytes) Result[Crypto_Hasher, Crypto_KeyErr] {
	if size < 1 || size > b2bSize {
		return Err[Crypto_Hasher](keyErr(b2bErrHashSize.Error()))
	}
	if len(key) > b2bSize {
		return Err[Crypto_Hasher](keyErr(b2bErrKeySize.Error()))
	}
	tag := hashTagBlake2b512
	switch size {
	case 32:
		tag = hashTagBlake2b256
	case 48:
		tag = hashTagBlake2b256 + 1
	}
	x := Crypto_Hasher{hash: Crypto_Hash{Tag: tag}, b2size: int(size), b2key: string(key)}
	x.state = hasherState(x.newHash())
	x.start = x.state
	return Ok[Crypto_Hasher, Crypto_KeyErr](x)
}

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
	if x.b2key != "" {
		return Err[Bytes, Crypto_HashErr](Crypto_HashErr{F_msg: "crypto/blake2b: cannot marshal MACs"})
	}
	return Ok[Bytes, Crypto_HashErr](Bytes(x.state))
}

// M_unmarshal_binary is Go's UnmarshalBinary into a new hash of x's; the
// length is the state's last 8 bytes, but for SHA-3, which keeps none.
func (x Crypto_Hasher) M_unmarshal_binary(_ *Task, state Bytes) Result[Crypto_Hasher, Crypto_HashErr] {
	if x.opad != "" {
		return Err[Crypto_Hasher, Crypto_HashErr](Crypto_HashErr{F_msg: hmacNoState})
	}
	h := x.newHash()
	if err := h.(encoding.BinaryUnmarshaler).UnmarshalBinary([]byte(state)); err != nil {
		return Err[Crypto_Hasher, Crypto_HashErr](Crypto_HashErr{F_msg: err.Error()})
	}
	var total int64
	switch d := h.(type) {
	case *sha3.SHA3, *keccakState:
	case *b2bDigest:
		// the bytes compressed and held, less a keyed digest's key block
		total = int64(d.c[0]) + int64(d.offset)
		if d.keyLen > 0 {
			total -= b2bBlockSize
		}
	default:
		total = int64(binary.BigEndian.Uint64([]byte(state[len(state)-8:])))
	}
	return Ok[Crypto_Hasher, Crypto_HashErr](Crypto_Hasher{hash: x.hash, state: hasherState(h), total: total, start: x.start, b2size: x.b2size, b2key: x.b2key})
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

// Crypto_Blake2bXof is `crypto.Blake2bXof`: a Go value copy of the XOF
// copies its whole state (x/crypto's Clone).
type Crypto_Blake2bXof struct{ x b2bXOF }

func (Crypto_Blake2bXof) XoDebug(b *strings.Builder) { b.WriteString("<blake2bxof>") }
func (x Crypto_Blake2bXof) M_debug(*Task) string     { return Debug(x) }

// Crypto_blake2b_xof is x/crypto's blake2b.NewXOF; a size outside uint32
// is its length error too.
func Crypto_blake2b_xof(_ *Task, size int64, key Bytes) Result[Crypto_Blake2bXof, Crypto_KeyErr] {
	if len(key) > b2bSize {
		return Err[Crypto_Blake2bXof](keyErr(b2bErrKeySize.Error()))
	}
	if size < 0 || size > b2bMagicUnknownOutputLength {
		size = b2bMagicUnknownOutputLength
	}
	x, err := newB2bXOF(uint32(size), []byte(key))
	if err != nil {
		return Err[Crypto_Blake2bXof](keyErr(err.Error()))
	}
	return Ok[Crypto_Blake2bXof, Crypto_KeyErr](Crypto_Blake2bXof{*x})
}

func (x *Crypto_Blake2bXof) M_write(_ *Task, data Bytes) {
	defer func() {
		if r := recover(); r != nil {
			FaultHere("%v", r)
		}
	}()
	x.x.Write([]byte(data))
}

// M_read is the XOF's Read: up to n bytes, fewer at the end of the
// output, then Go's io.EOF as an Err.
func (x *Crypto_Blake2bXof) M_read(_ *Task, n int64) Result[Bytes, Crypto_KeyErr] {
	if n < 0 {
		FaultHere("blake2b: a read of %d bytes", n)
	}
	if uint64(n) > x.x.remaining {
		n = int64(x.x.remaining)
	}
	out := make([]byte, n)
	got, err := x.x.Read(out)
	if err != nil {
		return Err[Bytes](keyErr(err.Error()))
	}
	return Ok[Bytes, Crypto_KeyErr](Bytes(out[:got]))
}

func (x *Crypto_Blake2bXof) M_reset(*Task) { x.x.Reset() }
