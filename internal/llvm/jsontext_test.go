package llvm_test

import (
	"encoding/hex"
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/xo-lang/xo/internal/build"
)

// jsonTextDiffProg reads lines of `<op> <arg>` from standard input and
// prints one line for each: `d` decodes the hex bytes as JSON into
// fourteen types (the parser's errors, the tape readers: strings with
// escapes, Ints, Floats, members with duplicate keys, fields) and encodes
// what decoded again; `f` encodes Float.parse of the text; `s` encodes
// the hex bytes as a Str when they are UTF-8 (decision 0193 step 4).
const jsonTextDiffProg = `use std/json

struct Obj {
  a: Int?
  b: Str?
  c: List[Bool]?
  k9: Float = 0.5
} derive Json

fn show[T: Json](data: Bytes) -> Str {
  match json.decode[T](data) {
    Ok(v) => "ok ${v.debug()} ${json.encode_str(v)}"
    Err(e) => "err ${e.debug()}"
  }
}

fn main(os: Os) {
  let input = match os.stdio.read_all() {
    Ok(s) => s
    Err(_) => ""
  }
  var out: List[Str] = []
  for line in input.lines() {
    if line.len() < 2 {
      out.push("?")
      continue
    }
    let arg = line[2..line.len()]
    let r = match line[0..1] {
      "d" => {
        let data = Bytes.from_hex(arg) ?? b""
        [
          show[Bool](data), show[List[Str]](data), show[Map[Str, Str]](data), show[List[Float]](data),
          show[List[Int]](data), show[Map[Str, List[Int?]]](data), show[Str](data), show[Float](data),
          show[Int](data), show[Obj](data), show[List[Map[Str, Float?]]](data), show[(Int, Str)](data),
          show[Map[Str, Bool?]](data), show[List[List[Int]]](data),
        ].join(" | ")
      }
      "f" => match Float.parse(arg) {
        Some(f) => json.encode_str([f, -f])
        None => "none"
      }
      "s" => match (Bytes.from_hex(arg) ?? b"").to_str() {
        Some(s) => json.encode_str({"k": s})
        None => "none"
      }
      _ => "?"
    }
    out.push(r)
  }
  os.stdio.println(out.join("\n"))
}
`

