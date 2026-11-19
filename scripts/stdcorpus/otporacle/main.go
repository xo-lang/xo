// Command otporacle is the oracle of TestOtpFuzz (internal/stdtest, task
// #117): github.com/pquerna/otp v1.5.0 (std/otp's reference, decision
// 0473) on seeded cases, many mutated. "go run . SEED N" prints N blocks
// "== i", "c CASE" (tab separated fields) and "w WANT", or "w skip" for a
// case outside what std/otp promises to match (each departure modeled
// here is in std/otp's doc). Cases:
//
//	h SECRETHEX COUNTER DIGITS ALGO ENC        hotp.GenerateCodeCustom
//	t SECRETHEX UNIX PERIOD DIGITS ALGO ENC    totp.GenerateCodeCustom
//	v SECRETHEX CODEHEX UNIX PERIOD DIGITS ALGO SKEW ENC   totp.ValidateCustom
//	u URIHEX                                   NewKeyFromURL and the Key's methods
//	g KIND ISSUERHEX ACCOUNTHEX SECRETHEX PERIOD DIGITS ALGO   totp/hotp.Generate
//	q URIHEX W H                               Key.Image: size and sha256 of its gray pixels
package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/base32"
	"encoding/hex"
	"fmt"
	"image/color"
	"math"
	"math/rand"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/boombuler/barcode"
	"github.com/boombuler/barcode/qr"
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/hotp"
	"github.com/pquerna/otp/totp"

	"stdcorpus/otporacle/hostile"
)

var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

var algos = []string{"Sha1", "Sha256", "Sha512"}

func algoOf(s string) otp.Algorithm {
	switch s {
	case "Sha256":
		return otp.AlgorithmSHA256
	case "Sha512":
		return otp.AlgorithmSHA512
	}
	return otp.AlgorithmSHA1
}

func encOf(s string) otp.Encoder {
	if s == "steam" {
		return otp.EncoderSteam
	}
	return otp.EncoderDefault
}

// hostileNonNeg is one of the hostile dictionary's Int edges that is not
// negative (task #352; counters and times are not), chosen by k so the
// cases after it stay as they were.
func hostileNonNeg(k int) int64 {
	var vs []int64
	for _, v := range hostile.Ints() {
		if v >= 0 {
			vs = append(vs, v)
		}
	}
	return vs[k%len(vs)]
}

type gen struct{ r *rand.Rand }

func (g gen) secret() []byte {
	n := []int{0, 1, 5, 10, 16, 20, 32, 64, 65, 128, 200}[g.r.Intn(11)]
	if g.r.Intn(3) == 0 {
		n = g.r.Intn(80)
	}
	b := make([]byte, n)
	g.r.Read(b)
	return b
}

// digits and encoder within std/otp's domain (6 to 8 decimal, 1 to 8 Steam)
func (g gen) digitsEnc() (int, string) {
	if g.r.Intn(4) == 0 {
		return 1 + g.r.Intn(8), "steam"
	}
	return 6 + g.r.Intn(3), "dec"
}

func (g gen) counter() uint64 {
	switch g.r.Intn(5) {
	case 0:
		return uint64(g.r.Intn(4))
	case 1:
		return uint64(math.MaxInt64 - g.r.Intn(4))
	case 2:
		if s := g.r.Intn(63); s%9 != 4 {
			return uint64(1) << uint(s)
		} else {
			return uint64(hostileNonNeg(s))
		}
	}
	return uint64(g.r.Int63n(1 << 40))
}

func (g gen) unix() int64 {
	switch g.r.Intn(6) {
	case 0:
		return int64(g.r.Intn(200))
	case 1:
		return math.MaxInt64 - int64(g.r.Intn(4))
	case 2:
		if s := g.r.Intn(63); s%9 != 4 {
			return int64(1) << uint(s)
		} else {
			return hostileNonNeg(s)
		}
	}
	return 1_500_000_000 + g.r.Int63n(2_000_000_000)
}

