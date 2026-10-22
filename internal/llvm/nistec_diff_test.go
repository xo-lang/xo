package llvm

import (
	"bufio"
	"bytes"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// nistecDiffProgram reads lines `curve op a b c` (hex arguments, "-" for
// empty) and prints internal.crypto_op's result in hex (std/native/llrt
// nistec.xo op).
const nistecDiffProgram = `use std/internal

fn h(s: Str) -> Bytes {
  if s == "-" { b"" } else { Bytes.from_hex(s) ?? b"" }
}

fn main(os: Os) {
  while let Some(l) = os.stdio.read_line().ok() ?? None {
    let f = l.split(" ")
    os.stdio.println(internal.crypto_op("${f[0]}.${f[1]}", h(f[2]), h(f[3]), h(f[4]), b"").to_hex())
  }
}
`

// TestNistecDiff runs nistec.xo's public keys, verification of Go's
// signatures (and of a changed hash), and signatures Go verifies, on
// every NIST curve the runtime has (P-224, P-256, P-384, P-521) against
// crypto/ecdsa, on each native backend: a change to one curve's field
// code that reaches another curve of the same limb count (P-224 and
// P-256 both have 4) shows here, not only in the tls_nistec golden.
// P-256's mul4p and sqr4p also run directly on field elements at the
// field's edges against math/big (nistec.xo's fieldmul op).
func TestNistecDiff(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a program on each native backend")
	}
	if _, err := xotool.Clang(); err != nil {
		t.Skip("clang not found")
	}
	type tcase struct {
		line  string
		check func(out string) error
	}
	curves := []struct {
		name  string
		curve elliptic.Curve
	}{{"p224", elliptic.P224()}, {"p256", elliptic.P256()}, {"p384", elliptic.P384()}, {"p521", elliptic.P521()}}
	var cases []tcase
	for _, c := range curves {
		size := (c.curve.Params().BitSize + 7) / 8
		for i := 0; i < 40; i++ {
			k, err := ecdsa.GenerateKey(c.curve, rand.Reader)
			if err != nil {
				t.Fatal(err)
			}
			priv := k.D.FillBytes(make([]byte, size))
			//lint:ignore SA1019 the uncompressed encoding nistec.xo uses
			pub := elliptic.Marshal(c.curve, k.X, k.Y)
			msg := make([]byte, 32)
			rand.Read(msg)
			hash := sha256.Sum256(msg)
			sig, err := ecdsa.SignASN1(rand.Reader, k, hash[:])
			if err != nil {
				t.Fatal(err)
			}
			other := sha256.Sum256(append(msg, 1))
			name := fmt.Sprintf("%s key %d", c.name, i)
			wantPub := hex.EncodeToString(pub)
			cases = append(cases,
				tcase{fmt.Sprintf("%s public %x - -", c.name, priv), func(out string) error {
					if out != wantPub {
						return fmt.Errorf("%s: public key %s, want %s", name, out, wantPub)
					}
					return nil
				}},
				tcase{fmt.Sprintf("%s verify %x %x %x", c.name, pub, hash[:], sig), func(out string) error {
					if out != "01" {
						return fmt.Errorf("%s: Go's signature: %s, want 01", name, out)
					}
					return nil
				}},
				tcase{fmt.Sprintf("%s verify %x %x %x", c.name, pub, other[:], sig), func(out string) error {
					if out != "00" {
						return fmt.Errorf("%s: a changed hash: %s, want 00", name, out)
					}
					return nil
				}},
				tcase{fmt.Sprintf("%s sign %x %x -", c.name, priv, hash[:]), func(out string) error {
					s, err := hex.DecodeString(out)
					if err != nil || !ecdsa.VerifyASN1(&k.PublicKey, hash[:], s) {
						return fmt.Errorf("%s: Go does not verify the signature %s", name, out)
					}
					return nil
				}})
		}
	}
	// P-256's field products (mul4p, sqr4p) at the field's edges: they
	// take operands below p only (their accumulators' widths rest on it),
	// so p and above give an empty result
	p := elliptic.P256().Params().P
	rinv := new(big.Int).ModInverse(new(big.Int).Lsh(big.NewInt(1), 256), p)
	pow := func(k uint) *big.Int { return new(big.Int).Lsh(big.NewInt(1), k) }
	sub := func(a *big.Int, k int64) *big.Int { return new(big.Int).Sub(a, big.NewInt(k)) }
	edges := []*big.Int{big.NewInt(0), big.NewInt(1), big.NewInt(2), sub(p, 1), sub(p, 2), sub(p, 3), new(big.Int).Rsh(p, 1),
		pow(64), sub(pow(64), 1), pow(192), sub(pow(192), 1), pow(224), pow(255), sub(pow(255), 1), new(big.Int).Sub(pow(256), pow(224))}
	for i := 0; i < 40; i++ {
		r, err := rand.Int(rand.Reader, p)
		if err != nil {
			t.Fatal(err)
		}
		edges = append(edges, r)
	}
	for i, f := range edges {
		for _, g := range []*big.Int{f, edges[(i*7+3)%len(edges)], sub(p, 1), edges[len(edges)-1-i]} {
			m := new(big.Int).Mul(f, g)
			m.Mul(m, rinv).Mod(m, p)
			s := new(big.Int).Mul(f, f)
			s.Mul(s, rinv).Mod(s, p)
			want := hex.EncodeToString(append(m.FillBytes(make([]byte, 32)), s.FillBytes(make([]byte, 32))...))
			name := fmt.Sprintf("p256 fieldmul %x %x", f, g)
			cases = append(cases, tcase{fmt.Sprintf("p256 fieldmul %x %x -", f.FillBytes(make([]byte, 32)), g.FillBytes(make([]byte, 32))), func(out string) error {
				if out != want {
					return fmt.Errorf("%s: %s, want %s", name, out, want)
				}
				return nil
			}})
		}
	}
	for _, f := range []*big.Int{p, new(big.Int).Add(p, big.NewInt(1)), sub(pow(256), 1)} {
		cases = append(cases, tcase{fmt.Sprintf("p256 fieldmul %x %x -", f.FillBytes(make([]byte, 32)), big.NewInt(5).FillBytes(make([]byte, 32))), func(out string) error {
			if out != "" {
				return fmt.Errorf("p256 fieldmul of an operand at or above p: %s, want empty", out)
			}
			return nil
		}})
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(nistecDiffProgram), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "xo.mod"), []byte("module github.com/xo-lang/xo/test/nistecdiff\nedition preview\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var in strings.Builder
	for _, c := range cases {
		in.WriteString(c.line + "\n")
	}
	backends := []string{xotool.BackendLLVM}
	if runtime.GOARCH == "arm64" {
		backends = append(backends, xotool.BackendArm64)
	}
	for _, be := range backends {
		res, err := xotool.Build([]string{dir}, xotool.Options{Backend: be, NoGC: true})
		if err != nil || res.Failed {
			t.Fatalf("build (%s): %v %v", be, err, res.Output)
		}
		cmd := testtmp.Guard(exec.Command(res.Binaries[xotool.Host()]))
		cmd.Stdin = strings.NewReader(in.String())
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("%s: %v", be, err)
		}
		sc := bufio.NewScanner(strings.NewReader(string(out)))
		sc.Buffer(make([]byte, 1<<20), 1<<20)
		i := 0
		for sc.Scan() {
			if i < len(cases) {
				if err := cases[i].check(sc.Text()); err != nil {
					t.Errorf("%s: %v", be, err)
				}
			}
			i++
		}
		if i != len(cases) {
			t.Errorf("%s: %d results for %d cases", be, i, len(cases))
		}
	}
}

