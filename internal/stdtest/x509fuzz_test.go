// A fuzz harness for std/x509's certificate parser against Go's
// crypto/x509 (decision 0461: a parser of untrusted input is Early on
// testing until it is fuzzed; decision 0616's fuzz kind). A TLS client
// parses every certificate a server sends before it checks a signature,
// so the parser sees hostile DER first.
//
// Each case is a DER: a certificate Go's CreateCertificate writes from a
// random template (names with every attribute std/x509 reads and extra
// ones, validity from year 1 to 9999, DNS names, IPs, emails, URIs, basic
// constraints, key usages, extended key usages known and unknown, key IDs,
// name constraints, policies, CRL points, unknown extensions, Ed25519,
// ECDSA and RSA subject keys), or one of Go's own test certificates
// (crypto/x509/testdata, every tenth NIST PKITS certificate); half of them
// then changed the way a hostile or broken input would be (a byte flipped,
// set, inserted or deleted, the DER cut, or a length octet moved by one).
// std/x509's parse must give what Go's ParseCertificate gives: the same
// summary of every field std/x509 has (scripts/stdcorpus x509Summary), or
// an error with Go's text.
//
// The cases from seed 1 are the committed corpus
// testdata/corpus/x509fuzz/x509fuzz_test.xo, which TestStdCorpora runs on
// every corpus backend; TestX509FuzzCorpusCurrent checks it is what this
// generator writes (XO_X509FUZZ_WRITE=1 rewrites it). TestX509Fuzz runs a
// fresh seed (XO_X509FUZZ_SEED, default 2; XO_X509FUZZ_N cases, default
// 1000) on XO_X509FUZZ_BACKENDS (default arm64 and llvm).
package stdtest

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"math/big"
	"math/rand"
	"net"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

// xzSummary is scripts/stdcorpus x509Summary: every Certificate field
// std/x509 has, on one line.
//
// URIs are the SAN's text as written, which std/x509 keeps: Go keeps a
// parsed url.URL, whose String() re-escapes (a mutated `n{/a` prints as
// `n%7B/a`; seed 2 of the 2026-11-13 1M run).
func xzSummary(c *x509.Certificate) string {
	var ips []string
	for _, ip := range c.IPAddresses {
		if a, ok := netip.AddrFromSlice(ip); ok {
			ips = append(ips, a.String())
		}
	}
	uris := xzRawURIs(c)
	mpl := "none"
	if c.BasicConstraintsValid && (c.MaxPathLen > 0 || c.MaxPathLenZero) {
		mpl = strconv.Itoa(c.MaxPathLen)
	}
	var ku []string
	for i := 0; i < 9; i++ {
		if c.KeyUsage&(1<<i) != 0 {
			ku = append(ku, strconv.Itoa(i))
		}
	}
	// the extension's order (spec/std.md 41), not Go's ExtKeyUsage then
	// UnknownExtKeyUsage: native std/x509 listed Go's named usages first
	// until this harness's first run (2026-11-13)
	var eku []string
	for _, e := range c.Extensions {
		if e.Id.Equal(asn1.ObjectIdentifier{2, 5, 29, 37}) {
			var oids []asn1.ObjectIdentifier
			if _, err := asn1.Unmarshal(e.Value, &oids); err == nil {
				for _, o := range oids {
					eku = append(eku, o.String())
				}
			}
		}
	}
	var exts []string
	for _, e := range c.Extensions {
		exts = append(exts, fmt.Sprintf("%s/%v/%d", e.Id, e.Critical, len(e.Value)))
	}
	key := "none"
	switch k := c.PublicKey.(type) {
	case *rsa.PublicKey:
		key = "Rsa"
	case *ecdsa.PublicKey:
		switch k.Curve {
		case elliptic.P224():
			key = "EcdsaP224"
		case elliptic.P256():
			key = "EcdsaP256"
		case elliptic.P384():
			key = "EcdsaP384"
		case elliptic.P521():
			key = "EcdsaP521"
		}
	case ed25519.PublicKey:
		key = "Ed25519"
	}
	var attrs []string
	for _, a := range c.Subject.Names {
		attrs = append(attrs, a.Type.String()+"="+fmt.Sprint(a.Value))
	}
	serial := []byte{}
	if c.SerialNumber != nil {
		serial = c.SerialNumber.Bytes()
	}
	clamp := func(t time.Time) int64 {
		s := t.Unix()
		if s > 9223372035 {
			return 9223372035
		}
		if s < -9223372035 {
			return -9223372035
		}
		return s
	}
	return fmt.Sprintf("v=%d serial=%x sig=%s subj=%s iss=%s nb=%d na=%d dns=%s ip=%s email=%s uri=%s ca=%v mpl=%s ku=%s eku=%s ski=%x aki=%x ext=%s key=%s attrs=%s",
		c.Version, serial, c.SignatureAlgorithm, c.Subject, c.Issuer, clamp(c.NotBefore), clamp(c.NotAfter),
		strings.Join(c.DNSNames, ","), strings.Join(ips, ","), strings.Join(c.EmailAddresses, ","), strings.Join(uris, ","),
		c.IsCA, mpl, strings.Join(ku, ","), strings.Join(eku, ","), c.SubjectKeyId, c.AuthorityKeyId,
		strings.Join(exts, ","), key, strings.Join(attrs, ";"))
}

