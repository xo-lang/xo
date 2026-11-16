package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base32"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"hash"
	"math/rand"
	"sort"
	"net/url"
	"strconv"
	"strings"
)

func otpHotp(secret []byte, counter uint64, digits int, algo string) string {
	return otpCode(secret, counter, digits, algo, false)
}

// otpCode is pquerna/otp's hotp.GenerateCodeCustom: decimal, or with
// steam the Steam Guard alphabet, lowest place first.
func otpCode(secret []byte, counter uint64, digits int, algo string, steam bool) string {
	var h func() hash.Hash
	switch algo {
	case "Sha256":
		h = sha256.New
	case "Sha512":
		h = sha512.New
	default:
		h = sha1.New
	}
	m := hmac.New(h, secret)
	var c [8]byte
	binary.BigEndian.PutUint64(c[:], counter)
	m.Write(c[:])
	s := m.Sum(nil)
	off := s[len(s)-1] & 15
	v := binary.BigEndian.Uint32(s[off:off+4]) & 0x7fffffff
	if steam {
		const alphabet = "23456789BCDFGHJKMNPQRTVWXY"
		out := ""
		for i := 0; i < digits; i++ {
			out += string(alphabet[v%26])
			v /= 26
		}
		return out
	}
	mod := uint32(1)
	for i := 0; i < digits; i++ {
		mod *= 10
	}
	return fmt.Sprintf("%0*d", digits, v%mod)
}

