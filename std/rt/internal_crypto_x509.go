//go:build !xo_no_x509

package rt

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// cryptoX509 is x509.xo's op.
func cryptoX509(what string, a, b Bytes) (Bytes, bool) {
	switch what {
	case "check":
		c, err := x509.ParseCertificate([]byte(a))
		if err != nil {
			return Bytes("error " + err.Error()), true
		}
		return Bytes(describeCert(c)), true
	case "parse":
		c, err := x509.ParseCertificate([]byte(a))
		if err != nil {
			return "error", true
		}
		return Bytes(describeCert(c)), true
	case "signed_by":
		c, err := x509.ParseCertificate([]byte(a))
		p, err2 := x509.ParseCertificate([]byte(b))
		return cryptoFlag(err == nil && err2 == nil && p.CheckSignature(c.SignatureAlgorithm, c.RawTBSCertificate, c.Signature) == nil), true
	}
	return "", false
}

// cryptoChain is std/native/llrt/x509verify.xo's op: x509.Verify with
// the outcome as x509verify.xo's code and the chain's common names.
func cryptoChain(what string, a, b, c, d Bytes) (Bytes, bool) {
	if what != "verify" && what != "text" {
		return "", false
	}
	leaf, err := x509.ParseCertificate([]byte(a))
	if err != nil {
		return "parse", true
	}
	args := strings.Split(string(d), " ")
	if len(args) != 3 {
		return "args", true
	}
	now, _ := strconv.ParseInt(args[0], 10, 64)
	opts := x509.VerifyOptions{
		Roots:         chainPool(c),
		Intermediates: chainPool(b),
		CurrentTime:   time.Unix(now, 0),
	}
	if args[1] != "-" {
		opts.DNSName = args[1]
	}
	switch args[2] {
	case "2.5.29.37.0":
		opts.KeyUsages = []x509.ExtKeyUsage{x509.ExtKeyUsageAny}
	case "1.3.6.1.5.5.7.3.1":
		opts.KeyUsages = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
	case "1.3.6.1.5.5.7.3.2":
		opts.KeyUsages = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
	}
	if what == "text" {
		// x509verify.xo's full_text: the time in UTC
		opts.CurrentTime = opts.CurrentTime.UTC()
	}
	chains, err := leaf.Verify(opts)
	if what == "text" && err != nil {
		return Bytes(err.Error()), true
	}
	code := 0
	var ua x509.UnknownAuthorityError
	var ci x509.CertificateInvalidError
	var he x509.HostnameError
	var uc x509.UnhandledCriticalExtension
	switch {
	case err == nil:
	case errors.As(err, &ua):
		code = 1
	case errors.As(err, &ci):
		code = map[x509.InvalidReason]int{x509.Expired: 2, x509.NotAuthorizedToSign: 3, x509.TooManyIntermediates: 4,
			x509.CANotAuthorizedForThisName: 5, x509.IncompatibleUsage: 6, x509.NoValidChains: 10}[ci.Reason]
	case errors.As(err, &he):
		code = 7
	case errors.As(err, &uc):
		code = 8
	default:
		return Bytes("error " + err.Error()), true
	}
	out := strconv.Itoa(code)
	if err == nil {
		for _, x := range chains[0] {
			out += " " + x.Subject.CommonName
		}
	}
	return Bytes(out), true
}

// chainPool is the pool of a list of 2 byte big endian lengths and DERs.
func chainPool(b Bytes) *x509.CertPool {
	p := x509.NewCertPool()
	for len(b) >= 2 {
		n := int(b[0])<<8 | int(b[1])
		if c, err := x509.ParseCertificate([]byte(b[2 : 2+n])); err == nil {
			p.AddCert(c)
		}
		b = b[2+n:]
	}
	return p
}