// TestX25519Diff runs x25519.xo's scalar multiplication against RFC
// 7748's arithmetic on math/big (x25519Ref, which takes the inputs
// crypto/ecdh refuses too): random scalars and
// points, the base point, and u coordinates at the field's edges (0, 1,
// p - 1, p, p + 1, 2^255 - 1, the top bit set, low order points, which
// give all zeros), on each native backend.
func TestX25519Diff(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a program on each native backend")
	}
	if _, err := xotool.Clang(); err != nil {
		t.Skip("clang not found")
	}
	p := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 255), big.NewInt(19))
	le := func(x *big.Int) []byte {
		b := x.FillBytes(make([]byte, 32))
		for i, j := 0, 31; i < j; i, j = i+1, j-1 {
			b[i], b[j] = b[j], b[i]
		}
		return b
	}
	var points [][]byte
	for _, x := range []*big.Int{big.NewInt(0), big.NewInt(1), new(big.Int).Sub(p, big.NewInt(1)), p, new(big.Int).Add(p, big.NewInt(1)), new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 255), big.NewInt(1))} {
		points = append(points, le(x))
	}
	// the top bit set on a random point (ignored), and low order points
	// (RFC 7748's and libsodium's blocklist entries)
	top := make([]byte, 32)
	rand.Read(top)
	top[31] |= 0x80
	points = append(points, top)
	for _, h := range []string{
		"e0eb7a7c3b41b8ae1656e3faf19fc46ada098deb9c32b1fd866205165f49b800",
		"5f9c95bca3508c24b1d0b1559c83ef5b04445cc4581c8e86d8224eddd09f1157",
	} {
		b, _ := hex.DecodeString(h)
		points = append(points, b)
	}
	for i := 0; i < 60; i++ {
		b := make([]byte, 32)
		rand.Read(b)
		points = append(points, b)
	}
	var in strings.Builder
	var want []string
	for _, pt := range points {
		k := make([]byte, 32)
		rand.Read(k)
		in.WriteString(fmt.Sprintf("x25519 mult %x %x -\n", k, pt))
		ref := x25519Ref(k, pt)
		// the reference agrees with crypto/ecdh where ecdh takes the input
		if priv, err := ecdh.X25519().NewPrivateKey(k); err == nil {
			if pub, err := ecdh.X25519().NewPublicKey(pt); err == nil {
				if sh, err := priv.ECDH(pub); err == nil && !bytes.Equal(sh, ref) {
					t.Fatalf("x25519Ref(%x, %x) = %x, crypto/ecdh %x", k, pt, ref, sh)
				}
			}
		}
		want = append(want, hex.EncodeToString(ref))
	}
	// base takes the twisted Edwards curve's fixed base table (signed
	// radix 16 digits): scalars whose clamped digits carry through every
	// position, all zeros and all ones, and many random ones
	var bases [][]byte
	for _, f := range []byte{0x00, 0xff, 0x88, 0x77, 0x08, 0xf8} {
		bases = append(bases, bytes.Repeat([]byte{f}, 32))
	}
	for i := 0; i < 300; i++ {
		k := make([]byte, 32)
		rand.Read(k)
		bases = append(bases, k)
	}
	for _, k := range bases {
		in.WriteString(fmt.Sprintf("x25519 base %x - -\n", k))
		nine := make([]byte, 32)
		nine[0] = 9
		want = append(want, hex.EncodeToString(x25519Ref(k, nine)))
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(nistecDiffProgram), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "xo.mod"), []byte("module github.com/xo-lang/xo/test/x25519diff\nedition preview\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	backends := []string{xotool.BackendLLVM}
	if runtime.GOARCH == "arm64" {
		backends = append(backends, xotool.BackendArm64)
	}
	for _, be := range backends {
		res, err := xotool.Build([]string{dir}, xotool.Options{Backend: be, NoGC: true})
		if err != nil || res.Failed {
			t.Fatalf("build (%s): %v %v", be, err, res.Output)
		}
		cmd := testtmp.Guard(exec.Command(res.Binaries[xotool.Host()]))
		cmd.Stdin = strings.NewReader(in.String())
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("%s: %v", be, err)
		}
		got := strings.Split(strings.TrimSpace(string(out)), "\n")
		if len(got) != len(want) {
			t.Fatalf("%s: %d results for %d cases", be, len(got), len(want))
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("%s: case %d: %s, want %s", be, i, got[i], want[i])
			}
		}
	}
}