func (g gen) period() int64 {
	switch g.r.Intn(5) {
	case 0:
		return 1 + int64(g.r.Intn(5))
	case 1:
		return 30
	case 2:
		return 60
	case 3:
		return int64(g.r.Int63n(1 << 40))
	}
	return 1 + int64(g.r.Intn(600))
}

// step is std/otp's unix / period; pquerna/otp floors the float64
// quotient, which rounds once the operands pass 2^53 (a modeled departure)
func floatStep(unix, period int64) (uint64, bool) {
	f := uint64(math.Floor(float64(unix) / float64(period)))
	return f, int64(f) == unix/period && f <= math.MaxInt64
}

var pieces = []string{"otpauth://", "totp", "hotp", "/", ":", "?", "&", "=", "%", "%3A", "%2", "%zz", "#", "+", " ", ";",
	"secret=", "issuer=", "period=", "digits=", "algorithm=", "encoder=", "steam", "STEAM", "sha256", "SHA512", "md5", "MD5",
	"JBSWY3DPEHPK3PXP", "jbswy3dp", "====", "8", "-1", "18446744073709551615", "9223372036854775808", "0", "007", "\t", "\n",
	"é", "x@y.test", "[::1]", "@", "Issuer Co", "a/b", " "}

func (g gen) uri() string {
	issuer := []string{"Example", "ACME Co", "a:b", "", "é", "x/y", "100%"}[g.r.Intn(7)]
	account := []string{"alice@example.com", "bob", "", "a b", "c:d", "ü"}[g.r.Intn(6)]
	kind := []string{"totp", "hotp", "TOTP", ""}[g.r.Intn(4)]
	var q []string
	add := func(k, v string) {
		if g.r.Intn(4) != 0 {
			q = append(q, k+"="+v)
		}
	}
	add("secret", b32.EncodeToString(g.secret()))
	add("issuer", url.QueryEscape(issuer))
	add("period", []string{"30", "60", "0", "x", "-5", "18446744073709551615", "9223372036854775807", "9223372036854775808"}[g.r.Intn(8)])
	add("digits", []string{"6", "7", "8", "1", "10", "x", "18446744073709551615"}[g.r.Intn(7)])
	add("algorithm", []string{"SHA1", "SHA256", "sha512", "MD5", "foo"}[g.r.Intn(5)])
	if g.r.Intn(5) == 0 {
		add("encoder", []string{"steam", "Steam", "none"}[g.r.Intn(3)])
	}
	g.r.Shuffle(len(q), func(i, j int) { q[i], q[j] = q[j], q[i] })
	s := "otpauth://" + kind + "/" + url.PathEscape(issuer) + ":" + url.PathEscape(account) + "?" + strings.Join(q, "&")
	if g.r.Intn(5) == 0 {
		return s
	}
	b := []byte(s)
	for m := 1 + g.r.Intn(3); m > 0; m-- {
		p := 0
		if len(b) > 0 {
			p = g.r.Intn(len(b) + 1)
		}
		switch g.r.Intn(4) {
		case 0:
			ins := pieces[g.r.Intn(len(pieces))]
			b = append(b[:p:p], append([]byte(ins), b[p:]...)...)
		case 1:
			if p < len(b) {
				b = append(b[:p:p], b[p+1:]...)
			}
		case 2:
			if p < len(b) {
				b[p] = byte(32 + g.r.Intn(95))
			}
		default:
			b = b[:p]
		}
	}
	return string(b)
}

func boolTxt(ok bool) string {
	if ok {
		return "true"
	}
	return "false"
}

func main() {
	seed, _ := strconv.ParseInt(os.Args[1], 10, 64)
	n, _ := strconv.Atoi(os.Args[2])
	g := gen{rand.New(rand.NewSource(seed))}
	out := bufio.NewWriter(os.Stdout)
	defer out.Flush()
	for i := 0; i < n; i++ {
		c, w := one(g)
		fmt.Fprintf(out, "== %d\nc %s\nw %s\n", i, c, w)
	}
}

