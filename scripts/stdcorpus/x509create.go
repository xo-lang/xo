package main

import (
	"crypto"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"math/big"
	mrand "math/rand"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// The API this corpus covers, as std/x509.xo declares it: create_certificate,
// self_signed, Certificate.pem, create_request, parse_request,
// CertificateRequest.check_signature, parse_revocation_list,
// RevocationList.check_signature_from, and std/crypto's
// marshal_private_key (Go's CreateCertificate, CreateCertificateRequest,
// ParseCertificateRequest, ParseRevocationList and MarshalPKCS8PrivateKey).
//
// Both directions: each template is made here by Go and in the test by Xo
// with the same key and serial. Ed25519 and RSA PKCS #1 v1.5 signatures
// are deterministic, so those certificates and requests must be equal byte
// for byte (Go's parser reads what Xo makes because it is what Go makes);
// ECDSA ones (a random nonce) must give the same fields and verify. Go's
// CRLs (CreateRevocationList) and requests, and mutated copies, are parsed
// by Xo against Go's fields and errors. The keys are Go's test keys
// (crypto/x509 x509_test.go pemPrivateKey, crypto/tls testdata
// example-key.pem) and an Ed25519 key from a fixed seed; Go's tests are
// Copyright 2009 The Go Authors, BSD-style license.

type ctRow struct {
	name              string
	serial            []byte
	subj              pkix.Name
	nb, na            int64
	dns, emails, uris []string
	ips               []string
	isCA              bool
	mpl               int // -1: none
	ku                []string
	eku               []string // variant names, or an OID for Other
	skid              []byte
	extra             []pkix.Extension
	key               string // "ed", "rsa", "ec"
	parent            bool   // issued by the Ed25519 root
}

var ctKU = []string{"DigitalSignature", "ContentCommitment", "KeyEncipherment", "DataEncipherment", "KeyAgreement", "CertSign", "CrlSign", "EncipherOnly", "DecipherOnly"}

var ctEKU = map[string]x509.ExtKeyUsage{
	"Any": x509.ExtKeyUsageAny, "ServerAuth": x509.ExtKeyUsageServerAuth, "ClientAuth": x509.ExtKeyUsageClientAuth,
	"CodeSigning": x509.ExtKeyUsageCodeSigning, "EmailProtection": x509.ExtKeyUsageEmailProtection,
	"TimeStamping": x509.ExtKeyUsageTimeStamping, "OcspSigning": x509.ExtKeyUsageOCSPSigning,
}

func ctOID(s string) asn1.ObjectIdentifier {
	var o asn1.ObjectIdentifier
	for _, p := range strings.Split(s, ".") {
		n, _ := strconv.Atoi(p)
		o = append(o, n)
	}
	return o
}

// ctKeys are the three keys, as Go keys and as PKCS #8 DER.
func ctKeys() (map[string]crypto.Signer, map[string][]byte) {
	keys := map[string]crypto.Signer{}
	ders := map[string][]byte{}
	seed := make([]byte, 32)
	for i := range seed {
		seed[i] = byte(i * 7)
	}
	keys["ed"] = ed25519.NewKeyFromSeed(seed)
	// x509_test.go's pemPrivateKey (a 1024 bit RSA key; testingKey's
	// "TESTING KEY" spelled out)
	path := filepath.Join(runtime.GOROOT(), "src", "crypto", "x509", "x509_test.go")
	f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		panic(err)
	}
	ast.Inspect(f, func(n ast.Node) bool {
		v, ok := n.(*ast.ValueSpec)
		if !ok || len(v.Names) != 1 || v.Names[0].Name != "pemPrivateKey" {
			return true
		}
		lit := v.Values[0].(*ast.CallExpr).Args[0].(*ast.BasicLit)
		s, _ := strconv.Unquote(lit.Value)
		b, _ := pem.Decode([]byte(strings.ReplaceAll(s, "TESTING KEY", "PRIVATE KEY")))
		k, err := x509.ParsePKCS1PrivateKey(b.Bytes)
		if err != nil {
			panic(err)
		}
		keys["rsa"] = k
		return false
	})
	eb, err := os.ReadFile(filepath.Join(runtime.GOROOT(), "src", "crypto", "tls", "testdata", "example-key.pem"))
	if err != nil {
		panic(err)
	}
	b, _ := pem.Decode(eb)
	ek, err := x509.ParseECPrivateKey(b.Bytes)
	if err != nil {
		panic(err)
	}
	keys["ec"] = ek
	for n, k := range keys {
		d, err := x509.MarshalPKCS8PrivateKey(k)
		if err != nil {
			panic(err)
		}
		ders[n] = d
	}
	return keys, ders
}

