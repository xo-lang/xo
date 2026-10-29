package rt

// std/x509 on the Go backend (spec/std.md 41): Go's crypto/x509, its
// Certificate converted to the generated X509_Certificate.
// std/native/x509.xo is the native backends'.

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	cryptorand "crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/netip"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

func x509Err(m string) X509_CertErr { return X509_CertErr{F_msg: m} }

// x509Time is t as a Time, clamped to the times a Time holds (native's
// time_of).
func x509Time(t time.Time) Time {
	s := t.Unix()
	if s > 9223372035 {
		s = 9223372035
	} else if s < -9223372035 {
		s = -9223372035
	}
	return Time(s * 1e9)
}

func (e *X509_CertErr) M_display(*Task) string { return e.F_msg }

// pkixName parses n's DER as Go's parseName does.
func (n *X509_Name) pkixName() pkix.Name {
	var rdns pkix.RDNSequence
	var name pkix.Name
	if rest, err := asn1.Unmarshal([]byte(n.F_raw), &rdns); err == nil && len(rest) == 0 {
		name.FillFromRDNSequence(&rdns)
	}
	return name
}

func (n *X509_Name) M_display(*Task) string { return textOf(n.pkixName().String()) }

func (n *X509_Name) M_attributes(*Task) List[Tuple2[string, string]] {
	var out []Tuple2[string, string]
	for _, a := range n.pkixName().Names {
		out = append(out, Tuple2[string, string]{F0: a.Type.String(), F1: textOf(fmt.Sprint(a.Value))})
	}
	return ListFromSlice(out)
}

func x509Strs(xs []string) List[string] {
	out := make([]string, len(xs))
	for i, x := range xs {
		out[i] = textOf(x)
	}
	return ListFromSlice(out)
}

func x509NameOf(raw []byte, n pkix.Name) X509_Name {
	return X509_Name{
		F_country:             x509Strs(n.Country),
		F_organization:        x509Strs(n.Organization),
		F_organizational_unit: x509Strs(n.OrganizationalUnit),
		F_locality:            x509Strs(n.Locality),
		F_province:            x509Strs(n.Province),
		F_street_address:      x509Strs(n.StreetAddress),
		F_postal_code:         x509Strs(n.PostalCode),
		F_serial_number:       textOf(n.SerialNumber),
		F_common_name:         textOf(n.CommonName),
		F_raw:                 Bytes(raw),
	}
}

// x509Ekus are ExtKeyUsage's tags (1 based) in Go's x509.ExtKeyUsage order
// for the ones std/x509 names.
var x509Ekus = map[x509.ExtKeyUsage]int{
	x509.ExtKeyUsageAny:             1,
	x509.ExtKeyUsageServerAuth:      2,
	x509.ExtKeyUsageClientAuth:      3,
	x509.ExtKeyUsageCodeSigning:     4,
	x509.ExtKeyUsageEmailProtection: 5,
	x509.ExtKeyUsageTimeStamping:    6,
	x509.ExtKeyUsageOCSPSigning:     7,
}

// x509EkuOids are the OIDs of x509Ekus's tags.
var x509EkuOids = []string{"", "2.5.29.37.0", "1.3.6.1.5.5.7.3.1", "1.3.6.1.5.5.7.3.2", "1.3.6.1.5.5.7.3.3", "1.3.6.1.5.5.7.3.4", "1.3.6.1.5.5.7.3.8", "1.3.6.1.5.5.7.3.9"}

func x509EkuOf(oid string) X509_ExtKeyUsage {
	for tag, o := range x509EkuOids {
		if tag > 0 && o == oid {
			return X509_ExtKeyUsage{Tag: tag}
		}
	}
	return X509_ExtKeyUsage{Tag: 8, P: X509_ExtKeyUsage_Other{F_oid: oid}}
}

