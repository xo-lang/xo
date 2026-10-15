package llvm

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"fmt"
	"math/big"
	mrand "math/rand"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/xo-lang/xo/internal/xotool"
)

// x509VerifyDiffProgram verifies each line of its input (the leaf, the
// intermediates and the roots, each a list of 2 byte lengths and DERs,
// and "time host usage", all in hex) with internal.crypto_op's
// chain.text and prints the result in hex: Go's x509.Verify on the Go
// backend, std/native/llrt/x509verify.xo on the native ones.
const x509VerifyDiffProgram = `use std/internal

fn h(s: Str) -> Bytes { Bytes.from_hex(s) ?? b"" }

fn main(os: Os) {
  let all = os.stdio.read_all().or_fault("stdin")
  for line in all.lines() {
    if line != "" {
      let f = line.split(" ")
      let r = internal.crypto_op("chain.text", h(f[0]), h(f[1]), h(f[2]), h(f[3]))
      os.stdio.println(r.to_hex())
    }
  }
}
`

func pick[T any](rng *mrand.Rand, xs []T, max int) []T {
	var out []T
	for i := rng.Intn(max + 1); i > 0; i-- {
		out = append(out, xs[rng.Intn(len(xs))])
	}
	return out
}

func cidr(s string) *net.IPNet {
	_, n, err := net.ParseCIDR(s)
	if err != nil {
		panic(err)
	}
	return n
}

// x509Constraints fills a CA template with random name constraints.
func x509Constraints(rng *mrand.Rand, t *x509.Certificate) {
	dns := []string{"example.com", ".example.com", "a.example.com", "evil.org", "EXAMPLE.com", "com", "b.example.com", ""}
	ips := []*net.IPNet{cidr("10.0.0.0/8"), cidr("10.1.0.0/16"), cidr("192.168.0.0/16"), cidr("2001:db8::/32"), cidr("10.1.2.0/24"), cidr("::/0")}
	emails := []string{"example.com", ".example.com", "u@example.com", "U@EXAMPLE.COM", "evil.org", "v@a.example.com"}
	uris := []string{"example.com", ".example.com", "evil.org", "a.example.com"}
	t.PermittedDNSDomains = pick(rng, dns, 2)
	t.ExcludedDNSDomains = pick(rng, dns, 2)
	t.PermittedIPRanges = pick(rng, ips, 2)
	t.ExcludedIPRanges = pick(rng, ips, 2)
	t.PermittedEmailAddresses = pick(rng, emails, 2)
	t.ExcludedEmailAddresses = pick(rng, emails, 2)
	t.PermittedURIDomains = pick(rng, uris, 2)
	t.ExcludedURIDomains = pick(rng, uris, 2)
	t.PermittedDNSDomainsCritical = rng.Intn(2) == 0
}

// x509SANs fills a template with random subject alternative names.
func x509SANs(rng *mrand.Rand, t *x509.Certificate) {
	dns := []string{"a.example.com", "b.example.com", "example.com", "x.test", "*.example.com", "A.Example.Com", "sub.a.example.com", "evil.org", "*.evil.org", "a.evil.org"}
	ips := []string{"10.0.0.1", "10.1.2.3", "192.168.1.1", "::1", "2001:db8::1", "8.8.8.8"}
	emails := []string{"u@example.com", "u@a.example.com", "U@Example.COM", "v@evil.org", "\"q x\"@example.com", "w@b.example.com"}
	uris := []string{"https://a.example.com/x", "https://example.com:8443/", "https://10.0.0.1/", "spiffe://example.com/w", "https://evil.org", "https://[::1]/", "https://B.Example.com/"}
	t.DNSNames = pick(rng, dns, 3)
	for _, s := range pick(rng, ips, 2) {
		t.IPAddresses = append(t.IPAddresses, net.ParseIP(s))
	}
	t.EmailAddresses = pick(rng, emails, 2)
	for _, s := range pick(rng, uris, 2) {
		u, _ := url.Parse(s)
		t.URIs = append(t.URIs, u)
	}
}

func x509Pack(certs ...[]byte) []byte {
	var out []byte
	for _, c := range certs {
		out = append(out, byte(len(c)>>8), byte(len(c)))
		out = append(out, c...)
	}
	return out
}

