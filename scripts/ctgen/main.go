// Command ctgen writes the ciphertext classes of the RSA decryption timing
// test (std/evidence/crypto/timing/rsa_decrypt/main.xo; decision 0616
// item 3, 0618): for the program's 1024-bit key, 128 ciphertexts a class,
// each the raw RSA encryption of a block built to fail exactly one
// padding check, every block checked again after decryption with Go's
// arithmetic (the 0616 RSA review's F8: an impure class tests nothing).
//
//	go run ./scripts/ctgen std/evidence/crypto/timing/rsa_decrypt/main.xo
//
// rewrites the class lists in place; the key and the rest of the file
// stay. Deterministic (seed 618).
package main

import (
	"bytes"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"math/big"
	"math/rand"
	"os"
	"regexp"
	"strings"
)

const hl = 32

func mgf1(seed []byte, n int) []byte {
	var out []byte
	for i := uint32(0); len(out) < n; i++ {
		h := sha256.New()
		h.Write(seed)
		h.Write([]byte{byte(i >> 24), byte(i >> 16), byte(i >> 8), byte(i)})
		out = h.Sum(out)
	}
	return out[:n]
}

func xor(a, b []byte) {
	for i := range a {
		a[i] ^= b[i]
	}
}

// oaep builds em = 0x00 || maskedSeed || maskedDB from db (k-1-hl bytes).
func oaep(r *rand.Rand, k int, db []byte) []byte {
	seed := make([]byte, hl)
	r.Read(seed)
	mdb := append([]byte{}, db...)
	xor(mdb, mgf1(seed, len(mdb)))
	ms := append([]byte{}, seed...)
	xor(ms, mgf1(mdb, hl))
	return append(append([]byte{0}, ms...), mdb...)
}

// oaepWhy decodes em as decryptOAEP does, naming each check's result.
func oaepWhy(em []byte) string {
	seed := append([]byte{}, em[1:1+hl]...)
	db := append([]byte{}, em[1+hl:]...)
	xor(seed, mgf1(db, hl))
	xor(db, mgf1(seed, len(db)))
	lh := sha256.Sum256(nil)
	sep := "nosep"
	for i := hl; i < len(db); i++ {
		if db[i] == 1 {
			sep = "sep"
			break
		}
		if db[i] != 0 {
			sep = "badps"
			break
		}
	}
	return fmt.Sprintf("first=%v lhash=%v %s", em[0] == 0, bytes.Equal(db[:hl], lh[:]), sep)
}

func v15Why(em []byte) string {
	z := -1
	for i := 2; i < len(em); i++ {
		if em[i] == 0 {
			z = i
			break
		}
	}
	return fmt.Sprintf("first=%v two=%v zero=%v ps8=%v", em[0] == 0, em[1] == 2, z >= 0, z >= 10)
}

func nonzero(r *rand.Rand, n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(1 + r.Intn(255))
	}
	return b
}

func main() {
	path := os.Args[1]
	src, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	m := regexp.MustCompile(`(?m)^const KEY = "([0-9a-f]+)"`).FindSubmatch(src)
	der, _ := hex.DecodeString(string(m[1]))
	pk, err := x509.ParsePKCS8PrivateKey(der)
	if err != nil {
		panic(err)
	}
	key := pk.(*rsa.PrivateKey)
	k := key.Size()
	r := rand.New(rand.NewSource(618))
	lh := sha256.Sum256(nil)
	type class struct {
		name, want string
		block      func() []byte
	}
	classes := []class{
		{"OAEP_FIRST", "first=false", func() []byte {
			em := oaep(r, k, nonzero(r, k-1-hl))
			em[0] = byte(1 + r.Intn(int(key.N.Bytes()[0])-1)) // nonzero, below the modulus's top byte
			return em
		}},
		{"OAEP_LHASH", "first=true lhash=false sep", func() []byte {
			db := make([]byte, k-1-hl)
			r.Read(db[:hl])
			db[k-1-hl-9] = 1 // the separator, then an 8 byte message
			r.Read(db[k-1-hl-8:])
			return oaep(r, k, db)
		}},
		{"OAEP_SEP", "first=true lhash=true nosep", func() []byte {
			db := make([]byte, k-1-hl)
			copy(db, lh[:])
			return oaep(r, k, db)
		}},
		{"V15_HEAD", "first=true two=false", func() []byte {
			em := append([]byte{0, byte(3 + r.Intn(253))}, nonzero(r, k-2)...)
			em[k-9] = 0
			return em
		}},
		{"V15_NOZERO", "first=true two=true zero=false", func() []byte {
			return append([]byte{0, 2}, nonzero(r, k-2)...)
		}},
		{"V15_SHORT", "first=true two=true zero=true ps8=false", func() []byte {
			em := append([]byte{0, 2}, nonzero(r, k-2)...)
			em[2+r.Intn(8)] = 0
			return em
		}},
	}
	out := string(src)
	for _, c := range classes {
		var b strings.Builder
		fmt.Fprintf(&b, "const %s: List[Str] = [\n", c.name)
		for i := 0; i < 128; i++ {
			em := c.block()
			ct := new(big.Int).Exp(new(big.Int).SetBytes(em), big.NewInt(int64(key.E)), key.N).FillBytes(make([]byte, k))
			back := new(big.Int).Exp(new(big.Int).SetBytes(ct), key.D, key.N).FillBytes(make([]byte, k))
			why := v15Why(back)
			if strings.HasPrefix(c.name, "OAEP") {
				why = oaepWhy(back)
			}
			if !strings.HasPrefix(why, c.want) {
				panic(fmt.Sprintf("%s %d: %s, want %s", c.name, i, why, c.want))
			}
			fmt.Fprintf(&b, "  %q,\n", hex.EncodeToString(ct))
		}
		b.WriteString("]\n")
		re := regexp.MustCompile(`(?ms)^const ` + c.name + `: List\[Str\] = \[\n.*?^\]\n`)
		if !re.MatchString(out) {
			panic("no list " + c.name)
		}
		out = re.ReplaceAllLiteralString(out, b.String())
	}
	if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
		panic(err)
	}
}
