// A fuzz harness for the PEM reader std/x509, std/tls and std/http share
// (std/native/llrt/tlsname.xo pem_next) against Go's encoding/pem
// (decision 0461's fuzz for a parser of untrusted input; the 0616 x509
// review). Each case is a PEM text of one to three certificate blocks
// (Go's CreateCertificate, Ed25519) and other blocks, changed the way a
// broken or hostile file would be: BEGIN lines without an END, headers,
// CRLF line ends, trailing white space, white space inside the base64,
// missing or extra padding, a wrong END type, text between blocks, a byte
// changed or the text cut. std/x509's parse_pem must give what Go gives
// for the same text: pem.Decode's CERTIFICATE blocks, each through
// x509.ParseCertificate, as their serial numbers, or the first error's
// text.
//
// The cases from seed 1 are the committed corpus
// testdata/corpus/pemfuzz/pemfuzz_test.xo (TestStdCorpora);
// TestPemFuzzCorpusCurrent checks it is what this generator writes
// (XO_PEMFUZZ_WRITE=1 rewrites it). TestPemFuzz runs a fresh seed
// (XO_PEMFUZZ_SEED, default 2; XO_PEMFUZZ_N, default 1000) on
// XO_PEMFUZZ_BACKENDS (default arm64 and llvm).
package stdtest

import (
	"crypto/ed25519"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"math/big"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// pzCerts are a few small certificates (Ed25519, deterministic).
func pzCerts(t *testing.T) [][]byte {
	t.Helper()
	var out [][]byte
	for i := 0; i < 4; i++ {
		seed := make([]byte, 32)
		seed[0] = byte(i + 1)
		k := ed25519.NewKeyFromSeed(seed)
		tmpl := &x509.Certificate{SerialNumber: big.NewInt(int64(1000 + i)), Subject: pkix.Name{CommonName: fmt.Sprintf("pem%d", i)},
			NotBefore: time.Unix(1700000000, 0), NotAfter: time.Unix(1900000000, 0), DNSNames: []string{fmt.Sprintf("h%d.example", i)}}
		der, err := x509.CreateCertificate(nil, tmpl, tmpl, k.Public(), k)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, der)
	}
	return out
}

// pzBlock is one PEM block, built by hand so its lines can be changed.
func pzBlock(r *rand.Rand, typ string, der []byte) string {
	b64 := base64.StdEncoding.EncodeToString(der)
	width := []int{64, 64, 64, 76, 16, 1 << 20}[r.Intn(6)]
	nl := "\n"
	if r.Intn(5) == 0 {
		nl = "\r\n"
	}
	var lines []string
	for len(b64) > width {
		lines = append(lines, b64[:width])
		b64 = b64[width:]
	}
	lines = append(lines, b64)
	var b strings.Builder
	b.WriteString("-----BEGIN " + typ + "-----")
	if r.Intn(8) == 0 {
		b.WriteString(" \t")
	}
	b.WriteString(nl)
	if r.Intn(6) == 0 {
		b.WriteString("Proc-Type: 4,ENCRYPTED" + nl + "DEK-Info: AES-128-CBC,00" + nl + nl)
	}
	for _, l := range lines {
		if r.Intn(12) == 0 && len(l) > 4 {
			l = l[:2] + []string{" ", "\t", "\r"}[r.Intn(3)] + l[2:]
		}
		b.WriteString(l + nl)
	}
	end := typ
	if r.Intn(12) == 0 {
		end = "OTHER"
	}
	b.WriteString("-----END " + end + "-----")
	if r.Intn(8) == 0 {
		b.WriteString("  ")
	}
	b.WriteString(nl)
	return b.String()
}

func pzMutate(r *rand.Rand, in string) string {
	b := []byte(in)
	if len(b) == 0 {
		return in
	}
	i := r.Intn(len(b))
	switch r.Intn(9) {
	case 0:
		const alpha = "-=ABz09+/ \n\r:\x00"
		b[i] = alpha[r.Intn(len(alpha))]
	case 1:
		b = append(b[:i], b[i+1:]...)
	case 2:
		b = b[:i]
	case 3: // a BEGIN line without an END
		b = append(b[:i], append([]byte("\n-----BEGIN CERTIFICATE-----\n"), b[i:]...)...)
	case 4: // padding removed or added
		s := string(b)
		if strings.Contains(s, "=") && r.Intn(2) == 0 {
			s = strings.Replace(s, "=", "", 1)
		} else {
			s = strings.Replace(s, "\n-----END", "=\n-----END", 1)
		}
		b = []byte(s)
	case 5: // an END line early
		b = append(b[:i], append([]byte("\n-----END CERTIFICATE-----\n"), b[i:]...)...)
	case 6: // text between blocks
		b = append(b[:i], append([]byte("junk: x\n"), b[i:]...)...)
	case 7: // BEGIN not at a line start
		s := strings.Replace(string(b), "\n-----BEGIN", "\nx-----BEGIN", 1)
		b = []byte(s)
	default:
		return in
	}
	return string(b)
}