func ctTemplate(r ctRow) *x509.Certificate {
	t := &x509.Certificate{
		SerialNumber: new(big.Int).SetBytes(r.serial), Subject: r.subj,
		NotBefore: time.Unix(r.nb, 0), NotAfter: time.Unix(r.na, 0),
		DNSNames: r.dns, EmailAddresses: r.emails, IsCA: r.isCA, SubjectKeyId: r.skid,
		ExtraExtensions: r.extra, MaxPathLen: -1,
	}
	for _, ip := range r.ips {
		t.IPAddresses = append(t.IPAddresses, net.ParseIP(ip))
	}
	for _, u := range r.uris {
		p, _ := url.Parse(u)
		t.URIs = append(t.URIs, p)
	}
	if r.isCA || r.mpl >= 0 {
		t.BasicConstraintsValid = true
	}
	if r.mpl >= 0 {
		t.MaxPathLen = r.mpl
		t.MaxPathLenZero = r.mpl == 0
	}
	for _, k := range r.ku {
		for i, n := range ctKU {
			if n == k {
				t.KeyUsage |= x509.KeyUsage(1 << i)
			}
		}
	}
	for _, e := range r.eku {
		if u, ok := ctEKU[e]; ok {
			t.ExtKeyUsage = append(t.ExtKeyUsage, u)
		} else {
			t.UnknownExtKeyUsage = append(t.UnknownExtKeyUsage, ctOID(e))
		}
	}
	return t
}

func ctStrs(xs []string) string {
	var q []string
	for _, x := range xs {
		q = append(q, xq(x))
	}
	return "[" + strings.Join(q, ", ") + "]"
}

func ctName(n pkix.Name) string {
	return fmt.Sprintf("x509.Name{country: %s, organization: %s, organizational_unit: %s, locality: %s, province: %s, street_address: %s, postal_code: %s, serial_number: %s, common_name: %s}",
		ctStrs(n.Country), ctStrs(n.Organization), ctStrs(n.OrganizationalUnit), ctStrs(n.Locality), ctStrs(n.Province),
		ctStrs(n.StreetAddress), ctStrs(n.PostalCode), xq(n.SerialNumber), xq(n.CommonName))
}

func ctExts(es []pkix.Extension) string {
	var q []string
	for _, e := range es {
		q = append(q, fmt.Sprintf("x509.Extension{oid: %s, critical: %v, value: %s}", xq(e.Id.String()), e.Critical, xbytes(e.Value)))
	}
	return "[" + strings.Join(q, ", ") + "]"
}

func ctIPs(ips []string) string {
	var q []string
	for _, ip := range ips {
		q = append(q, fmt.Sprintf("netip.parse_addr(%s).ok() ?? fault(\"ip\")", xq(ip)))
	}
	return "[" + strings.Join(q, ", ") + "]"
}

// ctXo is the row's Template as an Xo expression.
func ctXo(r ctRow) string {
	var ku, eku []string
	for _, k := range r.ku {
		ku = append(ku, "x509.KeyUsage."+k)
	}
	for _, e := range r.eku {
		if _, ok := ctEKU[e]; ok {
			eku = append(eku, "x509.ExtKeyUsage."+e)
		} else {
			eku = append(eku, "x509.ExtKeyUsage.Other(oid: "+xq(e)+")")
		}
	}
	mpl := "None"
	if r.mpl >= 0 {
		mpl = fmt.Sprintf("Some(%d)", r.mpl)
	}
	return fmt.Sprintf("x509.Template{serial: %s, subject: %s, not_before: Time.from_unix(%d), not_after: Time.from_unix(%d), dns_names: %s, ip_addresses: %s, email_addresses: %s, uris: %s, is_ca: %v, max_path_len: %s, key_usage: [%s], ext_key_usage: [%s], subject_key_id: %s, extra_extensions: %s}",
		xbytes(r.serial), ctName(r.subj), r.nb, r.na, ctStrs(r.dns), ctIPs(r.ips), ctStrs(r.emails), ctStrs(r.uris), r.isCA, mpl,
		strings.Join(ku, ", "), strings.Join(eku, ", "), xbytes(r.skid), ctExts(r.extra))
}

