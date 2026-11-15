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

// M_block_size is Block.BlockSize.
func (v Crypto_AesBlock) M_block_size(*Task) int64 { return aes.BlockSize }

// xoBlock is a caller's crypto.Block as Go's cipher.Block, so CbcMode and
// BlockCtr run Go's generic CBC and CTR over it.
type xoBlock struct {
	tk *Task
	b  Crypto_Block
	bs int
}

func newXoBlock(tk *Task, b Crypto_Block) xoBlock {
	return xoBlock{tk, b, int(b.M_block_size(tk))}
}

func (x xoBlock) BlockSize() int { return x.bs }

func xoOneBlock(out Bytes, bs int, what string) []byte {
	if len(out) != bs {
		FaultHere("crypto: the Block's %s gave %d bytes for a %d byte block", what, len(out), bs)
	}
	return []byte(out)
}

func (x xoBlock) Encrypt(dst, src []byte) {
	copy(dst, xoOneBlock(x.b.M_encrypt_block(x.tk, Bytes(src[:x.bs])), x.bs, "encrypt_block"))
}

func (x xoBlock) Decrypt(dst, src []byte) {
	copy(dst, xoOneBlock(x.b.M_decrypt_block(x.tk, Bytes(src[:x.bs])), x.bs, "decrypt_block"))
}

// Crypto_CbcMode is `crypto.CbcMode`: the Block and the IV the next block
// chains on, so a copy goes on by itself; each call runs Go's
// NewCBCEncrypter or NewCBCDecrypter from that IV.
type Crypto_CbcMode struct {
	b       Crypto_Block
	iv      string
	decrypt bool
}

func (v Crypto_CbcMode) XoEq(x any) bool {
	o, ok := x.(Crypto_CbcMode)
	return ok && v.iv == o.iv && v.decrypt == o.decrypt && Eq(v.b, o.b)
}

func (v Crypto_CbcMode) XoHash() uint64 { return Hash(v.iv) }

func (Crypto_CbcMode) XoDebug(b *strings.Builder) { b.WriteString("<cbcmode>") }

func (v Crypto_CbcMode) M_debug(*Task) string { return Debug(v) }

func cbcMode(tk *Task, b Crypto_Block, iv Bytes, decrypt bool, who string) Crypto_CbcMode {
	if len(iv) != int(b.M_block_size(tk)) {
		FaultHere("cipher.%s: IV length must equal block size", who)
	}
	return Crypto_CbcMode{b: b, iv: string(iv), decrypt: decrypt}
}

// Crypto_cbc_encrypter is NewCBCEncrypter over any Block.
func Crypto_cbc_encrypter(tk *Task, b Crypto_Block, iv Bytes) Crypto_CbcMode {
	return cbcMode(tk, b, iv, false, "NewCBCEncrypter")
}

// Crypto_cbc_decrypter is NewCBCDecrypter over any Block.
func Crypto_cbc_decrypter(tk *Task, b Crypto_Block, iv Bytes) Crypto_CbcMode {
	return cbcMode(tk, b, iv, true, "NewCBCDecrypter")
}

// M_block_size is BlockMode.BlockSize.
func (m Crypto_CbcMode) M_block_size(tk *Task) int64 { return m.b.M_block_size(tk) }

// M_crypt_blocks is CryptBlocks.
func (m *Crypto_CbcMode) M_crypt_blocks(tk *Task, src Bytes) Bytes {
	x := newXoBlock(tk, m.b)
	if len(src)%x.bs != 0 {
		FaultHere("crypto/cipher: input not full blocks")
	}
	out := make([]byte, len(src))
	if len(src) == 0 {
		return Bytes(out)
	}
	if m.decrypt {
		cipher.NewCBCDecrypter(x, []byte(m.iv)).CryptBlocks(out, []byte(src))
		m.iv = string(src[len(src)-x.bs:])
	} else {
		cipher.NewCBCEncrypter(x, []byte(m.iv)).CryptBlocks(out, []byte(src))
		m.iv = string(out[len(out)-x.bs:])
	}
	return Bytes(out)
}

// Crypto_BlockCtr is `crypto.BlockCtr`: the Block, the counter and the
// unused keystream, as Crypto_Ctr.
type Crypto_BlockCtr struct {
	b   Crypto_Block
	ctr []byte
	ks  []byte
}

func (v Crypto_BlockCtr) XoEq(x any) bool {
	o, ok := x.(Crypto_BlockCtr)
	return ok && string(v.ctr) == string(o.ctr) && string(v.ks) == string(o.ks) && Eq(v.b, o.b)
}

func (v Crypto_BlockCtr) XoHash() uint64 { return Hash(string(v.ctr) + string(v.ks)) }

func (Crypto_BlockCtr) XoDebug(b *strings.Builder) { b.WriteString("<blockctr>") }

func (v Crypto_BlockCtr) M_debug(*Task) string { return Debug(v) }

// Crypto_block_ctr is NewCTR over any Block.
func Crypto_block_ctr(tk *Task, b Crypto_Block, iv Bytes) Crypto_BlockCtr {
	if len(iv) != int(b.M_block_size(tk)) {
		FaultHere("cipher.NewCTR: IV length must equal block size")
	}
	return Crypto_BlockCtr{b: b, ctr: []byte(iv)}
}

// M_xor is XORKeyStream: Go's generic CTR from the counter for the blocks
// the data needs past the unused keystream.
func (c *Crypto_BlockCtr) M_xor(tk *Task, data Bytes) Bytes {
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
	x := newXoBlock(tk, c.b)
	nb := (len(rest) + x.bs - 1) / x.bs
	ks := make([]byte, nb*x.bs)
	cipher.NewCTR(x, c.ctr).XORKeyStream(ks, ks)
	for i := range rest {
		out[used+i] = rest[i] ^ ks[i]
	}
	c.ks = ks[len(rest):]
	ctr := append([]byte(nil), c.ctr...)
	for n := 0; n < nb; n++ {
		for i := len(ctr) - 1; i >= 0; i-- {
			ctr[i]++
			if ctr[i] != 0 {
				break
			}
		}
	}
	c.ctr = ctr
	return Bytes(out)
}