func pzCases(t *testing.T, r *rand.Rand, n int) []string {
	certs := pzCerts(t)
	var out []string
	for len(out) < n {
		var b strings.Builder
		if r.Intn(4) == 0 {
			b.WriteString("leading text\n")
		}
		for k := 1 + r.Intn(3); k > 0; k-- {
			typ := "CERTIFICATE"
			der := certs[r.Intn(len(certs))]
			switch r.Intn(6) {
			case 0:
				typ = "PRIVATE KEY"
				der = []byte("not a key, some bytes")
			case 1:
				typ = "X509 CERTIFICATE"
			}
			b.WriteString(pzBlock(r, typ, der))
		}
		text := b.String()
		for m := r.Intn(3); m > 0; m-- {
			text = pzMutate(r, text)
		}
		out = append(out, text)
	}
	return out
}

// pzWant is Go's answer: the CERTIFICATE blocks pem.Decode finds, each
// parsed, as "serials ..." (hex, comma separated), or "error: " and the
// first parse error.
func pzWant(text string) string {
	rest := []byte(text)
	var serials []string
	for {
		var p *pem.Block
		p, rest = pem.Decode(rest)
		if p == nil {
			break
		}
		if p.Type != "CERTIFICATE" {
			continue
		}
		c, err := x509.ParseCertificate(p.Bytes)
		if err != nil {
			return "error: " + err.Error()
		}
		serials = append(serials, fmt.Sprintf("%x", c.SerialNumber.Bytes()))
	}
	return "serials " + strings.Join(serials, ",")
}

const pzCheckXo = `use std/x509

test "std/x509's parse_pem finds the certificate blocks Go's pem.Decode finds" {
  var bad: List[Str] = []
  for (hex, want) in CASES {
    let text = Bytes.from_hex(hex) ?? fault("hex")
    let got = match x509.parse_pem(text) {
      Ok(cs) => "serials " + cs.map(fn(c) { c.serial.to_hex() }).join(",")
      Err(e) => "error: ${e.display()}"
    }
    if got != want {
      bad.push("${hex}\n  got  ${got}\n  want ${want}")
    }
  }
  if !bad.is_empty() {
    fault("${bad.len()} of ${CASES.len()} differ, first: ${bad[0]}")
  }
}

test "a run of BEGIN lines without an END is read in linear time" {
  // GO-2025-4009: each BEGIN searched to the end for its END line (1 MB
  // took 45 s); one megabyte now reads at once
  let text = "-----BEGIN A-----\n".repeat(60000)
  expect(x509.parse_pem(text.bytes()).ok().map(fn(l) { l.len() })) == Some(0)
}
`

func pzXoFile(seed int64, cases []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "// Generated by internal/stdtest/pemfuzz_test.go (seed %d, %d cases) from Go's\n", seed, len(cases))
	b.WriteString("// encoding/pem and crypto/x509; do not edit: XO_PEMFUZZ_WRITE=1 go test\n// ./internal/stdtest -run TestPemFuzzCorpusCurrent rewrites it.\n")
	b.WriteString(pzCheckXo)
	b.WriteString("\n/// (PEM text in hexadecimal, Go's serials or error)\nconst CASES: List[(Str, Str)] = [\n")
	for _, c := range cases {
		fmt.Fprintf(&b, "  (%s, %s),\n", xoStr(hex.EncodeToString([]byte(c))), xoStr(pzWant(c)))
	}
	b.WriteString("]\n")
	return b.String()
}

func pzCoverage(t *testing.T, cases []string) {
	t.Helper()
	none, some, errs := 0, 0, 0
	for _, c := range cases {
		w := pzWant(c)
		switch {
		case strings.HasPrefix(w, "error: "):
			errs++
		case w == "serials ":
			none++
		default:
			some++
		}
	}
	n := len(cases)
	if some < n/3 || none < n/20 || errs < n/50 {
		t.Fatalf("coverage fell: %d with certificates, %d with none, %d errors of %d", some, none, errs, n)
	}
}

const pzCorpus = "testdata/corpus/pemfuzz/pemfuzz_test.xo"

func TestPemFuzzCorpusCurrent(t *testing.T) {
	cases := pzCases(t, rand.New(rand.NewSource(1)), 1000)
	pzCoverage(t, cases)
	text := pzXoFile(1, cases)
	if os.Getenv("XO_PEMFUZZ_WRITE") == "1" {
		if err := os.MkdirAll(filepath.Dir(pzCorpus), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(pzCorpus, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	have, err := os.ReadFile(pzCorpus)
	if err != nil {
		t.Fatal(err)
	}
	if string(have) != text {
		t.Fatalf("%s is not what seed 1 writes: XO_PEMFUZZ_WRITE=1 go test ./internal/stdtest -run TestPemFuzzCorpusCurrent", pzCorpus)
	}
}

func TestPemFuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("the PEM fuzz is a heavy test; its seed 1 corpus runs in TestStdCorpora")
	}
	seed := int64(envInt(t, "XO_PEMFUZZ_SEED", 2))
	n := envInt(t, "XO_PEMFUZZ_N", 1000)
	cases := pzCases(t, rand.New(rand.NewSource(seed)), n)
	runFuzzDir(t, "pemfuzz_test.xo", pzXoFile(seed, cases), envBackends("XO_PEMFUZZ_BACKENDS", []string{"arm64", "llvm"}), len(cases))
}