// The std/otp corpus: HOTP, TOTP and the otpauth URI on seeded random
// secrets, counters, times, digits and algorithms, against an independent
// implementation on crypto/hmac and net/url (pquerna/otp's Key.URL layout),
// plus the RFC 4226 and RFC 6238 appendix vectors in the tests.
func otpCorpus() string {
	rg := rand.New(rand.NewSource(57))
	var o strings.Builder
	o.WriteString(header("RFC 4226 and RFC 6238 computed independently on Go's crypto/hmac, and net/url for the otpauth URI, against std/otp"))
	o.WriteString("use std/otp\nuse std/crypto\nuse std/png\n\n")
	algos := []string{"Sha1", "Sha256", "Sha512"}
	o.WriteString("/// (secret hex, counter, digits, algo, the hotp code)\nconst HOTPS: List[(Str, Int, Int, Str, Str)] = [\n")
	for i := 0; i < 400; i++ {
		sec := make([]byte, rg.Intn(70))
		rg.Read(sec)
		ctr := rg.Int63n(1 << 40)
		if i%5 == 0 {
			ctr = int64(rg.Intn(100))
		}
		d := 6 + rg.Intn(3)
		a := algos[rg.Intn(3)]
		o.WriteString(fmt.Sprintf("  (%q, %d, %d, %q, %q),\n", hex.EncodeToString(sec), ctr, d, a, otpHotp(sec, uint64(ctr), d, a)))
	}
	o.WriteString("]\n\n/// (secret hex, unix, period, digits, algo, the totp code)\nconst TOTPS: List[(Str, Int, Int, Int, Str, Str)] = [\n")
	for i := 0; i < 300; i++ {
		sec := make([]byte, 1+rg.Intn(70))
		rg.Read(sec)
		unix := rg.Int63n(1 << 36)
		per := []int64{1, 15, 30, 60, 90}[rg.Intn(5)]
		d := 6 + rg.Intn(3)
		a := algos[rg.Intn(3)]
		o.WriteString(fmt.Sprintf("  (%q, %d, %d, %d, %q, %q),\n", hex.EncodeToString(sec), unix, per, d, a, otpHotp(sec, uint64(unix/per), d, a)))
	}
	o.WriteString("]\n\n/// (issuer, account, secret hex, period, digits, algo, the uri)\nconst URIS: List[(Str, Str, Str, Int, Int, Str, Str)] = [\n")
	names := []string{"Example", "Ex ample", "a:b", "café", "x&y=z", "o'neil", "a/b", "", "100%", "日本", "a+b", "me@example.com"}
	for i := 0; i < 80; i++ {
		sec := make([]byte, rg.Intn(25))
		rg.Read(sec)
		is, ac := names[rg.Intn(len(names))], names[rg.Intn(len(names))]
		per := []int{30, 60}[rg.Intn(2)]
		d := 6 + rg.Intn(3)
		a := algos[rg.Intn(3)]
		q := url.Values{}
		q.Set("algorithm", strings.ToUpper(a))
		q.Set("digits", fmt.Sprint(d))
		q.Set("issuer", is)
		q.Set("period", fmt.Sprint(per))
		q.Set("secret", base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(sec))
		u := otpKeyURI("totp", is, ac, q)
		o.WriteString(fmt.Sprintf("  (%s, %s, %q, %d, %d, %q, %s),\n", xq(is), xq(ac), hex.EncodeToString(sec), per, d, a, xq(u)))
	}
	// hotp.Generate's URI, on its own seed so the sections after keep theirs
	hg := rand.New(rand.NewSource(58))
	o.WriteString("]\n\n/// (issuer, account, secret hex, digits, algo, pquerna/otp's hotp.Generate URI)\nconst HOTP_URIS: List[(Str, Str, Str, Int, Str, Str)] = [\n")
	for i := 0; i < 40; i++ {
		sec := make([]byte, hg.Intn(25))
		hg.Read(sec)
		is, ac := names[hg.Intn(len(names))], names[hg.Intn(len(names))]
		d := 6 + hg.Intn(3)
		a := algos[hg.Intn(3)]
		q := url.Values{}
		q.Set("algorithm", strings.ToUpper(a))
		q.Set("digits", fmt.Sprint(d))
		q.Set("issuer", is)
		q.Set("secret", base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(sec))
		u := otpKeyURI("hotp", is, ac, q)
		o.WriteString(fmt.Sprintf("  (%s, %s, %q, %d, %q, %s),\n", xq(is), xq(ac), hex.EncodeToString(sec), d, a, xq(u)))
	}
	o.WriteString("]\n\n/// (secret hex, counter, letters, algo, the Steam Guard code; pquerna/otp's EncoderSteam)\nconst STEAMS: List[(Str, Int, Int, Str, Str)] = [\n")
	algos4 := []string{"Sha1", "Sha256", "Sha512"}
	for i := 0; i < 120; i++ {
		sec := make([]byte, 1+rg.Intn(40))
		rg.Read(sec)
		ctr := rg.Int63n(1 << 40)
		d := 1 + rg.Intn(8)
		if i%3 == 0 {
			d = 5
		}
		a := algos4[rg.Intn(3)]
		o.WriteString(fmt.Sprintf("  (%q, %d, %d, %q, %q),\n", hex.EncodeToString(sec), ctr, d, a, otpCode(sec, uint64(ctr), d, a, true)))
	}
	o.WriteString("]\n\n/// parse_uri: (uri, kind, issuer, account, secret, period, digits, algo, steam) as pquerna/otp's Key methods read it on net/url\nconst KEYS: List[(Str, Str, Str, Str, Str, Int, Int, Str, Bool)] = [\n")
	for _, u := range otpKeyURIs() {
		k := otpKeyOf(u)
		if k == nil {
			continue
		}
		o.WriteString(fmt.Sprintf("  (%s, %s, %s, %s, %s, %d, %d, %q, %v),\n", xq(u), xq(k.kind), xq(k.issuer), xq(k.account), xq(k.secret), k.period, k.digits, k.algo, k.steam))
	}
	o.WriteString("]\n\n")
	o.WriteString(otpTests)
	otpImageCorpus(&o)
	return o.String()
}

