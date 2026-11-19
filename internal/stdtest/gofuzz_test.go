// A fuzz harness for std/go, the values that cross the `use go` boundary,
// against answers computed here with Go's own packages (decision 0616's
// fuzz kind; task #152). std/go builds only on the Go backend (decision
// 0248), so the harness runs there alone. Each case is one of:
//
//   - an Int, Str, Bytes or Float through go.any: its Go type, Go's %v
//     of it, the as_* reading back, and a Go call on it (strconv.Itoa and
//     fmt's %x, strings.Index and strconv.Quote, fmt's %x,
//     strconv.FormatFloat);
//   - strconv.Atoi of a string of digits: the value, or the go.Error's
//     text and Go type;
//   - a List[Int] or Map[Str, Int] through go.any: the Go type, %v and the
//     elements as_list or as_map gives back (an empty one is []any or
//     map[any]any, the runtime's rule when no element tells the type:
//     std/rt goany.go typedSlice);
//   - json.Unmarshal of a JSON text into go.out[List[Int]]: the list, or
//     the error's text and Go type;
//   - go.error(message): message, display and Go type.
//
// Ints, digit strings and Floats come from the hostile dictionary
// (internal/hostile) in a share of the cases. The cases from seed 1 are
// the committed corpus testdata/corpus/gofuzz/gofuzz_test.xo, which
// TestStdCorpora runs on the Go backend (goOnlyCorpora skips the native
// ones); TestGoFuzzCorpusCurrent checks it is what this generator writes
// (XO_GOFUZZ_WRITE=1 rewrites it). TestGoFuzz runs a fresh seed
// (XO_GOFUZZ_SEED, default 2; XO_GOFUZZ_N cases, default 2000) on
// XO_GOFUZZ_BACKENDS (default go).
package stdtest

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"sort"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/xo-lang/xo/internal/hostile"
)

type goxCase struct {
	kind int // 0 Int, 1 Str, 2 Bytes, 3 Float, 4 atoi, 5 List, 6 Map, 7 json, 8 go.error
	x    int64
	s    string
	sub  string
	b    []byte
	li   []int64
	keys []string
	want string
}

func goxInt(r *rand.Rand) int64 {
	switch r.Intn(3) {
	case 0:
		return hostile.Pick(r, hostile.Ints())
	case 1:
		return r.Int63n(2001) - 1000
	}
	return r.Int63() - r.Int63()
}

var goxRunes = []rune{'a', 'b', 'z', '0', '9', ' ', '|', '$', '"', '\\', '\n', '\t', 0, 0x7f, 'é', '日', '😀', 0xfffd, 0x10ffff, 0x2028, 0x7ff, 0x800, 0xffff, 0x10000}

func goxStr(r *rand.Rand, n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteRune(goxRunes[r.Intn(len(goxRunes))])
	}
	return b.String()
}

func goxOpt(ok bool, v string) string {
	if ok {
		return "some " + v
	}
	return "none"
}

func goxJoin(xs []int64) string {
	ss := make([]string, len(xs))
	for i, x := range xs {
		ss[i] = strconv.FormatInt(x, 10)
	}
	return strings.Join(ss, ",")
}

