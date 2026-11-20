//go:build !xo_no_tls

package rt

import (
	"crypto/tls"
	"crypto/x509"
)

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

// M_state is `t.state()` (decision 0644): Go's ConnectionState.
func (t Tls_TlsConn) M_state(tk *Task) Tls_ConnState {
	if t.t == nil {
		return Tls_ConnState{F_version: 0x0304, F_handshake_complete: true, F_negotiated_protocol_is_mutual: true}
	}
	return connStateOf(tk, t.t.ConnectionState())
}

// connStateOf is Go's ConnectionState as Xo's ConnState.
func connStateOf(tk *Task, cs tls.ConnectionState) Tls_ConnState {
	certs := func(l []*x509.Certificate) List[X509_Certificate] {
		var out []X509_Certificate
		for _, c := range l {
			out = append(out, x509CertOf(tk, c))
		}
		return ListFromSlice(out)
	}
	var chains []List[X509_Certificate]
	for _, ch := range cs.VerifiedChains {
		chains = append(chains, certs(ch))
	}
	var scts []Bytes
	for _, x := range cs.SignedCertificateTimestamps {
		scts = append(scts, Bytes(x))
	}
	return Tls_ConnState{
		F_version: int64(cs.Version), F_handshake_complete: cs.HandshakeComplete, F_did_resume: cs.DidResume,
		F_cipher_suite: int64(cs.CipherSuite), F_curve_id: int64(cs.CurveID), F_negotiated_protocol: cs.NegotiatedProtocol,
		F_negotiated_protocol_is_mutual: true, F_server_name: cs.ServerName, F_peer_certificates: certs(cs.PeerCertificates),
		F_verified_chains: ListFromSlice(chains), F_signed_certificate_timestamps: ListFromSlice(scts),
		F_ocsp_response: Bytes(cs.OCSPResponse), F_tls_unique: Bytes(cs.TLSUnique), F_ech_accepted: cs.ECHAccepted,
	}
}

// M_export_keying_material is `t.export_keying_material(label,
// context, length)` (decision 0644 T9): Go's
// ConnectionState.ExportKeyingMaterial, None as the nil context.
func (t Tls_TlsConn) M_export_keying_material(tk *Task, label string, context Option[Bytes], length int64) Result[Bytes, Tls_TlsErr] {
	if t.t == nil {
		return Err[Bytes](Tls_TlsErr{F_msg: "tls: ExportKeyingMaterial on a connection without TLS"})
	}
	cs := t.t.ConnectionState()
	if !cs.HandshakeComplete {
		return Err[Bytes](Tls_TlsErr{F_msg: "tls: ExportKeyingMaterial before the handshake completed"})
	}
	var ctx []byte
	if context.Some {
		ctx = append([]byte{}, context.V...)
	}
	b, err := cs.ExportKeyingMaterial(label, ctx, int(length))
	if err != nil {
		return Err[Bytes](Tls_TlsErr{F_msg: err.Error()})
	}
	return Ok[Bytes, Tls_TlsErr](Bytes(b))
}

// Tls_cipher_suite_name is `tls.cipher_suite_name(id)`: Go's CipherSuiteName.
func Tls_cipher_suite_name(_ *Task, id int64) string { return tls.CipherSuiteName(uint16(id)) }

// Tls_version_name is `tls.version_name(v)`: Go's VersionName.
func Tls_version_name(_ *Task, v int64) string { return tls.VersionName(uint16(v)) }

// Tls_curve_name is `tls.curve_name(id)`: Go's CurveID.String.
func Tls_curve_name(_ *Task, id int64) string { return tls.CurveID(uint16(id)).String() }
