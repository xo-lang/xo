package main

import (
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
	"net/url"
	"strings"
)

func otpHotp(secret []byte, counter uint64, digits int, algo string) string {
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
	o.WriteString("use std/otp\nuse std/crypto\n\n")
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
		u := "otpauth://totp/" + url.PathEscape(is) + ":" + url.PathEscape(ac) + "?" + q.Encode()
		o.WriteString(fmt.Sprintf("  (%s, %s, %q, %d, %d, %q, %s),\n", xq(is), xq(ac), hex.EncodeToString(sec), per, d, a, xq(u)))
	}
	o.WriteString("]\n\n")
	o.WriteString(otpTests)
	return o.String()
}

const otpTests = `fn hex(h: Str) -> Bytes { Bytes.from_hex(h) ?? b"" }

fn algo(name: Str) -> crypto.Hash {
  if name == "Sha256" {
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

test "uri matches net/url's layout" {
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

test "verify accepts the step and skew steps either side, and refuses bad input" {
  let key = b"12345678901234567890"
  let now = 1111111109
  let here = otp.totp(key, now)
  expect(otp.verify(key, here, now)) == true
  expect(otp.verify(key, otp.totp(key, now - 30), now)) == true
  expect(otp.verify(key, otp.totp(key, now + 30), now)) == true
  expect(otp.verify(key, otp.totp(key, now - 60), now)) == false
  expect(otp.verify(key, otp.totp(key, now - 60), now, skew: 2)) == true
  expect(otp.verify(key, otp.totp(key, now - 30), now, skew: 0)) == false
  expect(otp.verify(key, "12345", now)) == false
  expect(otp.verify(key, "1234567", now)) == false
  expect(otp.verify(key, "12345a", now)) == false
  expect(otp.verify(key, "", now)) == false
  expect(otp.verify(key, here, 5, skew: 3)) == false
  expect(otp.verify(key, otp.totp(key, 0), 5, skew: 3)) == true
}

test "generate_secret has the length asked for" {
  expect(otp.generate_secret(os.rand).len()) == 20
  expect(otp.generate_secret(os.rand, len: 32).len()) == 32
}
`