const otpTests = `fn hex(h: Str) -> Bytes { Bytes.from_hex(h) ?? b"" }

fn algo(name: Str) -> crypto.Hash {
  if name == "Md5" {
    crypto.Hash.Md5
  } else if name == "Sha256" {
    crypto.Hash.Sha256
  } else if name == "Sha512" {
    crypto.Hash.Sha512
  } else {
    crypto.Hash.Sha1
  }
}

test "hotp matches the independent implementation" {
  var bad: List[Int] = []
  var i = 0
  for (s, c, d, a, want) in HOTPS {
    i += 1
    if otp.hotp(hex(s), c, d, algo(a)) != want {
      bad.push(i)
    }
  }
  expect(bad) == []
}

test "totp matches the independent implementation" {
  var bad: List[Int] = []
  var i = 0
  for (s, u, p, d, a, want) in TOTPS {
    i += 1
    if otp.totp(hex(s), u, p, d, algo(a)) != want {
      bad.push(i)
    }
  }
  expect(bad) == []
}

test "uri matches pquerna/otp's totp.Generate URI" {
  var bad: List[Int] = []
  var i = 0
  for (is, ac, s, p, d, a, want) in URIS {
    i += 1
    if otp.uri(is, ac, hex(s), p, d, algo(a)) != want {
      bad.push(i)
    }
  }
  expect(bad) == []
}

test "hotp_uri matches pquerna/otp's hotp.Generate URI" {
  var bad: List[Int] = []
  var i = 0
  for (is, ac, s, d, a, want) in HOTP_URIS {
    i += 1
    if otp.hotp_uri(is, ac, hex(s), d, algo(a)) != want {
      bad.push(i)
    }
  }
  expect(bad) == []
}

test "Steam Guard codes match pquerna/otp's GenerateCodeCustom" {
  var bad: List[Int] = []
  for (i, (s, c, d, a, want)) in STEAMS.indexed() {
    if otp.hotp(hex(s), c, d, algo(a), encoder: otp.Encoder.Steam) != want {
      bad.push(i)
    }
    if otp.verify(hex(s), want, c * 30, digits: d, algo: algo(a), skew: 0, encoder: otp.Encoder.Steam) != Some(c) {
      bad.push(1000 + i)
    }
  }
  expect(bad) == []
  expect(otp.verify(b"k", "2345a", 0, digits: 5, encoder: otp.Encoder.Steam)) == None
}

test "parse_uri reads a Key as pquerna/otp's Key methods do" {
  var bad: List[Str] = []
  for (u, kind, is, ac, sec, per, dig, a, steam) in KEYS {
    match otp.parse_uri(u) {
      Ok(k) => {
        let got_steam = k.encoder is Steam
        if k.kind != kind || k.issuer != is || k.account != ac || k.secret != sec || k.period != per || k.digits != dig || k.algo != algo(a) || got_steam != steam || k.uri != u.trim() {
          bad.push(u)
        }
      }
      Err(e) => bad.push("${u}: ${e.display()}")
    }
  }
  expect(bad) == []
  expect((otp.parse_uri("otpauth://totp/a:b?secret=JBSWY3DPEHPK3PXP")?).secret_bytes()?) == b"Hello!\xde\xad\xbe\xef"
  expect((otp.parse_uri("otpauth://totp/a?secret=jbswy3dpehpk3pxp")?).secret_bytes()?) == b"Hello!\xde\xad\xbe\xef"
  expect(otp.parse_uri("otpauth://totp/a?secret=JBSWY3DP1").ok().map(fn(k: otp.Key) -> Bool { k.secret_bytes().is_err() })) == Some(true)
  expect((otp.parse_uri("otpauth://totp/a?secret=JBSWY3DP1")?).secret_bytes().err().map(fn(e: otp.OtpErr) -> Str { e.display() })) == Some("otp: Decoding of secret as base32 failed.")
  expect(otp.parse_uri("%zz").is_err()) == true
}

test "RFC 4226 appendix D" {
  let key = b"12345678901234567890"
  let want = ["755224", "287082", "359152", "969429", "338314", "254676", "287922", "162583", "399871", "520489"]
  var got: List[Str] = []
  for i in 0..10 {
    got.push(otp.hotp(key, i))
  }
  expect(got) == want
}

test "RFC 6238 appendix B" {
  let k1 = b"12345678901234567890"
  let k256 = b"12345678901234567890123456789012"
  let k512 = b"1234567890123456789012345678901234567890123456789012345678901234"
  let times = [59, 1111111109, 1111111111, 1234567890, 2000000000, 20000000000]
  let s1 = ["94287082", "07081804", "14050471", "89005924", "69279037", "65353130"]
  let s256 = ["46119246", "68084774", "67062674", "91819424", "90698825", "77737706"]
  let s512 = ["90693936", "25091201", "99943326", "93441116", "38618901", "47863826"]
  for (i, t) in times.indexed() {
    expect(otp.totp(k1, t, digits: 8)) == s1[i]
    expect(otp.totp(k256, t, digits: 8, algo: crypto.Hash.Sha256)) == s256[i]
    expect(otp.totp(k512, t, digits: 8, algo: crypto.Hash.Sha512)) == s512[i]
  }
}

test "verify returns the matched step: the step and skew steps either side, and refuses bad input" {
  let key = b"12345678901234567890"
  let now = 1111111109
  let step = now / 30
  let here = otp.totp(key, now)
  expect(otp.verify(key, here, now)) == Some(step)
  expect(otp.verify(key, otp.totp(key, now - 30), now)) == Some(step - 1)
  expect(otp.verify(key, otp.totp(key, now + 30), now)) == Some(step + 1)
  expect(otp.verify(key, otp.totp(key, now - 60), now)) == None
  expect(otp.verify(key, otp.totp(key, now - 60), now, skew: 2)) == Some(step - 2)
  expect(otp.verify(key, otp.totp(key, now - 30), now, skew: 0)) == None
  expect(otp.verify(key, "12345", now)) == None
  expect(otp.verify(key, "1234567", now)) == None
  expect(otp.verify(key, "12345a", now)) == None
  expect(otp.verify(key, "", now)) == None
  expect(otp.verify(key, here, 5, skew: 3)) == None
  expect(otp.verify(key, otp.totp(key, 0), 5, skew: 3)) == Some(0)
}

test "a second verify of the same code returns the same step, so a caller detects the replay" {
  let key = b"12345678901234567890"
  let code = otp.totp(key, 1111111109)
  var last = -1
  let first = otp.verify(key, code, 1111111109) ?? -1
  expect(first > last) == true
  last = first
  let again = otp.verify(key, code, 1111111109 + 20) ?? -1
  expect(again) == last
  expect(again > last) == false
  let next = otp.verify(key, otp.totp(key, 1111111109 + 30), 1111111109 + 30) ?? -1
  expect(next > last) == true
}

test "generate_secret has the length asked for" {
  expect(otp.generate_secret(os.rand).len()) == 20
  expect(otp.generate_secret(os.rand, len: 32).len()) == 32
}
`

