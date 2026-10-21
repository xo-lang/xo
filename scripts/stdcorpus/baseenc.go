package main

import (
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
)

type baseEnc struct {
	name string // the Xo variant
	enc  interface {
		EncodeToString([]byte) string
		DecodeString(string) ([]byte, error)
	}
}

// baseCorpus is the corpus of std/base64 or std/base32: Go's encoding of
// random bytes, and Go's decoding of encodings changed in the ways
// decoders trip on (newlines, padding, cuts, foreign bytes).
func baseCorpus(mod string, seed int64, encs []baseEnc, alphabet string) string {
	r := rand.New(rand.NewSource(seed))
	enumName := map[string]string{"base64": "Base64", "base32": "Base32"}[mod]
	var o strings.Builder
	o.WriteString(header("encoding/" + mod + "'s four encodings as std/rt/base64.go calls them, on seeded random bytes and on encodings with newlines, padding changes, cuts, and foreign bytes"))
	fmt.Fprintf(&o, "use std/%s\n\n/// (variant, data, Go's encoding)\nconst ENCODE: List[(Int, Bytes, Str)] = [\n", mod)
	for i := 0; i < 600; i++ {
		k := r.Intn(len(encs))
		b := make([]byte, r.Intn(40))
		r.Read(b)
		fmt.Fprintf(&o, "  (%d, %s, %s),\n", k, xbytes(b), xq(encs[k].enc.EncodeToString(b)))
	}
	o.WriteString("]\n\n/// (variant, text, Go's decoding: ok HEX or err AT)\nconst DECODE: List[(Int, Str, Str)] = [\n")
	for i := 0; i < 1500; i++ {
		k := r.Intn(len(encs))
		b := make([]byte, r.Intn(30))
		r.Read(b)
		s := []byte(encs[r.Intn(len(encs))].enc.EncodeToString(b))
		for j, n := 0, r.Intn(3); j < n; j++ {
			at := r.Intn(len(s) + 1)
			ins := []string{"\n", "\r\n", "=", "==", " ", "!", "é", string(alphabet[r.Intn(len(alphabet))]), "-", "_", "+", "/"}[r.Intn(12)]
			switch r.Intn(3) {
			case 0:
				s = append(s[:at:at], append([]byte(ins), s[at:]...)...)
			case 1:
				if len(s) > 0 {
					s = s[:r.Intn(len(s))]
				}
			default:
				if at < len(s) {
					s = append(s[:at:at], s[at+1:]...)
				}
			}
		}
		str := textOf(string(s))
		w := ""
		d, err := encs[k].enc.DecodeString(str)
		if err != nil {
			at, _ := strconv.Atoi(strings.TrimPrefix(err.Error(), "illegal "+mod+" data at input byte "))
			w = fmt.Sprintf("err %d", at)
		} else {
			w = "ok " + hex.EncodeToString(d)
		}
		fmt.Fprintf(&o, "  (%d, %s, %s),\n", k, xq(str), xq(w))
	}
	o.WriteString("]\n")
	var vs []string
	for _, e := range encs {
		vs = append(vs, mod+"."+enumName+"."+e.name)
	}
	fmt.Fprintf(&o, `
const VARIANTS: List[%s.%s] = [%s]

test "encode writes Go's text" {
  var bad: List[(Int, Bytes, Str, Str)] = []
  for (k, data, want) in ENCODE {
    let got = %s.encode(data, VARIANTS[k])
    if got != want {
      bad.push((k, data, got, want))
    }
  }
  expect(bad[0..(if bad.len() < 20 { bad.len() } else { 20 })]) == []
}

test "decode gives Go's bytes or Go's error offset" {
  var bad: List[(Int, Str, Str, Str)] = []
  for (k, s, want) in DECODE {
    let got = match %s.decode(s, VARIANTS[k]) {
      Ok(b) => "ok ${b.to_hex()}"
      Err(e) => "err ${e.at}"
    }
    if got != want {
      bad.push((k, s, got, want))
    }
  }
  expect(bad[0..(if bad.len() < 20 { bad.len() } else { 20 })]) == []
}
`, mod, enumName, strings.Join(vs, ", "), mod, mod)
	return o.String()
}

func base64Corpus() string {
	return baseCorpus("base64", 9, []baseEnc{{"Std", base64.StdEncoding}, {"Url", base64.URLEncoding}, {"RawStd", base64.RawStdEncoding}, {"RawUrl", base64.RawURLEncoding}}, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/-_")
}

func base32Corpus() string {
	return baseCorpus("base32", 10, []baseEnc{{"Std", base32.StdEncoding}, {"Hex", base32.HexEncoding}, {"RawStd", base32.StdEncoding.WithPadding(base32.NoPadding)}, {"RawHex", base32.HexEncoding.WithPadding(base32.NoPadding)}}, "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567abcdefghijklmnopqrstuv0189")
}