// x25519Ref is RFC 7748's X25519 (the u coordinate of [k]u, k clamped,
// u's top bit ignored and u taken modulo p) on math/big, the reference
// for inputs crypto/ecdh refuses (low order points give all zeros).
func x25519Ref(k, u []byte) []byte {
	p := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 255), big.NewInt(19))
	rev := func(b []byte) []byte {
		r := make([]byte, len(b))
		for i := range b {
			r[len(b)-1-i] = b[i]
		}
		return r
	}
	kc := append([]byte(nil), k...)
	kc[0] &= 248
	kc[31] &= 127
	kc[31] |= 64
	uc := append([]byte(nil), u...)
	uc[31] &= 127
	x1 := new(big.Int).Mod(new(big.Int).SetBytes(rev(uc)), p)
	kn := new(big.Int).SetBytes(rev(kc))
	x2, z2, x3, z3 := big.NewInt(1), big.NewInt(0), new(big.Int).Set(x1), big.NewInt(1)
	swap := uint(0)
	mod := func(x *big.Int) *big.Int { return x.Mod(x, p) }
	for t := 254; t >= 0; t-- {
		bit := kn.Bit(t)
		if swap^bit == 1 {
			x2, x3 = x3, x2
			z2, z3 = z3, z2
		}
		swap = bit
		a := mod(new(big.Int).Add(x2, z2))
		aa := mod(new(big.Int).Mul(a, a))
		b := mod(new(big.Int).Sub(x2, z2))
		bb := mod(new(big.Int).Mul(b, b))
		e := mod(new(big.Int).Sub(aa, bb))
		c := mod(new(big.Int).Add(x3, z3))
		d := mod(new(big.Int).Sub(x3, z3))
		da := mod(new(big.Int).Mul(d, a))
		cb := mod(new(big.Int).Mul(c, b))
		s := mod(new(big.Int).Add(da, cb))
		x3 = mod(new(big.Int).Mul(s, s))
		df := mod(new(big.Int).Sub(da, cb))
		z3 = mod(new(big.Int).Mul(x1, mod(new(big.Int).Mul(df, df))))
		x2 = mod(new(big.Int).Mul(aa, bb))
		z2 = mod(new(big.Int).Mul(e, mod(new(big.Int).Add(aa, mod(new(big.Int).Mul(big.NewInt(121665), e))))))
	}
	if swap == 1 {
		x2, z2 = x3, z3
	}
	r := mod(new(big.Int).Mul(x2, new(big.Int).Exp(z2, new(big.Int).Sub(p, big.NewInt(2)), p)))
	return rev(r.FillBytes(make([]byte, 32)))
}