// describeCert is std/native/llrt/x509cert.xo's describe of c: its fields one
// a line, in the numbering of x509.xo's constants.
func describeCert(c *x509.Certificate) string {
	h := hex.EncodeToString
	sigAlg := map[x509.SignatureAlgorithm]int{
		x509.SHA256WithRSA: 1, x509.SHA384WithRSA: 2, x509.SHA512WithRSA: 3,
		x509.ECDSAWithSHA256: 4, x509.ECDSAWithSHA384: 5, x509.ECDSAWithSHA512: 6,
		x509.PureEd25519: 7, x509.SHA256WithRSAPSS: 8, x509.SHA384WithRSAPSS: 9, x509.SHA512WithRSAPSS: 10,
		x509.MD5WithRSA: 11, x509.SHA1WithRSA: 11, x509.ECDSAWithSHA1: 11, x509.DSAWithSHA1: 11,
	}[c.SignatureAlgorithm]
	keyAlg, key := 0, []byte(nil)
	switch k := c.PublicKey.(type) {
	case *rsa.PublicKey:
		keyAlg, key = 1, x509.MarshalPKCS1PublicKey(k)
	case *ecdsa.PublicKey:
		keyAlg = 5
		switch k.Curve {
		case elliptic.P256():
			keyAlg = 2
		case elliptic.P384():
			keyAlg = 3
		case elliptic.P224():
			keyAlg = 6
		}
		key, _ = k.Bytes()
	case ed25519.PublicKey:
		keyAlg, key = 4, k
	}
	var ips []string
	for _, ip := range c.IPAddresses {
		ips = append(ips, h(ip))
	}
	maxPath := c.MaxPathLen
	if !c.BasicConstraintsValid {
		maxPath = -1
	}
	ekuOID := map[x509.ExtKeyUsage]string{
		x509.ExtKeyUsageAny: "2.5.29.37.0", x509.ExtKeyUsageServerAuth: "1.3.6.1.5.5.7.3.1",
		x509.ExtKeyUsageClientAuth: "1.3.6.1.5.5.7.3.2", x509.ExtKeyUsageCodeSigning: "1.3.6.1.5.5.7.3.3",
		x509.ExtKeyUsageEmailProtection: "1.3.6.1.5.5.7.3.4", x509.ExtKeyUsageTimeStamping: "1.3.6.1.5.5.7.3.8",
		x509.ExtKeyUsageOCSPSigning: "1.3.6.1.5.5.7.3.9", x509.ExtKeyUsageIPSECEndSystem: "1.3.6.1.5.5.7.3.5",
		x509.ExtKeyUsageIPSECTunnel: "1.3.6.1.5.5.7.3.6", x509.ExtKeyUsageIPSECUser: "1.3.6.1.5.5.7.3.7",
		x509.ExtKeyUsageMicrosoftServerGatedCrypto: "1.3.6.1.4.1.311.10.3.3", x509.ExtKeyUsageNetscapeServerGatedCrypto: "2.16.840.1.113730.4.1",
		x509.ExtKeyUsageMicrosoftCommercialCodeSigning: "1.3.6.1.4.1.311.2.1.22", x509.ExtKeyUsageMicrosoftKernelCodeSigning: "1.3.6.1.4.1.311.61.1.1",
	}
	var eku []string
	for _, u := range c.ExtKeyUsage {
		eku = append(eku, ekuOID[u])
	}
	for _, u := range c.UnknownExtKeyUsage {
		eku = append(eku, u.String())
	}
	j := func(xs []string) string { return strings.Join(xs, ",") }
	var b strings.Builder
	fmt.Fprintf(&b, "version %d\nserial %s\nsig_alg %d\nsignature %s\n", c.Version, h(c.SerialNumber.Bytes()), sigAlg, h(c.Signature))
	fmt.Fprintf(&b, "issuer %s\nsubject %s\ncn %s\n", h(c.RawIssuer), h(c.RawSubject), c.Subject.CommonName)
	fmt.Fprintf(&b, "not_before %d\nnot_after %d\nkey_alg %d\nkey %s\nspki %s\n", c.NotBefore.Unix(), c.NotAfter.Unix(), keyAlg, h(key), h(c.RawSubjectPublicKeyInfo))
	fmt.Fprintf(&b, "dns %s\nips %s\n", j(c.DNSNames), j(ips))
	fmt.Fprintf(&b, "ca %t bc %t max_path %d\nkey_usage %d\n", c.IsCA, c.BasicConstraintsValid, maxPath, c.KeyUsage)
	fmt.Fprintf(&b, "ext_usage %s\nski %s\naki %s\n", j(eku), h(c.SubjectKeyId), h(c.AuthorityKeyId))
	fmt.Fprintf(&b, "permitted %s\nexcluded %s\nunhandled %d\n", j(c.PermittedDNSDomains), j(c.ExcludedDNSDomains), len(c.UnhandledCriticalExtensions))
	fmt.Fprintf(&b, "tbs %d\n", len(c.RawTBSCertificate))
	return b.String()
}

// cryptoRSA is rsa.xo's op.
func cryptoRSA(what string, a, b, c, d Bytes) (Bytes, bool) {
	h := crypto.SHA256
	if len(a) > 0 && a[0] == 1 {
		h = crypto.SHA384
	} else if len(a) > 0 && a[0] == 2 {
		h = crypto.SHA512
	}
	priv := func(der Bytes) *rsa.PrivateKey {
		if k, err := x509.ParsePKCS1PrivateKey([]byte(der)); err == nil {
			return k
		}
		if k, err := x509.ParsePKCS8PrivateKey([]byte(der)); err == nil {
			if r, ok := k.(*rsa.PrivateKey); ok {
				return r
			}
		}
		return nil
	}
	switch what {
	case "public":
		k := priv(a)
		if k == nil {
			return "", true
		}
		return Bytes(x509.MarshalPKCS1PublicKey(&k.PublicKey)), true
	case "verify_pkcs1", "verify_pss":
		k, err := x509.ParsePKCS1PublicKey([]byte(b))
		if err != nil {
			return cryptoFlag(false), true
		}
		if what == "verify_pss" {
			return cryptoFlag(rsa.VerifyPSS(k, h, []byte(c), []byte(d), &rsa.PSSOptions{SaltLength: rsa.PSSSaltLengthEqualsHash}) == nil), true
		}
		return cryptoFlag(rsa.VerifyPKCS1v15(k, h, []byte(c), []byte(d)) == nil), true
	case "sign_pkcs1", "sign_pss":
		k := priv(b)
		if k == nil {
			return "", true
		}
		var sig []byte
		var err error
		if what == "sign_pss" {
			sig, err = rsa.SignPSS(rand.Reader, k, h, []byte(c), &rsa.PSSOptions{SaltLength: rsa.PSSSaltLengthEqualsHash})
		} else {
			sig, err = rsa.SignPKCS1v15(rand.Reader, k, h, []byte(c))
		}
		if err != nil {
			return "", true
		}
		return Bytes(sig), true
	}
	return "", false
}