func x509CertOf(tk *Task, c *x509.Certificate) X509_Certificate {
	var ips []Netip_Addr
	for _, ip := range c.IPAddresses {
		if a, ok := netip.AddrFromSlice(ip); ok {
			ips = append(ips, netipOf(a))
		}
	}
	var uris []string
	for _, u := range c.URIs {
		uris = append(uris, textOf(u.String()))
	}
	var ku []X509_KeyUsage
	for i := 0; i < 9; i++ {
		if c.KeyUsage&(1<<i) != 0 {
			ku = append(ku, X509_KeyUsage{Tag: i + 1})
		}
	}
	// the extension's own order, as native keeps it (Go splits known
	// usages from unknown ones)
	var eku []X509_ExtKeyUsage
	for _, e := range c.Extensions {
		if e.Id.Equal(asn1.ObjectIdentifier{2, 5, 29, 37}) {
			var oids []asn1.ObjectIdentifier
			if _, err := asn1.Unmarshal(e.Value, &oids); err == nil {
				for _, o := range oids {
					eku = append(eku, x509EkuOf(o.String()))
				}
			}
		}
	}
	var exts []X509_Extension
	for _, e := range c.Extensions {
		exts = append(exts, X509_Extension{F_oid: e.Id.String(), F_critical: e.Critical, F_value: Bytes(e.Value)})
	}
	var pk Option[Crypto_PublicKey]
	if r := Crypto_parse_public_key(tk, Bytes(c.RawSubjectPublicKeyInfo)); r.Ok {
		pk = Some(r.V)
	}
	var mpl Option[int64]
	if c.BasicConstraintsValid && (c.MaxPathLen > 0 || c.MaxPathLenZero) {
		mpl = Some(int64(c.MaxPathLen))
	}
	serial := []byte{}
	if c.SerialNumber != nil {
		serial = c.SerialNumber.Bytes()
	}
	return X509_Certificate{
		F_raw:                 Bytes(c.Raw),
		F_version:             int64(c.Version),
		F_serial:              Bytes(serial),
		F_signature_algorithm: textOf(c.SignatureAlgorithm.String()),
		F_issuer:              x509NameOf(c.RawIssuer, c.Issuer),
		F_subject:             x509NameOf(c.RawSubject, c.Subject),
		F_not_before:          x509Time(c.NotBefore),
		F_not_after:           x509Time(c.NotAfter),
		F_public_key:          pk,
		F_dns_names:           x509Strs(c.DNSNames),
		F_ip_addresses:        ListFromSlice(ips),
		F_email_addresses:     x509Strs(c.EmailAddresses),
		F_uris:                ListFromSlice(uris),
		F_is_ca:               c.IsCA,
		F_max_path_len:        mpl,
		F_key_usage:           ListFromSlice(ku),
		F_ext_key_usage:       ListFromSlice(eku),
		F_subject_key_id:      Bytes(c.SubjectKeyId),
		F_authority_key_id:    Bytes(c.AuthorityKeyId),
		F_extensions:          ListFromSlice(exts),
	}
}

func X509_parse(tk *Task, der Bytes) Result[X509_Certificate, X509_CertErr] {
	c, err := x509.ParseCertificate([]byte(der))
	if err != nil {
		return Err[X509_Certificate](x509Err(textOf(err.Error())))
	}
	return Ok[X509_Certificate, X509_CertErr](x509CertOf(tk, c))
}

func X509_parse_pem(tk *Task, text Bytes) Result[List[X509_Certificate], X509_CertErr] {
	var out []X509_Certificate
	rest := []byte(text)
	for {
		b, r := pem.Decode(rest)
		if b == nil {
			break
		}
		rest = r
		if b.Type != "CERTIFICATE" {
			continue
		}
		c := X509_parse(tk, Bytes(b.Bytes))
		if !c.Ok {
			return Err[List[X509_Certificate]](c.E)
		}
		out = append(out, c.V)
	}
	return Ok[List[X509_Certificate], X509_CertErr](ListFromSlice(out))
}

func x509Go(c X509_Certificate) (*x509.Certificate, error) {
	return x509.ParseCertificate([]byte(c.F_raw))
}