func ctRows() []ctRow {
	jan2025, jan2026 := int64(1735689600), int64(1767225600)
	base := ctRow{serial: []byte{1}, nb: jan2025, na: jan2026, mpl: -1}
	with := func(f func(*ctRow)) ctRow { r := base; f(&r); return r }
	var rows []ctRow
	for _, key := range []string{"ed", "rsa", "ec"} {
		for _, parent := range []bool{false, true} {
			k, p := key, parent
			pre := k
			if p {
				pre += " from root"
			}
			rows = append(rows,
				with(func(r *ctRow) {
					r.name = pre + " minimal"
					r.key = k
					r.parent = p
					r.subj = pkix.Name{CommonName: "min"}
				}),
				with(func(r *ctRow) {
					r.name = pre + " server leaf"
					r.key, r.parent = k, p
					r.serial = []byte{0x80, 0, 1}
					r.subj = pkix.Name{CommonName: "www.example.com", Organization: []string{"Example Org", "Second"}, Country: []string{"US"}}
					r.dns = []string{"www.example.com", "example.com"}
					r.ips = []string{"127.0.0.1", "::1", "2001:db8::5"}
					r.ku = []string{"DigitalSignature", "KeyEncipherment"}
					r.eku = []string{"ServerAuth", "ClientAuth"}
				}),
			)
		}
	}
	rows = append(rows,
		with(func(r *ctRow) {
			r.name = "every name attribute and escape"
			r.key = "ed"
			r.subj = pkix.Name{Country: []string{"GB"}, Province: []string{"Scotland"}, Locality: []string{"Edinburgh", "Leith"},
				StreetAddress: []string{"1 High St"}, PostalCode: []string{"EH1 1AA"}, Organization: []string{" a,b+c\"d\\e<f>g;h "},
				OrganizationalUnit: []string{"#unit", "café"}, CommonName: "Zoë & co *", SerialNumber: "SN-42"}
		}),
		with(func(r *ctRow) { r.name = "empty subject, critical SAN"; r.key = "ed"; r.dns = []string{"only.example"} }),
		with(func(r *ctRow) {
			r.name = "emails and URIs"
			r.key = "rsa"
			r.subj = pkix.Name{CommonName: "mail"}
			r.emails = []string{"a@example.com", "b.c@example.org"}
			r.uris = []string{"https://example.com/x?y=1", "spiffe://trust/ns/a"}
		}),
		with(func(r *ctRow) {
			r.name = "CA, path length 0, own key id"
			r.key = "ed"
			r.subj = pkix.Name{CommonName: "Int CA"}
			r.isCA, r.mpl = true, 0
			r.ku = []string{"CertSign", "CrlSign"}
			r.skid = []byte{1, 2, 3, 4}
		}),
		with(func(r *ctRow) {
			r.name = "CA, path length 2, generated key id"
			r.key = "rsa"
			r.parent = true
			r.subj = pkix.Name{CommonName: "Sub CA"}
			r.isCA, r.mpl = true, 2
			r.ku = []string{"CertSign"}
		}),
		with(func(r *ctRow) {
			r.name = "every key usage and an unknown extended one"
			r.key = "ed"
			r.subj = pkix.Name{CommonName: "usages"}
			r.ku = ctKU
			r.eku = []string{"Any", "CodeSigning", "EmailProtection", "TimeStamping", "OcspSigning", "1.3.6.1.4.1.311.10.3.3"}
		}),
		with(func(r *ctRow) {
			r.name = "extra extensions, one replacing the SAN"
			r.key = "ed"
			r.subj = pkix.Name{CommonName: "extra"}
			r.dns = []string{"ignored.example"}
			san, _ := asn1.Marshal([]asn1.RawValue{{Tag: 2, Class: 2, Bytes: []byte("replaced.example")}})
			r.extra = []pkix.Extension{{Id: ctOID("2.5.29.17"), Value: san}, {Id: ctOID("1.2.3.4.5"), Critical: false, Value: []byte{5, 0}}}
		}),
		with(func(r *ctRow) {
			r.name = "times before 1950 and from 2050 (GeneralizedTime), long serial"
			r.key = "rsa"
			r.subj = pkix.Name{CommonName: "time"}
			r.nb, r.na = -1000000000, 2600000000
			r.serial = []byte{0x7f, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19}
		}),
	)
	return rows
}