func one(g gen) (string, string) {
	switch k := g.r.Intn(20); {
	case k < 4:
		s := g.secret()
		ctr := g.counter()
		d, enc := g.digitsEnc()
		a := algos[g.r.Intn(3)]
		code, err := hotp.GenerateCodeCustom(b32.EncodeToString(s), ctr, hotp.ValidateOpts{Digits: otp.Digits(d), Algorithm: algoOf(a), Encoder: encOf(enc)})
		if err != nil {
			code = "err"
		}
		return fmt.Sprintf("h\t%x\t%d\t%d\t%s\t%s", s, ctr, d, a, enc), code
	case k < 8:
		s := g.secret()
		unix, period := g.unix(), g.period()
		d, enc := g.digitsEnc()
		a := algos[g.r.Intn(3)]
		c := fmt.Sprintf("t\t%x\t%d\t%d\t%d\t%s\t%s", s, unix, period, d, a, enc)
		if _, ok := floatStep(unix, period); !ok {
			return c, "skip"
		}
		code, err := totp.GenerateCodeCustom(b32.EncodeToString(s), time.Unix(unix, 0), totp.ValidateOpts{Period: uint(period), Digits: otp.Digits(d), Algorithm: algoOf(a), Encoder: encOf(enc)})
		if err != nil {
			code = "err"
		}
		return c, code
	case k < 12:
		s := g.secret()
		unix, period := g.unix(), g.period()
		if g.r.Intn(2) == 0 {
			unix = int64(g.r.Intn(1000))
			period = 1 + int64(g.r.Intn(40))
		}
		d, enc := g.digitsEnc()
		a := algos[g.r.Intn(3)]
		skew := []int{0, 1, 1, 2, 5}[g.r.Intn(5)]
		opts := totp.ValidateOpts{Period: uint(period), Skew: uint(skew), Digits: otp.Digits(d), Algorithm: algoOf(a), Encoder: encOf(enc)}
		step, fok := floatStep(unix, period)
		// a code of a nearby step, or that code changed
		near := int64(step) + int64(g.r.Intn(2*skew+3)) - int64(skew) - 1
		if near < 0 {
			near = 0
		}
		code, _ := hotp.GenerateCodeCustom(b32.EncodeToString(s), uint64(near), hotp.ValidateOpts{Digits: otp.Digits(d), Algorithm: algoOf(a), Encoder: encOf(enc)})
		switch g.r.Intn(6) {
		case 0:
			b := []byte(code)
			if len(b) > 0 {
				b[g.r.Intn(len(b))] = byte(32 + g.r.Intn(95))
			}
			code = string(b)
		case 1:
			if len(code) > 0 {
				code = code[1:]
			}
		case 2:
			code = []string{" ", "\t", "0", "x"}[g.r.Intn(4)] + code
		}
		c := fmt.Sprintf("v\t%x\t%x\t%d\t%d\t%d\t%s\t%d\t%s", s, code, unix, period, d, a, skew, enc)
		// departures: pquerna/otp trims the code (std/otp: a code of other
		// bytes matches nothing); a step under 0 wraps to 2^64 - k in
		// pquerna's (std/otp skips it); the float step past 2^53
		if !fok || strings.TrimSpace(code) != code || int64(step) < int64(skew) || int64(step)+int64(skew) < 0 {
			return c, "skip"
		}
		ok, err := totp.ValidateCustom(code, b32.EncodeToString(s), time.Unix(unix, 0), opts)
		if err != nil {
			ok = false
		}
		return c, boolTxt(ok)
	case k < 17:
		s := g.uri()
		c := fmt.Sprintf("u\t%x", s)
		if !utf8.ValidString(s) {
			return c, "skip"
		}
		key, err := otp.NewKeyFromURL(s)
		if err != nil {
			return c, "err"
		}
		sb := "err"
		raw := strings.TrimSpace(key.Secret())
		if n := len(raw) % 8; n != 0 {
			raw += strings.Repeat("=", 8-n)
		}
		if b, err := base32.StdEncoding.DecodeString(strings.ToUpper(raw)); err == nil {
			sb = hex.EncodeToString(b)
		}
		return c, fmt.Sprintf("%x|%x|%x|%x|%x|%d|%d|%s|%s|%s", key.String(), xoStr(key.Type()), xoStr(key.Issuer()), xoStr(key.AccountName()), xoStr(key.Secret()), fitInt(uint64(key.Period()), 30), fitInt(uint64(key.Digits()), 6), key.Algorithm(), encName(key.Encoder()), sb)
	case k < 19:
		kind := []string{"totp", "hotp"}[g.r.Intn(2)]
		iss := []string{"Example", "ACME Co", "a:b", "é", "x/y", "100%", "a?b#c", "a&b=c", "+ ;"}[g.r.Intn(9)]
		acc := []string{"alice@example.com", "bob", "a b", "c:d", "ü", "%41", "x?y"}[g.r.Intn(7)]
		s := g.secret()
		if len(s) == 0 {
			s = []byte{1}
		}
		period := uint(g.period())
		d := 6 + g.r.Intn(3)
		a := algos[g.r.Intn(3)]
		c := fmt.Sprintf("g\t%s\t%x\t%x\t%x\t%d\t%d\t%s", kind, iss, acc, s, period, d, a)
		var key *otp.Key
		var err error
		if kind == "totp" {
			key, err = totp.Generate(totp.GenerateOpts{Issuer: iss, AccountName: acc, Secret: s, Period: period, Digits: otp.Digits(d), Algorithm: algoOf(a)})
		} else {
			key, err = hotp.Generate(hotp.GenerateOpts{Issuer: iss, AccountName: acc, Secret: s, Digits: otp.Digits(d), Algorithm: algoOf(a)})
		}
		if err != nil {
			return c, "err"
		}
		return c, fmt.Sprintf("%x", key.String())
	default:
		s := g.uri()
		w, h := g.r.Intn(400), g.r.Intn(400)
		c := fmt.Sprintf("q\t%x\t%d\t%d", s, w, h)
		if !utf8.ValidString(s) {
			return c, "skip"
		}
		// Key.Image's body on the trimmed text (std/otp's Key holds any uri)
		b, err := qr.Encode(strings.TrimSpace(s), qr.M, qr.Auto)
		if err == nil {
			b, err = barcode.Scale(b, w, h)
		}
		if err != nil {
			return c, "err " + err.Error()
		}
		r := b.Bounds()
		px := make([]byte, 0, r.Dx()*r.Dy())
		for y := r.Min.Y; y < r.Max.Y; y++ {
			for x := r.Min.X; x < r.Max.X; x++ {
				px = append(px, color.GrayModel.Convert(b.At(x, y)).(color.Gray).Y)
			}
		}
		return c, fmt.Sprintf("%dx%d %x", r.Dx(), r.Dy(), sha256.Sum256(px))
	}
}

// fitInt: std/otp reads period and digits as Int, the default past
// Int's range (pquerna/otp keeps the uint64, or wraps it to an int)
func fitInt(v uint64, d int64) int64 {
	if v > math.MaxInt64 {
		return d
	}
	return int64(v)
}

func encName(e otp.Encoder) string {
	if e == otp.EncoderSteam {
		return "steam"
	}
	return "dec"
}

// xoStr: s as std/url hands it to std/otp, each byte of an invalid UTF-8
// sequence as its %XX escape (std/url's rule, spec/std.md 0: a Str is UTF-8)
func xoStr(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		r, n := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && n == 1 {
			fmt.Fprintf(&b, "%%%02X", s[i])
		} else {
			b.WriteString(s[i : i+n])
		}
		i += n
	}
	return b.String()
}