// otpKeyURIs are parse_uri's inputs: Google Authenticator's examples,
// the corpus's own uri layout, missing and bad parameters, labels with
// and without an issuer, escapes, steam, white space around.
func otpKeyURIs() []string {
	return []string{
		"otpauth://totp/Example:alice@google.com?secret=JBSWY3DPEHPK3PXP&issuer=Example",
		"otpauth://totp/ACME%20Co:john.doe@email.com?secret=HXDMVJECJJWSRB3HWIZR4IFUGFTMXBOZ&issuer=ACME%20Co&algorithm=SHA1&digits=6&period=30",
		"otpauth://hotp/Example:alice?secret=JBSWY3DPEHPK3PXP&counter=7",
		"otpauth://totp/alice@google.com?secret=JBSWY3DPEHPK3PXP",
		"otpauth://totp/Label:acct?secret=abc&issuer=Other&algorithm=sha256&digits=8&period=60",
		"otpauth://totp/a:b:c?secret=X&algorithm=SHA512",
		"otpauth://totp/a?algorithm=MD5&digits=x&period=-5",
		"otpauth://totp/a?digits=007&period=+30",
		"otpauth://totp/a?period=18446744073709551616&digits=99999999999999999999",
		"otpauth://totp/Steam:me?secret=ABCDEFGH&encoder=steam&digits=5",
		"otpauth://totp/Steam:me?secret=ABCDEFGH&encoder=STEAM",
		"otpauth://totp/Steam:me?secret=ABCDEFGH&encoder=other",
		"  otpauth://totp/Pad:me?secret=AB  \t",
		"otpauth://totp/%E6%97%A5%E6%9C%AC:caf%C3%A9?secret=AB&issuer=%E6%97%A5",
		"otpauth://totp/x%3Ay:z?secret=AB",
		"otpauth://totp/?secret=AB",
		"otpauth://totp",
		"otpauth://totp/a?secret=A%20B&issuer=",
		"otpauth://totp/a?secret=1&secret=2",
		"otpauth://totp/a?;bad&secret=Q",
		"http://example.com/x:y?issuer=i",
		"otpauth:opaque?secret=AB",
	}
}

