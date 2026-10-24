package main

import (
	"fmt"
	"math"
	"math/rand"
	"strings"
)

// jsonHelpers, urlHelpers, baseHelpers and timeHelpers wrap the match, whose
// arms Xo wants on separate lines, so each case stays one line of source.
const jsonHelpers = `
fn fz_json(text: Str) -> Str {
  match json.parse_value(text) {
    Ok(v) => fz_hx(v.encode())
    Err(e) => "err:${e.display()}"
  }
}
`

const urlHelpers = `
fn fz_un_q(s: Str) -> Str {
  match url.query_unescape(s) {
    Ok(v) => fz_hx(v)
    Err(e) => "err:${e.display()}"
  }
}
fn fz_un_p(s: Str) -> Str {
  match url.path_unescape(s) {
    Ok(v) => fz_hx(v)
    Err(e) => "err:${e.display()}"
  }
}
fn fz_query(s: Str) -> Str {
  match url.parse_query(s) {
    Ok(v) => fz_hx(v.encode())
    Err(e) => "err:${e.display()}"
  }
}
`

const baseHelpers = `
fn fz_b64(s: Str, enc: base64.Base64) -> Str {
  match base64.decode(s, enc) {
    Ok(v) => v.to_hex()
    Err(e) => "err:${e.display()}"
  }
}
fn fz_b32(s: Str) -> Str {
  match base32.decode(s, base32.Base32.Std) {
    Ok(v) => v.to_hex()
    Err(e) => "err:${e.display()}"
  }
}
`

const timeHelpers = `
fn fz_dur(s: Str) -> Str {
  match Duration.parse(s) {
    Ok(d) => fz_hx("${d}")
    Err(e) => "err:${e.display()}"
  }
}
fn fz_stamp(s: Str) -> Str {
  match Time.parse_rfc3339(s) {
    Some(t) => fz_hx(t.format_rfc3339())
    None => "none"
  }
}
`

const regexpHelpers = `
fn fz_re_m(p: Str, s: Str) -> Str {
  match regexp.compile(p) {
    Ok(r) => fz_bool(r.is_match(s))
    Err(e) => "err:${e.display()}"
  }
}
fn fz_re_f(p: Str, s: Str) -> Str {
  match regexp.compile(p) {
    Ok(r) => fz_opt_s(r.find(s))
    Err(e) => "err:${e.display()}"
  }
}
fn fz_re_a(p: Str, s: Str) -> Str {
  match regexp.compile(p) {
    Ok(r) => fz_ls(r.find_all(s))
    Err(e) => "err:${e.display()}"
  }
}
fn fz_re_sp(p: Str, s: Str) -> Str {
  match regexp.compile(p) {
    Ok(r) => fz_ls(r.split(s))
    Err(e) => "err:${e.display()}"
  }
}
fn fz_re_r(p: Str, s: Str, repl: Str) -> Str {
  match regexp.compile(p) {
    Ok(r) => fz_hx(r.replace_all(s, repl))
    Err(e) => "err:${e.display()}"
  }
}
`

const csvHelpers = `
fn fz_csv(s: Str) -> Str {
  match csv.parse(s) {
    Ok(rows) => fz_lss(rows)
    Err(e) => "err:${e.display()}"
  }
}
fn fz_csv_semi(s: Str) -> Str {
  match csv.parse(s, sep: ';') {
    Ok(rows) => fz_lss(rows)
    Err(e) => "err:${e.display()}"
  }
}
fn fz_csv_lazy(s: Str) -> Str {
  match csv.parse(s, lazy_quotes: true, trim_leading_space: true) {
    Ok(rows) => fz_lss(rows)
    Err(e) => "err:${e.display()}"
  }
}
`

const netipHelpers = `
fn fz_addr(s: Str) -> Str {
  match netip.parse_addr(s) {
    Ok(a) => fz_hx("${a}")
    Err(e) => "err:${e.display()}"
  }
}
fn fz_addr6(s: Str) -> Str {
  match netip.parse_addr(s) {
    Ok(a) => fz_hx("${fz_bools(a.is_4(), a.is_6(), a.is_4in6(), a.is_loopback(), a.is_private())}-${a.bytes().to_hex()}")
    Err(e) => "err:${e.display()}"
  }
}
fn fz_pfx(s: Str) -> Str {
  match netip.parse_prefix(s) {
    Ok(p) => fz_hx("${p} ${p.masked()}")
    Err(e) => "err:${e.display()}"
  }
}
`

