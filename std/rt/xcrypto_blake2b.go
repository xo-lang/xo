// Vendored from golang.org/x/crypto v0.55.0, package blake2b (blake2b.go,
// blake2b_generic.go), reduced to the unkeyed digest that Argon2 needs and
// renamed into package rt (decision 0059). Only the portable Go code is
// kept: no assembly, no golang.org/x/sys/cpu.
//
// Copyright 2016 The Go Authors. All rights reserved.
//
// Redistribution and use in source and binary forms, with or without
// modification, are permitted provided that the following conditions are
// met:
//
//   - Redistributions of source code must retain the above copyright
//     notice, this list of conditions and the following disclaimer.
//   - Redistributions in binary form must reproduce the above
//     copyright notice, this list of conditions and the following disclaimer
//     in the documentation and/or other materials provided with the
//     distribution.
//   - Neither the name of Google LLC nor the names of its
//     contributors may be used to endorse or promote products derived from
//     this software without specific prior written permission.
//
// THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS
// "AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT
// LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR
// A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT
// OWNER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL,
// SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT
// LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE,
// DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY
// THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
// (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
// OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.

package rt

import (
	"encoding/binary"
	"math/bits"
)

const (
	// b2bBlockSize is the block size of BLAKE2b in bytes.
	b2bBlockSize = 128
	// b2bSize is the hash size of BLAKE2b-512 in bytes.
	b2bSize = 64
)

var b2bIV = [8]uint64{
	0x6a09e667f3bcc908, 0xbb67ae8584caa73b, 0x3c6ef372fe94f82b, 0xa54ff53a5f1d36f1,
	0x510e527fade682d1, 0x9b05688c2b3e6c1f, 0x1f83d9abfb41bd6b, 0x5be0cd19137e2179,
}

// b2bDigest is an unkeyed BLAKE2b hash with an output size of 1 to 64 bytes.
type b2bDigest struct {
	h      [8]uint64
	c      [2]uint64
	size   int
	block  [b2bBlockSize]byte
	offset int
}

// newB2b returns a BLAKE2b digest of size bytes (1 to 64).
func newB2b(size int) *b2bDigest {
	if size < 1 || size > b2bSize {
		panic("blake2b: invalid hash size")
	}
	d := &b2bDigest{size: size}
	d.Reset()
	return d
}

func (d *b2bDigest) Reset() {
	d.h = b2bIV
	d.h[0] ^= uint64(d.size) | (1 << 16) | (1 << 24)
	d.offset, d.c[0], d.c[1] = 0, 0, 0
}

func (d *b2bDigest) Write(p []byte) {
	n := len(p)
	if d.offset > 0 {
		remaining := b2bBlockSize - d.offset
		if n <= remaining {
			d.offset += copy(d.block[d.offset:], p)
			return
		}
		copy(d.block[d.offset:], p[:remaining])
		b2bHashBlocks(&d.h, &d.c, 0, d.block[:])
		d.offset = 0
		p = p[remaining:]
	}
	if length := len(p); length > b2bBlockSize {
		nn := length &^ (b2bBlockSize - 1)
		if length == nn {
			nn -= b2bBlockSize
		}
		b2bHashBlocks(&d.h, &d.c, 0, p[:nn])
		p = p[nn:]
	}
	if len(p) > 0 {
		d.offset += copy(d.block[:], p)
	}
}

func (d *b2bDigest) Sum(sum []byte) []byte {
	var hash [b2bSize]byte
	d.finalize(&hash)
	return append(sum, hash[:d.size]...)
}

func (d *b2bDigest) finalize(hash *[b2bSize]byte) {
	var block [b2bBlockSize]byte
	copy(block[:], d.block[:d.offset])
	remaining := uint64(b2bBlockSize - d.offset)
	c := d.c
	if c[0] < remaining {
		c[1]--
	}
	c[0] -= remaining
	h := d.h
	b2bHashBlocks(&h, &c, 0xFFFFFFFFFFFFFFFF, block[:])
	for i, v := range h {
		binary.LittleEndian.PutUint64(hash[8*i:], v)
	}
}

// b2bPrecomputed holds the message schedule of each of the 12 rounds,
// computed from the sigma constants.
var b2bPrecomputed = [12][16]byte{
	{0, 2, 4, 6, 1, 3, 5, 7, 8, 10, 12, 14, 9, 11, 13, 15},
	{14, 4, 9, 13, 10, 8, 15, 6, 1, 0, 11, 5, 12, 2, 7, 3},
	{11, 12, 5, 15, 8, 0, 2, 13, 10, 3, 7, 9, 14, 6, 1, 4},
	{7, 3, 13, 11, 9, 1, 12, 14, 2, 5, 4, 15, 6, 10, 0, 8},
	{9, 5, 2, 10, 0, 7, 4, 15, 14, 11, 6, 3, 1, 12, 8, 13},
	{2, 6, 0, 8, 12, 10, 11, 3, 4, 7, 15, 1, 13, 5, 14, 9},
	{12, 1, 14, 4, 5, 15, 13, 10, 0, 6, 9, 8, 7, 3, 2, 11},
	{13, 7, 12, 3, 11, 14, 1, 9, 5, 15, 8, 2, 0, 4, 6, 10},
	{6, 14, 11, 0, 15, 9, 3, 8, 12, 13, 1, 10, 2, 7, 4, 5},
	{10, 8, 7, 1, 2, 4, 6, 5, 15, 9, 3, 13, 11, 14, 12, 0},
	{0, 2, 4, 6, 1, 3, 5, 7, 8, 10, 12, 14, 9, 11, 13, 15}, // equal to the first
	{14, 4, 9, 13, 10, 8, 15, 6, 1, 0, 11, 5, 12, 2, 7, 3}, // equal to the second
}

