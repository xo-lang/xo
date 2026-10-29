package main

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/pem"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"math/rand"
	"net/netip"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

// The API this corpus covers, as std/x509.xo declares it: parse,
// parse_pem, every Certificate field, Name.display and attributes, and
// Certificate.verify (Go's crypto/x509 ParseCertificate, pkix.Name.String
// and Certificate.Verify).
//
// The inputs: every certificate of Go's crypto/x509/testdata and
// crypto/tls/testdata, every fifth of the NIST PKITS certificates
// (testdata/nist-pkits/certs), and the PEM constants of verify_test.go,
// each parsed by Go here and summarized field by field; truncated and
// bit-flipped copies of a few of them, with Go's error text or summary;
// and verify_test.go's verifyTests rows (those with roots: the others
// use the system's), with Go's chains (each certificate's subject) or
// its error text. Go's tests and testdata are Copyright 2009 The Go
// Authors, BSD-style license; the NIST PKITS certificates are a work of
// the US government.

type x509Row struct {
	der  []byte
	want string // Go's summary, or "error: " and Go's text
	why  string
}

// x509Summary is the line the corpus's summary function writes for a
// certificate: every Certificate field std/x509 has.
func x509Summary(c *x509.Certificate) string {
	var ips []string
	for _, ip := range c.IPAddresses {
		if a, ok := netip.AddrFromSlice(ip); ok {
			ips = append(ips, a.String())
		}
	}
	var uris []string
	for _, u := range c.URIs {
		uris = append(uris, u.String())
	}
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
	return fmt.Sprintf("v=%d serial=%x sig=%s subj=%s iss=%s nb=%d na=%d dns=%s ip=%s email=%s uri=%s ca=%v mpl=%s ku=%s eku=%s ski=%x aki=%x ext=%s key=%s attrs=%s",
		c.Version, serial, c.SignatureAlgorithm, c.Subject, c.Issuer, x509Clamp(c.NotBefore), x509Clamp(c.NotAfter),
		strings.Join(c.DNSNames, ","), strings.Join(ips, ","), strings.Join(c.EmailAddresses, ","), strings.Join(uris, ","),
		c.IsCA, mpl, strings.Join(ku, ","), strings.Join(eku, ","), c.SubjectKeyId, c.AuthorityKeyId,
		strings.Join(exts, ","), key, strings.Join(attrs, ";"))
}

// x509Clamp is t in Unix seconds as std/x509 gives it: clamped to the
// times an Xo Time holds (1677 to 2262).
func x509Clamp(t time.Time) int64 {
	s := t.Unix()
	if s > 9223372035 {
		return 9223372035
	}
	if s < -9223372035 {
		return -9223372035
	}
	return s
}

func x509Want(der []byte) string {
	c, err := x509.ParseCertificate(der)
	if err != nil {
		return "error: " + err.Error()
	}
	return x509Summary(c)
}

// x509Ders are the DERs of a file: its CERTIFICATE blocks, or the file
// itself when it holds none and parses as one.
func x509Ders(b []byte) [][]byte {
	var out [][]byte
	rest := b
	for {
		p, r := pem.Decode(rest)
		if p == nil {
			break
		}
		rest = r
		if p.Type == "CERTIFICATE" {
			out = append(out, p.Bytes)
		}
	}
	if len(out) == 0 && len(b) > 0 && b[0] == 0x30 {
		out = append(out, b)
	}
	return out
}

func x509Files(dir string, every int) []string {
	es, err := os.ReadDir(dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "stdcorpus:", err)
		os.Exit(1)
	}
	var out []string
	n := 0
	for _, e := range es {
		if e.IsDir() || strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), ".json") || strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		if n%every == 0 {
			out = append(out, filepath.Join(dir, e.Name()))
		}
		n++
	}
	return out
}

// goVerifyTest is a verifyTests row of verify_test.go with roots.
type goVerifyTest struct {
	name          string
	leaf          string
	intermediates []string
	roots         []string
	currentTime   int64
	dnsName       string
	keyUsages     []string // the ExtKeyUsage names, without the prefix
}

