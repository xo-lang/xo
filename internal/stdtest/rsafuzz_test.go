package stdtest

// TestRsaFuzz (decision 0618; decision 0616's fuzz evidence for std/crypto)
// checks std/crypto's RSA decryption against Go's crypto/rsa on inputs
// built to reach every branch of the padding checks: for a fresh 2048-bit
// key, Go's own ciphertexts (OAEP with SHA-1, SHA-256 and SHA-512 and
// labels, PKCS #1 v1.5) changed at random, and encoded messages made by
// hand and raised to e (an OAEP block with a nonzero first byte, a wrong
// label hash, no 0x01 separator or a nonzero byte before it; a PKCS #1
// v1.5 block with a wrong head, no zero, or fewer than eight padding
// bytes; and valid ones of every length), plus ciphertexts at and above
// the modulus. Every case must decrypt to Go's outcome, the message or
// the error text, on each backend. XO_RSAFUZZ_SEED (default 2),
// XO_RSAFUZZ_N cases (default 600) and XO_RSAFUZZ_BACKENDS (default arm64
// and llvm) as the json fuzz's knobs.

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"hash"
	"math/big"
	mrand "math/rand"
	"strings"
	"testing"
)

type rsfCase struct {
	scheme, hash, label string
	c                   []byte
	want                string
}

var rsfHashes = []struct {
	name string
	new  func() hash.Hash
}{{"Sha1", sha1.New}, {"Sha256", sha256.New}, {"Sha512", sha512.New}}

// rsfRaw is em^e mod n as k bytes: a ciphertext whose decryption is em.
func rsfRaw(k *rsa.PrivateKey, em []byte) []byte {
	m := new(big.Int).SetBytes(em)
	c := new(big.Int).Exp(m, big.NewInt(int64(k.E)), k.N)
	return c.FillBytes(make([]byte, k.Size()))
}

// rsfMGF1 is MGF1 with h (RFC 8017 B.2.1).
func rsfMGF1(h func() hash.Hash, seed []byte, n int) []byte {
	var out []byte
	for i := uint32(0); len(out) < n; i++ {
		d := h()
		d.Write(seed)
		d.Write([]byte{byte(i >> 24), byte(i >> 16), byte(i >> 8), byte(i)})
		out = d.Sum(out)
	}
	return out[:n]
}

func rsfXor(a, b []byte) []byte {
	o := make([]byte, len(a))
	for i := range a {
		o[i] = a[i] ^ b[i]
	}
	return o
}

// rsfOAEPBlock is an OAEP encoded block of k bytes from its parts: the
// first byte, the label hash, the bytes between it and the message (PS
// and the separator, as given), and the message.
func rsfOAEPBlock(r *mrand.Rand, h func() hash.Hash, k int, first byte, lhash, mid, msg []byte) []byte {
	hl := h().Size()
	db := append(append(append([]byte{}, lhash...), mid...), msg...)
	if len(db) != k-hl-1 {
		return nil
	}
	seed := make([]byte, hl)
	r.Read(seed)
	mdb := rsfXor(db, rsfMGF1(h, seed, len(db)))
	ms := rsfXor(seed, rsfMGF1(h, mdb, hl))
	return append(append([]byte{first}, ms...), mdb...)
}