const bigHelpers = `
fn fz_big_ops(a: Int, b: Int) -> Str {
  let x = big.from_int(a)
  let y = big.from_int(b)
  let q = if y.sign() == 0 { "-" } else { "${x.quo(y)}/${x.rem(y)}" }
  fz_hx("${x.add(y)};${x.sub(y)};${x.mul(y)};${x.cmp(y)};${x.cmp_abs(y)};${q};${x.and(y)};${x.or(y)};${x.xor(y)};${x.gcd(y)};${x.lsh(3)};${x.rsh(3)}")
}
fn fz_big_parse(s: Str) -> Str {
  match big.parse(s) {
    Ok(x) => fz_hx("${x}")
    Err(e) => "err:${e.display()}"
  }
}
fn fz_big_hex(s: Str) -> Str {
  match big.parse(s, base: 16) {
    Ok(x) => fz_hx("${x}")
    Err(e) => "err:${e.display()}"
  }
}
`

// A fuzzCase is one line of the generated program: the name it prints and
// the Xo expression that computes its value as a Str, so every result
// compares as one line of text.
type fuzzCase struct {
	Name string // area.0001
	Expr string // already rendered to a Str
}

func (c fuzzCase) stmt() string {
	return fmt.Sprintf("  fz_emit(os.stdio, %s, %s)", xq(c.Name), c.Expr)
}

// An area is one std module's functions, the inputs it draws, and the Xo
// imports it needs.
type area struct {
	Name    string
	Uses    []string
	Helpers string
	Gen     func(r *rand.Rand, n int) []fuzzCase
}

func allAreas() []area {
	return []area{
		{"strings", nil, "", genStrings},
		{"bytes", nil, "", genBytes},
		{"strconv", nil, "", genStrconv},
		{"json", []string{"std/json"}, jsonHelpers, genJSON},
		{"path", []string{"std/path"}, "", genPath},
		{"url", []string{"std/url"}, urlHelpers, genURL},
		{"base64", []string{"std/base64", "std/base32"}, baseHelpers, genBase},
		{"time", []string{"std/time"}, timeHelpers, genTime},
		{"regexp", []string{"std/regexp"}, regexpHelpers, genRegexp},
		{"csv", []string{"std/csv"}, csvHelpers, genCSV},
		{"netip", []string{"std/netip"}, netipHelpers, genNetip},
		{"big", []string{"std/big"}, bigHelpers, genBig},
	}
}

// pool is the fixed part of every area's inputs, so a seed still meets the
// empty, separator, unicode and control cases.
var pool = []string{
	"", "a", "abc", "  x  ", "a,b,,c", "aaa", " Hello, World ", "héllo",
	"a\nb\n", "\t x \t", "ABC", "İstanbul", "ß", "🚀x🚀", "ab\u00a0cd",
	"  ", "a.b.c", "-1e5", "0x1F", "..", "\u2028x", "\"quoted\"", "a\\b",
	"line1\r\nline2", "aaa\taaa", "$var", "x\x00y", " ",
}

// subs are the separators, prefixes and cutsets cases draw from.
var subs = []string{"", "a", "b", "ab", ",", " ", "x", "aa", "é", "🚀", "\n", ".", "aaa", "Hé"}

func pick(r *rand.Rand, xs []string) string { return xs[r.Intn(len(xs))] }

// randStr draws a string: a pool entry, a random rune mix, or a long one.
func randStr(r *rand.Rand) string {
	switch r.Intn(3) {
	case 0:
		return pick(r, pool)
	case 1:
		var b strings.Builder
		alpha := []rune("ab c,\t\nABCéß🚀012.-_/\\\"'$")
		for i, n := 0, r.Intn(14); i < n; i++ {
			b.WriteRune(alpha[r.Intn(len(alpha))])
		}
		return b.String()
	}
	var b strings.Builder
	for i := 0; i < 3*r.Intn(20); i++ {
		b.WriteString(pick(r, pool))
	}
	return b.String()
}

// randBytes draws arbitrary bytes, invalid UTF-8 included: Bytes may hold
// what Str cannot.
func randBytes(r *rand.Rand) []byte {
	if r.Intn(3) == 0 {
		return []byte(pick(r, pool))
	}
	b := make([]byte, r.Intn(18))
	for i := range b {
		b[i] = byte(r.Intn(256))
	}
	return b
}

