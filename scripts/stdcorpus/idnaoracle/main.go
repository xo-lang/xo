// Command idnaoracle prints the std/idna corpus: golang.org/x/net v0.60.0's
// idna (Unicode 15.0.0 for the go1.26 build) ToASCII and ToUnicode for its
// four profiles on hand picked names and seeded random names (mixed scripts,
// joiners, hyphens, dots, punycode labels valid and not), with the error's
// code. scripts/stdcorpus runs it; it is its own module so the repository's
// go.mod has no third-party requirement.
package main

import (
	"fmt"
	"math/rand"
	"reflect"
	"strings"

	"golang.org/x/net/idna"
)

func xq(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"' || r == '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r == '$':
			b.WriteString(`\$`)
		case r >= 0x20 && r < 0x7f:
			b.WriteRune(r)
		default:
			fmt.Fprintf(&b, `\u{%x}`, r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

func code(err error) string {
	if err == nil {
		return ""
	}
	v := reflect.ValueOf(err)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	return v.FieldByName("code_").String()
}

// punycodeVectors are x/net/idna's punycode_test.go cases (the RFC 3492 section 7.1 samples with Errata 3026 and short ones; Copyright 2016 The Go Authors, BSD-style license): a string and its punycode.
var punycodeVectors = [][2]string{
	{"", ""},
	{"-", "--"},
	{"-a", "-a-"},
	{"-a-", "-a--"},
	{"a", "a-"},
	{"a-", "a--"},
	{"a-b", "a-b-"},
	{"books", "books-"},
	{"bücher", "bcher-kva"},
	{"Hello世界", "Hello-ck1hg65u"},
	{"ü", "tda"},
	{"üý", "tdac"},
	{"\u0644\u064A\u0647\u0645\u0627\u0628\u062A\u0643\u0644" + "\u0645\u0648\u0634\u0639\u0631\u0628\u064A\u061F", "egbpdaj6bu4bxfgehfvwxn"},
	{"\u4ED6\u4EEC\u4E3A\u4EC0\u4E48\u4E0D\u8BF4\u4E2D\u6587", "ihqwcrb4cv8a8dqg056pqjye"},
	{"\u4ED6\u5011\u7232\u4EC0\u9EBD\u4E0D\u8AAA\u4E2D\u6587", "ihqwctvzc91f659drss3x8bo0yb"},
	{"\u0050\u0072\u006F\u010D\u0070\u0072\u006F\u0073\u0074" + "\u011B\u006E\u0065\u006D\u006C\u0075\u0076\u00ED\u010D" + "\u0065\u0073\u006B\u0079", "Proprostnemluvesky-uyb24dma41a"},
	{"\u05DC\u05DE\u05D4\u05D4\u05DD\u05E4\u05E9\u05D5\u05D8" + "\u05DC\u05D0\u05DE\u05D3\u05D1\u05E8\u05D9\u05DD\u05E2" + "\u05D1\u05E8\u05D9\u05EA", "4dbcagdahymbxekheh6e0a7fei0b"},
	{"\u092F\u0939\u0932\u094B\u0917\u0939\u093F\u0928\u094D" + "\u0926\u0940\u0915\u094D\u092F\u094B\u0902\u0928\u0939" + "\u0940\u0902\u092C\u094B\u0932\u0938\u0915\u0924\u0947" + "\u0939\u0948\u0902", "i1baa7eci9glrd9b2ae1bj0hfcgg6iyaf8o0a1dig0cd"},
	{"\u306A\u305C\u307F\u3093\u306A\u65E5\u672C\u8A9E\u3092" + "\u8A71\u3057\u3066\u304F\u308C\u306A\u3044\u306E\u304B", "n8jok5ay5dzabd5bym9f0cm5685rrjetr6pdxa"},
	{"\uC138\uACC4\uC758\uBAA8\uB4E0\uC0AC\uB78C\uB4E4\uC774" + "\uD55C\uAD6D\uC5B4\uB97C\uC774\uD574\uD55C\uB2E4\uBA74" + "\uC5BC\uB9C8\uB098\uC88B\uC744\uAE4C", "989aomsvi5e83db1d2a355cv1e0vak1dwrv93d5xbh15a0dt30a5j" + "psd879ccm6fea98c"},
	{"\u043F\u043E\u0447\u0435\u043C\u0443\u0436\u0435\u043E" + "\u043D\u0438\u043D\u0435\u0433\u043E\u0432\u043E\u0440" + "\u044F\u0442\u043F\u043E\u0440\u0443\u0441\u0441\u043A" + "\u0438", "b1abfaaepdrnnbgefbadotcwatmq2g4l"},
	{"\u0050\u006F\u0072\u0071\u0075\u00E9\u006E\u006F\u0070" + "\u0075\u0065\u0064\u0065\u006E\u0073\u0069\u006D\u0070" + "\u006C\u0065\u006D\u0065\u006E\u0074\u0065\u0068\u0061" + "\u0062\u006C\u0061\u0072\u0065\u006E\u0045\u0073\u0070" + "\u0061\u00F1\u006F\u006C", "PorqunopuedensimplementehablarenEspaol-fmd56a"},
	{"\u0054\u1EA1\u0069\u0073\u0061\u006F\u0068\u1ECD\u006B" + "\u0068\u00F4\u006E\u0067\u0074\u0068\u1EC3\u0063\u0068" + "\u1EC9\u006E\u00F3\u0069\u0074\u0069\u1EBF\u006E\u0067" + "\u0056\u0069\u1EC7\u0074", "TisaohkhngthchnitingVit-kjcr8268qyxafd2f1b9g"},
	{"\u0033\u5E74\u0042\u7D44\u91D1\u516B\u5148\u751F", "3B-ww4c5e180e575a65lsy2b"},
	{"\u5B89\u5BA4\u5948\u7F8E\u6075\u002D\u0077\u0069\u0074" + "\u0068\u002D\u0053\u0055\u0050\u0045\u0052\u002D\u004D" + "\u004F\u004E\u004B\u0045\u0059\u0053", "-with-SUPER-MONKEYS-pc58ag80a8qai00g7n9n"},
	{"\u0048\u0065\u006C\u006C\u006F\u002D\u0041\u006E\u006F" + "\u0074\u0068\u0065\u0072\u002D\u0057\u0061\u0079\u002D" + "\u305D\u308C\u305E\u308C\u306E\u5834\u6240", "Hello-Another-Way--fc4qua05auwb3674vfr0b"},
	{"\u3072\u3068\u3064\u5C4B\u6839\u306E\u4E0B\u0032", "2-u9tlzr9756bt3uc0v"},
	{"\u004D\u0061\u006A\u0069\u3067\u004B\u006F\u0069\u3059" + "\u308B\u0035\u79D2\u524D", "MajiKoi5-783gue6qz075azm5e"},
	{"\u30D1\u30D5\u30A3\u30FC\u0064\u0065\u30EB\u30F3\u30D0", "de-jg4avhby1noc0d"},
	{"\u305D\u306E\u30B9\u30D4\u30FC\u30C9\u3067", "d9juau41awczczp"},
	{"\u002D\u003E\u0020\u0024\u0031\u002E\u0030\u0030\u0020" + "\u003C\u002D", "-> $1.00 <--"},
}

var profiles = []*idna.Profile{idna.Lookup, idna.Registration, idna.Display, idna.Punycode}

func main() {
	var inputs []string
	hand := []string{
		"example.com", "EXAMPLE.COM", "Example.Com.", "a.b.c", "", ".", "..", "a..b", "a..", ".a", "a.b.",
		"bücher.de", "BÜCHER.de", "faß.de", "ﬁsh.com", "ß", "ς.gr", "Σ.gr", "例え.jp", "例え.テスト", "日本語.jp", "правда.рф", "пример.испытание",
		"xn--bcher-kva.de", "xn--fa-hia.de", "xn--r8jz45g.jp", "xn--80aswg.xn--p1ai", "xn--", "xn---", "xn--a", "xn--abc-", "xn--zca.com",
		"xn--bcher-kva.", "XN--BCHER-KVA.de", "xn--0.pt", "xn--a.com", "xn--ls8h.la", "💩.la", "xn--ls8h",
		"a_b.com", "a b.com", "-a.com", "a-.com", "ab--c.com", "a--b.com", "xn--ab--c.com", "1.2.3.4", "[::1]", "a.com:80",
		"‍.com", "a‍b.com", "क्‍ष.in", "ب‌ا.com", "با‌ب.com", "a‌b",
		"العربية.com", "אבג.com", "אa.com", "aא.com", "א١.com", "א1.com", "١٢.com",
		"é.com", "́e.com", "ä.com", "가.kr", "한국.kr", "。", "a。b", "a．b", "a｡b", "Ａ.com", "ＡＢＣ.com",
		" .com", "�.com", "­.com", "a­b.com", "​.com", "⁠.com", "xn--aé.com", strings.Repeat("a", 63) + ".com", strings.Repeat("a", 64) + ".com",
		strings.Repeat("é", 40) + ".com", strings.Repeat("a.", 130) + "com", strings.Repeat("bücher", 12) + ".de",
	}
	inputs = append(inputs, hand...)
	for _, v := range punycodeVectors {
		inputs = append(inputs, v[0]+".com", "xn--"+v[1]+".com", "xn--"+v[1], v[0])
	}
	rg := rand.New(rand.NewSource(61))
	pools := [][]rune{
		[]rune("abcdefghijklmnopqrstuvwxyz"), []rune("ABCDEFGHIJKLMNOPQRSTUVWXYZ"), []rune("0123456789"), []rune("-_ !"),
		[]rune("éüñåøçßΣςλбжя日本語한ひらカ"), {0x301, 0x308, 0x323, 0x200c, 0x200d, 0x94d, 0x64b, 0x300},
		[]rune("אבגדהמ"), []rune("ابتثجحخ"), {0x660, 0x661, 0x6f1}, {0xff21, 0xfb01, 0x2126, 0x3002, 0xff0e, 0xad, 0x2028, 0x1f4a9, 0xfffd},
		[]rune("abcdefghijklmnopqrstuvwxyz"), []rune("abcdefghijklmnopqrstuvwxyz"),
	}
	punylabels := []string{"bcher-kva", "fa-hia", "r8jz45g", "80aswg", "p1ai", "ls8h", "zca", "abc-", "a", "0", "bcher-kvaa", "9ca", "fsq", "wgv71a", "mgbaakc7dvf", "d9juau41awczczp", "Hello-Another-Way--fc4qua05auwb3674vfr0b"}
	for i := 0; i < 900; i++ {
		nl := 1 + rg.Intn(3)
		var labels []string
		for l := 0; l < nl; l++ {
			if rg.Intn(6) == 0 {
				labels = append(labels, "xn--"+punylabels[rg.Intn(len(punylabels))])
				continue
			}
			n := rg.Intn(9)
			var rs []rune
			pi := rg.Intn(len(pools))
			for k := 0; k < n; k++ {
				if rg.Intn(4) == 0 {
					pi = rg.Intn(len(pools))
				}
				p := pools[pi]
				rs = append(rs, p[rg.Intn(len(p))])
			}
			labels = append(labels, string(rs))
		}
		s := strings.Join(labels, ".")
		if rg.Intn(10) == 0 {
			s += "."
		}
		inputs = append(inputs, s)
	}
	var o strings.Builder
	o.WriteString("// Generated by scripts/stdcorpus (scripts/stdcorpus/idnaoracle) from golang.org/x/net v0.60.0's idna (Unicode " + idna.UnicodeVersion + ").\n// DO NOT EDIT; regenerate with `go run ./scripts/stdcorpus idna`.\n\n")
	o.WriteString("use std/idna\n\n")
	o.WriteString("/// (input, profile 0 Lookup 1 Registration 2 Display 3 Punycode, direction 0 ToASCII 1 ToUnicode, ok, result, error code)\nconst CASES: List[(Str, Int, Int, Bool, Str, Str)] = [\n")
	n := 0
	for _, s := range inputs {
		for pi, p := range profiles {
			for dir := 0; dir < 2; dir++ {
				var r string
				var err error
				if dir == 0 {
					r, err = p.ToASCII(s)
				} else {
					r, err = p.ToUnicode(s)
				}
				ok := "true"
				if err != nil {
					ok = "false"
					r = ""
				}
				fmt.Fprintf(&o, "  (%s, %d, %d, %s, %s, %s),\n", xq(s), pi, dir, ok, xq(r), xq(code(err)))
				n++
			}
		}
	}
	o.WriteString("]\n\n")
	o.WriteString(tests)
	fmt.Print(o.String())
}

const tests = `fn profile_of(i: Int) -> idna.Profile {
  if i == 0 {
    idna.Profile.Lookup
  } else if i == 1 {
    idna.Profile.Registration
  } else if i == 2 {
    idna.Profile.Display
  } else {
    idna.Profile.Punycode
  }
}

test "ToASCII and ToUnicode agree with x/net/idna on every profile" {
  var bad: List[Str] = []
  for c in CASES {
    let p = profile_of(c.1)
    let got = if c.2 == 0 { idna.to_ascii(c.0, p) } else { idna.to_unicode(c.0, p) }
    match got {
      Ok(v) => {
        if !c.3 || v != c.4 {
          bad.push("${c.0.quote()} p${c.1} d${c.2}: got ${v.quote()}, want ok=${c.3} ${c.4.quote()} ${c.5}")
        }
      }
      Err(e) => {
        if c.3 || e.code != c.5 {
          bad.push("${c.0.quote()} p${c.1} d${c.2}: got error ${e.code}, want ok=${c.3} ${c.5}")
        }
      }
    }
  }
  expect(bad[0..(if bad.len() < 8 { bad.len() } else { 8 })]) == []
}

test "known names" {
  expect(idna.to_ascii("b\u{fc}cher.de").ok()) == Some("xn--bcher-kva.de")
  expect(idna.to_ascii("fa\u{df}.de").ok()) == Some("xn--fa-hia.de")
  expect(idna.to_ascii("\u{4f8b}\u{3048}.jp").ok()) == Some("xn--r8jz45g.jp")
  expect(idna.to_unicode("xn--bcher-kva.de").ok()) == Some("b\u{fc}cher.de")
  expect(idna.to_ascii("EXAMPLE.com").ok()) == Some("example.com")
  expect(idna.to_ascii("-a.com").ok()) == None
  expect(idna.to_ascii("a b.com").ok()) == None
  expect(idna.unicode_version()) == "15.0.0"
}
`
