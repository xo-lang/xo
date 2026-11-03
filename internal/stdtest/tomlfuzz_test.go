// A conformance corpus and a fuzz harness for std/toml (2026-11-02,
// decision 0461: a parser of untrusted input is Early on testing until it
// is fuzzed). Go's standard library has no TOML package and go.mod stays
// without dependencies (the lead, 2026-11-02), so the reference is the
// official TOML test suite, the conformance standard TOML libraries test
// against, and the fuzz checks invariants rather than a second parser.
//
// Conformance: testdata/toml-test holds toml-test v1.5.0's TOML 1.0.0
// documents (its tests/files-toml-1.0.0 list, MIT, COPYING beside them).
// Each valid document's to_json must give the values of its .json
// expectation (toml-test's typed JSON): a string, date or time as its
// text (a seconds fraction's trailing zeros dropped on both sides, as
// toml-test compares datetimes as instants), a number by value (an integer, and a float with no fraction that
// fits an Int, as that integer: to_json writes JSON, which has one number
// kind; any other float by its bits), arrays and tables in place, table
// keys sorted. A valid document holding inf or nan must be an error
// (std/toml: JSON has no form for them). Each invalid document must be an
// error. A document that is not UTF-8 cannot reach to_json (it takes a
// Str; Bytes.to_str refuses it), so the ten such documents of
// invalid/encoding are left out of testdata/toml-test (the push tools
// read every text file as UTF-8) and counted, not run.
//
// Fuzz: documents from pieces (keys bare, quoted and dotted, every value
// kind with its edge forms, tables, arrays of tables, inline tables,
// comments, a key defined twice, a table reopened) and each cut short or
// with a byte changed. Every one must hold these invariants: to_json and
// decode[Map[Str, json.Value]] succeed together; to_json's text parses as
// JSON; a decoded value encodes, the encoding decodes, and the two values
// write the same JSON. The generated test holds no expected answers, only
// the documents.
//
// TestTomlCorpusCurrent checks testdata/corpus/tomltest/tomltest_test.xo
// (the suite) and testdata/corpus/tomlfuzz/tomlfuzz_test.xo (seed 1, 2,000
// documents) are what this file writes (XO_TOMLFUZZ_WRITE=1 rewrites
// them); TestStdCorpora runs both on every corpus backend. TestTomlFuzz
// runs a fresh seed (XO_TOMLFUZZ_SEED, default 2; XO_TOMLFUZZ_N, default
// 2000) on XO_TOMLFUZZ_BACKENDS (default arm64 and llvm).
package stdtest

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"
)

const tfSuite = "testdata/toml-test"

// tfSuiteCase is one document of the suite and its expected answer.
type tfSuiteCase struct {
	name, text, want string
}