// genStrings covers Str's pure methods.
func genStrings(r *rand.Rand, n int) []fuzzCase {
	out := make([]fuzzCase, 0, n)
	for i := 0; i < n; i++ {
		s, sub, cut := randStr(r), pick(r, subs), pick(r, subs)
		S := xq(s)
		name := fmt.Sprintf("strings.%04d", i)
		k := r.Intn(4)
		switch i % 40 {
		case 0:
			out = append(out, fuzzCase{name, "fz_hx(" + S + ".trim())"})
		case 1:
			out = append(out, fuzzCase{name, "fz_hx(" + S + ".trim_start())"})
		case 2:
			out = append(out, fuzzCase{name, "fz_hx(" + S + ".trim_end())"})
		case 3:
			out = append(out, fuzzCase{name, "fz_hx(" + S + ".trim_chars(" + xq(cut) + "))"})
		case 4:
			out = append(out, fuzzCase{name, "fz_hx(" + S + ".trim_start_chars(" + xq(cut) + "))"})
		case 5:
			out = append(out, fuzzCase{name, "fz_hx(" + S + ".trim_end_chars(" + xq(cut) + "))"})
		case 6:
			out = append(out, fuzzCase{name, "fz_hx(" + S + ".to_lower())"})
		case 7:
			out = append(out, fuzzCase{name, "fz_hx(" + S + ".to_upper())"})
		case 8:
			out = append(out, fuzzCase{name, "fz_opt_i(" + S + ".find(" + xq(sub) + "))"})
		case 9:
			out = append(out, fuzzCase{name, "fz_opt_i(" + S + ".rfind(" + xq(sub) + "))"})
		case 10:
			out = append(out, fuzzCase{name, "fz_opt_i(" + S + ".find_any(" + xq(cut) + "))"})
		case 11:
			out = append(out, fuzzCase{name, "fz_opt_i(" + S + ".rfind_any(" + xq(cut) + "))"})
		case 12:
			out = append(out, fuzzCase{name, "fz_ls(" + S + ".split(" + xq(sub) + "))"})
		case 13:
			out = append(out, fuzzCase{name, "fz_ls(" + S + ".split_n(" + xq(sub) + ", " + fmt.Sprint(k) + "))"})
		case 14:
			out = append(out, fuzzCase{name, "fz_ls(" + S + ".split_after(" + xq(sub) + "))"})
		case 15:
			out = append(out, fuzzCase{name, "fz_ls(" + S + ".fields())"})
		case 16:
			out = append(out, fuzzCase{name, "fz_ls(" + S + ".lines())"})
		case 17:
			out = append(out, fuzzCase{name, "fz_hx(" + S + ".replace(" + xq(sub) + ", " + xq(cut) + "))"})
		case 18:
			out = append(out, fuzzCase{name, "fz_hx(" + S + ".replace_n(" + xq(sub) + ", " + xq(cut) + ", " + fmt.Sprint(k) + "))"})
		case 19:
			out = append(out, fuzzCase{name, "fz_hx(" + S + ".repeat(" + fmt.Sprint(k) + "))"})
		case 20:
			out = append(out, fuzzCase{name, "fz_hx(" + S + ".trim_prefix(" + xq(sub) + "))"})
		case 21:
			out = append(out, fuzzCase{name, "fz_hx(" + S + ".trim_suffix(" + xq(sub) + "))"})
		case 22:
			out = append(out, fuzzCase{name, "fz_opt_s(" + S + ".cut_prefix(" + xq(sub) + "))"})
		case 23:
			out = append(out, fuzzCase{name, "fz_opt_s(" + S + ".cut_suffix(" + xq(sub) + "))"})
		case 24:
			out = append(out, fuzzCase{name, "fz_bool(" + S + ".contains(" + xq(sub) + "))"})
		case 25:
			out = append(out, fuzzCase{name, "fz_bool(" + S + ".starts_with(" + xq(sub) + "))"})
		case 26:
			out = append(out, fuzzCase{name, "fz_bool(" + S + ".ends_with(" + xq(sub) + "))"})
		case 27:
			out = append(out, fuzzCase{name, "fz_bool(" + S + ".equal_fold(" + xq(sub) + "))"})
		case 28:
			out = append(out, fuzzCase{name, "fz_bool(" + S + ".contains_any(" + xq(cut) + "))"})
		case 29:
			out = append(out, fuzzCase{name, "fz_i(" + S + ".count(" + xq(sub) + "))"})
		case 30:
			out = append(out, fuzzCase{name, "fz_i(" + S + ".len())"})
		case 31:
			out = append(out, fuzzCase{name, fmt.Sprintf("fz_bools(%s.is_empty(), %s.contains_rune('a'), %s.runes().len() == %s.rune_count(), true, false)",
				S, S, S, S)})
		case 32:
			out = append(out, fuzzCase{name, "fz_opt_i(" + S + ".find_where(fz_is_letter))"})
		case 33:
			out = append(out, fuzzCase{name, "fz_opt_i(" + S + ".rfind_where(fz_is_digit))"})
		case 34:
			out = append(out, fuzzCase{name, "fz_bool(" + S + ".contains_where(fz_is_letter))"})
		case 35:
			out = append(out, fuzzCase{name, "fz_hx(" + S + ".trim_where(fz_is_digit))"})
		case 36:
			out = append(out, fuzzCase{name, "fz_hx(" + S + ".trim_start_where(fz_is_letter))"})
		case 37:
			out = append(out, fuzzCase{name, "fz_hx(" + S + ".trim_end_where(fz_is_letter))"})
		case 38:
			out = append(out, fuzzCase{name, "fz_ls(" + S + ".split_where(fz_is_letter))"})
		case 39:
			out = append(out, fuzzCase{name, "fz_hx(" + S + ".map_runes(fz_upper_keep))"})
		}
	}
	return out
}

