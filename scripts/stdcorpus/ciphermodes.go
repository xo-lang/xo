package main

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/hex"
	"fmt"
	mrand "math/rand"
	"strings"
)

// std/crypto's Block, KeyStream, cbc_encrypter, cbc_decrypter and
// block_ctr against Go's cipher.NewCBCEncrypter, NewCBCDecrypter and
// NewCTR over a Block other than AES's: ctr_test.go's noopBlock and
// TestCTR, cbc_test.go's TestCBCBlockMode (DES there; an 8 byte toy cipher
// here, written the same in Go and in Xo), AES behind a wrapper (Go's
// generic modes, not the AES ones) and AES itself (Copyright 2009 and
// 2015 The Go Authors, BSD-style license). Each mode runs over the data
// cut into seeded pieces, so the chaining across calls is Go's.

// noopBlock is ctr_test.go's: Encrypt copies (Decrypt too, here, where
// Go's panics, so CBC decryption runs over it).
type noopBlock int

func (b noopBlock) BlockSize() int        { return int(b) }
func (noopBlock) Encrypt(dst, src []byte) { copy(dst, src) }
func (noopBlock) Decrypt(dst, src []byte) { copy(dst, src) }

// toyBlock is an 8 byte cipher: byte i of the output is byte i + 1 of the
// input xor key byte i, rotated left by 3.
type toyBlock [8]byte

func (toyBlock) BlockSize() int { return 8 }
func (k toyBlock) Encrypt(dst, src []byte) {
	var t [8]byte
	for i := range t {
		v := src[(i+1)%8] ^ k[i]
		t[i] = v<<3 | v>>5
	}
	copy(dst, t[:])
}
func (k toyBlock) Decrypt(dst, src []byte) {
	var t [8]byte
	for i := range t {
		v := src[i]>>3 | src[i]<<5
		t[(i+1)%8] = v ^ k[i]
	}
	copy(dst, t[:])
}

// wrapped hides AES's own CBC and CTR from Go's NewCBC and NewCTR.
type wrapped struct{ b cipher.Block }

func (w wrapped) BlockSize() int          { return w.b.BlockSize() }
func (w wrapped) Encrypt(dst, src []byte) { w.b.Encrypt(dst, src) }
func (w wrapped) Decrypt(dst, src []byte) { w.b.Decrypt(dst, src) }

