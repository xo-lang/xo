package rt

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"strconv"
	"strings"
)

// fakeAuthority is the Host a handler sees when the request has none of its own: the URL's host and port as
// written (Go's Request.Host, decision 0544).
func fakeAuthority(u Http_Url) string {
	h := u.F_host
	if strings.Contains(h, ":") {
		h = "[" + h + "]"
	}
	if u.F_port.Some {
		h += ":" + strconv.FormatInt(int64(u.F_port.V), 10)
	}
	return h
}

// M_handle_tls is FakeNet.handle_tls (decision 0544): a handler that holds a certificate; an https request it
// matches runs the client's checks against the certificate before the handler.
func (f Testing_FakeNet) M_handle_tls(tk *Task, pattern string, cert Tls_TlsCert, h func(*Task, Http_Request) Result[Http_Response, Never]) {
	f.M_handle(tk, pattern, h)
	f.n.mu.Lock()
	defer f.n.mu.Unlock()
	m, prefix, _, rest := parseFakePattern(pattern)
	for _, old := range f.n.handlers {
		if old.method == m && old.prefix == prefix && old.rest == rest {
			old.cert = cert.c
			return
		}
	}
}

// fakeTLS runs the checks of a real handshake for client c against the certificate cert of a FakeNet handler: the
// chain against the client's roots and the name against the leaf (skipped as the mode says), then the verify hook.
// It returns the error text, "", or the state the handler sees.
func fakeTLS(tk *Task, c Http_Client, req Http_Request, cert *tls.Certificate) (string, *tlsState) {
	name := c.serverName
	if name == "" {
		name = req.F_url.F_host
	}
	if strings.HasPrefix(name, "[") && strings.HasSuffix(name, "]") {
		name = name[1 : len(name)-1]
	}
	if c.verify == 2 && !c.vconn.Some {
		return "tls: Verify.None needs a verify_conn (an unverified connection must be pinned)", nil
	}
	var certs []*x509.Certificate
	for _, der := range cert.Certificate {
		x, err := x509.ParseCertificate(der)
		if err != nil {
			return "tls: failed to parse certificate from server: " + err.Error(), nil
		}
		certs = append(certs, x)
	}
	if len(certs) == 0 {
		return "tls: received empty certificates message", nil
	}
	if c.verify != 2 {
		inter := x509.NewCertPool()
		for _, x := range certs[1:] {
			inter.AddCert(x)
		}
		opts := x509.VerifyOptions{Roots: c.roots, Intermediates: inter, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
		if c.verify == 0 {
			opts.DNSName = name
		}
		if _, err := certs[0].Verify(opts); err != nil {
			return "tls: failed to verify certificate: " + err.Error(), nil
		}
	}
	if c.vconn.Some {
		var xs []X509_Certificate
		for _, x := range certs {
			xs = append(xs, x509CertOf(tk, x))
		}
		if r := c.vconn.V(tk, ListFromSlice(xs)); !r.Ok {
			if r.E == "" {
				return errors.New("tls: verify_conn failed").Error(), nil
			}
			return r.E, nil
		}
	}
	sni := name
	if net.ParseIP(sni) != nil {
		sni = ""
	}
	return "", &tlsState{version: "TLS 1.3", cipher: "TLS_AES_128_GCM_SHA256", serverName: sni}
}