func goxCases(r *rand.Rand, n int) []goxCase {
	cs := make([]goxCase, 0, n)
	for len(cs) < n {
		c := goxCase{kind: r.Intn(9)}
		switch c.kind {
		case 0:
			c.x = goxInt(r)
			c.want = fmt.Sprintf("int64|%d|%s|none|%s|%x %d", c.x, goxOpt(true, strconv.FormatInt(c.x, 10)), strconv.Itoa(int(c.x)), c.x, c.x)
		case 1:
			c.s = goxStr(r, r.Intn(12))
			if r.Intn(2) == 0 && len(c.s) > 0 {
				i := r.Intn(len(c.s) + 1)
				j := i + r.Intn(len(c.s)-i+1)
				c.sub = c.s[i:j]
			} else {
				c.sub = goxStr(r, r.Intn(3))
			}
			if !utf8.ValidString(c.sub) {
				c.sub = ""
			}
			c.want = fmt.Sprintf("string|%v|%s|none|%d|%s", c.s, goxOpt(true, c.s), strings.Index(c.s, c.sub), strconv.Quote(c.s))
		case 2:
			if r.Intn(3) == 0 {
				c.b = []byte(hostile.Pick(r, hostile.UTF8()))
			} else {
				c.b = make([]byte, r.Intn(10))
				r.Read(c.b)
			}
			c.want = fmt.Sprintf("[]uint8|%v|true|none|%x", c.b, c.b)
		case 3:
			var f float64
			switch r.Intn(4) {
			case 0:
				f = []float64{0, math.Copysign(0, -1), math.Inf(1), math.Inf(-1), math.NaN(), math.MaxFloat64, math.SmallestNonzeroFloat64, 0.1, -2.5e-300, 1e21, 1e20}[r.Intn(11)]
			case 1:
				f = float64(hostile.Pick(r, hostile.Ints()))
			default:
				f = r.NormFloat64() * math.Pow(10, float64(r.Intn(40)-20))
			}
			c.x = int64(math.Float64bits(f))
			c.want = fmt.Sprintf("float64|%v|%t|%s", f, f == f, strconv.FormatFloat(f, 'g', -1, 64))
		case 4:
			if r.Intn(2) == 0 {
				c.s = hostile.Pick(r, hostile.Digits())
			} else {
				c.s = strconv.FormatInt(goxInt(r), 10)
				if r.Intn(3) == 0 && len(c.s) > 0 {
					c.s = c.s[:r.Intn(len(c.s))] + goxStr(r, 1) + c.s[r.Intn(len(c.s)):]
				}
			}
			if !utf8.ValidString(c.s) {
				c.s = "9" + strings.ToValidUTF8(c.s, "")
			}
			if v, err := strconv.Atoi(c.s); err == nil {
				c.want = fmt.Sprintf("ok %d", v)
			} else {
				c.want = fmt.Sprintf("err %s|%T", err.Error(), err)
			}
		case 5:
			c.li = make([]int64, r.Intn(8))
			for i := range c.li {
				c.li[i] = goxInt(r)
			}
			c.want = fmt.Sprintf("[]int64|%v|%s", c.li, goxJoin(c.li))
			if len(c.li) == 0 {
				// std/rt goany.go typedSlice: no element to tell the type by
				c.want = "[]interface {}|[]|"
			}
		case 6:
			m := map[string]int64{}
			for i := r.Intn(6); i > 0; i-- {
				k := goxStr(r, r.Intn(4))
				c.keys = append(c.keys, k)
				m[k] = int64(len(c.keys))
			}
			// the Xo side puts keys in order, so a repeated key keeps its last value
			ks := make([]string, 0, len(m))
			for k := range m {
				ks = append(ks, k)
			}
			sort.Strings(ks)
			es := make([]string, len(ks))
			for i, k := range ks {
				es[i] = fmt.Sprintf("%s=%d", k, m[k])
			}
			c.want = fmt.Sprintf("map[string]int64|%v|%s", m, strings.Join(es, ","))
			if len(m) == 0 {
				c.want = "map[interface {}]interface {}|map[]|"
			}
		case 7:
			xs := make([]any, r.Intn(6))
			for i := range xs {
				xs[i] = goxInt(r)
			}
			txt, _ := json.Marshal(xs)
			c.s = string(txt)
			switch r.Intn(5) {
			case 0:
				c.s = c.s[:r.Intn(len(c.s)+1)]
			case 1:
				c.s = strings.Replace(c.s, "[", "[1e400,", 1)
			case 2:
				c.s = strings.Replace(c.s, "[", "[\"x\",", 1)
			case 3:
				c.s = "null"
			}
			var got []int64
			if err := json.Unmarshal([]byte(c.s), &got); err == nil {
				c.want = "ok " + goxJoin(got)
			} else {
				c.want = fmt.Sprintf("err %s|%T", err.Error(), err)
			}
		default:
			c.s = goxStr(r, r.Intn(10))
			e := errors.New(c.s)
			c.want = fmt.Sprintf("%s|%s|%T", c.s, e.Error(), e)
		}
		cs = append(cs, c)
	}
	return cs
}