func rsfCases(r *mrand.Rand, k *rsa.PrivateKey, n int) []rsfCase {
	var cs []rsfCase
	ks := k.Size()
	add := func(scheme, hn, label string, c []byte) {
		var m []byte
		var err error
		if scheme == "oaep" {
			var h func() hash.Hash
			for _, x := range rsfHashes {
				if x.name == hn {
					h = x.new
				}
			}
			m, err = rsa.DecryptOAEP(h(), nil, k, c, []byte(label))
		} else {
			m, err = rsa.DecryptPKCS1v15(nil, k, c)
		}
		want := "ok:" + hex.EncodeToString(m)
		if err != nil {
			want = "err:" + err.Error()
		}
		cs = append(cs, rsfCase{scheme, hn, label, c, want})
	}
	labels := []string{"", "l", "a longer label"}
	for len(cs) < n {
		hx := rsfHashes[r.Intn(len(rsfHashes))]
		hl := hx.new().Size()
		label := labels[r.Intn(len(labels))]
		lh := hx.new()
		lh.Write([]byte(label))
		lhash := lh.Sum(nil)
		maxOAEP := ks - 2*hl - 2
		switch r.Intn(10) {
		case 0, 1: // Go's OAEP ciphertext, maybe changed
			msg := make([]byte, r.Intn(maxOAEP+1))
			r.Read(msg)
			c, err := rsa.EncryptOAEP(hx.new(), rand.Reader, &k.PublicKey, msg, []byte(label))
			if err != nil {
				panic(err)
			}
			add("oaep", hx.name, label, rsfMutate(r, c))
		case 2: // Go's PKCS #1 v1.5 ciphertext, maybe changed
			msg := make([]byte, r.Intn(ks-11+1))
			r.Read(msg)
			c, err := rsa.EncryptPKCS1v15(rand.Reader, &k.PublicKey, msg)
			if err != nil {
				panic(err)
			}
			add("pkcs1v15", "Sha256", "", rsfMutate(r, c))
		case 3, 4, 5: // a hand made OAEP block
			first := byte(0)
			if r.Intn(6) == 0 {
				first = byte(1 + r.Intn(255))
			}
			lhb := append([]byte{}, lhash...)
			if r.Intn(6) == 0 {
				lhb[r.Intn(hl)] ^= byte(1 + r.Intn(255))
			}
			mlen := r.Intn(maxOAEP + 1)
			mid := make([]byte, ks-hl-1-hl-mlen)
			if len(mid) > 0 {
				mid[len(mid)-1] = 1
				switch r.Intn(6) {
				case 0: // no separator
					mid[len(mid)-1] = 0
				case 1: // a nonzero byte in PS
					mid[r.Intn(len(mid))] = byte(2 + r.Intn(254))
				case 2: // the separator earlier, with the message after
					mid[r.Intn(len(mid))] = 1
				}
			}
			msg := make([]byte, mlen)
			r.Read(msg)
			if r.Intn(4) == 0 && mlen > 0 {
				msg[0] = byte(r.Intn(2)) // a message starting 0x00 or 0x01
			}
			em := rsfOAEPBlock(r, hx.new, ks, first, lhb, mid, msg)
			if em == nil {
				continue
			}
			add("oaep", hx.name, label, rsfRaw(k, em))
		case 6, 7, 8: // a hand made PKCS #1 v1.5 block
			em := make([]byte, ks)
			em[1] = 2
			switch r.Intn(8) {
			case 0:
				em[0] = byte(1 + r.Intn(255))
			case 1:
				em[1] = byte(r.Intn(256))
			}
			// padding of nonzero bytes, then a zero at position z (0 to
			// 12 bytes of padding is the interesting edge), or none
			z := 2 + r.Intn(14)
			if r.Intn(3) == 0 {
				z = 2 + r.Intn(ks-2)
			}
			for i := 2; i < ks; i++ {
				em[i] = byte(1 + r.Intn(255))
			}
			if r.Intn(10) != 0 {
				em[z] = 0
				for i := z + 1; i < ks; i++ {
					em[i] = byte(r.Intn(256))
				}
			}
			add("pkcs1v15", "Sha256", "", rsfRaw(k, em))
		default: // bytes at random, at and around the modulus and k
			var c []byte
			switch r.Intn(4) {
			case 0:
				c = k.N.FillBytes(make([]byte, ks))
				if r.Intn(2) == 0 {
					c[ks-1]++
				}
			case 1:
				c = make([]byte, r.Intn(ks+3))
				r.Read(c)
			case 2:
				c = bytes.Repeat([]byte{byte(r.Intn(256))}, ks)
			default:
				c = new(big.Int).Sub(k.N, big.NewInt(int64(1+r.Intn(3)))).FillBytes(make([]byte, ks))
			}
			if r.Intn(2) == 0 {
				add("oaep", hx.name, label, c)
			} else {
				add("pkcs1v15", "Sha256", "", c)
			}
		}
	}
	return cs
}

