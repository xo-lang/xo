package rt

import (
	"crypto/aes"
	"crypto/cipher"
	"strings"
)

// AES block, CBC and CTR (decision 0605): Go's crypto/aes and
// crypto/cipher. Ctr is a value in Xo (a copy continues independently),
// so it keeps the counter and the unused keystream rather than a shared
// cipher.Stream, and makes each call's keystream with cipher.NewCTR from
// the counter.

// Crypto_AesBlock is `crypto.AesBlock`.
type Crypto_AesBlock struct {
	b   cipher.Block
	key string
}

// Crypto_Ctr is `crypto.Ctr`.
type Crypto_Ctr struct {
	b   cipher.Block
	ctr [16]byte
	ks  []byte
}

func (v Crypto_AesBlock) XoEq(x any) bool {
	o, ok := x.(Crypto_AesBlock)
	return ok && v.key == o.key
}

func (v Crypto_AesBlock) XoHash() uint64 { return Hash(v.key) }

func (Crypto_AesBlock) XoDebug(b *strings.Builder) { b.WriteString("<aesblock>") }

func (v Crypto_AesBlock) M_debug(*Task) string { return Debug(v) }

func (v Crypto_Ctr) XoEq(x any) bool {
	o, ok := x.(Crypto_Ctr)
	return ok && v.ctr == o.ctr && string(v.ks) == string(o.ks)
}

func (v Crypto_Ctr) XoHash() uint64 { return Hash(string(v.ctr[:]) + string(v.ks)) }

func (Crypto_Ctr) XoDebug(b *strings.Builder) { b.WriteString("<ctr>") }

func (v Crypto_Ctr) M_debug(*Task) string { return Debug(v) }

// Crypto_aes_block is aes.NewCipher.
func Crypto_aes_block(_ *Task, key Bytes) Result[Crypto_AesBlock, Crypto_KeyErr] {
	b, err := aes.NewCipher([]byte(key))
	if err != nil {
		return Err[Crypto_AesBlock](keyErr(err.Error()))
	}
	return Ok[Crypto_AesBlock, Crypto_KeyErr](Crypto_AesBlock{b: b, key: string(key)})
}

func aesOneBlock(src Bytes) []byte {
	if len(src) != aes.BlockSize {
		FaultHere("crypto/aes: input not full block")
	}
	return make([]byte, aes.BlockSize)
}

func (v Crypto_AesBlock) M_encrypt_block(_ *Task, src Bytes) Bytes {
	out := aesOneBlock(src)
	v.b.Encrypt(out, []byte(src))
	return Bytes(out)
}

func (v Crypto_AesBlock) M_decrypt_block(_ *Task, src Bytes) Bytes {
	out := aesOneBlock(src)
	v.b.Decrypt(out, []byte(src))
	return Bytes(out)
}

func cbcCheck(iv, data Bytes, who string) {
	if len(iv) != aes.BlockSize {
		FaultHere("cipher.%s: IV length must equal block size", who)
	}
	if len(data)%aes.BlockSize != 0 {
		FaultHere("crypto/cipher: input not full blocks")
	}
}

// Crypto_cbc_encrypt is NewCBCEncrypter and CryptBlocks.
func Crypto_cbc_encrypt(_ *Task, b Crypto_AesBlock, iv Bytes, pt Bytes) Bytes {
	cbcCheck(iv, pt, "NewCBCEncrypter")
	out := make([]byte, len(pt))
	cipher.NewCBCEncrypter(b.b, []byte(iv)).CryptBlocks(out, []byte(pt))
	return Bytes(out)
}

// Crypto_cbc_decrypt is NewCBCDecrypter and CryptBlocks.
func Crypto_cbc_decrypt(_ *Task, b Crypto_AesBlock, iv Bytes, ct Bytes) Bytes {
	cbcCheck(iv, ct, "NewCBCDecrypter")
	out := make([]byte, len(ct))
	cipher.NewCBCDecrypter(b.b, []byte(iv)).CryptBlocks(out, []byte(ct))
	return Bytes(out)
}

// Crypto_ctr is NewCTR.
func Crypto_ctr(_ *Task, b Crypto_AesBlock, iv Bytes) Crypto_Ctr {
	if len(iv) != aes.BlockSize {
		FaultHere("cipher.NewCTR: IV length must equal block size")
	}
	c := Crypto_Ctr{b: b.b}
	copy(c.ctr[:], iv)
	return c
}

// M_xor is XORKeyStream.
func (c *Crypto_Ctr) M_xor(_ *Task, data Bytes) Bytes {
	src := []byte(data)
	out := make([]byte, len(src))
	used := min(len(c.ks), len(src))
	for i := 0; i < used; i++ {
		out[i] = src[i] ^ c.ks[i]
	}
	rest := src[used:]
	if len(rest) == 0 {
		c.ks = c.ks[used:]
		return Bytes(out)
	}
	nb := (len(rest) + 15) / 16
	ks := make([]byte, nb*16)
	cipher.NewCTR(c.b, c.ctr[:]).XORKeyStream(ks, ks)
	for i := range rest {
		out[used+i] = rest[i] ^ ks[i]
	}
	c.ks = ks[len(rest):]
	// the counter advances by nb blocks, wrapping at 2^128
	carry := uint64(nb)
	for i := 15; i >= 0 && carry != 0; i-- {
		s := uint64(c.ctr[i]) + carry
		c.ctr[i] = byte(s)
		carry = s >> 8
	}
	return Bytes(out)
}