// tfSuiteCases reads the suite's TOML 1.0.0 documents; skipped counts the
// listed invalid/encoding documents left out for not being UTF-8.
func tfSuiteCases(t *testing.T) (cases []tfSuiteCase, skipped int) {
	t.Helper()
	f, err := os.Open(filepath.Join(tfSuite, "files-toml-1.0.0"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		name := sc.Text()
		if !strings.HasSuffix(name, ".toml") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(tfSuite, name))
		if os.IsNotExist(err) && strings.HasPrefix(name, "invalid/encoding/") {
			skipped++ // not UTF-8, so not vendored (below)
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if !utf8.Valid(raw) {
			t.Fatalf("%s is not UTF-8: leave it out of %s, as the ten invalid/encoding documents are", name, tfSuite)
		}
		c := tfSuiteCase{name: name, text: string(raw), want: "err"}
		if strings.HasPrefix(name, "valid/") {
			exp, err := os.ReadFile(filepath.Join(tfSuite, strings.TrimSuffix(name, ".toml")+".json"))
			if err != nil {
				t.Fatal(err)
			}
			var v any
			if err := json.Unmarshal(exp, &v); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			if r, ok := tfTagged(v); ok {
				c.want = "ok " + r
			}
		}
		cases = append(cases, c)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return cases, skipped
}

// tfNum is a number as the generated test's tf_r writes it.
func tfNum(f float64) string {
	if f == math.Trunc(f) && math.Abs(f) < 9.2e18 {
		return "n" + strconv.FormatInt(int64(f), 10)
	}
	return "f" + strconv.FormatInt(int64(math.Float64bits(f)), 10)
}

// tfFrac is tf_frac: s with the trailing zeros of a seconds fraction
// dropped (toml-test compares datetimes as instants, so ".6" is ".600").
// Both sides apply it to every string.
func tfFrac(s string) string {
	i := 3
	for i < len(s) && !(s[i] == '.' && s[i-3] == ':') {
		i++
	}
	if i >= len(s) {
		return s
	}
	j := i + 1
	for j < len(s) && s[j] >= '0' && s[j] <= '9' {
		j++
	}
	k := j
	for k > i+1 && s[k-1] == '0' {
		k--
	}
	if k == i+1 {
		k = i
	}
	return s[:k] + s[j:]
}

// tfTagged renders toml-test's typed JSON; false when it holds inf or nan.
func tfTagged(v any) (string, bool) {
	switch v := v.(type) {
	case []any:
		parts := make([]string, len(v))
		for i, x := range v {
			r, ok := tfTagged(x)
			if !ok {
				return "", false
			}
			parts[i] = r
		}
		return "[" + strings.Join(parts, ",") + "]", true
	case map[string]any:
		if ty, ok := v["type"].(string); ok && len(v) == 2 {
			if val, ok := v["value"].(string); ok {
				switch ty {
				case "integer":
					n, err := strconv.ParseInt(val, 10, 64)
					if err != nil {
						panic(fmt.Sprintf("integer %q: %v", val, err))
					}
					return "n" + strconv.FormatInt(n, 10), true
				case "float":
					f, err := strconv.ParseFloat(strings.TrimPrefix(val, "+"), 64)
					if err != nil || math.IsInf(f, 0) || math.IsNaN(f) {
						return "", false
					}
					return tfNum(f), true
				case "bool":
					if val == "true" {
						return "t", true
					}
					return "f", true
				default: // string, datetime, datetime-local, date-local, time-local
					return fmt.Sprintf("s%x", tfFrac(val)), true
				}
			}
		}
		parts := make([]string, 0, len(v))
		for k, x := range v {
			r, ok := tfTagged(x)
			if !ok {
				return "", false
			}
			parts = append(parts, fmt.Sprintf("%x:%s", k, r))
		}
		sort.Strings(parts)
		return "{" + strings.Join(parts, ",") + "}", true
	}
	panic(fmt.Sprintf("tfTagged: %T", v))
}

const tfRenderXo = `use std/json
use std/toml

fn tf_r(v: json.Value) -> Str {
  match v {
    json.Value.Null => "null"
    json.Value.Bool(b) => if b { "t" } else { "f" }
    json.Value.Int(n) => "n${n}"
    json.Value.Num(f) => {
      if f == f.floor() && f.abs() < 9.2e18 {
        return "n${Int(f)}"
      }
      "f${f.to_bits()}"
    }
    json.Value.Str(s) => "s${tf_frac(s).bytes().to_hex()}"
    json.Value.List(items) => {
      var parts: List[Str] = []
      for x in items {
        parts.push(tf_r(x))
      }
      "[" + parts.join(",") + "]"
    }
    json.Value.Object(fields) => {
      var parts: List[Str] = []
      for (k, x) in fields {
        parts.push("${k.bytes().to_hex()}:${tf_r(x)}")
      }
      "{" + parts.sort().join(",") + "}"
    }
    _ => "other"
  }
}

/// s with the trailing zeros of a seconds fraction dropped (".600" is
/// ".6", ".000" nothing): toml-test compares datetimes as instants.
fn tf_frac(s: Str) -> Str {
  let b = s.bytes()
  var i = 3
  while i < b.len() && !(b[i] == 46 && b[i - 3] == 58) {
    i += 1
  }
  if i >= b.len() {
    return s
  }
  var j = i + 1
  while j < b.len() && b[j] >= 48 && b[j] <= 57 {
    j += 1
  }
  var k = j
  while k > i + 1 && b[k - 1] == 48 {
    k -= 1
  }
  if k == i + 1 {
    k = i
  }
  s[..k] + s[j..]
}

fn tf_line(text: Str) -> Str {
  match toml.to_json(text) {
    Ok(j) => match json.parse_value(j) {
      Ok(v) => "ok " + tf_r(v)
      Err(e) => "bad json " + e.msg
    }
    Err(_) => "err"
  }
}

`

func tfSuiteFile(cases []tfSuiteCase, skipped int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "// Generated by internal/stdtest/tomlfuzz_test.go from toml-test v1.5.0's TOML 1.0.0\n")
	fmt.Fprintf(&b, "// documents (internal/stdtest/testdata/toml-test, MIT): %d run, %d not UTF-8 and so\n", len(cases), skipped)
	b.WriteString("// not run; do not edit: XO_TOMLFUZZ_WRITE=1 go test ./internal/stdtest -run TestTomlCorpusCurrent\n")
	b.WriteString("// rewrites it.\n")
	b.WriteString(tfRenderXo)
	for _, c := range cases {
		fmt.Fprintf(&b, "test %s {\n  let got = tf_line(%s)\n  if got != %s {\n    fault(\"got \" + got)\n  }\n}\n\n", xoStr(c.name), xoStr(c.text), xoStr(c.want))
	}
	return b.String()
}

var tfKeys = []string{"a", "b", "a", "key", "bare-key_1", `"quoted"`, `"a"`, `'lit'`, `""`, "a.b", "a . b", `a."b.c"`, "1234", "true", "-"}
var tfBadKeys = []string{"a b", "=", "a..b", ".a", "a.", "é", "[a]", "#"}

var tfValues = []string{
	`"s"`, `""`, `"a\nb"`, `"\u00e9"`, `"\U0001F600"`, `"tab\there"`, `'lit\n'`, `"""multi
line"""`, `'''raw
'''`, `"""a\
   b"""`,
	"0", "1", "-1", "+1", "1_000", "0x1F", "0o17", "0b101", "9223372036854775807", "-9223372036854775808",
	"1.5", "-0.0", "1e10", "6.626e-34", "1_000.5", "inf", "-inf", "nan",
	"true", "false",
	"1979-05-27T07:32:00Z", "1979-05-27T00:32:00.999999-07:00", "1979-05-27 07:32:00Z", "1979-05-27T07:32:00",
	"1979-05-27", "07:32:00", "00:32:00.999999",
	"[]", "[1, 2, 3]", `["a", 'b']`, "[1, \"mixed\"]", "[[1], [2, 3]]", "[\n  1,\n  2,\n]",
	"{}", "{ x = 1 }", `{ x = 1, y = "z" }`, "{ a.b = 1 }",
}
var tfBadValues = []string{
	`"open`, `"\x"`, `"\uD800"`, "01", "1__0", "_1", "1_", "0x", "1.", ".5", "1e", "+0x1", "9223372036854775808",
	"1979-13-27", "1979-05-27T25:00:00", "07:32", "tru", "[1,,2]", "[1 2]", "{ x = 1, }", "{ x = 1\n}", "",
	"'''open", `"""open`, "nan1", "infinity",
}

var tfSpace = []string{" ", " ", "", "\t"}

func tfPick(r *rand.Rand, xs []string) string { return xs[r.Intn(len(xs))] }

func tfMix(r *rand.Rand, good, bad []string, n int) string {
	if r.Intn(n) == 0 {
		return tfPick(r, bad)
	}
	return tfPick(r, good)
}

// tfDoc is a document of 1 to 8 lines: key/value pairs, table headers,
// array of tables headers, comments and blank lines.
func tfDoc(r *rand.Rand) string {
	var b strings.Builder
	for i, n := 0, 1+r.Intn(8); i < n; i++ {
		switch r.Intn(10) {
		case 0:
			b.WriteString("[" + tfMix(r, tfKeys, tfBadKeys, 15) + "]")
		case 1:
			b.WriteString("[[" + tfMix(r, tfKeys, tfBadKeys, 15) + "]]")
		case 2:
			b.WriteString(tfPick(r, []string{"# comment", "", "  # x = 1", "\t"}))
		default:
			b.WriteString(tfMix(r, tfKeys, tfBadKeys, 15) + tfPick(r, tfSpace) + "=" + tfPick(r, tfSpace) + tfMix(r, tfValues, tfBadValues, 12))
			if r.Intn(8) == 0 {
				b.WriteString(" # trailing")
			}
		}
		b.WriteString(tfPick(r, []string{"\n", "\n", "\r\n"}))
	}
	return b.String()
}

// tfFuzzCases are n documents from seed r; one in ten is cut short, one
// in ten has a byte changed (a document that stops being UTF-8 is drawn
// again).
func tfFuzzCases(r *rand.Rand, n int) []string {
	var out []string
	for len(out) < n {
		raw := []byte(tfDoc(r))
		switch r.Intn(10) {
		case 0:
			raw = raw[:r.Intn(len(raw)+1)]
		case 1:
			if len(raw) > 0 {
				mutate := "=[]{}\"'.#,\n 0_-:"
				raw[r.Intn(len(raw))] = mutate[r.Intn(len(mutate))]
			}
		}
		if utf8.Valid(raw) {
			out = append(out, string(raw))
		}
	}
	return out
}

const tfFuzzXo = `/// The invariants every document holds: to_json and decode agree on
/// whether it is TOML, to_json's text is JSON, and a decoded value
/// encodes to a document that decodes to the same value (key order
/// aside: encode writes a table's plain keys before its tables).
fn tf_check(text: Str) -> Str {
  let j = toml.to_json(text)
  let d = toml.decode[Map[Str, json.Value]](text)
  if j.is_ok() != d.is_ok() {
    return "to_json ok ${j.is_ok()} but decode ok ${d.is_ok()}"
  }
  match j {
    Ok(s) => {
      if json.parse_value(s).is_err() {
        return "to_json wrote text that is not JSON: " + s
      }
    }
    Err(_) => return ""
  }
  match d {
    Ok(v) => {
      let first = tf_r(json.to_value(v))
      match toml.encode(v) {
        Ok(doc) => match toml.decode[Map[Str, json.Value]](doc) {
          Ok(v2) => {
            let second = tf_r(json.to_value(v2))
            if second != first {
              return "round trip changed " + first + " to " + second + " through " + doc
            }
          }
          Err(e) => return "encode wrote a document decode refuses: " + e.display() + " in " + doc
        }
        Err(e) => return "encode failed: " + e.display()
      }
    }
    Err(_) => {}
  }
  ""
}

`

func tfFuzzFile(seed int64, docs []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "// Generated by internal/stdtest/tomlfuzz_test.go (seed %d, %d documents); do not\n", seed, len(docs))
	b.WriteString("// edit: XO_TOMLFUZZ_WRITE=1 go test ./internal/stdtest -run TestTomlCorpusCurrent rewrites it.\n")
	b.WriteString(tfRenderXo)
	b.WriteString(tfFuzzXo)
	for i, d := range docs {
		fmt.Fprintf(&b, "test \"toml fuzz %d\" {\n  let bad = tf_check(%s)\n  if bad != \"\" {\n    fault(bad)\n  }\n}\n\n", i, xoStr(d))
	}
	return b.String()
}

