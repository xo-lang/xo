package main

import (
	"crypto"
	"crypto/ed25519"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
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
	// piece 3's template fields (TestCreateSelfSignedCertificate's)
	bcv                  bool
	ocsp, issuers, crldp []string
	ncCrit               bool
	permDNS, exclDNS     []string
	permIP, exclIP       []string // CIDR text
	permEmail, exclEmail []string
	permURI, exclURI     []string
	policies             []string // dotted, arcs of any size
	sigAlg               x509.SignatureAlgorithm
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
	if r.isCA || r.mpl >= 0 || r.bcv {
		t.BasicConstraintsValid = true
	}
	t.OCSPServer, t.IssuingCertificateURL, t.CRLDistributionPoints = r.ocsp, r.issuers, r.crldp
	t.PermittedDNSDomainsCritical = r.ncCrit
	t.PermittedDNSDomains, t.ExcludedDNSDomains = r.permDNS, r.exclDNS
	t.PermittedEmailAddresses, t.ExcludedEmailAddresses = r.permEmail, r.exclEmail
	t.PermittedURIDomains, t.ExcludedURIDomains = r.permURI, r.exclURI
	for _, c := range r.permIP {
		_, n, _ := net.ParseCIDR(c)
		t.PermittedIPRanges = append(t.PermittedIPRanges, n)
	}
	for _, c := range r.exclIP {
		_, n, _ := net.ParseCIDR(c)
		t.ExcludedIPRanges = append(t.ExcludedIPRanges, n)
	}
	for _, o := range r.policies {
		oid, err := x509.ParseOID(o)
		if err != nil {
			panic(err)
		}
		t.Policies = append(t.Policies, oid)
	}
	t.SignatureAlgorithm = r.sigAlg
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

func ctPrefixes(cs []string) string {
	var q []string
	for _, c := range cs {
		q = append(q, fmt.Sprintf("netip.parse_prefix(%s).ok() ?? fault(\"prefix\")", xq(c)))
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
	// piece 3's fields, only when set (the earlier rows read as before)
	var more []string
	add := func(name, v string, set bool) {
		if set {
			more = append(more, name+": "+v)
		}
	}
	add("basic_constraints_valid", "true", r.bcv)
	add("ocsp_servers", ctStrs(r.ocsp), len(r.ocsp) > 0)
	add("issuing_certificate_urls", ctStrs(r.issuers), len(r.issuers) > 0)
	add("crl_distribution_points", ctStrs(r.crldp), len(r.crldp) > 0)
	add("permitted_dns_domains_critical", "true", r.ncCrit)
	add("permitted_dns_domains", ctStrs(r.permDNS), len(r.permDNS) > 0)
	add("excluded_dns_domains", ctStrs(r.exclDNS), len(r.exclDNS) > 0)
	add("permitted_ip_ranges", ctPrefixes(r.permIP), len(r.permIP) > 0)
	add("excluded_ip_ranges", ctPrefixes(r.exclIP), len(r.exclIP) > 0)
	add("permitted_email_addresses", ctStrs(r.permEmail), len(r.permEmail) > 0)
	add("excluded_email_addresses", ctStrs(r.exclEmail), len(r.exclEmail) > 0)
	add("permitted_uri_domains", ctStrs(r.permURI), len(r.permURI) > 0)
	add("excluded_uri_domains", ctStrs(r.exclURI), len(r.exclURI) > 0)
	add("policies", ctStrs(r.policies), len(r.policies) > 0)
	add("signature_algorithm", xq(r.sigAlg.String()), r.sigAlg != 0)
	tail := ""
	if len(more) > 0 {
		tail = ", " + strings.Join(more, ", ")
	}
	return fmt.Sprintf("x509.Template{serial: %s, subject: %s, not_before: Time.from_unix(%d), not_after: Time.from_unix(%d), dns_names: %s, ip_addresses: %s, email_addresses: %s, uris: %s, is_ca: %v, max_path_len: %s, key_usage: [%s], ext_key_usage: [%s], subject_key_id: %s, extra_extensions: %s%s}",
		xbytes(r.serial), ctName(r.subj), r.nb, r.na, ctStrs(r.dns), ctIPs(r.ips), ctStrs(r.emails), ctStrs(r.uris), r.isCA, mpl,
		strings.Join(ku, ", "), strings.Join(eku, ", "), xbytes(r.skid), ctExts(r.extra), tail)
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
	// TestCreateSelfSignedCertificate's template (less its ExtraNames,
	// which Name does not have), for each of its key and signature
	// algorithm cases where the public key is the signer's
	every := func(r *ctRow) {
		r.subj = pkix.Name{CommonName: "test.example.com", Organization: []string{"Σ Acme Co"}, Country: []string{"US"}}
		r.nb, r.na = 1000, 100000
		r.skid = []byte{1, 2, 3, 4}
		r.ku = []string{"CertSign"}
		r.eku = []string{"ClientAuth", "ServerAuth", "1.2.3", "2.59.1"}
		r.bcv, r.isCA = true, true
		r.ocsp = []string{"http://ocsp.example.com"}
		r.issuers = []string{"http://crt.example.com/ca1.crt"}
		r.dns = []string{"test.example.com"}
		r.emails = []string{"gopher@golang.org"}
		r.ips = []string{"127.0.0.1", "2001:4860:0:2001::68"}
		r.uris = []string{"https://foo.com/wibble#foo"}
		r.policies = []string{"1.2.3.4294967295.18446744073709551615"}
		r.permDNS = []string{".example.com", "example.com"}
		r.exclDNS = []string{"bar.example.com"}
		r.permIP = []string{"192.168.1.1/16", "1.2.3.4/8"}
		r.exclIP = []string{"2001:db8::/48"}
		r.permEmail = []string{"foo@example.com"}
		r.exclEmail = []string{".example.com", "example.com"}
		r.permURI = []string{".bar.com", "bar.com"}
		r.exclURI = []string{".bar2.com", "bar2.com"}
		r.crldp = []string{"http://crl1.example.com/ca1.crl", "http://crl2.example.com/ca1.crl"}
		r.extra = []pkix.Extension{{Id: ctOID("1.2.3.4"), Value: []byte("extra extension")}, {Id: ctOID("2.5.29.14"), Value: []byte{0x04, 0x04, 4, 3, 2, 1}}}
	}
	for _, c := range []struct {
		name, key string
		alg       x509.SignatureAlgorithm
	}{
		{"RSA/RSA", "rsa", x509.SHA384WithRSA},
		{"RSA, SHA-1 (Go still signs it)", "rsa", x509.SHA1WithRSA},
		{"ECDSA/ECDSA", "ec", x509.ECDSAWithSHA256},
		{"ECDSA, SHA-384", "ec", x509.ECDSAWithSHA384},
		{"RSAPSS/RSAPSS", "rsa", x509.SHA256WithRSAPSS},
		{"RSAPSS, SHA-384", "rsa", x509.SHA384WithRSAPSS},
		{"Ed25519", "ed", x509.PureEd25519},
	} {
		c := c
		rows = append(rows, with(func(r *ctRow) {
			every(r)
			r.name = "every template field (TestCreateSelfSignedCertificate), " + c.name
			r.key, r.sigAlg = c.key, c.alg
		}))
	}
	rows = append(rows,
		with(func(r *ctRow) {
			every(r)
			r.name = "every template field, issued by the root"
			r.key, r.parent = "rsa", true
			r.isCA = false
			r.mpl = -1
		}),
		with(func(r *ctRow) {
			r.name = "basic constraints valid on a leaf"
			r.key = "ed"
			r.subj = pkix.Name{CommonName: "leaf"}
			r.bcv = true
		}),
		with(func(r *ctRow) {
			r.name = "critical name constraints, excluded only, IPv4 and IPv6 ranges"
			r.key = "ed"
			r.subj = pkix.Name{CommonName: "nc"}
			r.isCA = true
			r.ncCrit = true
			r.exclDNS = []string{"evil.example"}
			r.exclIP = []string{"10.0.0.0/8", "0.0.0.0/0", "::/0", "fe80::1/10", "192.0.2.7/32"}
		}),
		with(func(r *ctRow) {
			r.name = "permitted only, emails and URIs"
			r.key = "rsa"
			r.subj = pkix.Name{CommonName: "nc2"}
			r.isCA = true
			r.permEmail = []string{"example.com", "a@b.example"}
			r.permURI = []string{"x.example"}
		}),
		with(func(r *ctRow) {
			r.name = "AIA with OCSP only, CRL points only, several policies"
			r.key = "ed"
			r.subj = pkix.Name{CommonName: "urls"}
			r.ocsp = []string{"http://o1.example", "http://o2.example/path?q=1"}
			r.crldp = []string{"ldap://ldap.example/cn=crl"}
			r.policies = []string{"2.23.140.1.2.1", "2.5.29.32.0", "1.3.6.1.4.1.11129.2.5.3", "2.999.12345678901234567890"}
		}),
		with(func(r *ctRow) {
			r.name = "issuer URLs only, extra extensions replacing NC, AIA, CRL points and policies"
			r.key = "ed"
			r.subj = pkix.Name{CommonName: "replace"}
			r.issuers = []string{"http://i.example/ca.crt"}
			r.permDNS = []string{"ignored.example"}
			r.crldp = []string{"http://ignored.example/crl"}
			r.policies = []string{"1.2.3"}
			r.extra = []pkix.Extension{
				{Id: ctOID("2.5.29.30"), Critical: true, Value: []byte{0x30, 0x07, 0xa1, 0x05, 0x30, 0x03, 0x82, 0x01, 0x78}},
				{Id: ctOID("1.3.6.1.5.5.7.1.1"), Value: []byte{0x30, 0x00}},
				{Id: ctOID("2.5.29.31"), Value: []byte{0x30, 0x00}},
				{Id: ctOID("2.5.29.32"), Value: []byte{0x30, 0x05, 0x30, 0x03, 0x06, 0x01, 0x2a}},
			}
		}),
	)
	return rows
}

// ctErrRows are templates Go's CreateCertificate refuses.
func ctErrRows() []ctRow {
	base := ctRow{serial: []byte{1}, nb: 1735689600, na: 1767225600, mpl: -1, subj: pkix.Name{CommonName: "err"}}
	with := func(f func(*ctRow)) ctRow { r := base; f(&r); return r }
	return []ctRow{
		with(func(r *ctRow) { r.name = "RSA key, ECDSA algorithm"; r.key, r.sigAlg = "rsa", x509.ECDSAWithSHA384 }),
		with(func(r *ctRow) { r.name = "ECDSA key, RSA algorithm"; r.key, r.sigAlg = "ec", x509.SHA256WithRSA }),
		with(func(r *ctRow) { r.name = "Ed25519 key, PSS"; r.key, r.sigAlg = "ed", x509.SHA256WithRSAPSS }),
		with(func(r *ctRow) { r.name = "RSA key, MD5"; r.key, r.sigAlg = "rsa", x509.MD5WithRSA }),
		with(func(r *ctRow) {
			r.name = "permitted DNS not IA5"
			r.key, r.isCA = "ed", true
			r.permDNS = []string{"bü.example"}
		}),
		with(func(r *ctRow) {
			r.name = "excluded email not IA5"
			r.key, r.isCA = "ed", true
			r.exclEmail = []string{"ü@example.com"}
		}),
		with(func(r *ctRow) {
			r.name = "permitted URI not IA5"
			r.key, r.isCA = "ed", true
			r.permURI = []string{"ü.example"}
		}),
	}
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
	rootDER := ctMust(x509.CreateCertificate(ctRand(), rootT, rootT, keys["ed"].Public(), keys["ed"]))
	root, _ := x509.ParseCertificate(rootDER)
	fmt.Fprintf(&o, "const ROOT: Bytes = %s\n", xbytes(rootDER))

	o.WriteString("\n/// (name, key, from the root, Go's certificate)\nfn rows() -> List[(Str, x509.Template, Str, Bool, Bytes)] {\n  [\n")
	for _, r := range ctRows() {
		t := ctTemplate(r)
		parent, signer := t, keys[r.key]
		if r.parent {
			parent, signer = root, keys["ed"]
		}
		der := ctMust(x509.CreateCertificate(ctRand(), t, parent, keys[r.key].Public(), signer))
		fmt.Fprintf(&o, "    (%s, %s, %s, %v, %s),\n", xq(r.name), ctXo(r), xq(r.key), r.parent, xbytes(der))
	}
	o.WriteString("  ]\n}\n")

	// template errors, Go's text (signingParamsForKey, isIA5String in the
	// name constraints)
	o.WriteString("\n/// (name, key, template, Go's error)\nfn template_errors() -> List[(Str, x509.Template, Str, Str)] {\n  [\n")
	for _, r := range ctErrRows() {
		_, err := x509.CreateCertificate(ctRand(), ctTemplate(r), ctTemplate(r), keys[r.key].Public(), keys[r.key])
		if err == nil {
			panic("no error: " + r.name)
		}
		fmt.Fprintf(&o, "    (%s, %s, %s, %s),\n", xq(r.name), ctXo(r), xq(r.key), xq(err.Error()))
	}
	o.WriteString("  ]\n}\n")

	// verification errors of each of Go's types, on chains made here
	ctVerifyKinds(&o, keys["ed"])
	ctPools(&o, keys)

	// requests: Go's, for the parse rows and the byte equality of Xo's
	o.WriteString("\n/// (name, key, request template, Go's request)\nfn requests() -> List[(Str, Str, x509.RequestTemplate, Bytes)] {\n  [\n")
	var csrDERs [][]byte
	for _, r := range ctRows() {
		if r.isCA || r.parent {
			continue
		}
		t := ctTemplate(r)
		q := &x509.CertificateRequest{Subject: t.Subject, DNSNames: t.DNSNames, EmailAddresses: t.EmailAddresses, IPAddresses: t.IPAddresses, URIs: t.URIs, ExtraExtensions: t.ExtraExtensions}
		der := ctMust(x509.CreateCertificateRequest(ctRand(), q, keys[r.key]))
		csrDERs = append(csrDERs, der)
		fmt.Fprintf(&o, "    (%s, %s, x509.RequestTemplate{subject: %s, dns_names: %s, ip_addresses: %s, email_addresses: %s, uris: %s, extra_extensions: %s}, %s),\n",
			xq(r.name), xq(r.key), ctName(r.subj), ctStrs(r.dns), ctIPs(r.ips), ctStrs(r.emails), ctStrs(r.uris), ctExts(r.extra), xbytes(der))
	}
	o.WriteString("  ]\n}\n")

	// parse rows: x509_test.go's requests, these requests and mutated
	// copies, with Go's answer
	goCSRs, goCRLs := ctGoInputs()
	mr := mrand.New(mrand.NewSource(10))
	// pem of a parsed request: Go's pem.EncodeToMemory of its Raw
	o.WriteString("\n/// (request DER, Go's PEM CERTIFICATE REQUEST block of the parsed Raw)\nconst REQUEST_PEMS: List[(Bytes, Bytes)] = [\n")
	for _, der := range append(append([][]byte(nil), goCSRs...), csrDERs...) {
		q, err := x509.ParseCertificateRequest(der)
		if err != nil {
			continue
		}
		fmt.Fprintf(&o, "  (%s, %s),\n", xbytes(der), xbytes(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: q.Raw})))
	}
	o.WriteString("]\n")
	o.WriteString("\n/// (DER, Go's summary or error)\nconst REQUEST_ROWS: List[(Bytes, Str)] = [\n")
	for _, der := range goCSRs {
		fmt.Fprintf(&o, "  (%s, %s),\n", xbytes(der), xq(ctCSRWant(der)))
	}
	for _, der := range ctCSRAttrVariants(goCSRs[0]) {
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
		der := ctMust(x509.CreateRevocationList(ctRand(), tmpl, root, keys["ed"]))
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

	// CRLs Xo makes (create_revocation_list): Go's for the same template,
	// issuer and key, or Go's error; TestCreateRevocationList's cases with
	// times Xo's Time holds (Go's use year 1), and its UTF-8 CA
	ctCRLCreate(&o, keys, rootDER)

	o.WriteString(`
fn shown(bad: List[Str]) -> List[Str] {
  bad[0..(if bad.len() < 20 { bad.len() } else { 20 })]
}

fn key(name: Str) -> crypto.PrivateKey {
  let der = if name == "ed" { KEY_ED } else if name == "rsa" { KEY_RSA } else if name == "utf8" { KEY_UTF8 } else { KEY_EC }
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
  "v=${c.version} serial=${c.serial.to_hex()} sig=${c.signature_algorithm} subj=${c.subject.display()} iss=${c.issuer.display()} nb=${c.not_before.unix()} na=${c.not_after.unix()} dns=${c.dns_names.join(",")} ip=${ips} email=${c.email_addresses.join(",")} uri=${c.uris.join(",")} ca=${c.is_ca} mpl=${mpl} ku=${c.key_usage.map(fn(u) { "${ku_index(u)}" }).join(",")} eku=${c.ext_key_usage.map(fn(u) { eku_oid(u) }).join(",")} ski=${c.subject_key_id.to_hex()} aki=${c.authority_key_id.to_hex()} ext=${exts(c.extensions)} bcv=${c.basic_constraints_valid} ocsp=${c.ocsp_servers.join(",")} issuers=${c.issuing_certificate_urls.join(",")} crl=${c.crl_distribution_points.join(",")} pol=${c.policies.join(",")} ncc=${c.permitted_dns_domains_critical} nc=${c.permitted_dns_domains.join(",")}/${c.excluded_dns_domains.join(",")}/${c.permitted_ip_ranges.map(fn(p) { p.display() }).join(",")}/${c.excluded_ip_ranges.map(fn(p) { p.display() }).join(",")}/${c.permitted_email_addresses.join(",")}/${c.excluded_email_addresses.join(",")}/${c.permitted_uri_domains.join(",")}/${c.excluded_uri_domains.join(",")}"
}

test "create_certificate makes Go's certificate: equal bytes for Ed25519 and RSA PKCS #1 v1.5, equal TBS bytes for ECDSA and RSA PSS" {
  let root = x509.parse(ROOT).ok() ?? fault("root")
  var bad: List[Str] = []
  for (name, t, kname, from_root, want) in rows() {
    let k = key(kname)
    let parent = if from_root { Some(root) } else { None }
    let signer = if from_root { key("ed") } else { k }
    match x509.create_certificate(os.rand, t, k.public_key(), parent, signer) {
      Ok(c) => {
        let go = x509.parse(want).ok() ?? fault("go's")
        // an ECDSA or RSA PSS signature is random: a self-signed row
        // signed so compares the signed part's bytes and fields, every
        // other row the certificate's bytes
        if (kname == "ec" || t.signature_algorithm.ends_with("PSS")) && !from_root {
          if summary(c) != summary(go) {
            bad.push("${name}:\n  got  ${summary(c)}\n  want ${summary(go)}")
          } else if c.raw_tbs_certificate != go.raw_tbs_certificate {
            bad.push("${name}: other TBS bytes")
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

test "create_certificate refuses Go's bad templates with Go's text" {
  var bad: List[Str] = []
  for (name, t, kname, want) in template_errors() {
    let k = key(kname)
    match x509.create_certificate(os.rand, t, k.public_key(), None, k) {
      Ok(_) => bad.push("${name}: made a certificate, want ${want}")
      Err(e) => if e.display() != want { bad.push("${name}: ${e.display()}, want ${want}") }
    }
  }
  let k = key("ed")
  let pol = x509.create_certificate(os.rand, x509.Template{subject: x509.Name{common_name: "x"}, not_before: Time.from_unix(0), not_after: Time.from_unix(1), policies: ["1.50.3"]}, k.public_key(), None, k)
  if pol.err().map(fn(e) { e.display() }) != Some("invalid policy object identifier") { bad.push("policy 1.50.3: ${pol.is_ok()}") }
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
        "v=${q.version} sig=${q.signature_algorithm} subj=${q.subject.display()} dns=${q.dns_names.join(",")} ip=${q.ip_addresses.map(fn(a) { a.display() }).join(",")} email=${q.email_addresses.join(",")} uri=${q.uris.join(",")} ext=${exts(q.extensions)} key=${q.public_key.is_some()} check=${sig} attrs=${q.attributes.map(fn(a) { "${a.oid}=${a.values.map(fn(v) { v.to_hex() }).join("+")}" }).join(";")}"
      }
      Err(e) => "error: ${e.display()}"
    }
    if got != want {
      bad.push("${der.len()} bytes:\n  got  ${got}\n  want ${want}")
    }
  }
  expect(shown(bad)) == []
}

test "a parsed request's pem is Go's CERTIFICATE REQUEST block" {
  var bad: List[Str] = []
  for (der, want) in REQUEST_PEMS {
    match x509.parse_request(der) {
      Ok(q) => if q.pem() != want { bad.push("${der.len()} bytes: other PEM") }
      Err(e) => bad.push("${der.len()} bytes: ${e.display()}")
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

fn crl_summary(l: x509.RevocationList) -> Str {
  let next = match l.next_update {
    Some(t) => "${t.unix()}"
    None => "none"
  }
  "sig=${l.signature_algorithm} iss=${l.issuer.display()} num=${l.number.to_hex()} this=${l.this_update.unix()} next=${next} revoked=${l.revoked.map(fn(r) { "${r.serial.to_hex()}/${r.revoked_at.unix()}/${r.reason}/${exts(r.extensions)}" }).join(";")} aki=${l.authority_key_id.to_hex()} ext=${exts(l.extensions)}"
}

test "create_revocation_list makes Go's CRL: equal bytes for Ed25519 and RSA PKCS #1 v1.5, equal fields for ECDSA and RSA PSS, Go's errors" {
  var bad: List[Str] = []
  for (name, issuer_der, kname, t, want, want_err) in crl_create_rows() {
    let issuer = x509.parse(issuer_der).ok() ?? fault("issuer")
    match x509.create_revocation_list(os.rand, t, issuer, key(kname)) {
      Ok(l) => {
        if !want_err.is_empty() {
          bad.push("${name}: made a CRL, want ${want_err}")
        } else {
          let go = x509.parse_revocation_list(want).ok() ?? fault("go's")
          let random = kname == "ec" || t.signature_algorithm.ends_with("PSS")
          if random {
            if crl_summary(l) != crl_summary(go) {
              bad.push("${name}:\n  got  ${crl_summary(l)}\n  want ${crl_summary(go)}")
            }
          } else if l.raw != want {
            bad.push("${name}: other bytes\n  got  ${crl_summary(l)}\n  want ${crl_summary(go)}")
          }
          match l.check_signature_from(issuer) {
            Ok(_) => ()
            Err(e) => bad.push("${name}: check_signature_from: ${e.display()}")
          }
        }
      }
      Err(e) => if e.display() != want_err { bad.push("${name}: ${e.display()}, want ${if want_err.is_empty() { "a CRL" } else { want_err }}") }
    }
  }
  expect(shown(bad)) == []
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
	return fmt.Sprintf("v=%d sig=%s subj=%s dns=%s ip=%s email=%s uri=%s ext=%s key=%v check=%s attrs=%s", q.Version, q.SignatureAlgorithm, q.Subject,
		strings.Join(q.DNSNames, ","), strings.Join(ips, ","), strings.Join(q.EmailAddresses, ","), strings.Join(uris, ","),
		ctExtText(q.Extensions), q.PublicKey != nil, check, ctCSRAttrs(q))
}

// ctCSRAttrVariants is der with other [0] attributes in its
// CertificationRequestInfo (the signature no longer checks): elements that
// are not Attributes, Attributes that do not read, odd values, and the
// extension request among them.
func ctCSRAttrVariants(der []byte) [][]byte {
	var outer struct {
		TBS asn1.RawValue
		Alg asn1.RawValue
		Sig asn1.RawValue
	}
	if _, err := asn1.Unmarshal(der, &outer); err != nil {
		panic(err)
	}
	var tbs struct {
		Version   int
		Subject   asn1.RawValue
		PublicKey asn1.RawValue
		Attrs     asn1.RawValue
	}
	if _, err := asn1.Unmarshal(outer.TBS.FullBytes, &tbs); err != nil {
		panic(err)
	}
	h := func(s string) []byte {
		b, err := hex.DecodeString(strings.ReplaceAll(s, " ", ""))
		if err != nil {
			panic(err)
		}
		return b
	}
	tlv := func(tag byte, body []byte) []byte {
		if len(body) < 128 {
			return append([]byte{tag, byte(len(body))}, body...)
		}
		return append([]byte{tag, 0x82, byte(len(body) >> 8), byte(len(body))}, body...)
	}
	cat := func(xs ...[]byte) []byte {
		var b []byte
		for _, x := range xs {
			b = append(b, x...)
		}
		return b
	}
	cp := h("06 09 2a 86 48 86 f7 0d 01 09 07")                  // challengePassword
	un := h("06 09 2a 86 48 86 f7 0d 01 09 02")                  // unstructuredName
	er := h("06 09 2a 86 48 86 f7 0d 01 09 0e")                  // extensionRequest
	exts := h("30 0e 30 0c 06 03 55 1d 11 04 05 30 03 82 01 61") // SAN dNSName "a"
	exts2 := h("30 0e 30 0c 06 03 55 1d 11 04 05 30 03 82 01 62")
	sets := [][]byte{
		nil,
		tlv(0x30, cat(cp, tlv(0x31, tlv(0x0c, []byte("secret"))))),
		cat(tlv(0x30, cat(cp, tlv(0x31, tlv(0x13, []byte("pw"))))), tlv(0x30, cat(un, tlv(0x31, tlv(0x16, []byte("host.example")))))),
		cat(h("02 01 05"), tlv(0x30, cat(cp, tlv(0x31, tlv(0x0c, []byte("x")))))),
		cat(tlv(0x30, cat(h("06 00"), tlv(0x31, h("05 00")))), tlv(0x30, cat(un, tlv(0x31, h("05 00"))))),
		tlv(0x30, cat(cp, h("02 01 01"))),
		tlv(0x30, cat(cp, tlv(0x31, nil))),
		tlv(0x30, cat(cp, tlv(0x31, h("0c 05 61")))),
		tlv(0x30, cat(cp, tlv(0x31, cat(tlv(0x0c, []byte("a")), tlv(0x0c, []byte("b")))), h("02 01 07"))),
		tlv(0x30, cat(er, tlv(0x31, exts))),
		cat(tlv(0x30, cat(er, tlv(0x31, exts))), tlv(0x30, cat(er, tlv(0x31, exts2)))),
		cat(tlv(0x30, cat(er, tlv(0x31, exts))), tlv(0x30, cat(er, tlv(0x31, exts)))),
		tlv(0x30, cat(er, tlv(0x31, h("30 03 02 01")))),
		tlv(0x30, cat(er, tlv(0x31, cat(exts, tlv(0x0c, []byte("z")))))),
		tlv(0x30, cat(er, tlv(0x31, nil))),
		cat(tlv(0x31, nil), tlv(0x30, nil), tlv(0x30, cp)),
		tlv(0x30, cat(cp, tlv(0x30, tlv(0x0c, []byte("seq not set"))))),
		tlv(0x30, cat(h("06 03 2a 03 04"), tlv(0x31, tlv(0x30, cat(h("06 03 55 04 03"), tlv(0x0c, []byte("cn"))))))),
	}
	var out [][]byte
	for _, s := range sets {
		t := tlv(0x30, cat(asn1Int(tbs.Version), tbs.Subject.FullBytes, tbs.PublicKey.FullBytes, tlv(0xa0, s)))
		out = append(out, tlv(0x30, cat(t, outer.Alg.FullBytes, outer.Sig.FullBytes)))
	}
	return out
}

func asn1Int(v int) []byte {
	b, err := asn1.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

// ctCSRAttrs is a request's attributes as std/x509 reads them: each of
// RawAttributes that reads as parseCSRExtensions' pkcs10Attribute, "OID=V+V"
// with each value's DER in hex, joined by ";". The ones that read as
// pkix.AttributeTypeAndValueSET must be Go's (deprecated) Attributes.
func ctCSRAttrs(q *x509.CertificateRequest) string {
	var t struct {
		Raw           asn1.RawContent
		Version       int
		Subject       asn1.RawValue
		PublicKey     asn1.RawValue
		RawAttributes []asn1.RawValue `asn1:"tag:0"`
	}
	if _, err := asn1.Unmarshal(q.RawTBSCertificateRequest, &t); err != nil {
		panic(err)
	}
	type attr struct {
		Id     asn1.ObjectIdentifier
		Values []asn1.RawValue `asn1:"set"`
	}
	var parts []string
	atvs := 0
	for _, r := range t.RawAttributes {
		var a attr
		if rest, err := asn1.Unmarshal(r.FullBytes, &a); err != nil || len(rest) != 0 {
			continue
		}
		var vs []string
		for _, v := range a.Values {
			vs = append(vs, hex.EncodeToString(v.FullBytes))
		}
		parts = append(parts, a.Id.String()+"="+strings.Join(vs, "+"))
	}
	for _, r := range t.RawAttributes {
		var s pkix.AttributeTypeAndValueSET
		if rest, err := asn1.Unmarshal(r.FullBytes, &s); err == nil && len(rest) == 0 {
			atvs++
		}
	}
	if atvs != len(q.Attributes) {
		panic("ctCSRAttrs: Go's Attributes disagree")
	}
	return strings.Join(parts, ";")
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
	der := ctMust(x509.CreateCertificate(ctRand(), rootT, rootT, keys["ed"].Public(), keys["ed"]))
	root, _ := x509.ParseCertificate(der)
	if err := l.CheckSignatureFrom(root); err != nil {
		return err.Error()
	}
	return ok
}

type ctCRLRow struct {
	name   string
	issuer string // root, rsaca, ecca, utf8ca, nocrlsign, noskid
	key    string
	t      *x509.RevocationList
}

// ctCRLCreate writes the issuers, UTF-8 CA key and crl_create_rows.
func ctCRLCreate(o *strings.Builder, keys map[string]crypto.Signer, rootDER []byte) {
	path := filepath.Join(runtime.GOROOT(), "src", "crypto", "x509", "x509_test.go")
	f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		panic(err)
	}
	// TestCreateRevocationList's utf8CAStr and utf8KeyStr
	lits := map[string]string{}
	ast.Inspect(f, func(n ast.Node) bool {
		a, ok := n.(*ast.AssignStmt)
		if !ok || len(a.Lhs) != 1 || len(a.Rhs) != 1 {
			return true
		}
		id, ok := a.Lhs[0].(*ast.Ident)
		lit, ok2 := a.Rhs[0].(*ast.BasicLit)
		if ok && ok2 && (id.Name == "utf8CAStr" || id.Name == "utf8KeyStr") {
			lits[id.Name], _ = strconv.Unquote(lit.Value)
		}
		return true
	})
	if len(lits) != 2 {
		panic("x509create: x509_test.go has no utf8CAStr and utf8KeyStr")
	}
	utf8CA := ctMust(base64.StdEncoding.DecodeString(lits["utf8CAStr"]))
	utf8Key := ctMust(base64.StdEncoding.DecodeString(lits["utf8KeyStr"]))
	uk, err := x509.ParsePKCS8PrivateKey(utf8Key)
	if err != nil {
		panic(err)
	}
	keys["utf8"] = uk.(crypto.Signer)
	fmt.Fprintf(o, "\nconst KEY_UTF8: Bytes = %s\n", xbytes(utf8Key))

	ca := func(k string, ku x509.KeyUsage, isCA bool) []byte {
		t := &x509.Certificate{SerialNumber: big.NewInt(11), Subject: pkix.Name{CommonName: "Xo CRL issuer " + k, Organization: []string{"Xo"}},
			NotBefore: time.Unix(1704067200, 0), NotAfter: time.Unix(1893456000, 0), IsCA: isCA, BasicConstraintsValid: true, KeyUsage: ku}
		return ctMust(x509.CreateCertificate(ctRand(), t, t, keys[k].Public(), keys[k]))
	}
	issuers := map[string][]byte{
		"root": rootDER, "utf8ca": utf8CA,
		"rsaca":     ca("rsa", x509.KeyUsageCertSign|x509.KeyUsageCRLSign, true),
		"ecca":      ca("ec", x509.KeyUsageCertSign|x509.KeyUsageCRLSign, true),
		"nocrlsign": ca("ed", x509.KeyUsageCertSign, true),
		"noskid":    ca("ed", x509.KeyUsageCRLSign, false),
	}
	for _, n := range []string{"root", "utf8ca", "rsaca", "ecca", "nocrlsign", "noskid"} {
		fmt.Fprintf(o, "const ISSUER_%s: Bytes = %s\n", strings.ToUpper(n), xbytes(issuers[n]))
	}

	this, next := time.Unix(1735689600, 0), time.Unix(1738368000, 0)
	base := func() *x509.RevocationList {
		return &x509.RevocationList{Number: big.NewInt(5), ThisUpdate: this, NextUpdate: next}
	}
	with := func(f func(*x509.RevocationList)) *x509.RevocationList {
		t := base()
		f(t)
		return t
	}
	entry := x509.RevocationListEntry{SerialNumber: big.NewInt(2), RevocationTime: time.Unix(1735000000, 0)}
	extra := pkix.Extension{Id: ctOID("2.5.29.99"), Value: []byte{5, 0}}
	long := make([]byte, 21)
	long[0] = 1
	msb := make([]byte, 20)
	msb[0] = 0x80
	max := make([]byte, 20)
	max[0] = 0x7f
	rows := []ctCRLRow{
		{"valid", "ecca", "ec", base()},
		{"valid, reason code", "ecca", "ec", with(func(t *x509.RevocationList) {
			t.RevokedCertificateEntries = []x509.RevocationListEntry{{SerialNumber: big.NewInt(2), RevocationTime: entry.RevocationTime, ReasonCode: 1}}
		})},
		{"valid, extra entry extension", "root", "ed", with(func(t *x509.RevocationList) {
			t.RevokedCertificateEntries = []x509.RevocationListEntry{{SerialNumber: big.NewInt(2), RevocationTime: entry.RevocationTime, ExtraExtensions: []pkix.Extension{extra}}}
		})},
		{"valid, Ed25519 key", "root", "ed", with(func(t *x509.RevocationList) {
			t.RevokedCertificateEntries = []x509.RevocationListEntry{entry}
		})},
		{"valid, non-default signature algorithm", "ecca", "ec", with(func(t *x509.RevocationList) {
			t.SignatureAlgorithm = x509.ECDSAWithSHA512
			t.RevokedCertificateEntries = []x509.RevocationListEntry{entry}
		})},
		{"valid, extra extension", "root", "ed", with(func(t *x509.RevocationList) {
			t.RevokedCertificateEntries = []x509.RevocationListEntry{entry}
			t.ExtraExtensions = []pkix.Extension{extra}
		})},
		{"valid, critical extra extension and reason with entry extension", "rsaca", "rsa", with(func(t *x509.RevocationList) {
			t.RevokedCertificateEntries = []x509.RevocationListEntry{
				{SerialNumber: big.NewInt(2), RevocationTime: entry.RevocationTime, ReasonCode: 4, ExtraExtensions: []pkix.Extension{extra}},
				{SerialNumber: new(big.Int).SetBytes([]byte{0x80, 1, 2, 3}), RevocationTime: time.Unix(2600000000, 0), ReasonCode: 10},
			}
			t.ExtraExtensions = []pkix.Extension{{Id: ctOID("1.2.3.4"), Critical: true, Value: []byte{5, 0}}}
		})},
		{"valid, empty list", "rsaca", "rsa", base()},
		{"valid, RSA SHA-512", "rsaca", "rsa", with(func(t *x509.RevocationList) { t.SignatureAlgorithm = x509.SHA512WithRSA })},
		{"valid, RSA PSS SHA-256", "rsaca", "rsa", with(func(t *x509.RevocationList) { t.SignatureAlgorithm = x509.SHA256WithRSAPSS })},
		{"valid, RSA PSS SHA-384", "rsaca", "rsa", with(func(t *x509.RevocationList) { t.SignatureAlgorithm = x509.SHA384WithRSAPSS })},
		{"valid, SHA1-RSA", "rsaca", "rsa", with(func(t *x509.RevocationList) { t.SignatureAlgorithm = x509.SHA1WithRSA })},
		{"valid, ECDSA-SHA1", "ecca", "ec", with(func(t *x509.RevocationList) { t.SignatureAlgorithm = x509.ECDSAWithSHA1 })},
		{"valid CA with utf8 Subject fields including Email, empty list", "utf8ca", "utf8", base()},
		{"valid, 20 byte number", "root", "ed", with(func(t *x509.RevocationList) { t.Number = new(big.Int).SetBytes(max) })},
		{"valid, number 0, GeneralizedTime", "root", "ed", with(func(t *x509.RevocationList) {
			t.Number = big.NewInt(0)
			t.ThisUpdate = time.Unix(-1000000000, 0)
			t.NextUpdate = time.Unix(2600000000, 0)
		})},
		{"valid, this equals next", "root", "ed", with(func(t *x509.RevocationList) { t.NextUpdate = this })},
		{"issuer doesn't have crlSign key usage bit set", "nocrlsign", "ed", base()},
		{"issuer missing SubjectKeyId", "noskid", "ed", base()},
		{"nextUpdate before thisUpdate", "root", "ed", with(func(t *x509.RevocationList) { t.NextUpdate = this.Add(-time.Second) })},
		{"long Number", "root", "ed", with(func(t *x509.RevocationList) { t.Number = new(big.Int).SetBytes(long) })},
		{"long Number (20 bytes, MSB set)", "root", "ed", with(func(t *x509.RevocationList) { t.Number = new(big.Int).SetBytes(msb) })},
		{"invalid signature algorithm", "ecca", "ec", with(func(t *x509.RevocationList) { t.SignatureAlgorithm = x509.SHA256WithRSA })},
		{"Ed25519 key, ECDSA algorithm", "root", "ed", with(func(t *x509.RevocationList) { t.SignatureAlgorithm = x509.ECDSAWithSHA256 })},
		{"RSA key, Ed25519 algorithm", "rsaca", "rsa", with(func(t *x509.RevocationList) { t.SignatureAlgorithm = x509.PureEd25519 })},
		{"DSA algorithm", "rsaca", "rsa", with(func(t *x509.RevocationList) { t.SignatureAlgorithm = x509.DSAWithSHA256 })},
		{"MD5-RSA", "rsaca", "rsa", with(func(t *x509.RevocationList) { t.SignatureAlgorithm = x509.MD5WithRSA })},
		{"reason code as an extra entry extension", "root", "ed", with(func(t *x509.RevocationList) {
			t.RevokedCertificateEntries = []x509.RevocationListEntry{{SerialNumber: big.NewInt(2), RevocationTime: entry.RevocationTime, ExtraExtensions: []pkix.Extension{{Id: ctOID("2.5.29.21"), Value: []byte{10, 1, 1}}}}}
		})},
	}
	o.WriteString("\n/// (name, issuer, key, template, Go's CRL, Go's error)\nfn crl_create_rows() -> List[(Str, Bytes, Str, x509.RevocationListTemplate, Bytes, Str)] {\n  [\n")
	for _, r := range rows {
		iss, err := x509.ParseCertificate(issuers[r.issuer])
		if err != nil {
			panic(err)
		}
		der, err := x509.CreateRevocationList(ctRand(), r.t, iss, keys[r.key])
		msg := ""
		if err != nil {
			msg = err.Error()
			der = nil
		}
		var ents []string
		for _, e := range r.t.RevokedCertificateEntries {
			ents = append(ents, fmt.Sprintf("x509.RevokedCertificate{serial: %s, revoked_at: Time.from_unix(%d), reason: %d, extensions: %s}", xbytes(e.SerialNumber.Bytes()), e.RevocationTime.Unix(), e.ReasonCode, ctExts(e.ExtraExtensions)))
		}
		alg := ""
		if r.t.SignatureAlgorithm != 0 {
			alg = r.t.SignatureAlgorithm.String()
		}
		fmt.Fprintf(o, "    (%s, ISSUER_%s, %s, x509.RevocationListTemplate{number: %s, this_update: Time.from_unix(%d), next_update: Time.from_unix(%d), revoked: [%s], extra_extensions: %s, signature_algorithm: %s}, %s, %s),\n",
			xq(r.name), strings.ToUpper(r.issuer), xq(r.key), xbytes(r.t.Number.Bytes()), r.t.ThisUpdate.Unix(), r.t.NextUpdate.Unix(), strings.Join(ents, ", "), ctExts(r.t.ExtraExtensions), xq(alg), xbytes(der), xq(msg))
	}
	o.WriteString("  ]\n}\n")
}

// ctVerifyKinds writes KIND_ROWS: chains Go's Verify refuses with each of
// its error types (CertificateInvalidError's reasons, HostnameError,
// UnknownAuthorityError, UnhandledCriticalExtension), and one it accepts.
func ctVerifyKinds(o *strings.Builder, k crypto.Signer) {
	nb, na, now := time.Unix(1735689600, 0), time.Unix(1767225600, 0), int64(1750000000)
	mk := func(t, parent *x509.Certificate) *x509.Certificate {
		if parent == nil {
			parent = t
		}
		der := ctMust(x509.CreateCertificate(ctRand(), t, parent, k.Public(), k))
		c, err := x509.ParseCertificate(der)
		if err != nil {
			panic(err)
		}
		return c
	}
	ca := func(cn string, serial int64, f func(*x509.Certificate)) *x509.Certificate {
		t := &x509.Certificate{SerialNumber: big.NewInt(serial), Subject: pkix.Name{CommonName: cn}, NotBefore: nb, NotAfter: na,
			IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign, MaxPathLen: -1}
		if f != nil {
			f(t)
		}
		return t
	}
	leafT := func(serial int64, f func(*x509.Certificate)) *x509.Certificate {
		t := &x509.Certificate{SerialNumber: big.NewInt(serial), Subject: pkix.Name{CommonName: "leaf"}, NotBefore: nb, NotAfter: na,
			DNSNames: []string{"leaf.example"}, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
		if f != nil {
			f(t)
		}
		return t
	}
	rootT := ca("Kind Root", 1, nil)
	root := mk(rootT, nil)
	other := mk(ca("Other Root", 2, nil), nil)
	pl0T := ca("PL0 Root", 3, func(t *x509.Certificate) { t.MaxPathLen, t.MaxPathLenZero = 0, true })
	pl0 := mk(pl0T, nil)
	interT := ca("Inter", 4, nil)
	inter := mk(interT, pl0)
	notCAT := &x509.Certificate{SerialNumber: big.NewInt(5), Subject: pkix.Name{CommonName: "Not CA"}, NotBefore: nb, NotAfter: na, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	notCA := mk(notCAT, root)
	// a trust anchor's own name and policy constraints are not applied
	// (RFC 5280 6.1): these are intermediates
	ncT := ca("NC Inter", 6, func(t *x509.Certificate) { t.ExcludedDNSDomains = []string{"leaf.example"} })
	nc := mk(ncT, rootT)
	polT := ca("Policy Inter", 7, func(t *x509.Certificate) {
		t.ExtraExtensions = []pkix.Extension{{Id: ctOID("2.5.29.36"), Critical: true, Value: []byte{0x30, 0x03, 0x80, 0x01, 0x00}}}
	})
	pol := mk(polT, rootT)
	leaf := mk(leafT(10, nil), rootT)
	type row struct {
		name          string
		leaf          *x509.Certificate
		inters, roots []*x509.Certificate
		at            int64
		dns           string
		usages        []x509.ExtKeyUsage
	}
	rows := []row{
		{"a chain Go accepts", leaf, nil, []*x509.Certificate{root}, now, "leaf.example", nil},
		{"Hostname: another name", leaf, nil, []*x509.Certificate{root}, now, "other.example", nil},
		{"Hostname: an IP in brackets", leaf, nil, []*x509.Certificate{root}, now, "[::1]", nil},
		{"Expired: after", leaf, nil, []*x509.Certificate{root}, 1800000000, "", nil},
		{"Expired: before", leaf, nil, []*x509.Certificate{root}, 1700000000, "", nil},
		{"UnknownAuthority", leaf, nil, []*x509.Certificate{other}, now, "", nil},
		{"IncompatibleUsage", mk(leafT(11, func(t *x509.Certificate) { t.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth} }), rootT), nil, []*x509.Certificate{root}, now, "", nil},
		{"TooManyIntermediates", mk(leafT(12, nil), interT), []*x509.Certificate{inter}, []*x509.Certificate{pl0}, now, "", nil},
		// Go keeps NotAuthorizedToSign as the unknown authority's hint
		{"UnknownAuthority, hint NotAuthorizedToSign", mk(leafT(13, nil), notCAT), []*x509.Certificate{notCA}, []*x509.Certificate{root}, now, "", nil},
		{"CaNotAuthorizedForThisName", mk(leafT(14, nil), ncT), []*x509.Certificate{nc}, []*x509.Certificate{root}, now, "", nil},
		{"NoValidChains: invalid policies", mk(leafT(15, nil), polT), []*x509.Certificate{pol}, []*x509.Certificate{root}, now, "", nil},
		{"NoValidChains: invalid policies and usage", mk(leafT(16, nil), polT), []*x509.Certificate{pol}, []*x509.Certificate{root}, now, "", []x509.ExtKeyUsage{x509.ExtKeyUsageCodeSigning}},
		{"UnhandledCriticalExtension", mk(leafT(17, func(t *x509.Certificate) {
			t.ExtraExtensions = []pkix.Extension{{Id: ctOID("1.2.3.4.5"), Critical: true, Value: []byte{5, 0}}}
		}), rootT), nil, []*x509.Certificate{root}, now, "", nil},
	}
	ders := func(cs []*x509.Certificate) string {
		var q []string
		for _, c := range cs {
			q = append(q, xbytes(c.Raw))
		}
		return "[" + strings.Join(q, ", ") + "]"
	}
	o.WriteString("\n/// (name, leaf, intermediates, roots, Unix time, DNS name, server auth (false: code signing), Go's Verify: ok or the error and its type)\nfn kind_rows() -> List[(Str, Bytes, List[Bytes], List[Bytes], Int, Str, Bool, Str)] {\n  [\n")
	for _, r := range rows {
		ip, rp := x509.NewCertPool(), x509.NewCertPool()
		for _, c := range r.inters {
			ip.AddCert(c)
		}
		for _, c := range r.roots {
			rp.AddCert(c)
		}
		want := "ok"
		if _, err := r.leaf.Verify(x509.VerifyOptions{Roots: rp, Intermediates: ip, DNSName: r.dns, CurrentTime: time.Unix(r.at, 0).UTC(), KeyUsages: r.usages}); err != nil {
			want = err.Error() + x509KindOf(err)
		}
		fmt.Fprintf(o, "    (%s, %s, %s, %s, %d, %s, %v, %s),\n", xq(r.name), xbytes(r.leaf.Raw), ders(r.inters), ders(r.roots), r.at, xq(r.dns), r.usages == nil, xq(want))
	}
	o.WriteString("  ]\n}\n")
	o.WriteString(`
/// A CertErrKind as the generator's x509KindOf shows Go's error type.
fn kind_text(k: x509.CertErrKind) -> Str {
  match k {
    Other => " {Other}"
    CertificateInvalid(reason, detail) => " {CertificateInvalid ${reason_text(reason)}: ${detail}}"
    Hostname(host) => " {Hostname ${host}}"
    UnknownAuthority => " {UnknownAuthority}"
    InsecureAlgorithm(algorithm) => " {InsecureAlgorithm ${algorithm}}"
    ConstraintViolation => " {ConstraintViolation}"
    UnhandledCriticalExtension => " {UnhandledCriticalExtension}"
  }
}

fn reason_text(r: x509.InvalidReason) -> Str {
  match r {
    NotAuthorizedToSign => "NotAuthorizedToSign"
    Expired => "Expired"
    CaNotAuthorizedForThisName => "CaNotAuthorizedForThisName"
    TooManyIntermediates => "TooManyIntermediates"
    IncompatibleUsage => "IncompatibleUsage"
    NameMismatch => "NameMismatch"
    NameConstraintsWithoutSans => "NameConstraintsWithoutSans"
    UnconstrainedName => "UnconstrainedName"
    TooManyConstraints => "TooManyConstraints"
    CaNotAuthorizedForExtKeyUsage => "CaNotAuthorizedForExtKeyUsage"
    NoValidChains => "NoValidChains"
  }
}

test "verify's errors have Go's text and Go's error type (decision 0643)" {
  var bad: List[Str] = []
  for (name, leaf, inters, roots, at, dns, server, want) in kind_rows() {
    let l = x509.parse(leaf).ok() ?? fault("leaf")
    let parse_all = fn(ds: List[Bytes]) -> List[x509.Certificate] { ds.map(fn(d) { x509.parse(d).ok() ?? fault("cert") }) }
    let usages = if server { [x509.ExtKeyUsage.ServerAuth] } else { [x509.ExtKeyUsage.CodeSigning] }
    let opts = x509.VerifyOptions{roots: parse_all(roots), intermediates: parse_all(inters), dns_name: dns, at: Time.from_unix(at), key_usages: usages}
    let got = match l.verify(opts) {
      Ok(_) => "ok"
      Err(e) => "${e.display()}${kind_text(e.kind)}"
    }
    if got != want {
      bad.push("${name}:\n  got  ${got}\n  want ${want}")
    }
  }
  expect(shown(bad)) == []
}

test "a CertErr made without a kind is Other, so a literal still builds" {
  let e = x509.CertErr{msg: "x509: something"}
  expect(kind_text(e.kind)) == " {Other}"
}
`)
}

// ctPools writes the CertPool rows: AppendCertsFromPEM's result and
// contents (checked with Go's Equal), Equal on pairs of pools, and
// AddCertWithConstraint in Verify, where a constraint that refuses shows
// the chain Go passed it in the unknown authority's hint.
func ctPools(o *strings.Builder, keys map[string]crypto.Signer) {
	nb, na, now := time.Unix(1735689600, 0), time.Unix(1767225600, 0), int64(1750000000)
	k := keys["ed"]
	mk := func(t, parent *x509.Certificate, signer crypto.Signer) *x509.Certificate {
		if parent == nil {
			parent = t
		}
		c, err := x509.ParseCertificate(ctMust(x509.CreateCertificate(ctRand(), t, parent, k.Public(), signer)))
		if err != nil {
			panic(err)
		}
		return c
	}
	ca := func(cn string, serial int64, notAfter time.Time) *x509.Certificate {
		return &x509.Certificate{SerialNumber: big.NewInt(serial), Subject: pkix.Name{CommonName: cn}, NotBefore: nb, NotAfter: notAfter,
			IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign, MaxPathLen: -1}
	}
	rootT := ca("Pool Root", 1, na)
	root := mk(rootT, nil, k)
	root2 := mk(ca("Pool Root", 2, na), nil, k)
	expired := mk(ca("Pool Root", 3, time.Unix(1740000000, 0)), nil, k)
	interT := ca("Pool Inter", 4, na)
	inter := mk(interT, rootT, k)
	leafT := &x509.Certificate{SerialNumber: big.NewInt(10), Subject: pkix.Name{CommonName: "pool leaf"}, NotBefore: nb, NotAfter: na,
		DNSNames: []string{"leaf.example"}, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	leaf := mk(leafT, rootT, k)
	leaf2 := mk(&x509.Certificate{SerialNumber: big.NewInt(11), Subject: pkix.Name{CommonName: "pool leaf 2"}, NotBefore: nb, NotAfter: na,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}, interT, k)
	certs := []*x509.Certificate{root, root2, expired, inter, leaf, leaf2}
	fmt.Fprintf(o, "\n/// The pool rows' certificates.\nconst POOL_CERTS: List[Bytes] = [%s]\n", func() string {
		var q []string
		for _, c := range certs {
			q = append(q, xbytes(c.Raw))
		}
		return strings.Join(q, ", ")
	}())
	pemOf := func(c *x509.Certificate, typ string, h map[string]string) string {
		return string(pem.EncodeToMemory(&pem.Block{Type: typ, Headers: h, Bytes: c.Raw}))
	}
	// AppendCertsFromPEM: (pem, Go's bool, the indexes the pool holds)
	type pemCase struct {
		text string
		want []int
	}
	bad := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte{0x30, 0x03, 1, 2, 3}}))
	pems := []pemCase{
		{"", nil},
		{"no blocks here\n", nil},
		{pemOf(root, "CERTIFICATE", nil), []int{0}},
		{pemOf(root, "CERTIFICATE", nil) + "junk between\n" + pemOf(inter, "CERTIFICATE", nil), []int{0, 3}},
		{pemOf(root, "CERTIFICATE", map[string]string{"Proc-Type": "4,ENCRYPTED"}), nil},
		{pemOf(root, "TRUSTED CERTIFICATE", nil) + pemOf(leaf, "CERTIFICATE", nil), []int{4}},
		{bad + pemOf(leaf2, "CERTIFICATE", nil), []int{5}},
		{bad, nil},
		{pemOf(root, "CERTIFICATE", nil) + pemOf(root, "CERTIFICATE", nil), []int{0}},
		{"-----BEGIN CERTIFICATE-----\nnot base64!\n-----END CERTIFICATE-----\n" + pemOf(root2, "CERTIFICATE", nil), []int{1}},
	}
	o.WriteString("\n/// (PEM text, Go's AppendCertsFromPEM, the POOL_CERTS the pool then holds, by Go's Equal)\nconst PEM_ROWS: List[(Str, Bool, List[Int])] = [\n")
	for _, c := range pems {
		gp := x509.NewCertPool()
		ok := gp.AppendCertsFromPEM([]byte(c.text))
		want := x509.NewCertPool()
		for _, i := range c.want {
			want.AddCert(certs[i])
		}
		if !gp.Equal(want) {
			panic("pem row's contents are not Go's")
		}
		fmt.Fprintf(o, "  (%s, %v, %s),\n", xq(c.text), ok, oidInts(c.want))
	}
	o.WriteString("]\n")
	// Equal
	eqs := [][2][]int{{{}, {}}, {{0}, {0}}, {{0, 3}, {3, 0}}, {{0}, {1}}, {{0, 3}, {0}}, {{0, 0}, {0}}, {{}, {0}}, {{0, 1, 2, 3, 4, 5}, {5, 4, 3, 2, 1, 0}}}
	o.WriteString("\n/// (POOL_CERTS of a, of b, Go's Equal)\nconst EQUAL_ROWS: List[(List[Int], List[Int], Bool)] = [\n")
	for _, e := range eqs {
		a, b := x509.NewCertPool(), x509.NewCertPool()
		for _, i := range e[0] {
			a.AddCert(certs[i])
		}
		for _, i := range e[1] {
			b.AddCert(certs[i])
		}
		fmt.Fprintf(o, "  (%s, %s, %v),\n", oidInts(e[0]), oidInts(e[1]), a.Equal(b))
	}
	o.WriteString("]\n")
	// constraints: "" none, "show" refuses with the chain's subjects,
	// "accept" passes, "long" refuses a chain of more than one
	type entry struct {
		cert int
		con  string
	}
	conOf := func(kind string) func([]*x509.Certificate) error {
		switch kind {
		case "show":
			return func(ch []*x509.Certificate) error {
				var n []string
				for _, c := range ch {
					n = append(n, c.Subject.CommonName)
				}
				return fmt.Errorf("refused: %s", strings.Join(n, " > "))
			}
		case "accept":
			return func([]*x509.Certificate) error { return nil }
		case "long":
			return func(ch []*x509.Certificate) error {
				if len(ch) > 1 {
					return fmt.Errorf("refused: %d long", len(ch))
				}
				return nil
			}
		}
		return nil
	}
	type cRow struct {
		name          string
		leaf          int
		inters, roots []entry
	}
	cRows := []cRow{
		{"root refuses", 4, nil, []entry{{0, "show"}}},
		{"root accepts", 4, nil, []entry{{0, "accept"}}},
		{"root refuses, another root of the name accepts", 4, nil, []entry{{0, "show"}, {1, ""}}},
		{"an expired root, then a root that refuses", 4, nil, []entry{{2, ""}, {0, "show"}}},
		{"a root that refuses, then an expired root", 4, nil, []entry{{0, "show"}, {2, ""}}},
		{"intermediate refuses", 5, []entry{{3, "show"}}, []entry{{0, ""}}},
		{"root refuses above the intermediate", 5, []entry{{3, ""}}, []entry{{0, "show"}}},
		{"root refuses a chain of two", 5, []entry{{3, ""}}, []entry{{0, "long"}}},
		{"root allows a chain of one", 4, nil, []entry{{0, "long"}}},
		{"a duplicate keeps the first constraint", 4, nil, []entry{{0, ""}, {0, "show"}}},
		{"a duplicate keeps the first constraint (refusing)", 4, nil, []entry{{0, "show"}, {0, "accept"}}},
	}
	o.WriteString("\n/// (name, leaf, intermediates and roots as (POOL_CERTS index, constraint), Go's Verify)\nconst CONSTRAINT_ROWS: List[(Str, Int, List[(Int, Str)], List[(Int, Str)], Str)] = [\n")
	ents := func(es []entry) string {
		var q []string
		for _, e := range es {
			q = append(q, fmt.Sprintf("(%d, %s)", e.cert, xq(e.con)))
		}
		return "[" + strings.Join(q, ", ") + "]"
	}
	for _, r := range cRows {
		ip, rp := x509.NewCertPool(), x509.NewCertPool()
		for _, e := range r.inters {
			if f := conOf(e.con); f != nil {
				ip.AddCertWithConstraint(certs[e.cert], f)
			} else {
				ip.AddCert(certs[e.cert])
			}
		}
		for _, e := range r.roots {
			if f := conOf(e.con); f != nil {
				rp.AddCertWithConstraint(certs[e.cert], f)
			} else {
				rp.AddCert(certs[e.cert])
			}
		}
		want := ""
		chains, err := certs[r.leaf].Verify(x509.VerifyOptions{Roots: rp, Intermediates: ip, CurrentTime: time.Unix(now, 0).UTC()})
		if err != nil {
			want = err.Error() + x509KindOf(err)
		} else {
			want = fmt.Sprintf("ok %d", len(chains))
		}
		fmt.Fprintf(o, "  (%s, %d, %s, %s, %s),\n", xq(r.name), r.leaf, ents(r.inters), ents(r.roots), xq(want))
	}
	o.WriteString("]\n")
	fmt.Fprintf(o, "\nconst POOL_NOW: Int = %d\n", now)
	o.WriteString(`
fn pool_cert(i: Int) -> x509.Certificate { x509.parse(POOL_CERTS[i]).ok() ?? fault("pool cert") }

fn pool_of(es: List[(Int, Str)]) -> x509.CertPool {
  var p = x509.cert_pool([]).ok() ?? fault("pool")
  for (i, con) in es {
    let c = pool_cert(i)
    if con == "" {
      p.add(c).ok() ?? fault("add")
    } else {
      let f = if con == "show" {
        fn(ch: List[x509.Certificate]) -> () ! x509.CertErr { Err(x509.CertErr{msg: "refused: ${ch.map(fn(x) { x.subject.common_name }).join(" > ")}"}) }
      } else if con == "accept" {
        fn(ch: List[x509.Certificate]) -> () ! x509.CertErr { Ok(()) }
      } else {
        fn(ch: List[x509.Certificate]) -> () ! x509.CertErr { if ch.len() > 1 { Err(x509.CertErr{msg: "refused: ${ch.len()} long"}) } else { Ok(()) } }
      }
      p.add_cert_with_constraint(c, f).ok() ?? fault("add with constraint")
    }
  }
  p
}

test "append_certs_from_pem is Go's AppendCertsFromPEM: its result and the certificates added" {
  var bad: List[Str] = []
  for (text, want, held) in PEM_ROWS {
    var p = x509.cert_pool([]).ok() ?? fault("pool")
    let ok = p.append_certs_from_pem(text.bytes())
    let want_pool = x509.cert_pool(held.map(fn(i) { pool_cert(i) })).ok() ?? fault("want")
    if ok != want || !p.equal(want_pool) || p.len() != held.len() {
      bad.push("${text.quote()}: got ${ok} ${p.len()}, want ${want} ${held}")
    }
  }
  expect(shown(bad)) == []
}

test "CertPool.equal is Go's Equal" {
  var bad: List[Str] = []
  for (a, b, want) in EQUAL_ROWS {
    let pa = x509.cert_pool(a.map(fn(i) { pool_cert(i) })).ok() ?? fault("a")
    let pb = x509.cert_pool(b.map(fn(i) { pool_cert(i) })).ok() ?? fault("b")
    if pa.equal(pb) != want {
      bad.push("${a} ${b}: want ${want}")
    }
  }
  expect(bad) == []
}

test "add_cert_with_constraint is Go's AddCertWithConstraint in verify: the chain it is given and its error as the hint" {
  var bad: List[Str] = []
  for (name, leaf, inters, roots, want) in CONSTRAINT_ROWS {
    let opts = x509.VerifyOptions{root_pool: Some(pool_of(roots)), intermediate_pool: Some(pool_of(inters)), at: Time.from_unix(POOL_NOW)}
    let got = match pool_cert(leaf).verify(opts) {
      Ok(chains) => "ok ${chains.len()}"
      Err(e) => "${e.display()}${kind_text(e.kind)}"
    }
    if got != want {
      bad.push("${name}:\n  got  ${got}\n  want ${want}")
    }
  }
  expect(shown(bad)) == []
}
`)
}

// ctRand is a reader of zero bytes for Go's signing: with main.go's
// cryptocustomrand=1 (Go 1.26 ignores a custom reader without it), ECDSA's
// hedged nonce and PSS's salt come out the same on every run, so a
// regenerate gives identical bytes (stdcorpus -check). A constant stream,
// because Go's randutil.MaybeReadByte skips a byte at random first.
func ctRand() io.Reader { return ctZeros{} }

type ctZeros struct{}

func (ctZeros) Read(b []byte) (int, error) {
	clear(b)
	return len(b), nil
}