// goxLit is s as an Xo Str literal: printable ASCII as is, every other rune
// as \u{...}.
func goxLit(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, c := range s {
		switch {
		case c == '"' || c == '\\' || c == '$':
			b.WriteByte('\\')
			b.WriteRune(c)
		case c >= 0x20 && c < 0x7f:
			b.WriteRune(c)
		default:
			fmt.Fprintf(&b, "\\u{%x}", c)
		}
	}
	b.WriteByte('"')
	return b.String()
}

func goxBytes(p []byte) string {
	var b strings.Builder
	b.WriteString("b\"")
	for _, c := range p {
		fmt.Fprintf(&b, "\\x%02x", c)
	}
	b.WriteByte('"')
	return b.String()
}

const goxCheckXo = `use go "encoding/json"
use go "fmt"
use go "strconv"
use go "strings"
use std/go

fn opt(o: Str?) -> Str {
  match o {
    Some(v) => "some ${v}"
    None => "none"
  }
}

fn join(xs: List[Int]) -> Str { xs.map(fn(x: Int) -> Str { "${x}" }).join(",") }

/// A case's report, built from what std/go and the Go calls give back; the
/// generator builds the same report from Go's answers.
fn report(kind: Int, x: Int, s: Str, sub: Str, b: Bytes, li: List[Int], keys: List[Str]) -> Str {
  if kind == 0 {
    let a = go.any(x)
    let i = match a.as_int() {
      Some(v) => "some ${v}"
      None => "none"
    }
    return "${a.type_name()}|${a.display()}|${i}|${opt(a.as_str())}|${strconv.itoa(x)}|${fmt.sprintf("%x %d", [x, x])}"
  }
  if kind == 1 {
    let a = go.any(s)
    let f = if a.as_float() == None { "none" } else { "some" }
    return "${a.type_name()}|${a.display()}|${opt(a.as_str())}|${f}|${strings.index(s, sub)}|${strconv.quote(s)}"
  }
  if kind == 2 {
    let a = go.any(b)
    return "${a.type_name()}|${a.display()}|${a.as_bytes() == Some(b)}|${opt(a.as_str())}|${fmt.sprintf("%x", [b])}"
  }
  if kind == 3 {
    let f = Float.from_bits(x)
    let a = go.any(f)
    return "${a.type_name()}|${a.display()}|${a.as_float() == Some(f)}|${strconv.format_float(f, 103, -1, 64)}"
  }
  if kind == 4 {
    return match strconv.atoi(s) {
      Ok(v) => "ok ${v}"
      Err(e) => "err ${e.display()}|${e.go_type()}"
    }
  }
  if kind == 5 {
    let a = go.any(li)
    let back = (a.as_list() ?? []).map(fn(e: go.Any) -> Int { e.as_int() ?? -424242 })
    return "${a.type_name()}|${a.display()}|${join(back)}"
  }
  if kind == 6 {
    var m: Map[Str, Int] = {}
    for (i, k) in keys.indexed() {
      m.put(k, i + 1)
    }
    let a = go.any(m)
    let back = a.as_map() ?? {}
    let es = back.keys().sort().map(fn(k: Str) -> Str { "${k}=${back.get(k).map(fn(e: go.Any) -> Int { e.as_int() ?? -424242 }) ?? -1}" })
    return "${a.type_name()}|${a.display()}|${es.join(",")}"
  }
  if kind == 7 {
    let l = go.out[List[Int]]()
    return match json.unmarshal(s.bytes(), l.any()) {
      Ok(_) => "ok ${join(l.get())}"
      Err(e) => "err ${e.display()}|${e.go_type()}"
    }
  }
  let e = go.error(s)
  "${e.message()}|${e.display()}|${e.go_type()}"
}

test "std/go's values cross the Go boundary as Go's own packages say" {
  var bad: List[Str] = []
  for (i, (kind, x, s, sub, b, li, keys, want)) in CASES.indexed() {
    let got = report(kind, x, s, sub, b, li, keys)
    if got != want {
      bad.push("case ${i} (kind ${kind}): got ${got.debug()}, want ${want.debug()}")
    }
  }
  expect(bad[0..(if bad.len() < 10 { bad.len() } else { 10 })]) == []
}
`