func xzWant(der []byte) string {
	c, err := x509.ParseCertificate(der)
	if err != nil {
		return "error: " + err.Error()
	}
	return xzSummary(c)
}

// xzPool is Go's own test certificates: crypto/x509/testdata's PEM files
// and every tenth NIST PKITS certificate, in name order.
func xzPool(t *testing.T) [][]byte {
	t.Helper()
	dir := filepath.Join(runtime.GOROOT(), "src", "crypto", "x509", "testdata")
	var out [][]byte
	pems, _ := filepath.Glob(filepath.Join(dir, "*.pem"))
	sort.Strings(pems)
	for _, f := range pems {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for {
			p, rest := pem.Decode(b)
			if p == nil {
				break
			}
			b = rest
			if p.Type == "CERTIFICATE" {
				out = append(out, p.Bytes)
			}
		}
	}
	crts, _ := filepath.Glob(filepath.Join(dir, "nist-pkits", "certs", "*.crt"))
	sort.Strings(crts)
	for i, f := range crts {
		if i%10 != 0 {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, b)
	}
	if len(out) < 40 {
		t.Fatalf("only %d of Go's test certificates under %s", len(out), dir)
	}
	return out
}

// xzKeys are subject keys from the pool: the first RSA and the first
// ECDSA key of each curve Go's test certificates hold.
func xzKeys(pool [][]byte) []any {
	var keys []any
	seen := map[string]bool{}
	for _, der := range pool {
		c, err := x509.ParseCertificate(der)
		if err != nil {
			continue
		}
		name := ""
		switch k := c.PublicKey.(type) {
		case *rsa.PublicKey:
			name = "rsa"
		case *ecdsa.PublicKey:
			name = k.Curve.Params().Name
		}
		if name != "" && !seen[name] {
			seen[name] = true
			keys = append(keys, c.PublicKey)
		}
	}
	return keys
}

var xzText = []string{"example.com", "Example Co", "béta", "γάμμα", "a,b", "x+y", "quote\"d", "back\\slash", " lead", "trail ", "#hash", "<tag>", "a=b;c", "", "日本", "emoji 🚀", "x"}

func xzStr(r *rand.Rand) string {
	if r.Intn(5) == 0 {
		b := make([]byte, 1+r.Intn(12))
		for i := range b {
			b[i] = byte(0x20 + r.Intn(0x5f))
		}
		return string(b)
	}
	return xzText[r.Intn(len(xzText))]
}