// rsfMutate is c unchanged (a third of the time) or changed: a flipped
// byte, a byte dropped or added, or a prefix of zeros.
func rsfMutate(r *mrand.Rand, c []byte) []byte {
	d := bytes.Clone(c)
	switch r.Intn(6) {
	case 0, 1:
		return d
	case 2:
		d[r.Intn(len(d))] ^= byte(1 + r.Intn(255))
	case 3:
		i := r.Intn(len(d))
		d = append(d[:i], d[i+1:]...)
	case 4:
		i := r.Intn(len(d) + 1)
		d = append(d[:i], append([]byte{byte(r.Intn(256))}, d[i:]...)...)
	default:
		d = append(make([]byte, 1+r.Intn(2)), d...)
	}
	return d
}

const rsfRenderXo = `use std/crypto

fn rsf_hash(n: Str) -> crypto.Hash {
  match n {
    "Sha1" => crypto.Hash.Sha1
    "Sha512" => crypto.Hash.Sha512
    _ => crypto.Hash.Sha256
  }
}

fn rsf_check(scheme: Str, h: Str, label: Str, c: Str, want: Str) {
  let k = crypto.parse_private_key(Bytes.from_hex(KEY) ?? b"").or_fault("key")
  let ct = Bytes.from_hex(c) ?? b""
  let got = if scheme == "oaep" { crypto.decrypt_oaep(k, rsf_hash(h), ct, label.bytes()) } else { crypto.decrypt_pkcs1v15(k, ct) }
  let g = match got {
    Ok(m) => "ok:${m.to_hex()}"
    Err(e) => "err:${e.msg}"
  }
  if g != want {
    fault("${scheme} ${h} ${label}: got ${g} want ${want}")
  }
}

`

func rsfXoFile(seed int64, k *rsa.PrivateKey, cases []rsfCase) string {
	der, err := x509.MarshalPKCS8PrivateKey(k)
	if err != nil {
		panic(err)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "// Generated by internal/stdtest/rsafuzz_test.go (seed %d, %d cases) from Go's crypto/rsa.\n", seed, len(cases))
	b.WriteString(rsfRenderXo)
	fmt.Fprintf(&b, "const KEY = %q\n\n", hex.EncodeToString(der))
	for i, c := range cases {
		fmt.Fprintf(&b, "test \"rsa fuzz %d\" {\n  rsf_check(%q, %q, %q, %q, %q)\n}\n\n", i, c.scheme, c.hash, c.label, hex.EncodeToString(c.c), c.want)
	}
	return b.String()
}

func TestRsaFuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("the rsa fuzz is a heavy test; golden crypto_rsa_encrypt holds a fixed set")
	}
	seed := int64(envInt(t, "XO_RSAFUZZ_SEED", 2))
	n := envInt(t, "XO_RSAFUZZ_N", 600)
	// a fresh key each run (Go's GenerateKey takes no seed since Go 1.26)
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	cases := rsfCases(mrand.New(mrand.NewSource(seed)), k, n)
	ok := 0
	for _, c := range cases {
		if strings.HasPrefix(c.want, "ok:") {
			ok++
		}
	}
	t.Logf("%d cases, %d decrypt in Go", len(cases), ok)
	runFuzzDir(t, "rsafuzz_test.xo", rsfXoFile(seed, k, cases), envBackends("XO_RSAFUZZ_BACKENDS", []string{"arm64", "llvm"}), len(cases))
}
