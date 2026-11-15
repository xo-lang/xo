package rt

// std/x509 on the Go backend (spec/std.md 41): Go's crypto/x509, its
// Certificate converted to the generated X509_Certificate.
// std/native/x509.xo is the native backends'.

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	cryptorand "crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	mbits "math/bits"
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

// x509Err is an untyped error (kind Other: enum tags start at 1).
func x509Err(m string) X509_CertErr { return X509_CertErr{F_msg: m, F_kind: X509_CertErrKind{Tag: 1}} }

// x509ErrOf is err with Go's text and its error type as the kind
// (decision 0643).
func x509ErrOf(err error) X509_CertErr {
	e := x509Err(textOf(err.Error()))
	var ci x509.CertificateInvalidError
	var he x509.HostnameError
	var ua x509.UnknownAuthorityError
	var ia x509.InsecureAlgorithmError
	var cv x509.ConstraintViolationError
	var uc x509.UnhandledCriticalExtension
	switch {
	case errors.As(err, &ci):
		e.F_kind = X509_CertErrKind{Tag: 2, P: X509_CertErrKind_CertificateInvalid{F_reason: X509_InvalidReason{Tag: int(ci.Reason) + 1}, F_detail: textOf(ci.Detail)}}
	case errors.As(err, &he):
		e.F_kind = X509_CertErrKind{Tag: 3, P: X509_CertErrKind_Hostname{F_host: textOf(he.Host)}}
	case errors.As(err, &ua):
		e.F_kind = X509_CertErrKind{Tag: 4}
	case errors.As(err, &ia):
		e.F_kind = X509_CertErrKind{Tag: 5, P: X509_CertErrKind_InsecureAlgorithm{F_algorithm: textOf(x509.SignatureAlgorithm(ia).String())}}
	case errors.As(err, &cv):
		e.F_kind = X509_CertErrKind{Tag: 6}
	case errors.As(err, &uc):
		e.F_kind = X509_CertErrKind{Tag: 7}
	}
	return e
}

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

		F_raw_tbs_certificate:            Bytes(c.RawTBSCertificate),
		F_raw_subject_public_key_info:    Bytes(c.RawSubjectPublicKeyInfo),
		F_signature:                      Bytes(c.Signature),
		F_public_key_algorithm:           c.PublicKeyAlgorithm.String(),
		F_basic_constraints_valid:        c.BasicConstraintsValid,
		F_ocsp_servers:                   x509Strs(c.OCSPServer),
		F_issuing_certificate_urls:       x509Strs(c.IssuingCertificateURL),
		F_crl_distribution_points:        x509Strs(c.CRLDistributionPoints),
		F_permitted_dns_domains_critical: c.PermittedDNSDomainsCritical,
		F_permitted_dns_domains:          x509Strs(c.PermittedDNSDomains),
		F_excluded_dns_domains:           x509Strs(c.ExcludedDNSDomains),
		F_permitted_ip_ranges:            x509Ranges(tk, c.PermittedIPRanges),
		F_excluded_ip_ranges:             x509Ranges(tk, c.ExcludedIPRanges),
		F_permitted_email_addresses:      x509Strs(c.PermittedEmailAddresses),
		F_excluded_email_addresses:       x509Strs(c.ExcludedEmailAddresses),
		F_permitted_uri_domains:          x509Strs(c.PermittedURIDomains),
		F_excluded_uri_domains:           x509Strs(c.ExcludedURIDomains),
		F_policies:                       x509OIDs(c.Policies),
		F_policy_mappings:                x509Mappings(c.PolicyMappings),
		F_inhibit_any_policy:             x509Constraint(c.InhibitAnyPolicy, c.InhibitAnyPolicyZero),
		F_inhibit_policy_mapping:         x509Constraint(c.InhibitPolicyMapping, c.InhibitPolicyMappingZero),
		F_require_explicit_policy:        x509Constraint(c.RequireExplicitPolicy, c.RequireExplicitPolicyZero),
		F_unhandled_critical_extensions:  x509ASN1OIDs(c.UnhandledCriticalExtensions),
	}
}