func xzStrs(r *rand.Rand, n int) []string {
	var out []string
	for i := r.Intn(n + 1); i > 0; i-- {
		out = append(out, xzStr(r))
	}
	return out
}

func xzName(r *rand.Rand) pkix.Name {
	n := pkix.Name{
		Country:            xzStrs(r, 2),
		Organization:       xzStrs(r, 2),
		OrganizationalUnit: xzStrs(r, 2),
		Locality:           xzStrs(r, 1),
		Province:           xzStrs(r, 1),
		StreetAddress:      xzStrs(r, 1),
		PostalCode:         xzStrs(r, 1),
	}
	if r.Intn(2) == 0 {
		n.CommonName = xzStr(r)
	}
	if r.Intn(4) == 0 {
		n.SerialNumber = xzStr(r)
	}
	if r.Intn(4) == 0 {
		oids := []asn1.ObjectIdentifier{{2, 5, 4, 4}, {2, 5, 4, 42}, {1, 2, 840, 113549, 1, 9, 1}, {0, 9, 2342, 19200300, 100, 1, 25}, {1, 3, 6, 1, 4, 1, 99999, r.Intn(1000)}}
		n.ExtraNames = append(n.ExtraNames, pkix.AttributeTypeAndValue{Type: oids[r.Intn(len(oids))], Value: xzStr(r)})
	}
	return n
}

func xzTime(r *rand.Rand) time.Time {
	switch r.Intn(8) {
	case 0:
		return time.Date(1+r.Intn(1676), time.Month(1+r.Intn(12)), 1+r.Intn(28), r.Intn(24), r.Intn(60), r.Intn(60), 0, time.UTC)
	case 1:
		return time.Date(2263+r.Intn(7700), time.Month(1+r.Intn(12)), 1+r.Intn(28), r.Intn(24), r.Intn(60), r.Intn(60), 0, time.UTC)
	case 2:
		return time.Date(1949+r.Intn(3), 12, 31, 23, 59, 59, 0, time.UTC)
	case 3:
		return time.Date(2049+r.Intn(3), 12, 31, 23, 59, 59, 0, time.UTC)
	}
	return time.Unix(r.Int63n(130*365*86400)-20*365*86400, 0).UTC()
}

func xzBytes(r *rand.Rand, max int) []byte {
	b := make([]byte, r.Intn(max+1))
	r.Read(b)
	return b
}