// x509Chains makes n random chains: a root, sometimes an intermediate,
// a leaf; name constraints on the CAs, names on the intermediate and the
// leaf; each line the replay's inputs in hex.
func x509Chains(n int, seed int64) []string {
	rng := mrand.New(mrand.NewSource(seed))
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	now := time.Unix(1780000000, 0)
	serial := int64(1)
	mk := func(tmpl, parent *x509.Certificate) ([]byte, *x509.Certificate) {
		serial++
		tmpl.SerialNumber = big.NewInt(serial)
		tmpl.NotBefore, tmpl.NotAfter = now.Add(-time.Hour), now.Add(time.Hour)
		if parent == nil {
			parent = tmpl
		}
		der, err := x509.CreateCertificate(rand.Reader, tmpl, parent, key.Public(), key)
		if err != nil {
			return nil, nil
		}
		c, err := x509.ParseCertificate(der)
		if err != nil {
			return nil, nil
		}
		return der, c
	}
	var lines []string
	for len(lines) < n {
		root := &x509.Certificate{Subject: pkix.Name{CommonName: "root"}, IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
		if rng.Intn(3) != 0 {
			x509Constraints(rng, root)
		}
		rootDER, rootCert := mk(root, nil)
		if rootDER == nil {
			continue
		}
		var inters [][]byte
		parent := rootCert
		if rng.Intn(2) == 0 {
			in := &x509.Certificate{Subject: pkix.Name{CommonName: "inter"}, IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
			if rng.Intn(2) == 0 {
				x509Constraints(rng, in)
			}
			if rng.Intn(3) == 0 {
				x509SANs(rng, in)
			}
			inDER, inCert := mk(in, rootCert)
			if inDER == nil {
				continue
			}
			inters = append(inters, inDER)
			parent = inCert
		}
		leaf := &x509.Certificate{Subject: pkix.Name{CommonName: "leaf"}, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
		if rng.Intn(8) != 0 {
			x509SANs(rng, leaf)
		}
		leafDER, _ := mk(leaf, parent)
		if leafDER == nil {
			continue
		}
		args := fmt.Sprintf("%d - 1.3.6.1.5.5.7.3.1", now.Unix())
		lines = append(lines, strings.Join([]string{hex.EncodeToString(leafDER), hex.EncodeToString(x509Pack(inters...)), hex.EncodeToString(x509Pack(rootDER)), hex.EncodeToString([]byte(args))}, " "))
	}
	return lines
}

// TestX509VerifyDiff verifies random chains with name constraints (DNS,
// IP, e-mail and URI subtrees, permitted and excluded, on the root and an
// intermediate) over leaves and intermediates with every kind of name,
// with x509verify.xo (LLVM and arm64) and Go's x509.Verify (the Go
// backend), and diffs the outcome and its text. XO_X509_VERIFY_N chains
// (default 2000) from XO_X509_VERIFY_SEED (default 7).
func TestX509VerifyDiff(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a program with three backends")
	}
	if _, err := xotool.Clang(); err != nil {
		t.Skip("clang not found")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(x509VerifyDiffProgram), 0o644); err != nil {
		t.Fatal(err)
	}
	mod := "module github.com/xo-lang/xo/test/x509verifydiff\nedition preview\n"
	if err := os.WriteFile(filepath.Join(dir, "xo.mod"), []byte(mod), 0o644); err != nil {
		t.Fatal(err)
	}
	n := 2000
	if v, err := strconv.Atoi(os.Getenv("XO_X509_VERIFY_N")); err == nil {
		n = v
	}
	seed := int64(7)
	if v, err := strconv.ParseInt(os.Getenv("XO_X509_VERIFY_SEED"), 10, 64); err == nil {
		seed = v
	}
	lines := x509Chains(n, seed)
	backends := []string{xotool.BackendGo, xotool.BackendLLVM}
	if runtime.GOARCH == "arm64" {
		backends = append(backends, xotool.BackendArm64)
	}
	outs := make([][]string, len(backends))
	for i, be := range backends {
		res, err := xotool.Build([]string{dir}, xotool.Options{Backend: be})
		if err != nil {
			t.Fatalf("build (%s): %v", be, err)
		}
		if res.Failed {
			t.Fatalf("build (%s) failed: %v", be, res.Output)
		}
		cmd := exec.Command(res.Binaries[xotool.Host()])
		cmd.Stdin = strings.NewReader(strings.Join(lines, "\n") + "\n")
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("%s: %v\n%s", be, err, stderr.String())
		}
		outs[i] = strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")
		if len(outs[i]) != len(lines) {
			t.Fatalf("%s: %d results for %d chains", be, len(outs[i]), len(lines))
		}
	}
	kinds := map[string]int{}
	bad := 0
	for i := range lines {
		ref, _ := hex.DecodeString(outs[0][i])
		// the outcome's kind: the text up to the name in quotes
		k := string(ref)
		if j := strings.Index(k, "\""); j >= 0 {
			k = k[:j]
		}
		kinds[k]++
		for b := 1; b < len(backends); b++ {
			if outs[b][i] == outs[0][i] {
				continue
			}
			got, _ := hex.DecodeString(outs[b][i])
			if bad < 10 {
				t.Errorf("chain %d: %s differs\ngo:   %s\n%s: %s", i, backends[b], ref, backends[b], got)
			}
			bad++
		}
	}
	t.Logf("%d chains, %d differences", len(lines), bad)
	for k, v := range kinds {
		t.Logf("%6d  %s", v, k)
	}
}
