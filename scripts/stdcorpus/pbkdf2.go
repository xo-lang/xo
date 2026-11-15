package main

import (
	"crypto/pbkdf2"
	"encoding/hex"
	"fmt"
	"strings"
)

// std/crypto's pbkdf2 against Go's crypto/pbkdf2.Key: pbkdf2_test.go's
// RFC 6070 and SHA-256 vectors (Copyright 2012 The Go Authors, BSD-style
// license), every Hash at the iteration and block boundaries, and Go's
// errors (TestMaxKeyLength, TestZeroKeyLength).

func pbkdf2Corpus() string {
	var o strings.Builder
	o.WriteString(header("Go's crypto/pbkdf2 and its pbkdf2_test.go (Copyright 2012 The Go Authors, BSD-style license)"))
	o.WriteString("\nuse std/crypto\n\nconst HASHES: List[crypto.Hash] = [")
	for i, h := range cryptoHashes {
		if i > 0 {
			o.WriteString(", ")
		}
		o.WriteString("crypto.Hash." + h.enum)
	}
	o.WriteString("]\n")

	type row struct {
		h              int
		password, salt []byte
		iter, n        int
	}
	var rows []row
	for _, t := range []struct{ table, hash string }{{"sha1TestVectors", "sha1"}, {"sha256TestVectors", "sha256"}} {
		hi := 0
		for i, h := range cryptoHashes {
			if h.key == t.hash {
				hi = i
			}
		}
		for _, c := range goTestTable("crypto/pbkdf2", "pbkdf2_test.go", t.table) {
			pw, ok1 := strconvString(c.Elts[0])
			salt, ok2 := strconvString(c.Elts[1])
			iter, ok3 := strconvInt(c.Elts[2])
			out, ok4 := goBytes(c.Elts[3])
			if !ok1 || !ok2 || !ok3 || !ok4 {
				panic("pbkdf2: a test vector the generator cannot read")
			}
			rows = append(rows, row{hi, []byte(pw), []byte(salt), int(iter), len(out)})
		}
	}
	for hi, h := range cryptoHashes {
		hl := h.new().Size()
		for _, iter := range []int{-1, 0, 1, 2, 3, 100} {
			for _, n := range []int{1, hl - 1, hl, hl + 1, 2*hl + 3} {
				rows = append(rows, row{hi, []byte(fmt.Sprintf("pw %d", n)), []byte(fmt.Sprintf("salt %d %d", iter, hi)), iter, n})
			}
		}
	}
	o.WriteString("\n/// (index in HASHES, password, salt, iterations, key length, Go's key in hex)\nconst KEYS: List[(Int, Bytes, Bytes, Int, Int, Str)] = [\n")
	for _, r := range rows {
		k, err := pbkdf2.Key(cryptoHashes[r.h].new, string(r.password), r.salt, r.iter, r.n)
		if err != nil {
			panic(err)
		}
		fmt.Fprintf(&o, "  (%d, %s, %s, %d, %d, %s),\n", r.h, xbytes(r.password), xbytes(r.salt), r.iter, r.n, xq(hex.EncodeToString(k)))
	}
	o.WriteString("]\n")

	o.WriteString("\n/// (index in HASHES, key length, Go's error)\nconst ERRORS: List[(Int, Int, Str)] = [\n")
	for hi, h := range cryptoHashes {
		hl := h.new().Size()
		for _, n := range []int{0, -1, 1<<63 - 1, (1<<32 - 1) * (hl + 1)} {
			_, err := pbkdf2.Key(h.new, "password", []byte("salt"), 1, n)
			if err == nil {
				panic("pbkdf2: no error")
			}
			fmt.Fprintf(&o, "  (%d, %d, %s),\n", hi, n, xq(err.Error()))
		}
	}
	o.WriteString("]\n")

	o.WriteString(`
test "pbkdf2 is Go's pbkdf2.Key (RFC 6070, Go's vectors, every hash)" {
  var bad: List[Str] = []
  for (hi, pw, salt, iter, n, want) in KEYS {
    let got = crypto.pbkdf2(HASHES[hi], pw, salt, iter, n) ?? e => fault(e.msg)
    if got.to_hex() != want {
      bad.push("${HASHES[hi]:?} iter ${iter} n ${n}")
    }
  }
  expect(bad) == []
}

test "pbkdf2's errors are Go's (TestZeroKeyLength, TestMaxKeyLength)" {
  var bad: List[Str] = []
  for (hi, n, want) in ERRORS {
    match crypto.pbkdf2(HASHES[hi], b"password", b"salt", 1, n) {
      Ok(_) => bad.push("${HASHES[hi]:?} ${n}: no error")
      Err(e) => if e.msg != want {
        bad.push("${HASHES[hi]:?} ${n}: ${e.msg}")
      }
    }
  }
  expect(bad) == []
}
`)
	return o.String()
}