func cipherModesCorpus() string {
	var o strings.Builder
	o.WriteString(header("Go's crypto/cipher and its ctr_test.go and cbc_test.go (Copyright 2009 and 2015 The Go Authors, BSD-style license)"))
	o.WriteString("\nuse std/crypto\n")
	r := mrand.New(mrand.NewSource(2015))
	rnd := func(n int) []byte {
		b := make([]byte, n)
		r.Read(b)
		return b
	}
	type blk struct {
		kind, size int
		key        []byte
	}
	var blocks []blk
	for _, n := range []int{1, 2, 8, 16, 64, 128} {
		blocks = append(blocks, blk{0, n, nil})
	}
	for i := 0; i < 3; i++ {
		blocks = append(blocks, blk{1, 8, rnd(8)})
	}
	for _, kl := range []int{16, 24, 32} {
		blocks = append(blocks, blk{2, 16, rnd(kl)}, blk{3, 16, rnd(kl)})
	}
	goBlock := func(b blk) cipher.Block {
		switch b.kind {
		case 0:
			return noopBlock(b.size)
		case 1:
			return toyBlock(b.key)
		case 2:
			return wrapped{hpkeMust(aes.NewCipher(b.key))}
		}
		return hpkeMust(aes.NewCipher(b.key))
	}
	// pieces cuts n into seeded lengths, each a multiple of unit
	pieces := func(n, unit int) []int {
		var ps []int
		for n > 0 {
			p := unit * (r.Intn(4))
			if r.Intn(3) == 0 {
				p = unit * r.Intn(n/unit+1)
			}
			if p > n {
				p = n
			}
			ps = append(ps, p)
			n -= p
		}
		return append(ps, 0)
	}
	ints := func(xs []int) string {
		s := make([]string, len(xs))
		for i, x := range xs {
			s[i] = fmt.Sprint(x)
		}
		return "[" + strings.Join(s, ", ") + "]"
	}
	hexes := func(xs [][]byte) string {
		s := make([]string, len(xs))
		for i, x := range xs {
			s[i] = xq(hex.EncodeToString(x))
		}
		return "[" + strings.Join(s, ", ") + "]"
	}

	o.WriteString("\n/// (block kind: 0 noop, 1 toy, 2 AES wrapped, 3 AES; block size; key; IV; plaintext; encryption's pieces; Go's ciphertext pieces; decryption's pieces)\nconst CBC: List[(Int, Int, Bytes, Bytes, Bytes, List[Int], List[Str], List[Int])] = [\n")
	for _, b := range blocks {
		for rep := 0; rep < 3; rep++ {
			gb := goBlock(b)
			iv := rnd(b.size)
			if rep == 2 {
				iv = make([]byte, b.size)
			}
			pt := rnd(b.size * r.Intn(12))
			ep := pieces(len(pt), b.size)
			enc := cipher.NewCBCEncrypter(gb, iv)
			var cts [][]byte
			var all []byte
			at := 0
			for _, p := range ep {
				c := make([]byte, p)
				enc.CryptBlocks(c, pt[at:at+p])
				cts = append(cts, c)
				all = append(all, c...)
				at += p
			}
			back := make([]byte, len(all))
			cipher.NewCBCDecrypter(gb, iv).CryptBlocks(back, all)
			if string(back) != string(pt) {
				panic("ciphermodes: Go's CBC round trip")
			}
			fmt.Fprintf(&o, "  (%d, %d, %s, %s, %s, %s, %s, %s),\n", b.kind, b.size, xbytes(b.key), xbytes(iv), xbytes(pt), ints(ep), hexes(cts), ints(pieces(len(pt), b.size)))
		}
	}
	o.WriteString("]\n")

	o.WriteString("\n/// (block kind, block size, key, IV, data, pieces, Go's XORKeyStream of each piece in hex): TestCTR's noop blocks, the counter wrapping from all ones\nconst CTR: List[(Int, Int, Bytes, Bytes, Bytes, List[Int], List[Str])] = [\n")
	for _, b := range blocks {
		for rep := 0; rep < 3; rep++ {
			gb := goBlock(b)
			iv := rnd(b.size)
			if rep == 1 {
				for i := range iv {
					iv[i] = 0xff
				}
				if b.size > 2 {
					iv[0] = 0x7f
				}
			}
			data := rnd(r.Intn(b.size*5 + 40))
			if b.kind == 0 && rep == 0 {
				// TestCTR: 0xff bytes under a zero IV
				iv = make([]byte, b.size)
				data = []byte(strings.Repeat("\xff", b.size*4))
			}
			ps := pieces(len(data), 1)
			s := cipher.NewCTR(gb, iv)
			var outs [][]byte
			at := 0
			for _, p := range ps {
				d := make([]byte, p)
				s.XORKeyStream(d, data[at:at+p])
				outs = append(outs, d)
				at += p
			}
			fmt.Fprintf(&o, "  (%d, %d, %s, %s, %s, %s, %s),\n", b.kind, b.size, xbytes(b.key), xbytes(iv), xbytes(data), ints(ps), hexes(outs))
		}
	}
	o.WriteString("]\n")

	o.WriteString(`
/// ctr_test.go's noopBlock: a block of size bytes that copies.
struct Noop {
  size: Int
}

fn (b: Noop) block_size() -> Int { b.size }
fn (b: Noop) encrypt_block(src: Bytes) -> Bytes { src }
fn (b: Noop) decrypt_block(src: Bytes) -> Bytes { src }

/// The generator's 8 byte toy cipher.
struct Toy {
  key: Bytes
}

fn rotl3(v: Int) -> Int { ((v << 3) | (v >> 5)) & 0xff }
fn rotr3(v: Int) -> Int { ((v >> 3) | (v << 5)) & 0xff }

fn (b: Toy) block_size() -> Int { 8 }
fn (b: Toy) encrypt_block(src: Bytes) -> Bytes {
  var w = BytesBuilder.with_capacity(8)
  for i in 0..8 {
    w.write_byte(UInt8(rotl3(Int(src[(i + 1) % 8]) ^ Int(b.key[i]))))
  }
  w.freeze()
}
fn (b: Toy) decrypt_block(src: Bytes) -> Bytes {
  var t: List[Int] = [0, 0, 0, 0, 0, 0, 0, 0]
  for i in 0..8 {
    t[(i + 1) % 8] = rotr3(Int(src[i])) ^ Int(b.key[i])
  }
  var w = BytesBuilder.with_capacity(8)
  for v in t {
    w.write_byte(UInt8(v))
  }
  w.freeze()
}

/// AES through a caller's type, as the generator's wrapper.
struct Wrapped {
  b: crypto.AesBlock
}

fn (w: Wrapped) block_size() -> Int { w.b.block_size() }
fn (w: Wrapped) encrypt_block(src: Bytes) -> Bytes { w.b.encrypt_block(src) }
fn (w: Wrapped) decrypt_block(src: Bytes) -> Bytes { w.b.decrypt_block(src) }

fn block(kind: Int, size: Int, key: Bytes) -> crypto.Block {
  if kind == 0 {
    let b: crypto.Block = Noop{size: size}
    return b
  }
  if kind == 1 {
    let b: crypto.Block = Toy{key: key}
    return b
  }
  let a = crypto.aes_block(key) ?? e => fault(e.msg)
  if kind == 2 {
    let b: crypto.Block = Wrapped{b: a}
    return b
  }
  let b: crypto.Block = a
  b
}

fn show(bad: List[Str]) -> List[Str] { bad[0..(if bad.len() < 20 { bad.len() } else { 20 })] }

test "cbc_encrypter and cbc_decrypter are Go's CBC over any Block, chained across calls (TestCBCBlockMode)" {
  var bad: List[Str] = []
  for (kind, size, key, iv, pt, ep, want, dp) in CBC {
    let name = "kind ${kind} size ${size} pt ${pt.len()}"
    let b = block(kind, size, key)
    var enc = crypto.cbc_encrypter(b, iv)
    if enc.block_size() != size {
      bad.push("${name}: block_size")
    }
    var all = b""
    var at = 0
    for i in 0..ep.len() {
      let c = enc.crypt_blocks(pt[at..at + ep[i]])
      if c.to_hex() != want[i] {
        bad.push("${name}: piece ${i}")
      }
      all = all + c
      at += ep[i]
    }
    var dec = crypto.cbc_decrypter(b, iv)
    var back = b""
    at = 0
    for p in dp {
      back = back + dec.crypt_blocks(all[at..at + p])
      at += p
    }
    if back != pt {
      bad.push("${name}: decrypt")
    }
    // a copy goes on from the same place by itself
    if pt.len() >= 2 * size {
      var a = crypto.cbc_encrypter(b, iv)
      let _ = a.crypt_blocks(pt[0..size])
      var c = a
      if a.crypt_blocks(pt[size..2 * size]) != c.crypt_blocks(pt[size..2 * size]) || a.crypt_blocks(pt[0..size]) != c.crypt_blocks(pt[0..size]) {
        bad.push("${name}: copy")
      }
    }
  }
  expect(show(bad)) == []
}

/// The pieces of data through a KeyStream, Ctr or BlockCtr alike.
fn stream_pieces(k: crypto.KeyStream, data: Bytes, ps: List[Int]) -> List[Str] {
  var s = k
  var out: List[Str] = []
  var at = 0
  for p in ps {
    out.push(s.xor(data[at..at + p]).to_hex())
    at += p
  }
  out
}

test "block_ctr is Go's CTR over any Block, through KeyStream as Ctr (TestCTR)" {
  var bad: List[Str] = []
  for (kind, size, key, iv, data, ps, want) in CTR {
    let name = "kind ${kind} size ${size} data ${data.len()}"
    let b = block(kind, size, key)
    if stream_pieces(crypto.block_ctr(b, iv), data, ps) != want {
      bad.push("${name}: block_ctr")
    }
    if kind >= 2 {
      let a = crypto.aes_block(key) ?? e => fault(e.msg)
      if stream_pieces(crypto.ctr(a, iv), data, ps) != want {
        bad.push("${name}: ctr")
      }
    }
    // a copy goes on from the same place by itself
    var c = crypto.block_ctr(b, iv)
    let _ = c.xor(data[0..data.len() / 3])
    var d = c
    if c.xor(data) != d.xor(data) {
      bad.push("${name}: copy")
    }
  }
  expect(show(bad)) == []
}

test "AesBlock is a Block of 16 bytes" {
  let a = crypto.aes_block(b"0123456789abcdef") ?? e => fault(e.msg)
  let b: crypto.Block = a
  expect(b.block_size()) == 16
  expect(b.decrypt_block(b.encrypt_block(b"abcdefghijklmnop"))) == b"abcdefghijklmnop"
}
`)
	return o.String()
}