// x509Ranges are name constraint IP ranges as prefixes (a mask's ones
// counted, as the native code reads them).
func x509Ranges(tk *Task, ns []*net.IPNet) List[Netip_Prefix] {
	var out []Netip_Prefix
	for _, n := range ns {
		a, ok := netip.AddrFromSlice(n.IP)
		if !ok {
			continue
		}
		bits := 0
		for _, m := range n.Mask {
			bits += mbits.OnesCount8(m)
		}
		if p := Netip_prefix_from(tk, netipOf(a), int64(bits)); p.Some {
			out = append(out, p.V)
		}
	}
	return ListFromSlice(out)
}

func x509OIDs(os []x509.OID) List[string] {
	var out []string
	for _, o := range os {
		out = append(out, o.String())
	}
	return ListFromSlice(out)
}

func x509ASN1OIDs(os []asn1.ObjectIdentifier) List[string] {
	var out []string
	for _, o := range os {
		out = append(out, o.String())
	}
	return ListFromSlice(out)
}

func x509Mappings(ms []x509.PolicyMapping) List[X509_PolicyMapping] {
	var out []X509_PolicyMapping
	for _, m := range ms {
		out = append(out, X509_PolicyMapping{F_issuer_domain_policy: m.IssuerDomainPolicy.String(), F_subject_domain_policy: m.SubjectDomainPolicy.String()})
	}
	return ListFromSlice(out)
}

// x509Constraint is a policy constraint as an Option: Go's int with its
// *Zero flag (0 and false: absent).
func x509Constraint(n int, zero bool) Option[int64] {
	if n == 0 && !zero {
		return None[int64]()
	}
	return Some(int64(n))
}