func goxXoFile(seed int64, cases []goxCase) string {
	var b strings.Builder
	fmt.Fprintf(&b, "// Generated by internal/stdtest/gofuzz_test.go (seed %d, %d cases);\n", seed, len(cases))
	b.WriteString("// do not edit: XO_GOFUZZ_WRITE=1 go test ./internal/stdtest -run TestGoFuzzCorpusCurrent rewrites it.\n")
	b.WriteString(goxCheckXo)
	b.WriteString("\n/// (kind, Int or Float bits, Str, substring, Bytes, List, Map keys, the report Go's packages give)\nconst CASES: List[(Int, Int, Str, Str, Bytes, List[Int], List[Str], Str)] = [\n")
	for _, c := range cases {
		li := make([]string, len(c.li))
		for i, x := range c.li {
			li[i] = strconv.FormatInt(x, 10)
		}
		ks := make([]string, len(c.keys))
		for i, k := range c.keys {
			ks[i] = goxLit(k)
		}
		fmt.Fprintf(&b, "  (%d, %d, %s, %s, %s, [%s], [%s], %s),\n", c.kind, c.x, goxLit(c.s), goxLit(c.sub), goxBytes(c.b), strings.Join(li, ", "), strings.Join(ks, ", "), goxLit(c.want))
	}
	b.WriteString("]\n")
	return b.String()
}

// goxCoverage fails when the cases stop reaching every kind, atoi and json
// errors, and an empty list.
func goxCoverage(t *testing.T, cases []goxCase) {
	t.Helper()
	kinds := map[int]int{}
	atoiErr, jsonErr, empty := 0, 0, 0
	for _, c := range cases {
		kinds[c.kind]++
		if c.kind == 4 && strings.HasPrefix(c.want, "err ") {
			atoiErr++
		}
		if c.kind == 7 && strings.HasPrefix(c.want, "err ") {
			jsonErr++
		}
		if c.kind == 5 && len(c.li) == 0 {
			empty++
		}
	}
	if len(kinds) < 9 || atoiErr == 0 || jsonErr == 0 || empty == 0 {
		t.Fatalf("coverage fell: kinds %v, %d atoi errors, %d json errors, %d empty lists", kinds, atoiErr, jsonErr, empty)
	}
}

const goxCorpus = "testdata/corpus/gofuzz/gofuzz_test.xo"

// TestGoFuzzCorpusCurrent: the committed corpus is what seed 1 writes.
func TestGoFuzzCorpusCurrent(t *testing.T) {
	cases := goxCases(rand.New(rand.NewSource(1)), 400)
	goxCoverage(t, cases)
	bxCorpusCurrent(t, goxCorpus, "XO_GOFUZZ_WRITE", goxXoFile(1, cases))
}

// TestGoFuzz runs a fresh seed's cases.
func TestGoFuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("the go fuzz is a heavy test; its seed 1 corpus runs in TestStdCorpora")
	}
	seed := int64(envInt(t, "XO_GOFUZZ_SEED", 2))
	n := envInt(t, "XO_GOFUZZ_N", 2000)
	cases := goxCases(rand.New(rand.NewSource(seed)), n)
	runFuzzDir(t, "gofuzz_test.xo", goxXoFile(seed, cases), envBackends("XO_GOFUZZ_BACKENDS", []string{"go"}), len(cases))
}