func xzTemplate(r *rand.Rand) *x509.Certificate {
	serial := new(big.Int).SetBytes(xzBytes(r, 22))
	t := &x509.Certificate{
		SerialNumber: serial,
		Subject:      xzName(r),
		NotBefore:    xzTime(r),
		NotAfter:     xzTime(r),
	}
	for i := r.Intn(3); i > 0; i-- {
		t.DNSNames = append(t.DNSNames, []string{"example.com", "*.example.org", "xn--bcher-kva.example", "a.b.c.d", "localhost", "UPPER.Example.COM", "bad name", "", "under_score.test"}[r.Intn(9)])
	}
	for i := r.Intn(3); i > 0; i-- {
		t.EmailAddresses = append(t.EmailAddresses, []string{"a@example.com", "user+tag@example.org", "x@y", "no-at-sign", "ü@example.com"}[r.Intn(5)])
	}
	for i := r.Intn(3); i > 0; i-- {
		if r.Intn(2) == 0 {
			t.IPAddresses = append(t.IPAddresses, net.IP(xzBytes4(r)))
		} else {
			b := make([]byte, 16)
			r.Read(b)
			t.IPAddresses = append(t.IPAddresses, net.IP(b))
		}
	}
	for i := r.Intn(2); i > 0; i-- {
		u, err := url.Parse([]string{"https://example.com/path?q=1", "spiffe://trust.example/ns/a", "urn:example:x", "http://[::1]:8080/", "mailto:a@b"}[r.Intn(5)])
		if err == nil {
			t.URIs = append(t.URIs, u)
		}
	}
	if r.Intn(2) == 0 {
		t.BasicConstraintsValid = true
		t.IsCA = r.Intn(2) == 0
		switch r.Intn(3) {
		case 0:
			t.MaxPathLen = -1
		case 1:
			t.MaxPathLen = 0
			t.MaxPathLenZero = true
		default:
			t.MaxPathLen = r.Intn(10)
		}
	}
	t.KeyUsage = x509.KeyUsage(r.Intn(512))
	for i := r.Intn(4); i > 0; i-- {
		t.ExtKeyUsage = append(t.ExtKeyUsage, x509.ExtKeyUsage(r.Intn(14)))
	}
	if r.Intn(4) == 0 {
		t.UnknownExtKeyUsage = []asn1.ObjectIdentifier{{1, 3, 6, 1, 4, 1, 311, 10, 3, r.Intn(30)}}
	}
	if r.Intn(2) == 0 {
		t.SubjectKeyId = xzBytes(r, 32)
	}
	if r.Intn(2) == 0 {
		t.AuthorityKeyId = xzBytes(r, 32)
	}
	if r.Intn(5) == 0 {
		t.PermittedDNSDomains = xzStrs(r, 2)
		t.ExcludedDNSDomains = xzStrs(r, 1)
		t.PermittedDNSDomainsCritical = r.Intn(2) == 0
	}
	if r.Intn(5) == 0 {
		t.CRLDistributionPoints = []string{"http://crl.example.com/a.crl"}
		t.OCSPServer = []string{"http://ocsp.example.com"}
		t.IssuingCertificateURL = []string{"http://ca.example.com/ca.crt"}
	}
	if r.Intn(5) == 0 {
		t.PolicyIdentifiers = []asn1.ObjectIdentifier{{2, 5, 29, 32, 0}, {1, 3, 6, 1, 4, 1, r.Intn(99999)}}[:1+r.Intn(2)]
	}
	for i := r.Intn(3); i > 0; i-- {
		t.ExtraExtensions = append(t.ExtraExtensions, pkix.Extension{
			Id:       asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 99999, r.Intn(100)},
			Critical: r.Intn(4) == 0,
			Value:    xzBytes(r, 40),
		})
	}
	return t
}

func xzBytes4(r *rand.Rand) []byte {
	b := make([]byte, 4)
	r.Read(b)
	return b
}

// xzMutate changes a DER the way a hostile or broken input would be.
func xzMutate(r *rand.Rand, in []byte) []byte {
	b := append([]byte(nil), in...)
	if len(b) == 0 {
		return b
	}
	i := r.Intn(len(b))
	switch r.Intn(6) {
	case 0:
		b[i] ^= 1 << r.Intn(8)
	case 1:
		b[i] = []byte{0x00, 0x01, 0x7f, 0x80, 0x81, 0x82, 0xff, 0x30, 0x05}[r.Intn(9)]
	case 2:
		b = append(b[:i], append([]byte{byte(r.Intn(256))}, b[i:]...)...)
	case 3:
		b = append(b[:i], b[i+1:]...)
	case 4:
		b = b[:i]
	default:
		// a length octet one off: a short form length after a tag byte
		for k := 0; k < 64; k++ {
			j := 1 + r.Intn(len(b)-1)
			if b[j] < 0x80 && (b[j-1]&0x1f) != 0x1f && b[j-1] != 0 {
				if r.Intn(2) == 0 {
					b[j]++
				} else if b[j] > 0 {
					b[j]--
				}
				break
			}
		}
	}
	return b
}