// genBytes covers Bytes' pure methods.
func genBytes(r *rand.Rand, n int) []fuzzCase {
	out := make([]fuzzCase, 0, n)
	for i := 0; i < n; i++ {
		b, sub, cut := randBytes(r), randBytes(r), pick(r, subs)
		B, SUB := xbytes(b), xbytes(sub)
		name := fmt.Sprintf("bytes.%04d", i)
		k := r.Intn(4)
		switch i % 32 {
		case 0:
			out = append(out, fuzzCase{name, B + ".to_hex()"})
		case 1:
			out = append(out, fuzzCase{name, "fz_hx(" + B + ".to_base64())"})
		case 2:
			out = append(out, fuzzCase{name, "fz_opt_s(" + B + ".to_str())"})
		case 3:
			out = append(out, fuzzCase{name, "fz_opt_i(" + B + ".find(" + SUB + "))"})
		case 4:
			out = append(out, fuzzCase{name, "fz_opt_i(" + B + ".rfind(" + SUB + "))"})
		case 5:
			out = append(out, fuzzCase{name, "fz_opt_i(" + B + ".find_any(" + xq(cut) + "))"})
		case 6:
			out = append(out, fuzzCase{name, "fz_lb(" + B + ".split(" + SUB + "))"})
		case 7:
			out = append(out, fuzzCase{name, "fz_lb(" + B + ".split_n(" + SUB + ", " + fmt.Sprint(k) + "))"})
		case 8:
			out = append(out, fuzzCase{name, "fz_lb(" + B + ".split_after(" + SUB + "))"})
		case 9:
			out = append(out, fuzzCase{name, "fz_lb(" + B + ".fields())"})
		case 10:
			out = append(out, fuzzCase{name, B + ".trim().to_hex()"})
		case 11:
			out = append(out, fuzzCase{name, B + ".trim_prefix(" + SUB + ").to_hex()"})
		case 12:
			out = append(out, fuzzCase{name, B + ".trim_suffix(" + SUB + ").to_hex()"})
		case 13:
			out = append(out, fuzzCase{name, "fz_opt_b(" + B + ".cut_prefix(" + SUB + "))"})
		case 14:
			out = append(out, fuzzCase{name, "fz_opt_b(" + B + ".cut_suffix(" + SUB + "))"})
		case 15:
			out = append(out, fuzzCase{name, B + ".replace(" + SUB + ", " + xbytes([]byte(cut)) + ").to_hex()"})
		case 16:
			out = append(out, fuzzCase{name, B + ".replace_n(" + SUB + ", " + xbytes([]byte(cut)) + ", " + fmt.Sprint(k) + ").to_hex()"})
		case 17:
			out = append(out, fuzzCase{name, B + ".to_lower().to_hex()"})
		case 18:
			out = append(out, fuzzCase{name, B + ".to_upper().to_hex()"})
		case 19:
			out = append(out, fuzzCase{name, B + ".repeat(" + fmt.Sprint(k) + ").to_hex()"})
		case 20:
			out = append(out, fuzzCase{name, B + ".clone().to_hex()"})
		case 21:
			out = append(out, fuzzCase{name, "fz_bool(" + B + ".contains(" + SUB + "))"})
		case 22:
			out = append(out, fuzzCase{name, "fz_bool(" + B + ".starts_with(" + SUB + "))"})
		case 23:
			out = append(out, fuzzCase{name, fmt.Sprintf("fz_bools(%s.ends_with(%s), %s.equal_fold(%s), %s.contains_any(%s), %s.is_empty(), false)",
				B, SUB, xbytes([]byte(cut)), xbytes([]byte(cut)), B, xq(cut), B)})
		case 24:
			out = append(out, fuzzCase{name, "fz_opt_i(" + B + ".find_where(fz_is_letter))"})
		case 25:
			out = append(out, fuzzCase{name, "fz_opt_i(" + B + ".rfind_where(fz_is_digit))"})
		case 26:
			out = append(out, fuzzCase{name, "fz_bool(" + B + ".contains_where(fz_is_letter))"})
		case 27:
			out = append(out, fuzzCase{name, B + ".trim_where(fz_is_digit).to_hex()"})
		case 28:
			out = append(out, fuzzCase{name, B + ".trim_start_where(fz_is_letter).to_hex()"})
		case 29:
			out = append(out, fuzzCase{name, B + ".trim_end_where(fz_is_letter).to_hex()"})
		case 30:
			out = append(out, fuzzCase{name, "fz_lb(" + B + ".split_where(fz_is_letter))"})
		case 31:
			out = append(out, fuzzCase{name, B + ".map_runes(fz_upper_keep).to_hex()"})
		}
	}
	return out
}

