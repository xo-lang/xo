package main

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

// std/x509's VerifyOptions.certificate_policies against Go's
// CertificatePolicies: every leaf, intermediate and root of Go's
// testdata/policy_*.pem (TestPoliciesValid's certificates, from BoringSSL;
// the Go files are Copyright The Go Authors, BSD-style license) verified
// with several acceptable policy sets, with Go's Verify answer and error
// type.

func x509PolicyCorpus() string {
	dir := filepath.Join(runtime.GOROOT(), "src", "crypto", "x509", "testdata")
	names, err := filepath.Glob(filepath.Join(dir, "policy_*.pem"))
	if err != nil {
		panic(err)
	}
	sort.Strings(names)
	type cert struct {
		name string
		c    *x509.Certificate
	}
	var leaves, inters, roots []cert
	for _, n := range names {
		b, err := os.ReadFile(n)
		if err != nil {
			panic(err)
		}
		p, _ := pem.Decode(b)
		c, err := x509.ParseCertificate(p.Bytes)
		if err != nil {
			continue // the _invalid and _duplicate ones are parse tests
		}
		base := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(n), "policy_"), ".pem")
		switch {
		case strings.HasPrefix(base, "leaf"):
			leaves = append(leaves, cert{base, c})
		case strings.HasPrefix(base, "intermediate"):
			inters = append(inters, cert{base, c})
		default:
			roots = append(roots, cert{base, c})
		}
	}
	oid := func(s string) x509.OID {
		o, err := x509.ParseOID(s)
		if err != nil {
			panic(err)
		}
		return o
	}
	t := func(n int) string { return fmt.Sprintf("1.2.840.113554.4.1.72585.2.%d", n) }
	sets := [][]string{nil, {t(1)}, {t(2)}, {t(3)}, {t(1), t(2)}, {t(4), t(5)}, {t(5)}, {"2.5.29.32.0"}, {t(9)}}

	var o strings.Builder
	o.WriteString(header("Go's crypto/x509 Verify with CertificatePolicies on testdata/policy_*.pem against std/x509's verify (Go's test certificates, BSD-style license)"))
	o.WriteString("\nuse std/x509\n")
	o.WriteString("\n/// The certificates: (name, DER).\nconst CERTS: List[(Str, Bytes)] = [\n")
	idx := map[string]int{}
	for _, l := range [][]cert{leaves, inters, roots} {
		for _, c := range l {
			idx[c.name] = len(idx)
			fmt.Fprintf(&o, "  (%s, %s),\n", xq(c.name), xbytes(c.c.Raw))
		}
	}
	o.WriteString("]\n")
	o.WriteString("\n/// (leaf, intermediate (-1 none), root indexes in CERTS, Unix time, acceptable policies, Go's Verify with any key usage: ok N chains or the error and its type)\nconst ROWS: List[(Int, Int, Int, Int, List[Str], Str)] = [\n")
	for _, l := range leaves {
		at := l.c.NotBefore.Add(24 * time.Hour)
		for _, r := range roots {
			for ii := -1; ii < len(inters); ii++ {
				ip := x509.NewCertPool()
				in := -1
				if ii >= 0 {
					ip.AddCert(inters[ii].c)
					in = idx[inters[ii].name]
				}
				rp := x509.NewCertPool()
				rp.AddCert(r.c)
				for _, set := range sets {
					var ps []x509.OID
					for _, s := range set {
						ps = append(ps, oid(s))
					}
					chains, err := l.c.Verify(x509.VerifyOptions{Roots: rp, Intermediates: ip, CurrentTime: at,
						KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny}, CertificatePolicies: ps})
					want := fmt.Sprintf("ok %d", len(chains))
					if err != nil {
						want = err.Error() + x509KindOf(err)
					}
					fmt.Fprintf(&o, "  (%d, %d, %d, %d, %s, %s),\n", idx[l.name], in, idx[r.name], at.Unix(), ctStrs(set), xq(want))
				}
			}
		}
	}
	o.WriteString("]\n")
	o.WriteString(`
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

test "verify with certificate_policies is Go's Verify with CertificatePolicies" {
  let certs = CERTS.map(fn(c) { x509.parse(c.1).ok() ?? fault(c.0) })
  var bad: List[Str] = []
  for (l, i, r, at, set, want) in ROWS {
    let policies = set.map(fn(s) { x509.parse_oid(s).ok() ?? fault(s) })
    let inters = if i < 0 { [] } else { [certs[i]] }
    let opts = x509.VerifyOptions{roots: [certs[r]], intermediates: inters, at: Time.from_unix(at), key_usages: [x509.ExtKeyUsage.Any], certificate_policies: policies}
    let got = match certs[l].verify(opts) {
      Ok(chains) => "ok ${chains.len()}"
      Err(e) => "${e.display()}${kind_text(e.kind)}"
    }
    if got != want {
      let iname = if i < 0 { "none" } else { CERTS[i].0 }
      bad.push("${CERTS[l].0} < ${iname} < ${CERTS[r].0} ${set}:\n  got  ${got}\n  want ${want}")
    }
  }
  expect(bad.len()) == 0
  if !bad.is_empty() {
    expect(bad[0..bad.len().min(8)]) == []
  }
}
`)
	return o.String()
}