type otpKey struct {
	kind, issuer, account, secret, algo string
	period, digits                      int64
	steam                               bool
}

// otpKeyOf mirrors pquerna/otp v1.5.0's NewKeyFromURL and Key methods
// (otp.go) on net/url; nil when url.Parse fails.
func otpKeyOf(orig string) *otpKey {
	u, err := url.Parse(strings.TrimSpace(orig))
	if err != nil {
		return nil
	}
	q := u.Query()
	p := strings.TrimPrefix(u.Path, "/")
	k := &otpKey{kind: u.Host, secret: q.Get("secret"), period: 30, digits: 6, algo: "Sha1"}
	k.issuer = q.Get("issuer")
	k.account = p
	if i := strings.Index(p, ":"); i >= 0 {
		if k.issuer == "" {
			k.issuer = p[:i]
		}
		k.account = p[i+1:]
	}
	if v, err := strconv.ParseUint(q.Get("period"), 10, 64); err == nil && v <= 1<<63-1 {
		k.period = int64(v)
	}
	if v, err := strconv.ParseUint(q.Get("digits"), 10, 64); err == nil && v <= 1<<63-1 {
		k.digits = int64(v)
	}
	switch strings.ToLower(q.Get("algorithm")) {
	case "md5":
		k.algo = "Md5"
	case "sha256":
		k.algo = "Sha256"
	case "sha512":
		k.algo = "Sha512"
	}
	k.steam = strings.ToLower(q.Get("encoder")) == "steam"
	return k
}