func (c *X509_Certificate) M_verify(tk *Task, o X509_VerifyOptions) Result[List[List[X509_Certificate]], X509_CertErr] {
	leaf, err := x509Go(*c)
	if err != nil {
		return Err[List[List[X509_Certificate]]](x509Err(textOf(err.Error())))
	}
	roots := x509.NewCertPool()
	for _, r := range o.F_roots.s {
		g, err := x509Go(r)
		if err != nil {
			return Err[List[List[X509_Certificate]]](x509Err(textOf(err.Error())))
		}
		roots.AddCert(g)
	}
	inters := x509.NewCertPool()
	for _, r := range o.F_intermediates.s {
		g, err := x509Go(r)
		if err != nil {
			return Err[List[List[X509_Certificate]]](x509Err(textOf(err.Error())))
		}
		inters.AddCert(g)
	}
	var usages []x509.ExtKeyUsage
	for _, u := range o.F_key_usages.s {
		found := false
		for k, tag := range x509Ekus {
			if tag == u.Tag {
				usages = append(usages, k)
				found = true
			}
		}
		if !found {
			// an OID Go has no name for: Go's Verify cannot ask for it,
			// so it accepts any (native checks it)
			usages = append(usages, x509.ExtKeyUsageAny)
		}
	}
	if len(usages) == 0 {
		usages = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
	}
	chains, err := leaf.Verify(x509.VerifyOptions{
		Roots:         roots,
		Intermediates: inters,
		DNSName:       string(o.F_dns_name),
		CurrentTime:   time.Unix(0, int64(o.F_at)).UTC(),
		KeyUsages:     usages,
	})
	if err != nil {
		return Err[List[List[X509_Certificate]]](x509Err(textOf(err.Error())))
	}
	var out []List[X509_Certificate]
	for _, ch := range chains {
		var one []X509_Certificate
		for _, x := range ch {
			one = append(one, x509CertOf(tk, x))
		}
		out = append(out, ListFromSlice(one))
	}
	return Ok[List[List[X509_Certificate]], X509_CertErr](ListFromSlice(out))
}

func (c *X509_Certificate) M_verify_hostname(_ *Task, host string) Result[Unit, X509_CertErr] {
	g, err := x509Go(*c)
	if err == nil {
		err = g.VerifyHostname(string(host))
	}
	if err != nil {
		return Err[Unit](x509Err(textOf(err.Error())))
	}
	return Ok[Unit, X509_CertErr](Unit{})
}

func (c *X509_Certificate) M_check_signature_from(_ *Task, parent X509_Certificate) Result[Unit, X509_CertErr] {
	g, err := x509Go(*c)
	if err == nil {
		var p *x509.Certificate
		if p, err = x509Go(parent); err == nil {
			err = g.CheckSignatureFrom(p)
		}
	}
	if err != nil {
		return Err[Unit](x509Err(textOf(err.Error())))
	}
	return Ok[Unit, X509_CertErr](Unit{})
}