// goVerifyTests reads verify_test.go: its string constants and the
// verifyTests rows that name their roots.
func goVerifyTests() ([]goVerifyTest, map[string]string) {
	path := filepath.Join(runtime.GOROOT(), "src", "crypto", "x509", "verify_test.go")
	f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		fmt.Fprintln(os.Stderr, "stdcorpus:", err)
		os.Exit(1)
	}
	consts := map[string]string{}
	var table *ast.CompositeLit
	for _, d := range f.Decls {
		g, ok := d.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, s := range g.Specs {
			v, ok := s.(*ast.ValueSpec)
			if !ok || len(v.Values) != 1 {
				continue
			}
			if lit, ok := v.Values[0].(*ast.BasicLit); ok && lit.Kind == token.STRING && g.Tok == token.CONST {
				if s, err := strconv.Unquote(lit.Value); err == nil {
					consts[v.Names[0].Name] = s
				}
			}
			if v.Names[0].Name == "verifyTests" {
				table = v.Values[0].(*ast.CompositeLit)
			}
		}
	}
	str := func(e ast.Expr) string {
		switch x := e.(type) {
		case *ast.Ident:
			return consts[x.Name]
		case *ast.BasicLit:
			s, _ := strconv.Unquote(x.Value)
			return s
		}
		return ""
	}
	list := func(e ast.Expr) []string {
		var out []string
		if cl, ok := e.(*ast.CompositeLit); ok {
			for _, x := range cl.Elts {
				switch y := x.(type) {
				case *ast.SelectorExpr:
					out = append(out, strings.TrimPrefix(y.Sel.Name, "ExtKeyUsage"))
				case *ast.Ident:
					if strings.HasPrefix(y.Name, "ExtKeyUsage") {
						out = append(out, strings.TrimPrefix(y.Name, "ExtKeyUsage"))
					} else {
						out = append(out, consts[y.Name])
					}
				default:
					out = append(out, str(x))
				}
			}
		}
		return out
	}
	var out []goVerifyTest
	for _, el := range table.Elts {
		var t goVerifyTest
		for _, kv := range el.(*ast.CompositeLit).Elts {
			kv := kv.(*ast.KeyValueExpr)
			switch kv.Key.(*ast.Ident).Name {
			case "name":
				t.name = str(kv.Value)
			case "leaf":
				t.leaf = str(kv.Value)
			case "intermediates":
				t.intermediates = list(kv.Value)
			case "roots":
				t.roots = list(kv.Value)
			case "currentTime":
				t.currentTime, _ = strconv.ParseInt(kv.Value.(*ast.BasicLit).Value, 10, 64)
			case "dnsName":
				t.dnsName = str(kv.Value)
			case "keyUsages":
				t.keyUsages = list(kv.Value)
			}
		}
		// rows whose certificates are not string constants are left out
		ok := len(t.roots) > 0 && len(x509Ders([]byte(t.leaf))) > 0
		for _, x := range append(append([]string{}, t.roots...), t.intermediates...) {
			ok = ok && len(x509Ders([]byte(x))) > 0
		}
		if ok {
			out = append(out, t)
		} else if len(t.roots) > 0 {
			fmt.Fprintln(os.Stderr, "stdcorpus: x509: verifyTests row left out:", t.name)
		}
	}
	return out, consts
}

var x509EkuNames = map[string]x509.ExtKeyUsage{
	"Any": x509.ExtKeyUsageAny, "ServerAuth": x509.ExtKeyUsageServerAuth, "ClientAuth": x509.ExtKeyUsageClientAuth,
	"CodeSigning": x509.ExtKeyUsageCodeSigning, "EmailProtection": x509.ExtKeyUsageEmailProtection,
	"TimeStamping": x509.ExtKeyUsageTimeStamping, "OCSPSigning": x509.ExtKeyUsageOCSPSigning,
}

// x509Eku is a usage's ExtKeyUsage variant in the corpus.
func x509Eku(n string) string {
	if n == "OCSPSigning" {
		return "x509.ExtKeyUsage.OcspSigning"
	}
	return "x509.ExtKeyUsage." + n
}

// x509Verify is Go's Verify of a row: each chain's subjects joined by
// " > ", the chains sorted, joined by " | "; or "error: " and its text.
func x509Verify(t goVerifyTest) string {
	pool := func(pems []string) *x509.CertPool {
		p := x509.NewCertPool()
		for _, s := range pems {
			p.AppendCertsFromPEM([]byte(s))
		}
		return p
	}
	leaf, err := x509.ParseCertificate(x509Ders([]byte(t.leaf))[0])
	if err != nil {
		return "error: " + err.Error()
	}
	var usages []x509.ExtKeyUsage
	for _, u := range t.keyUsages {
		usages = append(usages, x509EkuNames[u])
	}
	chains, err := leaf.Verify(x509.VerifyOptions{
		Roots: pool(t.roots), Intermediates: pool(t.intermediates), DNSName: t.dnsName,
		CurrentTime: time.Unix(t.currentTime, 0), KeyUsages: usages,
	})
	if err != nil {
		return "error: " + err.Error()
	}
	var cs []string
	for _, ch := range chains {
		var names []string
		for _, c := range ch {
			names = append(names, c.Subject.String())
		}
		cs = append(cs, strings.Join(names, " > "))
	}
	sort.Strings(cs)
	return strings.Join(cs, " | ")
}