// otpImageCorpus: pquerna/otp's Key.Image (boombuler/barcode's QR code at
// level M, scaled) run by internal/stdtest/qrref on the key URIs above,
// on contents for the numeric and alphanumeric modes and on long URIs up
// to version 40 and past it, at several sizes; the Xo test hashes its
// image's dark modules as qrref prints them.
func otpImageCorpus(o *strings.Builder) {
	var contents []string
	for _, u := range otpKeyURIs() {
		if u == strings.TrimSpace(u) {
			contents = append(contents, u)
		}
	}
	contents = append(contents, "", "0123456789", "+12", "-0", "-12", "12a", "HELLO WORLD $%*+-./:", "HELLO world", "A",
		strings.Repeat("7", 3000), strings.Repeat("8", 3100), strings.Repeat("AB:", 600), strings.Repeat("Z", 1900))
	for _, n := range []int{60, 120, 200, 300, 500, 800, 1200, 1700, 2300, 2331, 2332, 2400} {
		u := "otpauth://totp/Issuer:me?secret=JBSWY3DPEHPK3PXP&issuer="
		contents = append(contents, u+strings.Repeat("x", n-len(u)))
	}
	sizes := [][2]int{{200, 200}, {256, 128}, {21, 21}, {25, 25}, {20, 20}, {0, 0}, {300, 41}, {57, 299}}
	type qrCase struct {
		s    string
		w, h int
	}
	var cases []qrCase
	for _, c := range contents {
		if len(c) > 400 {
			cases = append(cases, qrCase{c, 200, 200}, qrCase{c, 400, 380})
			continue
		}
		for _, sz := range sizes {
			cases = append(cases, qrCase{c, sz[0], sz[1]})
		}
	}
	var in bytes.Buffer
	for _, c := range cases {
		fmt.Fprintf(&in, "%s\t%d\t%d\n", c.s, c.w, c.h)
	}
	cmd := exec.Command("go", "run", ".")
	cmd.Dir = filepath.Join("internal", "stdtest", "qrref")
	cmd.Stdin = &in
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "stdcorpus: the QR oracle (internal/stdtest/qrref) failed: %v\n", err)
		os.Exit(1)
	}
	lines := strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")
	if len(lines) != len(cases) {
		fmt.Fprintf(os.Stderr, "stdcorpus: qrref gave %d lines for %d cases\n", len(lines), len(cases))
		os.Exit(1)
	}
	o.WriteString("\n/// (content, width, height, qrref's Key.Image: \"W H\" and the SHA-256 of its dark modules as 1 and light as 0 row by row, or ERR and the error)\nconst IMAGES: List[(Str, Int, Int, Str)] = [\n")
	for i, c := range cases {
		want := lines[i]
		if !strings.HasPrefix(want, "ERR ") {
			f := strings.SplitN(want, " ", 3)
			want = f[0] + " " + f[1] + " " + hex.EncodeToString(sha256Of(f[2]))
		}
		fmt.Fprintf(o, "  (%s, %d, %d, %s),\n", xq(c.s), c.w, c.h, xq(want))
	}
	o.WriteString("]\n")
	o.WriteString(`
fn image_of(s: Str, w: Int, h: Int) -> Str {
  let k = otp.Key{uri: s, kind: "totp", issuer: "", account: "", secret: "", period: 30, digits: 6, algo: crypto.Hash.Sha1, encoder: otp.Encoder.Decimal}
  match k.image(w, h) {
    Ok(img) => {
      var b = BytesBuilder.with_capacity(img.pix.len())
      for p in img.pix {
        b.write_byte(if p == 0 { '1' } else { '0' })
      }
      "${img.width} ${img.height} ${crypto.sha256(b.freeze()).to_hex()}"
    }
    Err(e) => "ERR ${e.msg}"
  }
}

test "image is pquerna's Key.Image (boombuler's QR code at level M, scaled)" {
  var bad: List[Str] = []
  for (s, w, h, want) in IMAGES {
    let got = image_of(s, w, h)
    if got != want {
      bad.push("${s.len()} bytes ${s[0..(if s.len() < 40 { s.len() } else { 40 })]} ${w}x${h}: ${got}")
    }
  }
  expect(bad) == []
  // the image is a PNG Gray image
  let k = otp.parse_uri("otpauth://totp/Example:alice@google.com?secret=JBSWY3DPEHPK3PXP&issuer=Example")?
  let img = k.image(200, 200)?
  expect(img.model) == png.Model.Gray
  expect(png.decode(png.encode(img)?)?.pix) == img.pix
}
`)
}

func sha256Of(s string) []byte {
	h := sha256.Sum256([]byte(s))
	return h[:]
}

// otpKeyURI is pquerna/otp's Generate URI: url.URL{Scheme: "otpauth",
// Host: kind, Path: "/ISSUER:ACCOUNT", RawQuery: internal.EncodeQuery(v)}
// with EncodeQuery's sorted keys and url.PathEscape on keys and values.
func otpKeyURI(kind, issuer, account string, v url.Values) string {
	keys := make([]string, 0, len(v))
	for k := range v {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var q strings.Builder
	for _, k := range keys {
		for _, x := range v[k] {
			if q.Len() > 0 {
				q.WriteByte('&')
			}
			q.WriteString(url.PathEscape(k) + "=" + url.PathEscape(x))
		}
	}
	u := url.URL{Scheme: "otpauth", Host: kind, Path: "/" + issuer + ":" + account, RawQuery: q.String()}
	return u.String()
}