// ctGoInputs are x509_test.go's requests and CRLs: csrBase64Array,
// dupExtCSR and dupAttCSR, derCRLBase64 and pemCRLBase64.
func ctGoInputs() (csrs, crls [][]byte) {
	path := filepath.Join(runtime.GOROOT(), "src", "crypto", "x509", "x509_test.go")
	f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		panic(err)
	}
	str := func(e ast.Expr) string {
		lit, ok := e.(*ast.BasicLit)
		if !ok {
			return ""
		}
		v, _ := strconv.Unquote(lit.Value)
		return v
	}
	pemDER := func(text []byte) []byte {
		b, _ := pem.Decode(text)
		if b == nil {
			panic("x509create: no PEM block")
		}
		return b.Bytes
	}
	ast.Inspect(f, func(n ast.Node) bool {
		v, ok := n.(*ast.ValueSpec)
		if !ok || len(v.Names) != 1 || len(v.Values) != 1 {
			return true
		}
		switch v.Names[0].Name {
		case "csrBase64Array":
			for _, e := range v.Values[0].(*ast.CompositeLit).Elts {
				csrs = append(csrs, ctMust(base64.StdEncoding.DecodeString(str(e))))
			}
		case "dupExtCSR", "dupAttCSR":
			csrs = append(csrs, pemDER([]byte(str(v.Values[0]))))
		case "derCRLBase64":
			crls = append(crls, ctMust(base64.StdEncoding.DecodeString(str(v.Values[0]))))
		case "pemCRLBase64":
			crls = append(crls, pemDER(ctMust(base64.StdEncoding.DecodeString(str(v.Values[0])))))
		}
		return true
	})
	if len(csrs) < 3 || len(crls) != 2 {
		panic(fmt.Sprintf("x509create: x509_test.go gave %d requests and %d CRLs", len(csrs), len(crls)))
	}
	return csrs, crls
}

func ctMust(b []byte, err error) []byte {
	if err != nil {
		panic(err)
	}
	return b
}