func xzCases(t *testing.T, r *rand.Rand, n int) [][]byte {
	pool := xzPool(t)
	keys := xzKeys(pool)
	var out [][]byte
	for len(out) < n {
		var der []byte
		if r.Intn(3) == 0 {
			der = pool[r.Intn(len(pool))]
		} else {
			seed := make([]byte, ed25519.SeedSize)
			r.Read(seed)
			signer := ed25519.NewKeyFromSeed(seed)
			tmpl := xzTemplate(r)
			var pub any = signer.Public()
			if r.Intn(2) == 0 && len(keys) > 0 {
				pub = keys[r.Intn(len(keys))]
			}
			parent := tmpl
			if r.Intn(2) == 0 {
				parent = &x509.Certificate{Subject: xzName(r), SubjectKeyId: xzBytes(r, 20)}
			}
			d, err := x509.CreateCertificate(nil, tmpl, parent, pub, signer)
			if err != nil {
				continue
			}
			der = d
		}
		if r.Intn(2) == 0 {
			der = xzMutate(r, der)
		}
		out = append(out, der)
	}
	return out
}

const xzCheckXo = `use std/crypto
use std/x509

fn ku_index(u: x509.KeyUsage) -> Int {
  match u {
    DigitalSignature => 0
    ContentCommitment => 1
    KeyEncipherment => 2
    DataEncipherment => 3
    KeyAgreement => 4
    CertSign => 5
    CrlSign => 6
    EncipherOnly => 7
    DecipherOnly => 8
  }
}

fn eku_oid(u: x509.ExtKeyUsage) -> Str {
  match u {
    Any => "2.5.29.37.0"
    ServerAuth => "1.3.6.1.5.5.7.3.1"
    ClientAuth => "1.3.6.1.5.5.7.3.2"
    CodeSigning => "1.3.6.1.5.5.7.3.3"
    EmailProtection => "1.3.6.1.5.5.7.3.4"
    TimeStamping => "1.3.6.1.5.5.7.3.8"
    OcspSigning => "1.3.6.1.5.5.7.3.9"
    Other(oid) => oid
  }
}

fn key_name(k: crypto.PublicKey?) -> Str {
  match k {
    Some(x) => match x.alg() {
      Rsa => "Rsa"
      EcdsaP224 => "EcdsaP224"
      EcdsaP256 => "EcdsaP256"
      EcdsaP384 => "EcdsaP384"
      EcdsaP521 => "EcdsaP521"
      Ed25519 => "Ed25519"
    }
    None => "none"
  }
}

fn summary(c: x509.Certificate) -> Str {
  let ips = c.ip_addresses.map(fn(a) { a.display() }).join(",")
  let mpl = match c.max_path_len {
    Some(n) => "${n}"
    None => "none"
  }
  let ku = c.key_usage.map(fn(u) { "${ku_index(u)}" }).join(",")
  let eku = c.ext_key_usage.map(fn(u) { eku_oid(u) }).join(",")
  let ext = c.extensions.map(fn(e) { "${e.oid}/${e.critical}/${e.value.len()}" }).join(",")
  let attrs = c.subject.attributes().map(fn(a) { "${a.0}=${a.1}" }).join(";")
  "v=${c.version} serial=${c.serial.to_hex()} sig=${c.signature_algorithm} subj=${c.subject.display()} iss=${c.issuer.display()} nb=${c.not_before.unix()} na=${c.not_after.unix()} dns=${c.dns_names.join(",")} ip=${ips} email=${c.email_addresses.join(",")} uri=${c.uris.join(",")} ca=${c.is_ca} mpl=${mpl} ku=${ku} eku=${eku} ski=${c.subject_key_id.to_hex()} aki=${c.authority_key_id.to_hex()} ext=${ext} key=${key_name(c.public_key)} attrs=${attrs}"
}

test "std/x509 parses every certificate, whole or changed, as Go's ParseCertificate" {
  var bad: List[Str] = []
  for (hex, want) in CASES {
    let der = Bytes.from_hex(hex) ?? fault("hex")
    let got = match x509.parse(der) {
      Ok(c) => summary(c)
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
`