// genStrconv covers Int, Float and Bool parsing and formatting.
func genStrconv(r *rand.Rand, n int) []fuzzCase {
	ints := []string{"0", "1", "-1", "42", "-42", "9223372036854775807", "-9223372036854775808",
		"9223372036854775808", "007", "+7", " 7", "7 ", "0x1F", "1_000", "", "-", "12a", "1e3", "٣"}
	floats := []string{"0", "-0", "1", "-1", "1.5", ".5", "5.", "1e10", "1E-10", "1e999",
		"-1e999", "NaN", "nan", "Inf", "+Inf", "-Inf", "infinity", "0x1p-2", "3.141592653589793238",
		"1.7976931348623157e308", "5e-324", "4.9e-324", "", " ", "1,5", "1e", "e1", "+.5", "- 1"}
	bools := []string{"1", "0", "t", "f", "T", "F", "TRUE", "true", "True", "false", "False", "yes", "", "2", " true", "TRUE "}
	out := make([]fuzzCase, 0, n)
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("strconv.%04d", i)
		base := []int{2, 8, 10, 16, 36}[r.Intn(5)]
		switch i % 10 {
		case 0:
			out = append(out, fuzzCase{name, "fz_opt_i(Int.parse(" + xq(pick(r, ints)) + "))"})
		case 1:
			out = append(out, fuzzCase{name, "fz_opt_i(Int.parse_base(" + xq(pick(r, ints)) + ", " + fmt.Sprint(base) + "))"})
		case 2:
			out = append(out, fuzzCase{name, "fz_hx((" + fmt.Sprint(r.Int63()-r.Int63()) + ").to_str_base(" + fmt.Sprint(base) + "))"})
		case 3:
			out = append(out, fuzzCase{name, "fz_opt_f(Float.parse(" + xq(pick(r, floats)) + "))"})
		case 4:
			out = append(out, fuzzCase{name, "fz_opt_bool(Bool.parse(" + xq(pick(r, bools)) + "))"})
		case 5:
			out = append(out, fuzzCase{name, "fz_i(" + fmt.Sprint(r.Int63()-r.Int63()) + ")"})
		case 6:
			out = append(out, fuzzCase{name, "fz_f(Float.from_bits(" + fmt.Sprint(int64(r.Uint64())) + "))"})
		case 7:
			out = append(out, fuzzCase{name, "fz_bool(Float.from_bits(" + fmt.Sprint(int64(r.Uint64())) + ").is_nan())"})
		case 8:
			out = append(out, fuzzCase{name, "fz_opt_i(Int.parse(" + xq(fmt.Sprint(r.Int63())) + "))"})
		case 9:
			out = append(out, fuzzCase{name, "fz_opt_f(Float.parse(" + xq(fmt.Sprintf("%v", r.NormFloat64()*math.Pow(10, float64(r.Intn(40)-20)))) + "))"})
		}
	}
	return out
}

// genJSON covers json.parse_value and Value.encode over numbers, escapes,
// nesting and junk.
func genJSON(r *rand.Rand, n int) []fuzzCase {
	atoms := []string{"null", "true", "false", "0", "-0", "1", "-1", "1.5", "1e21", "1e-7",
		"123456789012345678901234567890", "-9223372036854775808", "9223372036854775807",
		"1e999", "0.1", "\"\"", "\"a\"", "\"a\\\"b\"", "\"tab\\there\"", "\"\\u00e9\"",
		"\"\\ud834\\udd1e\"", "\"\\ud834\"", "\"a\nb\"", "\"\\u0000\"", "[]", "{}",
		"{\"a\":1}", "{\"a\":1,\"b\":[true,null]}", "{\"a\":1,\"a\":2}", "[1,2,3]",
		"{\"z\":1,\"a\":2}", "{", "[1,", "tru", "01", "+1", ".5", "1.", "NaN", " ", "",
		"[1 2]", "{\"a\" 1}", "{\"a\":}", "\"\x01\"", "{\"a\":[{\"b\":{\"c\":[]}}]}"}
	out := make([]fuzzCase, 0, n)
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("json.%04d", i)
		var text string
		switch i % 3 {
		case 0:
			text = pick(r, atoms)
		case 1:
			depth := 1 + r.Intn(40)
			text = strings.Repeat("[", depth) + strings.Repeat("]", depth)
		case 2:
			var b strings.Builder
			b.WriteString("{")
			for k, m := 0, 1+r.Intn(4); k < m; k++ {
				if k > 0 {
					b.WriteString(",")
				}
				fmt.Fprintf(&b, "%q:%d", fmt.Sprintf("k%d", r.Intn(6)), r.Intn(1000)-500)
			}
			b.WriteString("}")
			text = b.String()
		}
		out = append(out, fuzzCase{name, "fz_json(" + xq(text) + ")"})
	}
	return out
}