// jsonTextCorpus is the input of jsonTextDiffProg: the C parser's corpus
// (json_diff_test.go) and random valid and damaged texts, plus deep
// nesting, invalid UTF-8, surrogate escapes, huge numbers, duplicate
// keys in small and large objects (the two ways members finds them),
// trailing data, strings with an escape at every offset around a word
// (the eight byte scans), literals at the end; Floats from random bits
// and edge texts; random UTF-8 strings with the bytes encoding/json
// escapes.
func jsonTextCorpus(n int) string {
	r := rand.New(rand.NewSource(7))
	var b strings.Builder
	doc := func(s string) { b.WriteString("d " + hex.EncodeToString([]byte(s)) + "\n") }
	for _, s := range jsonCorpus {
		doc(s)
	}
	for _, s := range []string{
		strings.Repeat("[", 3000) + strings.Repeat("]", 3000),
		strings.Repeat("[", 3000) + strings.Repeat("]", 2999),
		strings.Repeat(`{"a":`, 2000) + "1" + strings.Repeat("}", 2000),
		strings.Repeat(`[{"a":`, 500) + "[]" + strings.Repeat("}]", 500) + "x",
		"\"\xff\xfe\xfd\"", "[\"a\xc3\"]", "[\"\xe2\x80\"]", "[\"\xed\xa0\x80\"]", "[\"\xf4\x90\x80\x80\"]", "[\"\xc0\xaf\"]", "[\"ok\xe2\x80\xa8\"]",
		`["\ud800"]`, `["\udbff\udfff"]`, `["\ud800\ud800"]`, `["\ud800\u00"]`, `["\ud800\\u0041"]`, `["\ud800x\udc00"]`, `["\uDC00\uD800"]`, `["\u0000"]`, `["\uFFFF"]`,
		"[1e400]", "[-1e400]", "[1e-400]", "[" + strings.Repeat("9", 400) + "]", "[" + strings.Repeat("9", 19) + "]", "[-" + strings.Repeat("9", 19) + "]",
		"[9223372036854775807, -9223372036854775808, 9223372036854775808, -9223372036854775809, 18446744073709551616]",
		"[0.1, 1.0, 1E2, 1e-0, -0, -0.0, 0e5, 2.2250738585072011e-308, 4.9406564584124654e-324, 1.7976931348623159e308]",
		`{"a":1,"a":2}`, `{"b":"x","a":1,"b":"y"}`, `{"a":1,"\u0061":2,"\u0061":3}`, `{"\ud800":1,"\udc00":2}`, "{\"\xff\":1,\"\xfe\":2}",
		`{"k0":1,"k1":2,"k2":3,"k3":4,"k4":5,"k5":6,"k6":7,"k7":8,"k8":9,"k9":10,"k0":11,"k\u0031":12,"k10":13}`,
		`{"a":5,"b":"s","c":[true],"k9":2.5,"z":null}`, `{"a":null,"b":null,"c":null}`, `{"a":"x"}`, `{"c":[1]}`, `{}`,
		"1 x", "1 ]", "1 [", "1 {", "1 \"a\"", "1 \"a", "1 tru", "1 true", "[] ,", "[]:", "{} 1", "\t\n\r 7 \t\n\r", "  null  null",
		"tru", "fals", "nul", "t", "true", "false", "null", "[true", "[tru", "{\"a\":fal", "[nulll]", "[truex]", "[nul]",
		"[\"1234567\"]", "[\"12345678\"]", "[\"123456789\"]", "[\"123456789012345\"]", "[\"1234567890123456\"]", "[\"12345678901234567\"]",
		"\"" + strings.Repeat("x", 100) + "\"", "\"" + strings.Repeat("\u00e9", 50) + "\"", "\"" + strings.Repeat("x", 100),
		"[1,2,3", "[1,2,3,", `{"a":1,"b"`, `{"a"`, `{"a":`, `{"a":1,`, "[\"a\x7fb\"]", "[\"a\x1fb\"]", "\"\\", "\"\\u", "\"\\u00", "\"\\u00e",
	} {
		doc(s)
	}
	// an escape or a control byte at each offset of a string, around the
	// eight byte words of the scans
	for k := 0; k < 20; k++ {
		for _, e := range []string{`\n`, `\"`, `\\`, `\u00e9`, `\ud83d\ude00`, "\x01", "\u00e9", "\xff", "\"", "\\q"} {
			doc(`["` + strings.Repeat("a", k) + e + strings.Repeat("b", 20-k) + `"]`)
			doc(`{"` + strings.Repeat("k", k) + e + `":` + strconv.Itoa(k) + `,"x":"` + strings.Repeat("y", k) + `"}`)
		}
	}
	// random valid and damaged texts (one in three as generated)
	for i := 0; i < n; i++ {
		var w strings.Builder
		randJSON(r, &w, 0)
		s := w.String()
		if i%3 != 0 {
			s = string(mutate(r, []byte(s)))
		}
		doc(s)
	}
	// large objects of a few keys: duplicates through the hash table
	for i := 0; i < n/20; i++ {
		var w strings.Builder
		w.WriteByte('{')
		m := 9 + r.Intn(40)
		for j := 0; j < m; j++ {
			if j > 0 {
				w.WriteByte(',')
			}
			k := r.Intn(30)
			if r.Intn(5) == 0 {
				// the same key with its first digit escaped
				ks := strconv.Itoa(k)
				fmt.Fprintf(&w, `"k\u00%x%s":%d`, ks[0], ks[1:], j)
			} else {
				fmt.Fprintf(&w, `"k%d":%d`, k, j)
			}
		}
		w.WriteByte('}')
		doc(w.String())
	}
	// Floats
	for _, s := range []string{"0", "-0", "1e21", "1e20", "999999999999999999999", "1e-6", "9.999999999999999e-7", "1e-7", "1.5e-7", "123456.789", "5e-324", "1.7976931348623157e308",
		"0.000001", "0.0000012345", "1234567890123456.7", "12345678.5", "0.1", "100", "nan", "inf", "-inf", "1e100", "1.2345678901234567e-300"} {
		b.WriteString("f " + s + "\n")
	}
	for i := 0; i < n; i++ {
		f := math.Float64frombits(r.Uint64())
		if math.IsNaN(f) || math.IsInf(f, 0) {
			continue
		}
		b.WriteString("f " + strconv.FormatFloat(f, 'g', -1, 64) + "\n")
		b.WriteString("f " + strconv.FormatFloat(math.Ldexp(r.Float64(), r.Intn(160)-80), 'g', -1, 64) + "\n")
		b.WriteString("f " + strconv.Itoa(r.Intn(1000000)) + "." + strconv.Itoa(r.Intn(1000)) + "\n")
	}
	// strings to encode: random runes, controls, the escaped bytes
	runes := []rune{'a', 'Z', ' ', '"', '\\', '/', '<', '>', '&', 0, 1, 8, 9, 10, 12, 13, 0x1f, 0x7f, 0x80, 0xe9, 0x2028, 0x2029, 0xfffd, 0x1f600, 0x10ffff, 0x65e5}
	for i := 0; i < n/2; i++ {
		var w strings.Builder
		m := r.Intn(40)
		for j := 0; j < m; j++ {
			if r.Intn(3) == 0 {
				w.WriteString("plain")
			} else {
				w.WriteRune(runes[r.Intn(len(runes))])
			}
		}
		s := w.String()
		if !utf8.ValidString(s) {
			continue
		}
		b.WriteString("s " + hex.EncodeToString([]byte(s)) + "\n")
	}
	return b.String()
}