func x509CreateCorpus() string {
	var o strings.Builder
	o.WriteString(header("certificates, requests and CRLs made by Go's crypto/x509 against std/x509's create and parse functions (Go's test keys are Copyright 2009 The Go Authors, BSD-style license)"))
	o.WriteString("\nuse std/crypto\nuse std/netip\nuse std/x509\n")
	keys, ders := ctKeys()
	fmt.Fprintf(&o, "\nconst KEY_ED: Bytes = %s\nconst KEY_RSA: Bytes = %s\nconst KEY_EC: Bytes = %s\n", xbytes(ders["ed"]), xbytes(ders["rsa"]), xbytes(ders["ec"]))

	// the root every "from root" row is issued by
	rootT := &x509.Certificate{SerialNumber: big.NewInt(7), Subject: pkix.Name{CommonName: "Xo Test Root", Organization: []string{"Xo"}},
		NotBefore: time.Unix(1704067200, 0), NotAfter: time.Unix(1893456000, 0), IsCA: true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign, MaxPathLen: -1}
	rootDER := ctMust(x509.CreateCertificate(rand.Reader, rootT, rootT, keys["ed"].Public(), keys["ed"]))
	root, _ := x509.ParseCertificate(rootDER)
	fmt.Fprintf(&o, "const ROOT: Bytes = %s\n", xbytes(rootDER))

	o.WriteString("\n/// (name, key, from the root, Go's certificate)\nfn rows() -> List[(Str, x509.Template, Str, Bool, Bytes)] {\n  [\n")
	for _, r := range ctRows() {
		t := ctTemplate(r)
		parent, signer := t, keys[r.key]
		if r.parent {
			parent, signer = root, keys["ed"]
		}
		der := ctMust(x509.CreateCertificate(rand.Reader, t, parent, keys[r.key].Public(), signer))
		fmt.Fprintf(&o, "    (%s, %s, %s, %v, %s),\n", xq(r.name), ctXo(r), xq(r.key), r.parent, xbytes(der))
	}
	o.WriteString("  ]\n}\n")

	// requests: Go's, for the parse rows and the byte equality of Xo's
	o.WriteString("\n/// (name, key, request template, Go's request)\nfn requests() -> List[(Str, Str, x509.RequestTemplate, Bytes)] {\n  [\n")
	var csrDERs [][]byte
	for _, r := range ctRows() {
		if r.isCA || r.parent {
			continue
		}
		t := ctTemplate(r)
		q := &x509.CertificateRequest{Subject: t.Subject, DNSNames: t.DNSNames, EmailAddresses: t.EmailAddresses, IPAddresses: t.IPAddresses, URIs: t.URIs, ExtraExtensions: t.ExtraExtensions}
		der := ctMust(x509.CreateCertificateRequest(rand.Reader, q, keys[r.key]))
		csrDERs = append(csrDERs, der)
		fmt.Fprintf(&o, "    (%s, %s, x509.RequestTemplate{subject: %s, dns_names: %s, ip_addresses: %s, email_addresses: %s, uris: %s, extra_extensions: %s}, %s),\n",
			xq(r.name), xq(r.key), ctName(r.subj), ctStrs(r.dns), ctIPs(r.ips), ctStrs(r.emails), ctStrs(r.uris), ctExts(r.extra), xbytes(der))
	}
	o.WriteString("  ]\n}\n")

	// parse rows: x509_test.go's requests, these requests and mutated
	// copies, with Go's answer
	goCSRs, goCRLs := ctGoInputs()
	mr := mrand.New(mrand.NewSource(10))
	o.WriteString("\n/// (DER, Go's summary or error)\nconst REQUEST_ROWS: List[(Bytes, Str)] = [\n")
	for _, der := range goCSRs {
		fmt.Fprintf(&o, "  (%s, %s),\n", xbytes(der), xq(ctCSRWant(der)))
	}
	for i, der := range csrDERs {
		fmt.Fprintf(&o, "  (%s, %s),\n", xbytes(der), xq(ctCSRWant(der)))
		if i < 3 {
			for _, n := range []int{0, 5, len(der) / 2, len(der) - 1} {
				fmt.Fprintf(&o, "  (%s, %s),\n", xbytes(der[:n]), xq(ctCSRWant(der[:n])))
			}
			for j := 0; j < 25; j++ {
				m := append([]byte(nil), der...)
				m[mr.Intn(len(m))] ^= byte(1 << mr.Intn(8))
				fmt.Fprintf(&o, "  (%s, %s),\n", xbytes(m), xq(ctCSRWant(m)))
			}
		}
	}
	o.WriteString("]\n")

	// CRLs signed by the root
	o.WriteString("\n/// (DER, Go's summary or error)\nconst CRL_ROWS: List[(Bytes, Str)] = [\n")
	for _, der := range goCRLs {
		fmt.Fprintf(&o, "  (%s, %s),\n", xbytes(der), xq(ctCRLWant(der)))
	}
	for i, tmpl := range []*x509.RevocationList{
		{Number: big.NewInt(1), ThisUpdate: time.Unix(1735689600, 0), NextUpdate: time.Unix(1735689601, 0)},
		{Number: big.NewInt(300), ThisUpdate: time.Unix(1735689600, 0), NextUpdate: time.Unix(1736294400, 0),
			RevokedCertificateEntries: []x509.RevocationListEntry{
				{SerialNumber: big.NewInt(5), RevocationTime: time.Unix(1735000000, 0)},
				{SerialNumber: new(big.Int).SetBytes([]byte{0x80, 1, 2, 3}), RevocationTime: time.Unix(1735100000, 0), ReasonCode: 1},
				{SerialNumber: big.NewInt(9), RevocationTime: time.Unix(2600000000, 0), ReasonCode: 4, ExtraExtensions: []pkix.Extension{{Id: ctOID("2.5.29.24"), Value: []byte{0x18, 0x0f, 0x32, 0x30, 0x32, 0x35, 0x30, 0x31, 0x30, 0x31, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x5a}}}},
			}},
		{Number: new(big.Int).SetBytes([]byte{1, 2, 3, 4, 5, 6, 7, 8, 9}), ThisUpdate: time.Unix(-1000000000, 0), NextUpdate: time.Unix(2600000000, 0),
			ExtraExtensions: []pkix.Extension{{Id: ctOID("1.2.3.4"), Value: []byte{5, 0}}}},
	} {
		der := ctMust(x509.CreateRevocationList(rand.Reader, tmpl, root, keys["ed"]))
		fmt.Fprintf(&o, "  (%s, %s),\n", xbytes(der), xq(ctCRLWant(der)))
		if i == 1 {
			for _, n := range []int{0, 3, len(der) / 2, len(der) - 1} {
				fmt.Fprintf(&o, "  (%s, %s),\n", xbytes(der[:n]), xq(ctCRLWant(der[:n])))
			}
			for j := 0; j < 30; j++ {
				m := append([]byte(nil), der...)
				m[mr.Intn(len(m))] ^= byte(1 << mr.Intn(8))
				fmt.Fprintf(&o, "  (%s, %s),\n", xbytes(m), xq(ctCRLWant(m)))
			}
		}
	}
	o.WriteString("]\n")

	o.WriteString(`
fn shown(bad: List[Str]) -> List[Str] {
  bad[0..(if bad.len() < 20 { bad.len() } else { 20 })]
}

fn key(name: Str) -> crypto.PrivateKey {
  let der = if name == "ed" { KEY_ED } else if name == "rsa" { KEY_RSA } else { KEY_EC }
  crypto.parse_private_key(der).ok() ?? fault("key")
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

fn exts(es: List[x509.Extension]) -> Str { es.map(fn(e) { "${e.oid}/${e.critical}/${e.value.to_hex()}" }).join(",") }

/// The fields a certificate made from a template has (no serial for
/// ECDSA rows' comparison is needed: the serial is the template's).
fn summary(c: x509.Certificate) -> Str {
  let ips = c.ip_addresses.map(fn(a) { a.display() }).join(",")
  let mpl = match c.max_path_len {
    Some(n) => "${n}"
    None => "none"
  }
  "v=${c.version} serial=${c.serial.to_hex()} sig=${c.signature_algorithm} subj=${c.subject.display()} iss=${c.issuer.display()} nb=${c.not_before.unix()} na=${c.not_after.unix()} dns=${c.dns_names.join(",")} ip=${ips} email=${c.email_addresses.join(",")} uri=${c.uris.join(",")} ca=${c.is_ca} mpl=${mpl} ku=${c.key_usage.map(fn(u) { "${ku_index(u)}" }).join(",")} eku=${c.ext_key_usage.map(fn(u) { eku_oid(u) }).join(",")} ski=${c.subject_key_id.to_hex()} aki=${c.authority_key_id.to_hex()} ext=${exts(c.extensions)}"
}

test "create_certificate makes Go's certificate: equal bytes for Ed25519 and RSA, equal fields for ECDSA" {
  let root = x509.parse(ROOT).ok() ?? fault("root")
  var bad: List[Str] = []
  for (name, t, kname, from_root, want) in rows() {
    let k = key(kname)
    let parent = if from_root { Some(root) } else { None }
    let signer = if from_root { key("ed") } else { k }
    match x509.create_certificate(os.rand, t, k.public_key(), parent, signer) {
      Ok(c) => {
        let go = x509.parse(want).ok() ?? fault("go's")
        // an ECDSA signature has a random nonce: a self-signed ECDSA row
        // compares fields, every other row bytes
        if kname == "ec" && !from_root {
          if summary(c) != summary(go) {
            bad.push("${name}:\n  got  ${summary(c)}\n  want ${summary(go)}")
          }
          match c.check_signature_from(if from_root { root } else { c }) {
            Ok(_) => ()
            Err(e) => if from_root || c.is_ca { bad.push("${name}: signature: ${e.display()}") }
          }
        } else if c.raw != want {
          bad.push("${name}: other bytes\n  got  ${summary(c)}\n  want ${summary(go)}")
        }
      }
      Err(e) => bad.push("${name}: ${e.display()}")
    }
  }
  expect(shown(bad)) == []
}

test "create_certificate's errors are Go's" {
  let k = key("ed")
  let t = x509.Template{subject: x509.Name{common_name: "x"}, not_before: Time.from_unix(0), not_after: Time.from_unix(1)}
  let path = x509.create_certificate(os.rand, x509.Template{subject: t.subject, not_before: t.not_before, not_after: t.not_after, max_path_len: Some(1)}, k.public_key(), None, k)
  expect(path.err().map(fn(e) { e.display() })) == Some("x509: only CAs are allowed to specify MaxPathLen")
  let ia5 = x509.create_certificate(os.rand, x509.Template{subject: t.subject, not_before: t.not_before, not_after: t.not_after, dns_names: ["bü.example"]}, k.public_key(), None, k)
  expect(ia5.err().map(fn(e) { e.display() })) == Some("x509: \"bü.example\" cannot be encoded as an IA5String")
  let root = x509.parse(ROOT).ok() ?? fault("root")
  let other = x509.create_certificate(os.rand, t, k.public_key(), Some(root), key("rsa"))
  expect(other.err().map(fn(e) { e.display() })) == Some("x509: provided PrivateKey doesn't match parent's PublicKey")
  let serial = x509.create_certificate(os.rand, t, k.public_key(), None, k).ok() ?? fault("create")
  expect(serial.serial.len() > 0 && serial.serial.len() <= 20) == true
  expect(x509.parse_pem(serial.pem()).ok().map(fn(cs) { cs[0].raw })) == Some(serial.raw)
}

test "create_request makes Go's request, and parse_request reads Go's and its mutations as Go does" {
  var bad: List[Str] = []
  for (name, kname, t, want) in requests() {
    match x509.create_request(os.rand, t, key(kname)) {
      Ok(q) => {
        if kname != "ec" && q.raw != want {
          bad.push("${name}: other bytes")
        }
        match q.check_signature() {
          Ok(_) => ()
          Err(e) => bad.push("${name}: check_signature: ${e.display()}")
        }
      }
      Err(e) => bad.push("${name}: ${e.display()}")
    }
  }
  for (der, want) in REQUEST_ROWS {
    let got = match x509.parse_request(der) {
      Ok(q) => {
        let sig = match q.check_signature() {
          Ok(_) => "ok"
          Err(e) => e.display()
        }
        "v=${q.version} sig=${q.signature_algorithm} subj=${q.subject.display()} dns=${q.dns_names.join(",")} ip=${q.ip_addresses.map(fn(a) { a.display() }).join(",")} email=${q.email_addresses.join(",")} uri=${q.uris.join(",")} ext=${exts(q.extensions)} key=${q.public_key.is_some()} check=${sig}"
      }
      Err(e) => "error: ${e.display()}"
    }
    if got != want {
      bad.push("${der.len()} bytes:\n  got  ${got}\n  want ${want}")
    }
  }
  expect(shown(bad)) == []
}

test "parse_revocation_list reads Go's CRLs and their mutations as Go does" {
  let root = x509.parse(ROOT).ok() ?? fault("root")
  var bad: List[Str] = []
  for (der, want) in CRL_ROWS {
    let got = match x509.parse_revocation_list(der) {
      Ok(l) => {
        let next = match l.next_update {
          Some(t) => "${t.unix()}"
          None => "none"
        }
        let sig = match l.check_signature_from(root) {
          Ok(_) => "ok"
          Err(e) => e.display()
        }
        "sig=${l.signature_algorithm} iss=${l.issuer.display()} num=${l.number.to_hex()} this=${l.this_update.unix()} next=${next} revoked=${l.revoked.map(fn(r) { "${r.serial.to_hex()}/${r.revoked_at.unix()}/${r.reason}/${exts(r.extensions)}" }).join(";")} aki=${l.authority_key_id.to_hex()} ext=${exts(l.extensions)} check=${sig}"
      }
      Err(e) => "error: ${e.display()}"
    }
    if got != want {
      bad.push("${der.len()} bytes:\n  got  ${got}\n  want ${want}")
    }
  }
  expect(shown(bad)) == []
}

test "self_signed makes a development certificate for its hosts and a key that parses" {
  let (cert, keypem) = x509.self_signed(os.rand, ["localhost", "127.0.0.1", "::1"], Time.from_unix(1767225600)).ok() ?? fault("self_signed")
  let c = (x509.parse_pem(cert).ok() ?? fault("parse"))[0]
  expect(c.dns_names) == ["localhost"]
  expect(c.ip_addresses.map(fn(a) { a.display() })) == ["127.0.0.1", "::1"]
  expect(c.subject.organization) == ["Xo Development"]
  expect(c.not_after.unix() - c.not_before.unix()) == 30 * 86400
  expect(c.public_key.map(fn(k) { k.alg() == crypto.KeyAlg.EcdsaP256 })) == Some(true)
  let blocks = crypto.pem_decode(keypem.to_str() ?? "")
  expect(blocks.map(fn(b) { b.kind })) == ["PRIVATE KEY"]
  let k = crypto.parse_private_key(blocks[0].bytes).ok() ?? fault("key")
  expect(c.public_key) == Some(k.public_key())
  expect(c.verify(x509.VerifyOptions{roots: [c], dns_name: "localhost", at: Time.from_unix(1767225600 + 86400)}).is_ok()) == true
}

test "marshal_private_key is Go's PKCS #8 and parses back" {
  for (name, der) in [("ed", KEY_ED), ("rsa", KEY_RSA), ("ec", KEY_EC)] {
    let k = key(name)
    expect(crypto.marshal_private_key(k)) == der
    expect(crypto.parse_private_key(crypto.marshal_private_key(k)).ok().map(fn(x) { x.public_key() })) == Some(k.public_key())
  }
}
`)
	return o.String()
}