// genPath covers std/path's pure functions.
func genPath(r *rand.Rand, n int) []fuzzCase {
	paths := []string{"", ".", "..", "/", "//", "/a", "/a/b", "a", "a/", "a//b", "./a", "../a",
		"a/../b", "a/./b", "/a/../../b", "C:\\a\\b", "C:/a/b", "a/b/", ".hidden", "a.b.c",
		"a/b.ext", "/usr/lib/libx.so.1", " ", "a b/c d", "/a/", "//host/share", "a\x00b"}
	out := make([]fuzzCase, 0, n)
	for i := 0; i < n; i++ {
		a, b := pick(r, paths), pick(r, paths)
		name := fmt.Sprintf("path.%04d", i)
		switch i % 6 {
		case 0:
			out = append(out, fuzzCase{name, "fz_hx(path.clean(" + xq(a) + "))"})
		case 1:
			out = append(out, fuzzCase{name, "fz_hx(path.join(" + xq(a) + ", " + xq(b) + "))"})
		case 2:
			out = append(out, fuzzCase{name, "fz_hx(path.base(" + xq(a) + "))"})
		case 3:
			out = append(out, fuzzCase{name, "fz_hx(path.dir(" + xq(a) + "))"})
		case 4:
			out = append(out, fuzzCase{name, "fz_hx(path.ext(" + xq(a) + "))"})
		case 5:
			out = append(out, fuzzCase{name, "fz_bool(path.is_abs(" + xq(a) + "))"})
		}
	}
	return out
}

// genURL covers std/url's escapes and query parsing.
func genURL(r *rand.Rand, n int) []fuzzCase {
	out := make([]fuzzCase, 0, n)
	for i := 0; i < n; i++ {
		s := randStr(r)
		if i%4 == 3 {
			s = pick(r, []string{"%", "%2", "%zz", "a%20b", "+", "a+b", "%e2%9c%93", "%ff", "a b&c=d/é", "?x=1&y=2"})
		}
		name := fmt.Sprintf("url.%04d", i)
		switch i % 6 {
		case 0:
			out = append(out, fuzzCase{name, "fz_hx(url.query_escape(" + xq(s) + "))"})
		case 1:
			out = append(out, fuzzCase{name, "fz_hx(url.path_escape(" + xq(s) + "))"})
		case 2:
			out = append(out, fuzzCase{name, "fz_un_q(" + xq(s) + ")"})
		case 3:
			out = append(out, fuzzCase{name, "fz_un_p(" + xq(s) + ")"})
		case 4:
			out = append(out, fuzzCase{name, "fz_query(" + xq(s) + ")"})
		case 5:
			out = append(out, fuzzCase{name, "fz_hx(url.values().encode())"})
		}
	}
	return out
}

// genBase covers base64, base32 and hex.
func genBase(r *rand.Rand, n int) []fuzzCase {
	texts := []string{"", "A", "AA", "AAA", "AAAA", "aGVsbG8=", "aGVsbG8", "!!!!", "a b", "a\nb",
		"AB==", "A===", "abcd", "-_-_", "+//+", "0O1l", "MZXW6===", "MZXW6YQ=", "MZXW6YTB"}
	out := make([]fuzzCase, 0, n)
	for i := 0; i < n; i++ {
		b := randBytes(r)
		B := xbytes(b)
		name := fmt.Sprintf("base64.%04d", i)
		switch i % 8 {
		case 0:
			out = append(out, fuzzCase{name, "fz_hx(base64.encode(" + B + ", base64.Base64.Std))"})
		case 1:
			out = append(out, fuzzCase{name, "fz_hx(base64.encode(" + B + ", base64.Base64.Url))"})
		case 2:
			out = append(out, fuzzCase{name, "fz_hx(base64.encode(" + B + ", base64.Base64.RawStd))"})
		case 3:
			out = append(out, fuzzCase{name, "fz_b64(" + xq(pick(r, texts)) + ", base64.Base64.Std)"})
		case 4:
			out = append(out, fuzzCase{name, "fz_b64(" + xq(pick(r, texts)) + ", base64.Base64.RawUrl)"})
		case 5:
			out = append(out, fuzzCase{name, "fz_hx(base32.encode(" + B + ", base32.Base32.Std))"})
		case 6:
			out = append(out, fuzzCase{name, "fz_b32(" + xq(pick(r, texts)) + ")"})
		case 7:
			out = append(out, fuzzCase{name, "fz_opt_b(Bytes.from_hex(" + xq(pick(r, texts)) + "))"})
		}
	}
	return out
}

