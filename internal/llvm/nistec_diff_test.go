package llvm

import (
	"bufio"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

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
		cmd := exec.Command(res.Binaries[xotool.Host()])
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