// x509RootPEMs are the roots crypto/tls verifies servers against, as PEM
// or DER texts (native's tlscert.root_ders): Go's root_unix.go files on
// Linux, the system root keychain's anchors on macOS (the security tool,
// as Go's root_darwin.go did before 1.18), none elsewhere.
func x509RootPEMs() [][]byte {
	switch runtime.GOOS {
	case "darwin":
		out, err := exec.Command("/usr/bin/security", "find-certificate", "-a", "-p", "/System/Library/Keychains/SystemRootCertificates.keychain").Output()
		if err != nil {
			return nil
		}
		return [][]byte{out}
	case "linux":
	default:
		return nil
	}
	var out [][]byte
	files := []string{"/etc/ssl/certs/ca-certificates.crt", "/etc/pki/tls/certs/ca-bundle.crt", "/etc/ssl/ca-bundle.pem", "/etc/pki/tls/cacert.pem", "/etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem", "/etc/ssl/cert.pem"}
	if f := os.Getenv("SSL_CERT_FILE"); f != "" {
		files = []string{f}
	}
	for _, f := range files {
		if b, err := os.ReadFile(f); err == nil {
			out = append(out, b)
			break
		}
	}
	dirs := "/etc/ssl/certs:/etc/pki/tls/certs"
	if d := os.Getenv("SSL_CERT_DIR"); d != "" {
		dirs = d
	}
	for _, d := range strings.Split(dirs, ":") {
		if d == "" {
			continue
		}
		es, err := os.ReadDir(d)
		if err != nil {
			continue
		}
		for _, e := range es {
			if strings.HasPrefix(e.Name(), ".") {
				continue
			}
			p := filepath.Join(d, e.Name())
			if t, err := os.Readlink(p); err == nil && !strings.Contains(t, "/") {
				continue
			}
			if fi, err := os.Stat(p); err != nil || !fi.Mode().IsRegular() {
				continue
			}
			if b, err := os.ReadFile(p); err == nil {
				out = append(out, b)
			}
		}
	}
	return out
}

func X509_system_roots(tk *Task, _ Fs) Result[List[X509_Certificate], X509_CertErr] {
	var out []X509_Certificate
	for _, text := range x509RootPEMs() {
		rest := text
		for {
			b, r := pem.Decode(rest)
			if b == nil {
				break
			}
			rest = r
			if b.Type != "CERTIFICATE" {
				continue
			}
			if c, err := x509.ParseCertificate(b.Bytes); err == nil {
				out = append(out, x509CertOf(tk, c))
			}
		}
	}
	return Ok[List[X509_Certificate], X509_CertErr](ListFromSlice(out))
}

// Tls_roots_of is `tls.roots_of(certs)`.
func Tls_roots_of(_ *Task, certs List[X509_Certificate]) Result[Tls_Roots, Tls_TlsErr] {
	pool := x509.NewCertPool()
	for _, c := range certs.s {
		if g, err := x509Go(c); err == nil {
			pool.AddCert(g)
		}
	}
	if len(certs.s) == 0 {
		return Err[Tls_Roots](Tls_TlsErr{F_msg: "tls: no certificates"})
	}
	return Ok[Tls_Roots, Tls_TlsErr](Tls_Roots{pool: pool})
}

// M_peer_certificates is `t.peer_certificates()`.
func (t Tls_TlsConn) M_peer_certificates(tk *Task) List[X509_Certificate] {
	if t.t == nil {
		return List[X509_Certificate]{}
	}
	var out []X509_Certificate
	for _, c := range t.t.ConnectionState().PeerCertificates {
		out = append(out, x509CertOf(tk, c))
	}
	return ListFromSlice(out)
}

// x509GoName is n as Go's pkix.Name for building (RawSubject is set
// apart, by the caller).
func x509GoName(n X509_Name) pkix.Name {
	strs := func(l List[string]) []string {
		out := make([]string, len(l.s))
		for i, s := range l.s {
			out[i] = string(s)
		}
		return out
	}
	return pkix.Name{
		Country: strs(n.F_country), Organization: strs(n.F_organization), OrganizationalUnit: strs(n.F_organizational_unit),
		Locality: strs(n.F_locality), Province: strs(n.F_province), StreetAddress: strs(n.F_street_address),
		PostalCode: strs(n.F_postal_code), SerialNumber: string(n.F_serial_number), CommonName: string(n.F_common_name),
	}
}