func X509_parse(tk *Task, der Bytes) Result[X509_Certificate, X509_CertErr] {
	c, err := x509.ParseCertificate([]byte(der))
	if err != nil {
		return Err[X509_Certificate](x509ErrOf(err))
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

// X509_CertPool is `x509.CertPool` (decision 0626): certificates parsed
// once. The store is shared and only appended to; a pool is a view of
// its first n entries, so a copy that adds after another copy did
// copies the store first (Xo values do not alias).
type X509_CertPool struct {
	d *x509PoolData
	n int
}

type x509PoolData struct {
	certs []X509_Certificate
	gos   []*x509.Certificate
	at    map[string]int
	// AddCertWithConstraint's constraints, by entry (nil: none)
	cons []func(*Task, List[X509_Certificate]) Result[Unit, X509_CertErr]
}

func (X509_CertPool) XoDebug(b *strings.Builder) { b.WriteString("<certpool>") }

func X509_cert_pool(tk *Task, certs List[X509_Certificate]) Result[X509_CertPool, X509_CertErr] {
	var p X509_CertPool
	for _, c := range certs.s {
		if r := p.M_add(tk, c); !r.Ok {
			return Err[X509_CertPool](r.E)
		}
	}
	return Ok[X509_CertPool, X509_CertErr](p)
}

func (p *X509_CertPool) M_add(_ *Task, c X509_Certificate) Result[Unit, X509_CertErr] {
	if p.d != nil {
		if i, ok := p.d.at[string(c.F_raw)]; ok && i < p.n {
			return Ok[Unit, X509_CertErr](Unit{})
		}
	}
	g, err := x509Go(c)
	if err != nil {
		return Err[Unit](x509ErrOf(err))
	}
	if p.d == nil || p.n != len(p.d.certs) {
		d := &x509PoolData{at: map[string]int{}}
		if p.d != nil {
			d.certs = append([]X509_Certificate(nil), p.d.certs[:p.n]...)
			d.gos = append([]*x509.Certificate(nil), p.d.gos[:p.n]...)
			d.cons = append(d.cons[:0:0], p.d.cons[:p.n]...)
			for i, x := range d.certs {
				d.at[string(x.F_raw)] = i
			}
		}
		p.d = d
	}
	p.d.at[string(c.F_raw)] = len(p.d.certs)
	p.d.certs = append(p.d.certs, c)
	p.d.gos = append(p.d.gos, g)
	p.d.cons = append(p.d.cons, nil)
	p.n++
	return Ok[Unit, X509_CertErr](Unit{})
}

func (p *X509_CertPool) M_add_cert_with_constraint(tk *Task, c X509_Certificate, f func(*Task, List[X509_Certificate]) Result[Unit, X509_CertErr]) Result[Unit, X509_CertErr] {
	if p.d != nil {
		if i, ok := p.d.at[string(c.F_raw)]; ok && i < p.n {
			return Ok[Unit, X509_CertErr](Unit{})
		}
	}
	if r := p.M_add(tk, c); !r.Ok {
		return r
	}
	p.d.cons[p.n-1] = f
	return Ok[Unit, X509_CertErr](Unit{})
}

func (p *X509_CertPool) M_append_certs_from_pem(tk *Task, pemCerts Bytes) bool {
	// Go's AppendCertsFromPEM, adding through M_add
	ok := false
	rest := []byte(pemCerts)
	for len(rest) > 0 {
		var b *pem.Block
		b, rest = pem.Decode(rest)
		if b == nil {
			break
		}
		if b.Type != "CERTIFICATE" || len(b.Headers) != 0 {
			continue
		}
		if _, err := x509.ParseCertificate(b.Bytes); err != nil {
			continue
		}
		r := X509_parse(tk, Bytes(b.Bytes))
		if r.Ok && p.M_add(tk, r.V).Ok {
			ok = true
		}
	}
	return ok
}

func (p *X509_CertPool) M_equal(tk *Task, o X509_CertPool) bool {
	// Go's Equal, by the certificates' DER
	g := func(q *X509_CertPool) *x509.CertPool {
		gp := x509.NewCertPool()
		if q.d != nil {
			for _, x := range q.d.gos[:q.n] {
				gp.AddCert(x)
			}
		}
		return gp
	}
	return g(p).Equal(g(&o))
}

func (p *X509_CertPool) M_certificates(*Task) List[X509_Certificate] {
	if p.d == nil {
		return ListFromSlice[X509_Certificate](nil)
	}
	return ListFromSlice(append([]X509_Certificate(nil), p.d.certs[:p.n]...))
}

func (p *X509_CertPool) M_len(*Task) int64 { return int64(p.n) }

// x509PoolAdd adds a pool's parsed certificates to a Go pool.
func x509PoolAdd(tk *Task, to *x509.CertPool, p Option[X509_CertPool]) {
	if p.Some && p.V.d != nil {
		for i, g := range p.V.d.gos[:p.V.n] {
			f := p.V.d.cons[i]
			if f == nil {
				to.AddCert(g)
				continue
			}
			to.AddCertWithConstraint(g, func(chain []*x509.Certificate) error {
				cs := make([]X509_Certificate, 0, len(chain))
				for _, x := range chain {
					cs = append(cs, x509CertOf(tk, x))
				}
				if r := f(tk, ListFromSlice(cs)); !r.Ok {
					return errors.New(string(r.E.F_msg))
				}
				return nil
			})
		}
	}
}

func (c *X509_Certificate) M_verify(tk *Task, o X509_VerifyOptions) Result[List[List[X509_Certificate]], X509_CertErr] {
	leaf, err := x509Go(*c)
	if err != nil {
		return Err[List[List[X509_Certificate]]](x509ErrOf(err))
	}
	roots := x509.NewCertPool()
	x509PoolAdd(tk, roots, o.F_root_pool)
	for _, r := range o.F_roots.s {
		g, err := x509Go(r)
		if err != nil {
			return Err[List[List[X509_Certificate]]](x509ErrOf(err))
		}
		roots.AddCert(g)
	}
	inters := x509.NewCertPool()
	x509PoolAdd(tk, inters, o.F_intermediate_pool)
	for _, r := range o.F_intermediates.s {
		g, err := x509Go(r)
		if err != nil {
			return Err[List[List[X509_Certificate]]](x509ErrOf(err))
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
	var policies []x509.OID
	for _, p := range o.F_certificate_policies.s {
		var g x509.OID
		if len(p.F_der) > 0 {
			if err := g.UnmarshalBinary([]byte(p.F_der)); err != nil {
				return Err[List[List[X509_Certificate]]](x509Err("invalid oid"))
			}
		}
		policies = append(policies, g)
	}
	chains, err := leaf.Verify(x509.VerifyOptions{
		Roots:               roots,
		Intermediates:       inters,
		DNSName:             string(o.F_dns_name),
		CurrentTime:         time.Unix(0, int64(o.F_at)).UTC(),
		KeyUsages:           usages,
		CertificatePolicies: policies,
	})
	if err != nil {
		return Err[List[List[X509_Certificate]]](x509ErrOf(err))
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
		return Err[Unit](x509ErrOf(err))
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
		return Err[Unit](x509ErrOf(err))
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
	if t.F_is_ca || t.F_max_path_len.Some || t.F_basic_constraints_valid {
		g.BasicConstraintsValid = true
	}
	g.OCSPServer = x509GoStrs(t.F_ocsp_servers)
	g.IssuingCertificateURL = x509GoStrs(t.F_issuing_certificate_urls)
	g.CRLDistributionPoints = x509GoStrs(t.F_crl_distribution_points)
	g.PermittedDNSDomainsCritical = t.F_permitted_dns_domains_critical
	g.PermittedDNSDomains = x509GoStrs(t.F_permitted_dns_domains)
	g.ExcludedDNSDomains = x509GoStrs(t.F_excluded_dns_domains)
	g.PermittedIPRanges = x509IPNets(t.F_permitted_ip_ranges)
	g.ExcludedIPRanges = x509IPNets(t.F_excluded_ip_ranges)
	g.PermittedEmailAddresses = x509GoStrs(t.F_permitted_email_addresses)
	g.ExcludedEmailAddresses = x509GoStrs(t.F_excluded_email_addresses)
	g.PermittedURIDomains = x509GoStrs(t.F_permitted_uri_domains)
	g.ExcludedURIDomains = x509GoStrs(t.F_excluded_uri_domains)
	for _, o := range t.F_policies.s {
		oid, err := x509.ParseOID(string(o))
		if err != nil {
			return nil, errors.New("invalid policy object identifier")
		}
		g.Policies = append(g.Policies, oid)
	}
	if name := string(t.F_signature_algorithm); name != "" {
		a, err := x509SigAlg(name)
		if err != nil {
			return nil, err
		}
		g.SignatureAlgorithm = a
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

// x509SigAlg is the SignatureAlgorithm Go names name.
func x509SigAlg(name string) (x509.SignatureAlgorithm, error) {
	for a := x509.SignatureAlgorithm(1); a < 64; a++ {
		if a.String() == name {
			return a, nil
		}
	}
	return 0, errors.New("x509: unknown SignatureAlgorithm")
}

// x509ReadSalt reads r as the native code does for a PSS signature: its
// salt, the hash's size.
func x509ReadSalt(tk *Task, r Rand, a x509.SignatureAlgorithm) {
	switch a {
	case x509.SHA256WithRSAPSS:
		r.M_bytes(tk, 32)
	case x509.SHA384WithRSAPSS:
		r.M_bytes(tk, 48)
	case x509.SHA512WithRSAPSS:
		r.M_bytes(tk, 64)
	}
}

func x509GoStrs(l List[string]) []string {
	var out []string
	for _, x := range l.s {
		out = append(out, string(x))
	}
	return out
}

// x509IPNets is Go's form of prefixes: the address in its own length (4
// bytes for IPv4) and the mask of that length.
func x509IPNets(l List[Netip_Prefix]) []*net.IPNet {
	var out []*net.IPNet
	for _, p := range l.s {
		a := p.F_addr.goAddr()
		out = append(out, &net.IPNet{IP: net.IP(a.AsSlice()), Mask: net.CIDRMask(int(p.F_len), a.BitLen())})
	}
	return out
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
		return Err[X509_Certificate](x509ErrOf(err))
	}
	par := tmpl
	if parent.Some {
		if par, err = x509Go(parent.V); err != nil {
			return Err[X509_Certificate](x509ErrOf(err))
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
	x509ReadSalt(tk, r, tmpl.SignatureAlgorithm)
	der, err := x509.CreateCertificate(cryptorand.Reader, tmpl, par, pub.k, key.k)
	if err != nil {
		return Err[X509_Certificate](x509ErrOf(err))
	}
	return X509_parse(tk, Bytes(der))
}

func (c *X509_Certificate) M_pem(*Task) Bytes {
	return Bytes(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte(c.F_raw)}))
}

func X509_self_signed(tk *Task, r Rand, hosts List[string], now Time, days int64) Result[Tuple2[Bytes, Bytes], X509_CertErr] {
	kr := Crypto_generate_key(tk, r, Crypto_KeyAlg{Tag: 3}, 2048)
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
		return Err[X509_RevocationList](x509ErrOf(err))
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

// X509_create_revocation_list is Go's CreateRevocationList of t, issued
// by issuer and signed with key.
func X509_create_revocation_list(tk *Task, r Rand, t X509_RevocationListTemplate, issuer X509_Certificate, key Crypto_PrivateKey) Result[X509_RevocationList, X509_CertErr] {
	fail := func(err error) Result[X509_RevocationList, X509_CertErr] {
		return Err[X509_RevocationList](x509ErrOf(err))
	}
	var iss *x509.Certificate
	if len(issuer.F_raw) > 0 {
		var err error
		if iss, err = x509Go(issuer); err != nil {
			return fail(err)
		}
	} else {
		// a Certificate built in Xo: the fields CreateRevocationList reads
		iss = &x509.Certificate{Subject: x509GoName(issuer.F_subject), RawSubject: []byte(issuer.F_subject.F_raw), SubjectKeyId: []byte(issuer.F_subject_key_id)}
		for _, u := range issuer.F_key_usage.s {
			iss.KeyUsage |= x509.KeyUsage(1) << uint(u.Tag-1)
		}
	}
	tmpl := &x509.RevocationList{
		Number:     new(big.Int).SetBytes([]byte(t.F_number)),
		ThisUpdate: time.Unix(0, int64(t.F_this_update)),
		NextUpdate: time.Unix(0, int64(t.F_next_update)),
	}
	if name := string(t.F_signature_algorithm); name != "" {
		a, err := x509SigAlg(name)
		if err != nil {
			return fail(err)
		}
		tmpl.SignatureAlgorithm = a
	}
	for _, e := range t.F_revoked.s {
		ent := x509.RevocationListEntry{SerialNumber: new(big.Int).SetBytes([]byte(e.F_serial)), RevocationTime: time.Unix(0, int64(e.F_revoked_at)), ReasonCode: int(e.F_reason)}
		for _, x := range e.F_extensions.s {
			oid, err := x509ParseOID(string(x.F_oid))
			if err != nil {
				return fail(fmt.Errorf("x509: invalid extension OID %q", string(x.F_oid)))
			}
			ent.ExtraExtensions = append(ent.ExtraExtensions, pkix.Extension{Id: oid, Critical: x.F_critical, Value: []byte(x.F_value)})
		}
		tmpl.RevokedCertificateEntries = append(tmpl.RevokedCertificateEntries, ent)
	}
	for _, x := range t.F_extra_extensions.s {
		oid, err := x509ParseOID(string(x.F_oid))
		if err != nil {
			return fail(fmt.Errorf("x509: invalid extension OID %q", string(x.F_oid)))
		}
		tmpl.ExtraExtensions = append(tmpl.ExtraExtensions, pkix.Extension{Id: oid, Critical: x.F_critical, Value: []byte(x.F_value)})
	}
	signer, ok := key.k.(crypto.Signer)
	if !ok {
		return fail(errors.New("x509: only RSA, ECDSA and Ed25519 keys supported"))
	}
	if k, ok := key.k.(*ecdsa.PrivateKey); ok {
		if k.Curve == elliptic.P224() {
			return fail(errors.New("x509: unknown elliptic curve"))
		}
		// r is read as the native code reads it: an ECDSA signature's hedge
		r.M_bytes(tk, int64((k.Curve.Params().N.BitLen()+7)/8))
	}
	x509ReadSalt(tk, r, tmpl.SignatureAlgorithm)
	der, err := x509.CreateRevocationList(cryptorand.Reader, tmpl, iss, signer)
	if err != nil {
		return fail(err)
	}
	return X509_parse_revocation_list(tk, Bytes(der))
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
		return Err[Unit](x509ErrOf(err))
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
		return Err[Unit](x509ErrOf(err))
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
		return Err[X509_CertificateRequest](x509ErrOf(err))
	}
	tmpl := &x509.CertificateRequest{Subject: ct.Subject, RawSubject: ct.RawSubject, DNSNames: ct.DNSNames, EmailAddresses: ct.EmailAddresses,
		IPAddresses: ct.IPAddresses, URIs: ct.URIs, ExtraExtensions: ct.ExtraExtensions}
	der, err := x509.CreateCertificateRequest(cryptorand.Reader, tmpl, key.k)
	if err != nil {
		return Err[X509_CertificateRequest](x509ErrOf(err))
	}
	return X509_parse_request(tk, Bytes(der))
}

// X509_parse_pkcs12 is x509.parse_pkcs12 on the vendored x/crypto/pkcs12 (xcrypto_pkcs12.go): any
// number of bags, the leaf the certificate whose localKeyID names the key.
func X509_parse_pkcs12(tk *Task, data Bytes, password string) Result[X509_Pkcs12, X509_CertErr] {
	fail := func(msg string) Result[X509_Pkcs12, X509_CertErr] {
		return Err[X509_Pkcs12](x509Err(textOf(msg)))
	}
	enc, err := xp12_bmpString(password)
	if err != nil {
		return fail(xp12_ErrIncorrectPassword.Error())
	}
	bags, pw, err := xp12_getSafeContents([]byte(data), enc)
	if err != nil {
		return fail(err.Error())
	}
	var certs []*x509.Certificate
	var ids [][]byte
	var keyDER []byte
	var keyID []byte
	for _, bag := range bags {
		var id []byte
		for _, a := range bag.Attributes {
			if a.Id.Equal(xp12_oidLocalKeyID) {
				var v []byte
				if _, e := asn1.Unmarshal(a.Value.Bytes, &v); e == nil {
					id = v
				}
			}
		}
		switch {
		case bag.Id.Equal(xp12_oidCertBag):
			der, err := xp12_decodeCertBag(bag.Value.Bytes)
			if err != nil {
				return fail(err.Error())
			}
			cs, err := x509.ParseCertificates(der)
			if err != nil {
				return fail(err.Error())
			}
			for _, c := range cs {
				certs = append(certs, c)
				ids = append(ids, id)
			}
		case bag.Id.Equal(xp12_oidPKCS8ShroundedKeyBag):
			if keyDER != nil {
				return fail("pkcs12: expected exactly one key bag")
			}
			k, err := xp12_decodePkcs8ShroudedKeyBag(bag.Value.Bytes, pw)
			if err != nil {
				return fail(err.Error())
			}
			keyDER, err = x509.MarshalPKCS8PrivateKey(k)
			if err != nil {
				return fail("pkcs12: error parsing PKCS#8 private key: " + err.Error())
			}
			keyID = id
		}
	}
	if len(certs) == 0 {
		return fail("pkcs12: certificate missing")
	}
	if keyDER == nil {
		return fail("pkcs12: private key missing")
	}
	kr := Crypto_parse_private_key(tk, Bytes(keyDER))
	if !kr.Ok {
		return fail("pkcs12: error parsing PKCS#8 private key")
	}
	leaf := 0
	for i := range certs {
		if len(keyID) > 0 && bytes.Equal(ids[i], keyID) {
			leaf = i
			break
		}
	}
	var ca []X509_Certificate
	for i, c := range certs {
		if i != leaf {
			ca = append(ca, x509CertOf(tk, c))
		}
	}
	return Ok[X509_Pkcs12, X509_CertErr](X509_Pkcs12{F_key: kr.V, F_certificate: x509CertOf(tk, certs[leaf]), F_ca: ListFromSlice(ca)})
}

// x509OidErr is Go's errInvalidOID.
func x509OidErr() Result[X509_Oid, X509_CertErr] {
	return Err[X509_Oid](x509Err("invalid oid"))
}

func x509OidOf(o x509.OID, err error) Result[X509_Oid, X509_CertErr] {
	if err != nil {
		return Err[X509_Oid](x509ErrOf(err))
	}
	b, _ := o.MarshalBinary()
	return Ok[X509_Oid, X509_CertErr](X509_Oid{F_der: Bytes(b)})
}

func X509_parse_oid(_ *Task, s string) Result[X509_Oid, X509_CertErr] {
	return x509OidOf(x509.ParseOID(string(s)))
}

func X509_oid_from_ints(_ *Task, arcs List[uint64]) Result[X509_Oid, X509_CertErr] {
	var a []uint64
	for _, x := range arcs.s {
		a = append(a, x)
	}
	return x509OidOf(x509.OIDFromInts(a))
}

func X509_oid_from_asn1_oid(_ *Task, arcs List[int64]) Result[X509_Oid, X509_CertErr] {
	var a asn1.ObjectIdentifier
	for _, x := range arcs.s {
		if x < 0 {
			// Go's int is 64 bits here: OIDFromASN1OID's own check
			a = append(a, -1)
			continue
		}
		a = append(a, int(x))
	}
	return x509OidOf(x509.OIDFromASN1OID(a))
}

func X509_oid_from_binary(_ *Task, der Bytes) Result[X509_Oid, X509_CertErr] {
	var o x509.OID
	if err := o.UnmarshalBinary([]byte(der)); err != nil {
		return x509OidErr()
	}
	return Ok[X509_Oid, X509_CertErr](X509_Oid{F_der: Bytes(append([]byte(nil), der...))})
}

// x509GoOID is o as Go's OID; ok false for contents Go would not make.
func x509GoOID(o X509_Oid) (x509.OID, bool) {
	var g x509.OID
	if len(o.F_der) == 0 {
		return g, true
	}
	if err := g.UnmarshalBinary([]byte(o.F_der)); err != nil {
		return g, false
	}
	return g, true
}

func (o *X509_Oid) M_display(_ *Task) string {
	g, ok := x509GoOID(*o)
	if !ok {
		return ""
	}
	return textOf(g.String())
}

func (o *X509_Oid) M_equal_asn1_oid(_ *Task, arcs List[int64]) bool {
	g, ok := x509GoOID(*o)
	if !ok {
		return false
	}
	var a asn1.ObjectIdentifier
	for _, x := range arcs.s {
		a = append(a, int(x))
	}
	return g.EqualASN1OID(a)
}

func (o *X509_Oid) M_to_asn1_oid(_ *Task) Option[List[int64]] {
	if _, ok := x509GoOID(*o); !ok {
		return None[List[int64]]()
	}
	// Go's toASN1OID (unexported), line for line
	out := []int64{}
	val := 0
	for _, v := range []byte(o.F_der) {
		if val > (1<<24)-1 {
			return None[List[int64]]()
		}
		val <<= 7
		val |= int(v & 0x7f)
		if v&0x80 == 0 {
			if len(out) == 0 {
				if val < 80 {
					out = append(out, int64(val/40), int64(val%40))
				} else {
					out = append(out, 2, int64(val-80))
				}
				val = 0
				continue
			}
			out = append(out, int64(val))
			val = 0
		}
	}
	return Some(ListFromSlice(out))
}

func X509_parse_certificates(tk *Task, der Bytes) Result[List[X509_Certificate], X509_CertErr] {
	cs, err := x509.ParseCertificates([]byte(der))
	if err != nil {
		return Err[List[X509_Certificate]](x509ErrOf(err))
	}
	out := []X509_Certificate{}
	for _, c := range cs {
		out = append(out, x509CertOf(tk, c))
	}
	return Ok[List[X509_Certificate], X509_CertErr](ListFromSlice(out))
}

// Key encodings (std/x509's parse_pkcs1_private_key and the rest): Go's
// typed parsers for the error, std/crypto's parse for the key; asn1's
// errors are std/crypto's texts.
func x509KeyErr(err error, bare string) X509_CertErr {
	m := err.Error()
	if strings.HasPrefix(m, "asn1:") {
		m = bare
	} else if i := strings.Index(m, "asn1:"); i >= 0 {
		m = m[:i] + "malformed DER"
	}
	return x509Err(m)
}

func x509PrivOf(tk *Task, der Bytes, err error, bare string) Result[Crypto_PrivateKey, X509_CertErr] {
	if err != nil {
		return Err[Crypto_PrivateKey](x509KeyErr(err, bare))
	}
	r := Crypto_parse_private_key(tk, der)
	if !r.Ok {
		return Err[Crypto_PrivateKey](x509Err(r.E.F_msg))
	}
	return Ok[Crypto_PrivateKey, X509_CertErr](r.V)
}

func x509PubOf(tk *Task, der Bytes, err error) Result[Crypto_PublicKey, X509_CertErr] {
	if err != nil {
		return Err[Crypto_PublicKey](x509KeyErr(err, "x509: malformed public key"))
	}
	r := Crypto_parse_public_key(tk, der)
	if !r.Ok {
		return Err[Crypto_PublicKey](x509Err(r.E.F_msg))
	}
	return Ok[Crypto_PublicKey, X509_CertErr](r.V)
}

func X509_parse_pkcs1_private_key(tk *Task, der Bytes) Result[Crypto_PrivateKey, X509_CertErr] {
	_, err := x509.ParsePKCS1PrivateKey([]byte(der))
	return x509PrivOf(tk, der, err, "x509: malformed private key")
}

func X509_parse_ec_private_key(tk *Task, der Bytes) Result[Crypto_PrivateKey, X509_CertErr] {
	_, err := x509.ParseECPrivateKey([]byte(der))
	return x509PrivOf(tk, der, err, "x509: failed to parse EC private key: malformed DER")
}

func X509_parse_pkcs8_private_key(tk *Task, der Bytes) Result[Crypto_PrivateKey, X509_CertErr] {
	_, err := x509.ParsePKCS8PrivateKey([]byte(der))
	return x509PrivOf(tk, der, err, "x509: malformed private key")
}

func X509_parse_pkcs1_public_key(tk *Task, der Bytes) Result[Crypto_PublicKey, X509_CertErr] {
	_, err := x509.ParsePKCS1PublicKey([]byte(der))
	return x509PubOf(tk, der, err)
}

func X509_parse_pkix_public_key(tk *Task, der Bytes) Result[Crypto_PublicKey, X509_CertErr] {
	_, err := x509.ParsePKIXPublicKey([]byte(der))
	return x509PubOf(tk, der, err)
}

func X509_marshal_pkcs1_private_key(_ *Task, k Crypto_PrivateKey) Result[Bytes, X509_CertErr] {
	x, ok := k.k.(*rsa.PrivateKey)
	if !ok {
		return Err[Bytes](x509Err("x509: not an RSA key"))
	}
	return Ok[Bytes, X509_CertErr](Bytes(x509.MarshalPKCS1PrivateKey(x)))
}

func X509_marshal_ec_private_key(_ *Task, k Crypto_PrivateKey) Result[Bytes, X509_CertErr] {
	x, ok := k.k.(*ecdsa.PrivateKey)
	if !ok {
		return Err[Bytes](x509Err("x509: not an ECDSA key"))
	}
	d, err := x509.MarshalECPrivateKey(x)
	if err != nil {
		return Err[Bytes](x509Err(err.Error()))
	}
	return Ok[Bytes, X509_CertErr](Bytes(d))
}

func X509_marshal_pkcs1_public_key(_ *Task, k Crypto_PublicKey) Result[Bytes, X509_CertErr] {
	x, ok := k.k.(*rsa.PublicKey)
	if !ok {
		return Err[Bytes](x509Err("x509: not an RSA key"))
	}
	return Ok[Bytes, X509_CertErr](Bytes(x509.MarshalPKCS1PublicKey(x)))
}