func x509Corpus() string {
	var o strings.Builder
	o.WriteString(header("Go's crypto/x509 and crypto/tls testdata certificates, verify_test.go's certificates and verifyTests, the NIST PKITS certificates, and mutated copies, against std/x509 (Go's tests are Copyright 2009 The Go Authors, BSD-style license)"))
	o.WriteString("\nuse std/crypto\nuse std/x509\n")

	src := filepath.Join(runtime.GOROOT(), "src", "crypto")
	var rows []x509Row
	seen := map[string]bool{}
	add := func(der []byte, why string) {
		if seen[string(der)] {
			return
		}
		seen[string(der)] = true
		rows = append(rows, x509Row{der, x509Want(der), why})
	}
	var files []string
	files = append(files, x509Files(filepath.Join(src, "x509", "testdata"), 1)...)
	files = append(files, x509Files(filepath.Join(src, "tls", "testdata"), 1)...)
	files = append(files, x509Files(filepath.Join(src, "x509", "testdata", "nist-pkits", "certs"), 5)...)
	for _, path := range files {
		b, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, "stdcorpus:", err)
			os.Exit(1)
		}
		for i, der := range x509Ders(b) {
			add(der, fmt.Sprintf("%s#%d", filepath.Base(path), i))
		}
	}
	tests, consts := goVerifyTests()
	var names []string
	for n := range consts {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		for i, der := range x509Ders([]byte(consts[n])) {
			add(der, fmt.Sprintf("verify_test.go %s#%d", n, i))
		}
	}
	// mutated copies: truncations and single bit flips of a few
	// certificates (an RSA, an ECDSA and an Ed25519 one when present)
	r := rand.New(rand.NewSource(509))
	base := len(rows)
	for _, k := range []int{0, base / 3, 2 * base / 3} {
		der := rows[k].der
		for _, n := range []int{0, 1, 2, 4, 10, len(der) / 2, len(der) - 1} {
			add(der[:n], fmt.Sprintf("%s truncated to %d", rows[k].why, n))
		}
		for j := 0; j < 40; j++ {
			m := append([]byte(nil), der...)
			p := r.Intn(len(m))
			m[p] ^= byte(1 << r.Intn(8))
			add(m, fmt.Sprintf("%s bit flip at %d", rows[k].why, p))
		}
	}
	o.WriteString("\n/// (DER, Go's summary or error, where it comes from)\nconst PARSE_ROWS: List[(Bytes, Str, Str)] = [\n")
	for _, x := range rows {
		fmt.Fprintf(&o, "  (%s, %s, %s),\n", xbytes(x.der), xq(x.want), xq(x.why))
	}
	o.WriteString("]\n")

	// VerifyHostname: each parseable certificate's names, and names
	// around them (a wildcard's label, case, a trailing dot, an IP)
	o.WriteString("\n/// (DER index in PARSE_ROWS, host, Go's VerifyHostname: ok or the error)\nconst HOST_ROWS: List[(Int, Str, Str)] = [\n")
	hosts := 0
	for i, x := range rows {
		c, err := x509.ParseCertificate(x.der)
		if err != nil || hosts > 400 {
			continue
		}
		cands := []string{"example.com", "127.0.0.1", "::1", "[::1]", ""}
		for _, d := range c.DNSNames {
			cands = append(cands, d, strings.ToUpper(d), d+".", "x."+strings.TrimPrefix(d, "*."), "a.b."+strings.TrimPrefix(d, "*."))
		}
		for _, a := range c.IPAddresses {
			cands = append(cands, a.String())
		}
		if c.Subject.CommonName != "" {
			cands = append(cands, c.Subject.CommonName)
		}
		for _, h := range cands {
			want := "ok"
			if err := c.VerifyHostname(h); err != nil {
				want = err.Error()
			}
			fmt.Fprintf(&o, "  (%d, %s, %s),\n", i, xq(h), xq(want))
			hosts++
		}
	}
	o.WriteString("]\n")

	o.WriteString("\n/// verify_test.go's verifyTests with roots: (name, leaf, intermediates, roots, Unix time, DNS name, key usages, Go's chains or error)\nconst VERIFY_ROWS: List[(Str, Str, List[Str], List[Str], Int, Str, List[x509.ExtKeyUsage], Str)] = [\n")
	for _, t := range tests {
		var ks []string
		for _, u := range t.keyUsages {
			ks = append(ks, x509Eku(u))
		}
		var is, rs []string
		for _, s := range t.intermediates {
			is = append(is, xq(s))
		}
		for _, s := range t.roots {
			rs = append(rs, xq(s))
		}
		fmt.Fprintf(&o, "  (%s, %s, [%s], [%s], %d, %s, [%s], %s),\n", xq(t.name), xq(t.leaf), strings.Join(is, ", "), strings.Join(rs, ", "),
			t.currentTime, xq(t.dnsName), strings.Join(ks, ", "), xq(x509Verify(t)))
	}
	o.WriteString("]\n")

	// one Name with every escape RDNSequence.String writes, an unknown
	// attribute (shown as #hex of its marshaled value) and two values in
	// one RDN
	name := pkix.RDNSequence{
		{{Type: asn1.ObjectIdentifier{2, 5, 4, 6}, Value: "US"}},
		{{Type: asn1.ObjectIdentifier{2, 5, 4, 10}, Value: " a,b+c\"d\\e<f>g;h "}, {Type: asn1.ObjectIdentifier{2, 5, 4, 11}, Value: "#unit"}},
		{{Type: asn1.ObjectIdentifier{1, 2, 3, 4}, Value: "extra é"}},
		{{Type: asn1.ObjectIdentifier{2, 5, 4, 3}, Value: "common"}},
	}
	nder, err := asn1.Marshal(name)
	if err != nil {
		panic(err)
	}
	var pn pkix.Name
	pn.FillFromRDNSequence(&name)
	fmt.Fprintf(&o, "\nconst NAME_DER: Bytes = %s\nconst NAME_TEXT: Str = %s\n", xbytes(nder), xq(pn.String()))

	o.WriteString(`
fn shown(bad: List[Str]) -> List[Str] {
  bad[0..(if bad.len() < 20 { bad.len() } else { 20 })]
}

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

test "every certificate parses as Go's ParseCertificate, field by field, errors with Go's text" {
  var bad: List[Str] = []
  for (der, want, why) in PARSE_ROWS {
    let got = match x509.parse(der) {
      Ok(c) => summary(c)
      Err(e) => "error: ${e.display()}"
    }
    if got != want {
      bad.push("${why}:\n  got  ${got}\n  want ${want}")
    }
  }
  expect(shown(bad)) == []
}

fn certs(pems: List[Str]) -> List[x509.Certificate] {
  var out: List[x509.Certificate] = []
  for p in pems {
    for c in x509.parse_pem(p.bytes()).ok() ?? [] {
      out.push(c)
    }
  }
  out
}

test "verify_test.go's verifyTests verify as Go's Verify: the chains or the error" {
  var bad: List[Str] = []
  for (name, leaf, inters, roots, at, dns, usages, want) in VERIFY_ROWS {
    let l = certs([leaf])[0]
    let opts = if usages.is_empty() {
      x509.VerifyOptions{roots: certs(roots), intermediates: certs(inters), dns_name: dns, at: Time.from_unix(at)}
    } else {
      x509.VerifyOptions{roots: certs(roots), intermediates: certs(inters), dns_name: dns, at: Time.from_unix(at), key_usages: usages}
    }
    let got = match l.verify(opts) {
      Ok(chains) => chains.map(fn(ch) { ch.map(fn(c) { c.subject.display() }).join(" > ") }).sort().join(" | ")
      Err(e) => "error: ${e.display()}"
    }
    if got != want {
      bad.push("${name}:\n  got  ${got}\n  want ${want}")
    }
  }
  expect(shown(bad)) == []
}

test "verify_hostname answers as Go's VerifyHostname: ok or the error" {
  var bad: List[Str] = []
  for (i, host, want) in HOST_ROWS {
    let c = x509.parse(PARSE_ROWS[i].0).ok() ?? fault("parse")
    let got = match c.verify_hostname(host) {
      Ok(_) => "ok"
      Err(e) => e.display()
    }
    if got != want {
      bad.push("${PARSE_ROWS[i].2} ${host.quote()}:\n  got  ${got}\n  want ${want}")
    }
  }
  expect(shown(bad)) == []
}

test "a Name's text escapes as Go's RDNSequence.String" {
  let leaf = x509.parse(PARSE_ROWS[0].0).ok() ?? fault("parse")
  let n = x509.Name{country: ["US"], organization: [" a,b+c\"d\\e<f>g;h "], organizational_unit: ["#unit"], locality: [], province: [], street_address: [], postal_code: [], serial_number: "", common_name: "common", raw: NAME_DER}
  expect(n.display()) == NAME_TEXT
  expect(n.attributes().len()) == 5
  expect(leaf.subject.raw.len() > 0) == true
}

test "parse_pem reads every CERTIFICATE block and stops at the first bad one" {
  let pem = "-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----\n"
  match x509.parse_pem(pem.bytes()) {
    Ok(_) => fault("a bad certificate parsed")
    Err(e) => expect(e.display().starts_with("x509: ")) == true
  }
  expect((x509.parse_pem(b"no blocks").ok() ?? [x509.parse(PARSE_ROWS[0].0).ok() ?? fault("parse")]).len()) == 0
}
`)
	return o.String()
}