func x509Template(t X509_Template) (*x509.Certificate, error) {
	g := &x509.Certificate{
		Subject:      x509GoName(t.F_subject),
		RawSubject:   []byte(t.F_subject.F_raw),
		NotBefore:    time.Unix(0, int64(t.F_not_before)),
		NotAfter:     time.Unix(0, int64(t.F_not_after)),
		IsCA:         t.F_is_ca,
		SubjectKeyId: []byte(t.F_subject_key_id),
		MaxPathLen:   -1,
	}
	if len(t.F_serial) > 0 {
		g.SerialNumber = new(big.Int).SetBytes([]byte(t.F_serial))
	}
	for _, d := range t.F_dns_names.s {
		g.DNSNames = append(g.DNSNames, string(d))
	}
	for _, e := range t.F_email_addresses.s {
		g.EmailAddresses = append(g.EmailAddresses, string(e))
	}
	for _, a := range t.F_ip_addresses.s {
		g.IPAddresses = append(g.IPAddresses, net.IP(a.goAddr().AsSlice()))
	}
	for _, u := range t.F_uris.s {
		p, err := url.Parse(string(u))
		if err != nil {
			return nil, err
		}
		g.URIs = append(g.URIs, p)
	}
	if t.F_is_ca || t.F_max_path_len.Some {
		g.BasicConstraintsValid = true
	}
	if t.F_max_path_len.Some {
		g.MaxPathLen = int(t.F_max_path_len.V)
		g.MaxPathLenZero = t.F_max_path_len.V == 0
	}
	for _, u := range t.F_key_usage.s {
		g.KeyUsage |= x509.KeyUsage(1 << (u.Tag - 1))
	}
	for _, u := range t.F_ext_key_usage.s {
		known := false
		for k, tag := range x509Ekus {
			if tag == u.Tag {
				g.ExtKeyUsage = append(g.ExtKeyUsage, k)
				known = true
			}
		}
		if !known {
			oid, err := x509ParseOID(string(u.P.(X509_ExtKeyUsage_Other).F_oid))
			if err != nil {
				return nil, errors.New("x509: unknown extended key usage")
			}
			g.UnknownExtKeyUsage = append(g.UnknownExtKeyUsage, oid)
		}
	}
	for _, e := range t.F_extra_extensions.s {
		oid, err := x509ParseOID(string(e.F_oid))
		if err != nil {
			return nil, fmt.Errorf("x509: invalid extension OID %q", string(e.F_oid))
		}
		g.ExtraExtensions = append(g.ExtraExtensions, pkix.Extension{Id: oid, Critical: e.F_critical, Value: []byte(e.F_value)})
	}
	return g, nil
}

func x509ParseOID(s string) (asn1.ObjectIdentifier, error) {
	var oid asn1.ObjectIdentifier
	for _, p := range strings.Split(s, ".") {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return nil, errors.New("bad OID")
		}
		oid = append(oid, n)
	}
	if len(oid) < 2 {
		return nil, errors.New("bad OID")
	}
	return oid, nil
}

func X509_create_certificate(tk *Task, r Rand, t X509_Template, pub Crypto_PublicKey, parent Option[X509_Certificate], key Crypto_PrivateKey) Result[X509_Certificate, X509_CertErr] {
	if _, ok := key.k.(*ecdsa.PrivateKey); ok && key.k.(*ecdsa.PrivateKey).Curve == elliptic.P224() {
		return Err[X509_Certificate](x509Err("x509: unknown elliptic curve"))
	}
	tmpl, err := x509Template(t)
	if err != nil {
		return Err[X509_Certificate](x509Err(textOf(err.Error())))
	}
	par := tmpl
	if parent.Some {
		if par, err = x509Go(parent.V); err != nil {
			return Err[X509_Certificate](x509Err(textOf(err.Error())))
		}
	}
	// r is read as the native code reads it: the serial's 20 bytes, then
	// an ECDSA signature's hedge (Go's signer uses crypto/rand)
	if tmpl.SerialNumber == nil {
		b := []byte(r.M_bytes(tk, 20))
		b[0] &= 0x7f
		tmpl.SerialNumber = new(big.Int).SetBytes(b)
	}
	if k, ok := key.k.(*ecdsa.PrivateKey); ok {
		r.M_bytes(tk, int64((k.Curve.Params().N.BitLen()+7)/8))
	}
	der, err := x509.CreateCertificate(cryptorand.Reader, tmpl, par, pub.k, key.k)
	if err != nil {
		return Err[X509_Certificate](x509Err(textOf(err.Error())))
	}
	return X509_parse(tk, Bytes(der))
}