// genTime covers Duration parsing and display, RFC3339 parsing and
// formatting, and formatting a Time at UTC and at a fixed zone.
func genTime(r *rand.Rand, n int) []fuzzCase {
	durs := []string{"0", "1s", "1.5s", "-2h45m", "300ms", "1us", "1µs", "1ns", "1m", "1h",
		"", "s", "1", "1 s", " 1s ", "+1s", "1.5h", "0.5m", "1d", "1S", "1e3s", "2562047h",
		"9223372036854775807ns", "-9223372036854775808ns", "1h1m1s1ms1us1ns"}
	stamps := []string{"1970-01-01T00:00:00Z", "2026-10-24T12:34:56Z", "2026-10-24T12:34:56.123456789Z",
		"2026-10-24T12:34:56+02:00", "2026-10-24T12:34:56-07:30", "2026-02-29T00:00:00Z",
		"2024-02-29T23:59:59Z", "0000-01-01T00:00:00Z", "9999-12-31T23:59:59Z",
		"2026-10-24T24:00:00Z", "2026-10-24t12:34:56z", "2026-10-24T12:34:56", "2026-10-24T12:34:56.Z",
		"2026-10-24T12:34:56.1Z", "2026-10-24T12:34:56+24:00", "2026-10-24T12:34:56+23:59",
		"2026-10-24 12:34:56Z", "", "not a time", "2026-13-01T00:00:00Z"}
	out := make([]fuzzCase, 0, n)
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("time.%04d", i)
		switch i % 6 {
		case 0:
			out = append(out, fuzzCase{name, "fz_dur(" + xq(pick(r, durs)) + ")"})
		case 1:
			out = append(out, fuzzCase{name, "fz_hx(Duration.from_nanos(" + fmt.Sprint(r.Int63()-r.Int63()) + ").display())"})
		case 2:
			out = append(out, fuzzCase{name, "fz_stamp(" + xq(pick(r, stamps)) + ")"})
		case 3:
			out = append(out, fuzzCase{name, "fz_hx(time.format(Time.from_unix(" + fmt.Sprint(r.Int63n(2000000000)-1000000000) + "), time.utc(), time.RFC3339))"})
		case 4:
			out = append(out, fuzzCase{name, "fz_hx(time.format(Time.from_unix_millis(" + fmt.Sprint(r.Int63n(2000000000000)-1000000000000) + "), time.utc(), time.DATE))"})
		case 5:
			off := r.Intn(47*3600) - 23*3600
			out = append(out, fuzzCase{name, "fz_hx(time.format(Time.from_unix(" + fmt.Sprint(r.Int63n(2000000000)) + "), time.fixed(\"X\", Duration.from_secs_int(" + fmt.Sprint(off) + ")), time.RFC3339))"})
		}
	}
	return out
}

// genRegexp covers compile, match, find, find_all, split and replace_all.
func genRegexp(r *rand.Rand, n int) []fuzzCase {
	pats := []string{"a", "a+", "^a", "a$", "(a)(b)?", "[a-c]+", "[^x]", "\\d+", "\\s", "(?i)a",
		"a|b", ".", "a{2,3}", "(?P<n>a)", "\\b", "^$", "[", "(a", "*x", "a**", "\\p{L}", "é", "\\p{Han}"}
	repls := []string{"", "X", "$1", "${n}", "a$", "\\n"}
	out := make([]fuzzCase, 0, n)
	for i := 0; i < n; i++ {
		p, s := pick(r, pats), randStr(r)
		name := fmt.Sprintf("regexp.%04d", i)
		switch i % 6 {
		case 0:
			out = append(out, fuzzCase{name, "fz_re_m(" + xq(p) + ", " + xq(s) + ")"})
		case 1:
			out = append(out, fuzzCase{name, "fz_re_f(" + xq(p) + ", " + xq(s) + ")"})
		case 2:
			out = append(out, fuzzCase{name, "fz_re_a(" + xq(p) + ", " + xq(s) + ")"})
		case 3:
			out = append(out, fuzzCase{name, "fz_re_sp(" + xq(p) + ", " + xq(s) + ")"})
		case 4:
			out = append(out, fuzzCase{name, "fz_re_r(" + xq(p) + ", " + xq(s) + ", " + xq(pick(r, repls)) + ")"})
		case 5:
			out = append(out, fuzzCase{name, "fz_hx(regexp.quote(" + xq(s) + "))"})
		}
	}
	return out
}