// TestJSONTextDiff runs jsonTextDiffProg over jsonTextCorpus on the Go
// backend (std/rt's jsonParse over encoding/json, the reference) and on
// both native backends with std/json's text in Xo (std/native/llrt/
// jsontext.xo) and with its C path (XO_LLRT_C=jsontext), and requires
// the Go backend's lines; the LLVM build of the Xo path also frees every
// allocation (decision 0193 step 4).
func TestJSONTextDiff(t *testing.T) {
	if testing.Short() {
		t.Skip("builds five programs")
	}
	n := 6000
	if s := os.Getenv("XO_JSONDIFF_N"); s != "" {
		// a larger corpus for a one off run
		n, _ = strconv.Atoi(s)
	}
	in := jsonTextCorpus(n)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(jsonTextDiffProg), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "stdin.txt"), []byte(in), 0o644); err != nil {
		t.Fatal(err)
	}
	inLines := strings.Split(strings.TrimSuffix(in, "\n"), "\n")
	t.Logf("%d input lines", len(inLines))
	ref := buildAndRun(t, dir, build.Options{Backend: build.BackendGo})
	if ref.exit != 0 {
		t.Fatalf("go backend: exit %d: %s", ref.exit, ref.stderr)
	}
	want := strings.Split(strings.TrimSuffix(ref.stdout, "\n"), "\n")
	if len(want) != len(inLines) {
		t.Fatalf("go backend: %d lines for %d inputs", len(want), len(inLines))
	}
	for _, path := range []string{"xo", "c"} {
		for _, be := range []string{build.BackendArm64, build.BackendLLVM} {
			t.Run(path+"/"+be, func(t *testing.T) {
				if path == "c" {
					t.Setenv("XO_LLRT_C", "jsontext")
				}
				r, res := buildRun(t, dir, build.Options{Backend: be})
				if r.exit != 0 {
					t.Fatalf("exit %d: %s", r.exit, r.stderr)
				}
				got := strings.Split(strings.TrimSuffix(r.stdout, "\n"), "\n")
				bad := 0
				for i := 0; i < len(want) || i < len(got); i++ {
					var g, w string
					if i < len(got) {
						g = got[i]
					}
					if i < len(want) {
						w = want[i]
					}
					if g != w {
						if bad < 10 {
							x := ""
							if i < len(inLines) {
								x = inLines[i]
							}
							t.Errorf("%.200q:\n got %.300q\nwant %.300q", x, g, w)
						}
						bad++
					}
				}
				if bad > 0 {
					t.Errorf("%d of %d lines differ", bad, len(want))
				}
				if path == "xo" && be == build.BackendLLVM {
					leakCheck(t, res.Binaries[build.Host()], dir)
				}
			})
		}
	}
}