func (c *X509_Certificate) M_pem(*Task) Bytes {
	return Bytes(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte(c.F_raw)}))
}

func X509_self_signed(tk *Task, r Rand, hosts List[string], now Time, days int64) Result[Tuple2[Bytes, Bytes], X509_CertErr] {
	kr := Crypto_generate_key(tk, r, Crypto_KeyAlg{Tag: 3})
	if !kr.Ok {
		return Err[Tuple2[Bytes, Bytes]](x509Err(kr.E.F_msg))
	}
	key := kr.V
	var t X509_Template
	t.F_subject.F_organization = ListOf[string]("Xo Development")
	t.F_not_before = now
	t.F_not_after = Time(int64(now) + days*86400*1e9)
	var dns []string
	var ips []Netip_Addr
	for _, h := range hosts.s {
		if a, err := netip.ParseAddr(string(h)); err == nil {
			ips = append(ips, netipOf(a))
		} else {
			dns = append(dns, h)
		}
	}
	t.F_dns_names = ListFromSlice(dns)
	t.F_ip_addresses = ListFromSlice(ips)
	t.F_key_usage = ListOf(X509_KeyUsage{Tag: 1})
	t.F_ext_key_usage = ListOf(X509_ExtKeyUsage{Tag: 2})
	c := X509_create_certificate(tk, r, t, key.pub, None[X509_Certificate](), key)
	if !c.Ok {
		return Err[Tuple2[Bytes, Bytes]](c.E)
	}
	kp := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte(Crypto_marshal_private_key(tk, key))})
	return Ok[Tuple2[Bytes, Bytes], X509_CertErr](Tuple2[Bytes, Bytes]{F0: c.V.M_pem(tk), F1: Bytes(kp)})
}

func x509Exts(es []pkix.Extension) List[X509_Extension] {
	var out []X509_Extension
	for _, e := range es {
		out = append(out, X509_Extension{F_oid: e.Id.String(), F_critical: e.Critical, F_value: Bytes(e.Value)})
	}
	return ListFromSlice(out)
}

func X509_parse_revocation_list(tk *Task, der Bytes) Result[X509_RevocationList, X509_CertErr] {
	l, err := x509.ParseRevocationList([]byte(der))
	if err != nil {
		return Err[X509_RevocationList](x509Err(textOf(err.Error())))
	}
	var revoked []X509_RevokedCertificate
	for _, e := range l.RevokedCertificateEntries {
		revoked = append(revoked, X509_RevokedCertificate{
			F_serial: Bytes(e.SerialNumber.Bytes()), F_revoked_at: x509Time(e.RevocationTime),
			F_reason: int64(e.ReasonCode), F_extensions: x509Exts(e.Extensions),
		})
	}
	var next Option[Time]
	if !l.NextUpdate.IsZero() {
		next = Some(x509Time(l.NextUpdate))
	}
	number := []byte{}
	if l.Number != nil {
		number = l.Number.Bytes()
	}
	return Ok[X509_RevocationList, X509_CertErr](X509_RevocationList{
		F_raw: Bytes(l.Raw), F_signature_algorithm: textOf(l.SignatureAlgorithm.String()),
		F_issuer: x509NameOf(l.RawIssuer, l.Issuer), F_number: Bytes(number),
		F_this_update: x509Time(l.ThisUpdate), F_next_update: next,
		F_revoked: ListFromSlice(revoked), F_authority_key_id: Bytes(l.AuthorityKeyId),
		F_extensions: x509Exts(l.Extensions),
	})
}

func (l *X509_RevocationList) M_check_signature_from(_ *Task, issuer X509_Certificate) Result[Unit, X509_CertErr] {
	g, err := x509.ParseRevocationList([]byte(l.F_raw))
	if err == nil {
		var p *x509.Certificate
		if p, err = x509Go(issuer); err == nil {
			err = g.CheckSignatureFrom(p)
		}
	}
	if err != nil {
		return Err[Unit](x509Err(textOf(err.Error())))
	}
	return Ok[Unit, X509_CertErr](Unit{})
}