func xzXoFile(seed int64, ders [][]byte) string {
	var b strings.Builder
	fmt.Fprintf(&b, "// Generated by internal/stdtest/x509fuzz_test.go (seed %d, %d cases) from Go's\n", seed, len(ders))
	b.WriteString("// crypto/x509 and its testdata certificates (Copyright 2009 The Go Authors,\n// BSD-style license; the NIST PKITS certificates are a work of the US\n// government); do not edit: XO_X509FUZZ_WRITE=1 go test ./internal/stdtest\n// -run TestX509FuzzCorpusCurrent rewrites it.\n")
	b.WriteString(xzCheckXo)
	b.WriteString("\n/// (DER in hexadecimal, Go's summary or error)\nconst CASES: List[(Str, Str)] = [\n")
	for _, d := range ders {
		fmt.Fprintf(&b, "  (%s, %s),\n", xoStr(hex.EncodeToString(d)), xoStr(xzWant(d)))
	}
	b.WriteString("]\n")
	return b.String()
}

// xzCoverage fails when the cases stop reaching the parser's outcomes.
func xzCoverage(t *testing.T, ders [][]byte) {
	t.Helper()
	ok, errs, kinds := 0, 0, map[string]bool{}
	for _, d := range ders {
		w := xzWant(d)
		if strings.HasPrefix(w, "error: ") {
			errs++
			kinds[w] = true
		} else {
			ok++
		}
	}
	n := len(ders)
	if ok < n/3 || errs < n/5 || len(kinds) < 15 {
		t.Fatalf("coverage fell: %d parse, %d errors of %d kinds, of %d", ok, errs, len(kinds), n)
	}
}

const xzCorpus = "testdata/corpus/x509fuzz/x509fuzz_test.xo"

// TestX509FuzzCorpusCurrent: the committed corpus is what seed 1 writes.
func TestX509FuzzCorpusCurrent(t *testing.T) {
	ders := xzCases(t, rand.New(rand.NewSource(1)), 1000)
	xzCoverage(t, ders)
	text := xzXoFile(1, ders)
	if os.Getenv("XO_X509FUZZ_WRITE") == "1" {
		if err := os.MkdirAll(filepath.Dir(xzCorpus), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(xzCorpus, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	have, err := os.ReadFile(xzCorpus)
	if err != nil {
		t.Fatal(err)
	}
	if string(have) != text {
		t.Fatalf("%s is not what seed 1 writes (Go's crypto/x509 or the generator changed): XO_X509FUZZ_WRITE=1 go test ./internal/stdtest -run TestX509FuzzCorpusCurrent", xzCorpus)
	}
}

// TestX509Fuzz runs a fresh seed's cases.
func TestX509Fuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("the x509 fuzz is a heavy test; its seed 1 corpus runs in TestStdCorpora")
	}
	seed := int64(envInt(t, "XO_X509FUZZ_SEED", 2))
	n := envInt(t, "XO_X509FUZZ_N", 1000)
	ders := xzCases(t, rand.New(rand.NewSource(seed)), n)
	runFuzzDir(t, "x509fuzz_test.xo", xzXoFile(seed, ders), envBackends("XO_X509FUZZ_BACKENDS", []string{"arm64", "llvm"}), len(ders))
}

// xzRawURIs is the text of each uniformResourceIdentifier in c's subject
// alternative name extension, in order (Go parsed each with url.Parse,
// so this runs only for a certificate Go accepted).
func xzRawURIs(c *x509.Certificate) []string {
	var out []string
	for _, e := range c.Extensions {
		if !e.Id.Equal(asn1.ObjectIdentifier{2, 5, 29, 17}) {
			continue
		}
		var names []asn1.RawValue
		if _, err := asn1.Unmarshal(e.Value, &names); err != nil {
			return nil
		}
		for _, n := range names {
			if n.Class == asn1.ClassContextSpecific && n.Tag == 6 {
				out = append(out, string(n.Bytes))
			}
		}
	}
	return out
}