func ctExtText(es []pkix.Extension) string {
	var q []string
	for _, e := range es {
		q = append(q, fmt.Sprintf("%s/%v/%x", e.Id, e.Critical, e.Value))
	}
	return strings.Join(q, ",")
}

func ctCSRWant(der []byte) string {
	q, err := x509.ParseCertificateRequest(der)
	if err != nil {
		msg := err.Error()
		if strings.HasPrefix(msg, "asn1: ") {
			msg = "x509: malformed certificate request"
		}
		return "error: " + msg
	}
	var ips, uris []string
	for _, ip := range q.IPAddresses {
		ips = append(ips, ip.String())
	}
	for _, u := range q.URIs {
		uris = append(uris, u.String())
	}
	check := "ok"
	if err := q.CheckSignature(); err != nil {
		check = err.Error()
	}
	return fmt.Sprintf("v=%d sig=%s subj=%s dns=%s ip=%s email=%s uri=%s ext=%s key=%v check=%s", q.Version, q.SignatureAlgorithm, q.Subject,
		strings.Join(q.DNSNames, ","), strings.Join(ips, ","), strings.Join(q.EmailAddresses, ","), strings.Join(uris, ","),
		ctExtText(q.Extensions), q.PublicKey != nil, check)
}

func ctCRLWant(der []byte) string {
	l, err := x509.ParseRevocationList(der)
	if err != nil {
		return "error: " + err.Error()
	}
	next := "none"
	if !l.NextUpdate.IsZero() {
		next = strconv.FormatInt(l.NextUpdate.Unix(), 10)
	}
	var revoked []string
	for _, e := range l.RevokedCertificateEntries {
		revoked = append(revoked, fmt.Sprintf("%x/%d/%d/%s", e.SerialNumber.Bytes(), e.RevocationTime.Unix(), e.ReasonCode, ctExtText(e.Extensions)))
	}
	num := []byte{}
	if l.Number != nil {
		num = l.Number.Bytes()
	}
	check := "ok"
	return fmt.Sprintf("sig=%s iss=%s num=%x this=%d next=%s revoked=%s aki=%x ext=%s check=%s", l.SignatureAlgorithm, l.Issuer, num,
		l.ThisUpdate.Unix(), next, strings.Join(revoked, ";"), l.AuthorityKeyId, ctExtText(l.Extensions), ctCRLCheck(l, check))
}

// ctCRLCheck is Go's CheckSignatureFrom of l by the corpus root, made
// again here from the same Ed25519 seed.
func ctCRLCheck(l *x509.RevocationList, ok string) string {
	keys, _ := ctKeys()
	rootT := &x509.Certificate{SerialNumber: big.NewInt(7), Subject: pkix.Name{CommonName: "Xo Test Root", Organization: []string{"Xo"}},
		NotBefore: time.Unix(1704067200, 0), NotAfter: time.Unix(1893456000, 0), IsCA: true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign, MaxPathLen: -1}
	der := ctMust(x509.CreateCertificate(rand.Reader, rootT, rootT, keys["ed"].Public(), keys["ed"]))
	root, _ := x509.ParseCertificate(der)
	if err := l.CheckSignatureFrom(root); err != nil {
		return err.Error()
	}
	return ok
}