func b2bHashBlocks(h *[8]uint64, c *[2]uint64, flag uint64, blocks []byte) {
	var m [16]uint64
	c0, c1 := c[0], c[1]

	for i := 0; i < len(blocks); {
		c0 += b2bBlockSize
		if c0 < b2bBlockSize {
			c1++
		}

		v0, v1, v2, v3, v4, v5, v6, v7 := h[0], h[1], h[2], h[3], h[4], h[5], h[6], h[7]
		v8, v9, v10, v11, v12, v13, v14, v15 := b2bIV[0], b2bIV[1], b2bIV[2], b2bIV[3], b2bIV[4], b2bIV[5], b2bIV[6], b2bIV[7]
		v12 ^= c0
		v13 ^= c1
		v14 ^= flag

		for j := range m {
			m[j] = binary.LittleEndian.Uint64(blocks[i:])
			i += 8
		}

		for j := range b2bPrecomputed {
			s := &(b2bPrecomputed[j])

			v0 += m[s[0]]
			v0 += v4
			v12 ^= v0
			v12 = bits.RotateLeft64(v12, -32)
			v8 += v12
			v4 ^= v8
			v4 = bits.RotateLeft64(v4, -24)
			v1 += m[s[1]]
			v1 += v5
			v13 ^= v1
			v13 = bits.RotateLeft64(v13, -32)
			v9 += v13
			v5 ^= v9
			v5 = bits.RotateLeft64(v5, -24)
			v2 += m[s[2]]
			v2 += v6
			v14 ^= v2
			v14 = bits.RotateLeft64(v14, -32)
			v10 += v14
			v6 ^= v10
			v6 = bits.RotateLeft64(v6, -24)
			v3 += m[s[3]]
			v3 += v7
			v15 ^= v3
			v15 = bits.RotateLeft64(v15, -32)
			v11 += v15
			v7 ^= v11
			v7 = bits.RotateLeft64(v7, -24)

			v0 += m[s[4]]
			v0 += v4
			v12 ^= v0
			v12 = bits.RotateLeft64(v12, -16)
			v8 += v12
			v4 ^= v8
			v4 = bits.RotateLeft64(v4, -63)
			v1 += m[s[5]]
			v1 += v5
			v13 ^= v1
			v13 = bits.RotateLeft64(v13, -16)
			v9 += v13
			v5 ^= v9
			v5 = bits.RotateLeft64(v5, -63)
			v2 += m[s[6]]
			v2 += v6
			v14 ^= v2
			v14 = bits.RotateLeft64(v14, -16)
			v10 += v14
			v6 ^= v10
			v6 = bits.RotateLeft64(v6, -63)
			v3 += m[s[7]]
			v3 += v7
			v15 ^= v3
			v15 = bits.RotateLeft64(v15, -16)
			v11 += v15
			v7 ^= v11
			v7 = bits.RotateLeft64(v7, -63)

			v0 += m[s[8]]
			v0 += v5
			v15 ^= v0
			v15 = bits.RotateLeft64(v15, -32)
			v10 += v15
			v5 ^= v10
			v5 = bits.RotateLeft64(v5, -24)
			v1 += m[s[9]]
			v1 += v6
			v12 ^= v1
			v12 = bits.RotateLeft64(v12, -32)
			v11 += v12
			v6 ^= v11
			v6 = bits.RotateLeft64(v6, -24)
			v2 += m[s[10]]
			v2 += v7
			v13 ^= v2
			v13 = bits.RotateLeft64(v13, -32)
			v8 += v13
			v7 ^= v8
			v7 = bits.RotateLeft64(v7, -24)
			v3 += m[s[11]]
			v3 += v4
			v14 ^= v3
			v14 = bits.RotateLeft64(v14, -32)
			v9 += v14
			v4 ^= v9
			v4 = bits.RotateLeft64(v4, -24)

			v0 += m[s[12]]
			v0 += v5
			v15 ^= v0
			v15 = bits.RotateLeft64(v15, -16)
			v10 += v15
			v5 ^= v10
			v5 = bits.RotateLeft64(v5, -63)
			v1 += m[s[13]]
			v1 += v6
			v12 ^= v1
			v12 = bits.RotateLeft64(v12, -16)
			v11 += v12
			v6 ^= v11
			v6 = bits.RotateLeft64(v6, -63)
			v2 += m[s[14]]
			v2 += v7
			v13 ^= v2
			v13 = bits.RotateLeft64(v13, -16)
			v8 += v13
			v7 ^= v8
			v7 = bits.RotateLeft64(v7, -63)
			v3 += m[s[15]]
			v3 += v4
			v14 ^= v3
			v14 = bits.RotateLeft64(v14, -16)
			v9 += v14
			v4 ^= v9
			v4 = bits.RotateLeft64(v4, -63)
		}

		h[0] ^= v0 ^ v8
		h[1] ^= v1 ^ v9
		h[2] ^= v2 ^ v10
		h[3] ^= v3 ^ v11
		h[4] ^= v4 ^ v12
		h[5] ^= v5 ^ v13
		h[6] ^= v6 ^ v14
		h[7] ^= v7 ^ v15
	}
	c[0], c[1] = c0, c1
}