const (
	tfSuiteCorpus = "testdata/corpus/tomltest/tomltest_test.xo"
	tfFuzzCorpus  = "testdata/corpus/tomlfuzz/tomlfuzz_test.xo"
)

// TestTomlCorpusCurrent: the two committed corpora are what this file
// writes.
func TestTomlCorpusCurrent(t *testing.T) {
	cases, skipped := tfSuiteCases(t)
	valid, errs := 0, 0
	for _, c := range cases {
		if c.want == "err" {
			errs++
		} else {
			valid++
		}
	}
	t.Logf("suite: %d documents, %d with values, %d errors, %d not UTF-8", len(cases), valid, errs, skipped)
	if valid < 150 || errs < 300 {
		t.Fatalf("the suite shrank: %d with values, %d errors", valid, errs)
	}
	files := map[string]string{
		tfSuiteCorpus: tfSuiteFile(cases, skipped),
		tfFuzzCorpus:  tfFuzzFile(1, tfFuzzCases(rand.New(rand.NewSource(1)), 2000)),
	}
	for path, text := range files {
		if os.Getenv("XO_TOMLFUZZ_WRITE") == "1" {
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		have, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(have) != text {
			t.Errorf("%s is not what this file writes: XO_TOMLFUZZ_WRITE=1 go test ./internal/stdtest -run TestTomlCorpusCurrent", path)
		}
	}
}

// TestTomlFuzz runs a fresh seed's documents.
func TestTomlFuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("the toml fuzz is a heavy test; its seed 1 corpus runs in TestStdCorpora")
	}
	seed := int64(envInt(t, "XO_TOMLFUZZ_SEED", 2))
	n := envInt(t, "XO_TOMLFUZZ_N", 2000)
	docs := tfFuzzCases(rand.New(rand.NewSource(seed)), n)
	for i, d := range docs {
		t.Logf("case %d %q", i, d)
	}
	runFuzzDir(t, "tomlfuzz_test.xo", tfFuzzFile(seed, docs), envBackends("XO_TOMLFUZZ_BACKENDS", []string{"arm64", "llvm"}), len(docs))
}
