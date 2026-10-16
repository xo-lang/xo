package llvm

import (
	"bufio"
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
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

// rsaDiffProgram reads lines `op a b c d` (hex arguments, "-" for empty)
// and prints internal.crypto_op("rsa.<op>", ...) in hex (std/native/llrt
// rsa.xo op).
const rsaDiffProgram = `use std/internal

fn h(s: Str) -> Bytes {
  if s == "-" { b"" } else { Bytes.from_hex(s) ?? b"" }
}

fn main(os: Os) {
  while let Some(l) = os.stdio.read_line().ok() ?? None {
    let f = l.split(" ")
    os.stdio.println(internal.crypto_op("rsa.${f[0]}", h(f[1]), h(f[2]), h(f[3]), h(f[4])).to_hex())
  }
}
`

// TestRSADiff runs rsa.xo's signatures and verifications (bigmod.xo's
// Montgomery arithmetic under them) against crypto/rsa on keys of 1024,
// 2048, 2056 (a limb count that is not a multiple of 4), 3072, and 4096
// bits, with SHA-256, SHA-384, and SHA-512: PKCS #1 v1.5 signatures equal
// to Go's (deterministic), Go's PKCS #1 v1.5 and PSS signatures verified
// (and a changed hash refused), and PSS signatures Go verifies; on each
// native backend.
func TestRSADiff(t *testing.T) {
	if testing.Short() {
		t.Skip("generates RSA keys and builds a program on each native backend")
	}
	if _, err := xotool.Clang(); err != nil {
		t.Skip("clang not found")
	}
	type tcase struct {
		line  string
		check func(out string) error
	}
	hashes := []struct {
		id  byte
		h   crypto.Hash
		sum func([]byte) []byte
	}{
		{0, crypto.SHA256, func(b []byte) []byte { s := sha256.Sum256(b); return s[:] }},
		{1, crypto.SHA384, func(b []byte) []byte { s := sha512.Sum384(b); return s[:] }},
		{2, crypto.SHA512, func(b []byte) []byte { s := sha512.Sum512(b); return s[:] }},
	}
	var cases []tcase
	for _, bits := range []int{1024, 2048, 2056, 3072, 4096} {
		k, err := rsa.GenerateKey(rand.Reader, bits)
		if err != nil {
			t.Fatal(err)
		}
		priv := x509.MarshalPKCS1PrivateKey(k)
		pub := x509.MarshalPKCS1PublicKey(&k.PublicKey)
		for _, hs := range hashes {
			if 2*hs.h.Size()+2 > bits/8 {
				continue // PSS with a salt of the hash's length does not fit
			}
			for i := 0; i < 3; i++ {
				msg := make([]byte, 40)
				rand.Read(msg)
				d := hs.sum(msg)
				other := hs.sum(append(msg, 1))
				name := fmt.Sprintf("%d bits %v case %d", bits, hs.h, i)
				want, err := rsa.SignPKCS1v15(nil, k, hs.h, d)
				if err != nil {
					t.Fatal(err)
				}
				pss, err := rsa.SignPSS(rand.Reader, k, hs.h, d, &rsa.PSSOptions{SaltLength: rsa.PSSSaltLengthEqualsHash})
				if err != nil {
					t.Fatal(err)
				}
				wantHex := hex.EncodeToString(want)
				cases = append(cases,
					tcase{fmt.Sprintf("sign_pkcs1 %02x %x %x -", hs.id, priv, d), func(out string) error {
						if out != wantHex {
							return fmt.Errorf("%s: PKCS #1 v1.5 signature differs from Go's", name)
						}
						return nil
					}},
					tcase{fmt.Sprintf("verify_pkcs1 %02x %x %x %x", hs.id, pub, d, want), func(out string) error {
						if out != "01" {
							return fmt.Errorf("%s: Go's PKCS #1 v1.5 signature: %s", name, out)
						}
						return nil
					}},
					tcase{fmt.Sprintf("verify_pkcs1 %02x %x %x %x", hs.id, pub, other, want), func(out string) error {
						if out != "00" {
							return fmt.Errorf("%s: PKCS #1 v1.5, a changed hash: %s", name, out)
						}
						return nil
					}},
					tcase{fmt.Sprintf("verify_pss %02x %x %x %x", hs.id, pub, d, pss), func(out string) error {
						if out != "01" {
							return fmt.Errorf("%s: Go's PSS signature: %s", name, out)
						}
						return nil
					}},
					tcase{fmt.Sprintf("verify_pss %02x %x %x %x", hs.id, pub, other, pss), func(out string) error {
						if out != "00" {
							return fmt.Errorf("%s: PSS, a changed hash: %s", name, out)
						}
						return nil
					}},
					tcase{fmt.Sprintf("sign_pss %02x %x %x -", hs.id, priv, d), func(out string) error {
						s, err := hex.DecodeString(out)
						if err != nil || rsa.VerifyPSS(&k.PublicKey, hs.h, d, s, &rsa.PSSOptions{SaltLength: rsa.PSSSaltLengthEqualsHash}) != nil {
							return fmt.Errorf("%s: Go does not verify the PSS signature %s", name, out)
						}
						return nil
					}})
			}
		}
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(rsaDiffProgram), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "xo.mod"), []byte("module github.com/xo-lang/xo/test/rsadiff\nedition preview\n"), 0o644); err != nil {
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
		sc := bufio.NewScanner(bytes.NewReader(out))
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