// csvText draws CSV-ish text: quoted fields, embedded separators and
// newlines, comments, blank lines and broken quoting.
func csvText(r *rand.Rand) string {
	if r.Intn(3) == 0 {
		return pick(r, []string{"", "a", "a,b", "a,b\nc,d", "\"a\",\"b\"", "a,\"b,c\",d",
			"#c\na,b", "a;b", "a,\"b", "\"a\"x,b", " a , b ", "a\r\nb", "\n\n", ",\n,,"})
	}
	fields := []string{"a", "b", "x,y", "q\"r", " lead", "trail ", "é", "🚀", "", ";"}
	var b strings.Builder
	for i, rows := 0, r.Intn(4); i < rows; i++ {
		for j, cols := 0, 1+r.Intn(4); j < cols; j++ {
			if j > 0 {
				b.WriteString(pick(r, []string{",", ";", ",\"", "\""}))
			}
			f := pick(r, fields)
			if r.Intn(4) == 0 {
				b.WriteString("\"" + f + "\"")
			} else {
				b.WriteString(f)
			}
		}
		b.WriteString(pick(r, []string{"\n", "\r\n", "\n#c\n"}))
	}
	return b.String()
}

// genCSV covers parse with its options and format.
func genCSV(r *rand.Rand, n int) []fuzzCase {
	out := make([]fuzzCase, 0, n)
	for i := 0; i < n; i++ {
		s := csvText(r)
		name := fmt.Sprintf("csv.%04d", i)
		switch i % 4 {
		case 0:
			out = append(out, fuzzCase{name, "fz_csv(" + xq(s) + ")"})
		case 1:
			out = append(out, fuzzCase{name, "fz_csv_semi(" + xq(s) + ")"})
		case 2:
			out = append(out, fuzzCase{name, "fz_csv_lazy(" + xq(s) + ")"})
		case 3:
			rows := fmt.Sprintf("[[%s, %s], [%s, %s]]", xq(pick(r, pool)), xq(pick(r, subs)),
				xq(pick(r, []string{"a\"b", "c,d", " e"})), xq(pick(r, subs)))
			out = append(out, fuzzCase{name, "fz_hx(csv.format(" + rows + ", crlf: " + fmt.Sprint(r.Intn(2) == 0) + "))"})
		}
	}
	return out
}

// genNetip covers address and prefix parsing, display and predicates.
func genNetip(r *rand.Rand, n int) []fuzzCase {
	addrs := []string{"1.2.3.4", "0.0.0.0", "255.255.255.255", "127.0.0.1", "192.168.1.1",
		"10.0.0.1", "169.254.1.1", "224.0.0.1", "::1", "::", "2001:db8::1", "fe80::1",
		"::ffff:1.2.3.4", "64:ff9b::1.2.3.4", "1.2.3", "01.2.3.4", "1.2.3.4.5", "::g",
		"2001:db8::1%eth0", "", "1.2.3.4 ", " 1.2.3.4", "[::1]"}
	prefixes := []string{"10.0.0.0/8", "192.168.0.0/16", "0.0.0.0/0", "1.2.3.4/32", "1.2.3.4/33",
		"2001:db8::/32", "::/0", "::1/128", "1.2.3.4/-1", "10.0.0.0", "10.0.0.0/8/9", ""}
	out := make([]fuzzCase, 0, n)
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("netip.%04d", i)
		switch i % 3 {
		case 0:
			out = append(out, fuzzCase{name, "fz_addr(" + xq(pick(r, addrs)) + ")"})
		case 1:
			out = append(out, fuzzCase{name, "fz_addr6(" + xq(pick(r, addrs)) + ")"})
		case 2:
			out = append(out, fuzzCase{name, "fz_pfx(" + xq(pick(r, prefixes)) + ")"})
		}
	}
	return out
}

// genBig covers big.parse and the arithmetic, bitwise and compare methods.
func genBig(r *rand.Rand, n int) []fuzzCase {
	dec := []string{"0", "1", "-1", "42", "-42", "9223372036854775807",
		"-9223372036854775808", "123456789012345678901234567890",
		"-123456789012345678901234567890", "0007", "+7", " 7", "7 ", "", "-", "12a",
		"1e3", "ffffffffffffffffffffffff", "-0"}
	hex := []string{"0", "1", "ff", "-ff", "deadBEEF", "7fffffffffffffffffffffffffffffff",
		"-80000000000000000000000000000000", "g", ""}
	out := make([]fuzzCase, 0, n)
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("big.%04d", i)
		switch i % 3 {
		case 0:
			out = append(out, fuzzCase{name, "fz_big_parse(" + xq(pick(r, dec)) + ")"})
		case 1:
			out = append(out, fuzzCase{name, "fz_big_hex(" + xq(pick(r, hex)) + ")"})
		case 2:
			out = append(out, fuzzCase{name, "fz_big_ops(" + fmt.Sprint(r.Int63()-r.Int63()) + ", " + fmt.Sprint(r.Int63()-r.Int63()) + ")"})
		}
	}
	return out
}
