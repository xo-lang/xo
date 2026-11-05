package main

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/xo-lang/xo/std/rt"
)

// The API this corpus covers: x509.parse_pkcs12 (golang.org/x/crypto/pkcs12 v0.58.0, vendored as
// std/rt/xcrypto_pkcs12.go for the Go backend). The inputs are the three .pfx files of its
// pkcs12_test.go (3DES and 40 bit RC2 bags, null and empty passwords), each with the right
// password, a wrong one, truncated and bit-flipped copies. Go's result is summarized (the leaf's
// common name, the CA count, the key's kind) or its error text; a text built from a Go
// encoding/asn1 error is compared by its first word only. Go's pkcs12 tests are Copyright 2015
// The Go Authors, BSD-style license.

type p12Row struct {
	name, pw string
	data     []byte
	why      string
}

// p12Fixtures reads the base64 PFX files out of pkcs12_test.go.
func p12Fixtures() map[string][]byte {
	out, err := exec.Command("go", "env", "GOMODCACHE").Output()
	if err != nil {
		panic(err)
	}
	matches, _ := filepath.Glob(filepath.Join(strings.TrimSpace(string(out)), "golang.org", "x", "crypto@v0.58.0", "pkcs12", "pkcs12_test.go"))
	if len(matches) != 1 {
		panic("golang.org/x/crypto@v0.58.0 is not in the module cache")
	}
	src, err := os.ReadFile(matches[0])
	if err != nil {
		panic(err)
	}
	i := strings.Index(string(src), "var testdata")
	res := map[string][]byte{}
	re := regexp.MustCompile("(?s)\"([^\"]+)\": `([^`]*)`")
	for _, m := range re.FindAllStringSubmatch(string(src)[i:], -1) {
		b, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(m[2]), ""))
		if err != nil {
			panic(err)
		}
		res[m[1]] = b
	}
	return res
}

// p12Summary is what the Xo test prints for a result.
func p12Summary(data []byte, pw string) string {
	r := rt.X509_parse_pkcs12(nil, rt.Bytes(data), pw)
	if !r.Ok {
		msg := r.E.F_msg
		if strings.Contains(msg, "asn1:") || strings.Contains(msg, "x509:") {
			return "ERR asn1"
		}
		return "ERR " + msg
	}
	return fmt.Sprintf("OK cn=%s ca=%d", r.V.F_certificate.F_subject.F_common_name, rt.List_len(nil, r.V.F_ca))
}

func pkcs12Corpus() string {
	var o strings.Builder
	o.WriteString(header("golang.org/x/crypto/pkcs12's three test files, with passwords, truncations and bit flips, against std/x509's parse_pkcs12 (Go's pkcs12 tests are Copyright 2015 The Go Authors, BSD-style license)"))
	o.WriteString("\nuse std/base64\nuse std/x509\n\n")
	fx := p12Fixtures()
	var names []string
	for n := range fx {
		names = append(names, n)
	}
	sortStrings(names)
	var rows []p12Row
	// the files of openssl: a chain, and a leaf with an empty password
	for _, f := range []struct{ file, pw string }{{"chain.p12", "s3cret"}, {"nochain.p12", ""}} {
		d, err := os.ReadFile(filepath.Join("scripts", "stdcorpus", "testdata", "pkcs12", f.file))
		if err != nil {
			panic(err)
		}
		rows = append(rows, p12Row{f.file, f.pw, d, "openssl, right password"}, p12Row{f.file, "wrong", d, "openssl, wrong password"})
		c := append([]byte(nil), d...)
		c[len(c)/2] ^= 0x10
		rows = append(rows, p12Row{f.file, f.pw, c, "openssl, bit flipped"})
	}
	for _, n := range names {
		d := fx[n]
		rows = append(rows, p12Row{n, "", d, "empty password"}, p12Row{n, "password", d, "wrong password"})
		rows = append(rows, p12Row{n, "", d[:len(d)/2], "truncated"}, p12Row{n, "", d[:len(d)-1], "one byte short"})
		for _, at := range []int{4, 20, 60, 400, 900, len(d) - 30} {
			c := append([]byte(nil), d...)
			c[at] ^= 0x01
			rows = append(rows, p12Row{n, "", c, fmt.Sprintf("bit flipped at %d", at)})
		}
	}
	o.WriteString("const PFX: List[(Str, Str, Str, Str)] = [\n")
	for _, r := range rows {
		fmt.Fprintf(&o, "  (%s, %s, %s, %s),\n", xq(r.why+": "+r.name), xq(base64.StdEncoding.EncodeToString(r.data)), xq(r.pw), xq(p12Summary(r.data, r.pw)))
	}
	o.WriteString("]\n\n")
	o.WriteString(`fn summary(data: Bytes, pw: Str) -> Str {
  match x509.parse_pkcs12(data, pw) {
    Ok(p) => "OK cn=${p.certificate.subject.common_name} ca=${p.ca.len()}"
    Err(e) => if e.msg.contains("asn1:") || e.msg.contains("x509:") { "ERR asn1" } else { "ERR ${e.msg}" }
  }
}

test "parse_pkcs12 reads what golang.org/x/crypto/pkcs12 reads" {
  for (_, b64, pw, want) in PFX {
    let data = base64.decode(b64, base64.Base64.Std).ok() ?? b""
    let got = summary(data, pw)
    if want == "ERR asn1" {
      // a malformed file: an error, with its own wording
      expect(got.starts_with("ERR")) == true
    } else {
      expect(got) == want
    }
  }
}
`)
	return o.String()
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}