func x509RequestOf(tk *Task, q *x509.CertificateRequest) X509_CertificateRequest {
	var ips []Netip_Addr
	for _, ip := range q.IPAddresses {
		if a, ok := netip.AddrFromSlice(ip); ok {
			ips = append(ips, netipOf(a))
		}
	}
	var uris []string
	for _, u := range q.URIs {
		uris = append(uris, textOf(u.String()))
	}
	var pk Option[Crypto_PublicKey]
	if r := Crypto_parse_public_key(tk, Bytes(q.RawSubjectPublicKeyInfo)); r.Ok {
		pk = Some(r.V)
	}
	return X509_CertificateRequest{
		F_raw: Bytes(q.Raw), F_version: int64(q.Version), F_signature_algorithm: textOf(q.SignatureAlgorithm.String()),
		F_subject: x509NameOf(q.RawSubject, q.Subject), F_public_key: pk,
		F_dns_names: x509Strs(q.DNSNames), F_ip_addresses: ListFromSlice(ips),
		F_email_addresses: x509Strs(q.EmailAddresses), F_uris: ListFromSlice(uris),
		F_extensions: x509Exts(q.Extensions),
	}
}

func X509_parse_request(tk *Task, der Bytes) Result[X509_CertificateRequest, X509_CertErr] {
	q, err := x509.ParseCertificateRequest([]byte(der))
	if err != nil {
		msg := err.Error()
		if strings.HasPrefix(msg, "asn1: ") {
			msg = "x509: malformed certificate request"
		}
		return Err[X509_CertificateRequest](x509Err(textOf(msg)))
	}
	return Ok[X509_CertificateRequest, X509_CertErr](x509RequestOf(tk, q))
}

func (q *X509_CertificateRequest) M_check_signature(*Task) Result[Unit, X509_CertErr] {
	g, err := x509.ParseCertificateRequest([]byte(q.F_raw))
	if err == nil {
		err = g.CheckSignature()
	}
	if err != nil {
		return Err[Unit](x509Err(textOf(err.Error())))
	}
	return Ok[Unit, X509_CertErr](Unit{})
}

func (q *X509_CertificateRequest) M_pem(*Task) Bytes {
	return Bytes(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: []byte(q.F_raw)}))
}

func X509_create_request(tk *Task, r Rand, t X509_RequestTemplate, key Crypto_PrivateKey) Result[X509_CertificateRequest, X509_CertErr] {
	if k, ok := key.k.(*ecdsa.PrivateKey); ok {
		if k.Curve == elliptic.P224() {
			return Err[X509_CertificateRequest](x509Err("x509: unknown elliptic curve"))
		}
		r.M_bytes(tk, int64((k.Curve.Params().N.BitLen()+7)/8))
	}
	ct, err := x509Template(X509_Template{F_subject: t.F_subject, F_dns_names: t.F_dns_names, F_ip_addresses: t.F_ip_addresses,
		F_email_addresses: t.F_email_addresses, F_uris: t.F_uris, F_extra_extensions: t.F_extra_extensions})
	if err != nil {
		return Err[X509_CertificateRequest](x509Err(textOf(err.Error())))
	}
	tmpl := &x509.CertificateRequest{Subject: ct.Subject, RawSubject: ct.RawSubject, DNSNames: ct.DNSNames, EmailAddresses: ct.EmailAddresses,
		IPAddresses: ct.IPAddresses, URIs: ct.URIs, ExtraExtensions: ct.ExtraExtensions}
	der, err := x509.CreateCertificateRequest(cryptorand.Reader, tmpl, key.k)
	if err != nil {
		return Err[X509_CertificateRequest](x509Err(textOf(err.Error())))
	}
	return X509_parse_request(tk, Bytes(der))
}
