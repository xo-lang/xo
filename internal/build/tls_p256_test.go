package build

import (
	"bufio"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/asn1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// buildAccel links testdata/tls_accel.c with the TLS library of the user
// cache (built if missing) compiled with flags (sanitizers make an
// instrumented library).
func buildAccel(t *testing.T, flags ...string) string {
	t.Helper()
	clang, err := Clang()
	if err != nil {
		t.Skip("clang not found")
	}
	root := sysrootFor(t, clang)
	opt := append([]string{"-O1"}, flags...)
	compile, link, err := tlsArgs(clang, opt, root)
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(t.TempDir(), "accel")
	args := append(append(append(append([]string{}, opt...), "-o", bin), compile...), filepath.Join("testdata", "tls_accel.c"))
	args = append(args, link...)
	if root != "" {
		args = append(args, "-isysroot", root)
	}
	if out, err := exec.Command(clang, args...).CombinedOutput(); err != nil {
		t.Fatalf("link: %v\n%s", err, out)
	}
	return bin
}

// accelServer drives "tls_accel serve": one request line, one answer.
type accelServer struct {
	cmd *exec.Cmd
	in  io.WriteCloser
	out *bufio.Scanner
}

func startAccel(t *testing.T, bin string) *accelServer {
	t.Helper()
	cmd := exec.Command(bin, "serve")
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	outp, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	s := &accelServer{cmd: cmd, in: in, out: bufio.NewScanner(outp)}
	s.out.Buffer(make([]byte, 1<<16), 1<<16)
	t.Cleanup(func() {
		in.Close()
		cmd.Wait()
	})
	return s
}

func (s *accelServer) ask(t *testing.T, format string, args ...any) []string {
	t.Helper()
	fmt.Fprintf(s.in, format+"\n", args...)
	if !s.out.Scan() {
		t.Fatalf("tls_accel serve: no answer (%v)", s.out.Err())
	}
	return strings.Fields(s.out.Text())
}

func hexOrDash(b []byte) string {
	if len(b) == 0 {
		return "-"
	}
	return hex.EncodeToString(b)
}

// TestP256 checks xo_p256.c (decision 0132) against Go's crypto/ecdsa:
// deterministic signatures (RFC 6979, which Go's Sign makes when its
// random source is nil) byte for byte with the library's and Go's, over
// random keys and messages with SHA-256, SHA-384, and SHA-512, and
// verification of Go's randomized signatures. With XO_WYCHEPROOF_DIR set
// to a directory of Wycheproof's test vectors it also runs
// ecdsa_secp256r1_sha{256,512}_p1363_test.json through both verifiers.
func TestP256(t *testing.T) {
	if testing.Short() {
		t.Skip("may compile the TLS library (a few seconds)")
	}
	s := startAccel(t, buildAccel(t))
	hashes := []struct {
		bits int
		h    crypto.Hash
	}{{256, crypto.SHA256}, {384, crypto.SHA384}, {512, crypto.SHA512}}
	for i := 0; i < 300; i++ {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		d, err := key.Bytes()
		if err != nil {
			t.Fatal(err)
		}
		pub, err := key.PublicKey.Bytes()
		if err != nil {
			t.Fatal(err)
		}
		msg := make([]byte, i%97)
		rand.Read(msg)
		hc := hashes[i%3]
		hh := hc.h.New()
		hh.Write(msg)
		digest := hh.Sum(nil)
		der, err := key.Sign(nil, digest, hc.h)
		if err != nil {
			t.Fatal(err)
		}
		var rs struct{ R, S *big.Int }
		if _, err := asn1.Unmarshal(der, &rs); err != nil {
			t.Fatal(err)
		}
		want := make([]byte, 64)
		rs.R.FillBytes(want[:32])
		rs.S.FillBytes(want[32:])
		got := s.ask(t, "S %d %x %s", hc.bits, d, hexOrDash(digest))
		if len(got) != 2 || got[0] != hex.EncodeToString(want) || got[1] != got[0] {
			t.Fatalf("round %d (SHA-%d): fast, library %v, Go %x", i, hc.bits, got, want)
		}
		// Go's randomized signature verifies; with another digest it does not
		der, err = ecdsa.SignASN1(rand.Reader, key, digest)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := asn1.Unmarshal(der, &rs); err != nil {
			t.Fatal(err)
		}
		rs.R.FillBytes(want[:32])
		rs.S.FillBytes(want[32:])
		if v := s.ask(t, "V %x %x %x", pub, digest, want); len(v) != 2 || v[0] != "0" || v[1] != "0" {
			t.Fatalf("round %d: Go's signature: fast, library %v, want 0 0", i, v)
		}
		digest[0] ^= 1
		if v := s.ask(t, "V %x %x %x", pub, digest, want); len(v) != 2 || v[0] == "0" || v[0] != v[1] {
			t.Fatalf("round %d: Go's signature of another digest: fast, library %v", i, v)
		}
	}
	dir := os.Getenv("XO_WYCHEPROOF_DIR")
	if dir == "" {
		t.Log("XO_WYCHEPROOF_DIR not set: Wycheproof vectors skipped")
		return
	}
	for _, f := range []string{"ecdsa_secp256r1_sha256_p1363_test.json", "ecdsa_secp256r1_sha512_p1363_test.json"} {
		wycheproof(t, s, filepath.Join(dir, f))
	}
}

func wycheproof(t *testing.T, s *accelServer, file string) {
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		TestGroups []struct {
			PublicKey struct{ Uncompressed string }
			Sha       string
			Tests     []struct {
				TcID    int
				Comment string
				Msg     string
				Sig     string
				Result  string
			}
		}
	}
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, g := range v.TestGroups {
		var h hash.Hash
		switch g.Sha {
		case "SHA-256":
			h = sha256.New()
		case "SHA-512":
			h = sha512.New()
		default:
			t.Fatalf("%s: hash %s", file, g.Sha)
		}
		for _, tc := range g.Tests {
			msg, _ := hex.DecodeString(tc.Msg)
			h.Reset()
			h.Write(msg)
			sig, _ := hex.DecodeString(tc.Sig)
			got := s.ask(t, "V %s %x %s", g.PublicKey.Uncompressed, h.Sum(nil), hexOrDash(sig))
			if len(got) != 2 || got[0] != got[1] {
				t.Errorf("%s #%d (%s): fast, library %v", filepath.Base(file), tc.TcID, tc.Comment, got)
				continue
			}
			ok := got[0] == "0"
			if (tc.Result == "valid" && !ok) || (tc.Result == "invalid" && ok) {
				t.Errorf("%s #%d (%s): status %s, want %s", filepath.Base(file), tc.TcID, tc.Comment, got[0], tc.Result)
			}
			n++
		}
	}
	t.Logf("%s: %d vectors", filepath.Base(file), n)
}

// TestTLSAccelSanitizers runs tls_accel.c's checks with AddressSanitizer
// and UndefinedBehaviorSanitizer (an instrumented library and xo_p256.c).
func TestTLSAccelSanitizers(t *testing.T) {
	if testing.Short() {
		t.Skip("compiles an instrumented TLS library (a few seconds)")
	}
	bin := buildAccel(t, "-fsanitize=address,undefined", "-fno-sanitize-recover=undefined")
	out, err := exec.Command(bin, "100").CombinedOutput()
	if err != nil || strings.TrimSpace(string(out)) != "ok" {
		t.Fatalf("run: %v\n%s", err, out)
	}
}

// TestP256Bench prints tls_accel's signing and verification times (set
// XO_P256_BENCH to the number of operations).
func TestP256Bench(t *testing.T) {
	n := os.Getenv("XO_P256_BENCH")
	if n == "" {
		t.Skip("set XO_P256_BENCH=<operations>")
	}
	out, err := exec.Command(buildAccel(t, "-O2"), "bench", n).CombinedOutput()
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	t.Log(strings.TrimSpace(string(out)))
}
