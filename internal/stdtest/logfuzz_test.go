// A fuzz harness for std/log's records (decision 0616's fuzz kind; task
// #108). Each case is a level, a message and fields (Str, Int, Float and
// Bool values; keys that repeat, that name level, msg or module, empty,
// with spaces or UTF-8; strings with quotes, backslashes, control bytes,
// <, > and &, U+2028 and invalid ranges of Unicode), some carried by a
// Logger made with `with`. Each is written through log.json_handler and
// log.text_handler at Debug into a func_sink, and must give:
//
//   - JSON: Go's encoding/json Marshal of the same map (std/log's JSON
//     record is encoding/json of a map: sorted keys, a record's field
//     replacing the logger's, level and msg replaced by a field of that
//     name), the time compared as a placeholder;
//   - text: `TIME LEVEL msg k=v ...`, the logger's fields first in the
//     order given, a record's field replacing the logger's in its place,
//     each value's text (spec/std.md 6); no Go twin, so a model.
//
// Floats are finite (encoding/json refuses NaN and infinities).
//
// The cases from seed 1 are the committed corpus
// testdata/corpus/logfuzz/logfuzz_test.xo, which TestStdCorpora runs on
// every corpus backend; TestLogFuzzCorpusCurrent checks it is what this
// generator writes (XO_LOGFUZZ_WRITE=1 rewrites it). TestLogFuzz runs a
// fresh seed (XO_LOGFUZZ_SEED, default 2; XO_LOGFUZZ_N cases, default
// 2000) on XO_LOGFUZZ_BACKENDS (default arm64 and llvm).
package stdtest

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"strconv"
	"strings"
	"testing"
)

type lgField struct {
	key, kind, text string // kind: s i f b
}

type lgCase struct {
	level        string
	msg          string
	with, fields []lgField
	json, text   string
}

var lgLevels = []string{"DEBUG", "INFO", "WARN", "ERROR"}
var lgKeys = []string{"a", "n", "port", "addr", "level", "msg", "module", "a b", "é", "k=v", "x\"y", "", "user_id", "n"}

func lgStr(r *rand.Rand) string {
	pieces := []string{"a", "hello", " ", "\"", "\\", "\n", "\t", "\x01", "\x7f", "<", ">", "&", " ", " ", "é", "日本", "🙂", "=", "{", "}", ",", ":", "�", "%v", "$"}
	var b strings.Builder
	for k := r.Intn(6); k > 0; k-- {
		b.WriteString(pieces[r.Intn(len(pieces))])
	}
	return b.String()
}

func lgFloat(r *rand.Rand) float64 {
	switch r.Intn(6) {
	case 0:
		return []float64{0, -0.0, 1, -1, 0.5, 2.5, 1e21, 1e20, 1e-6, 1e-7, 123456789.125, math.MaxFloat64, math.SmallestNonzeroFloat64, 0.1, 1.0 / 3}[r.Intn(15)]
	case 1:
		return float64(r.Intn(1000)) / 8
	case 2:
		return r.NormFloat64() * math.Pow(10, float64(r.Intn(60)-30))
	default:
		return math.Float64frombits(r.Uint64()&^(0x7ff<<52) | uint64(r.Intn(0x7fe)+1)<<52)
	}
}

func lgFieldOf(r *rand.Rand) lgField {
	key := lgKeys[r.Intn(len(lgKeys))]
	if r.Intn(5) == 0 {
		key = lgStr(r)
	}
	switch r.Intn(4) {
	case 0:
		return lgField{key, "s", lgStr(r)}
	case 1:
		n := []int64{0, 1, -1, 42, math.MaxInt64, math.MinInt64, r.Int63() - r.Int63()}[r.Intn(7)]
		return lgField{key, "i", strconv.FormatInt(n, 10)}
	case 2:
		return lgField{key, "f", strconv.FormatInt(int64(math.Float64bits(lgFloat(r))), 10)}
	}
	return lgField{key, "b", []string{"true", "false"}[r.Intn(2)]}
}

func lgAny(f lgField) any {
	switch f.kind {
	case "i":
		n, _ := strconv.ParseInt(f.text, 10, 64)
		return n
	case "f":
		b, _ := strconv.ParseInt(f.text, 10, 64)
		return math.Float64frombits(uint64(b))
	case "b":
		return f.text == "true"
	}
	return f.text
}

// lgValueText is a value as a text record shows it: its Xo text (a Str
// as it is; a Float's shortest digits, with .0 when they have no point or
// exponent).
func lgValueText(f lgField) string {
	if f.kind == "f" {
		t := strconv.FormatFloat(lgAny(f).(float64), 'g', -1, 64)
		if !strings.ContainsAny(t, ".eIN") {
			t += ".0"
		}
		return t
	}
	return fmt.Sprint(lgAny(f))
}

func lgCaseOf(level, msg string, with, fields []lgField) lgCase {
	// the fields in record order: the logger's, a record's field replacing
	// the logger's in its place, then the record's new ones
	type kv struct {
		k string
		f lgField
	}
	var order []kv
	put := func(f lgField) {
		for i := range order {
			if order[i].k == f.key {
				order[i].f = f
				return
			}
		}
		order = append(order, kv{f.key, f})
	}
	for _, f := range with {
		put(f)
	}
	for _, f := range fields {
		put(f)
	}
	m := map[string]any{"time": "T", "level": level, "msg": msg}
	text := level + " " + msg
	for _, e := range order {
		m[e.k] = lgAny(e.f)
		text += " " + e.k + "=" + lgValueText(e.f)
	}
	j, err := json.Marshal(m)
	if err != nil {
		panic(err)
	}
	return lgCase{level, msg, with, fields, string(j), text}
}

func lgCases(r *rand.Rand, n int) []lgCase {
	cs := make([]lgCase, 0, n)
	for len(cs) < n {
		var with, fields []lgField
		if r.Intn(3) == 0 {
			for k := 1 + r.Intn(3); k > 0; k-- {
				with = append(with, lgFieldOf(r))
			}
		}
		for k := r.Intn(5); k > 0; k-- {
			fields = append(fields, lgFieldOf(r))
		}
		// "time" stays out of the keys: the record's time is compared as a placeholder
		ok := true
		for _, f := range append(append([]lgField{}, with...), fields...) {
			if f.key == "time" {
				ok = false
			}
		}
		if !ok {
			continue
		}
		cs = append(cs, lgCaseOf(lgLevels[r.Intn(4)], lgStr(r), with, fields))
	}
	return cs
}

const lgCheckXo = `use std/log

fn fields_of(rows: List[(Str, Str, Str)]) -> Map[Str, Debug] {
  var m: Map[Str, Debug] = {}
  for (k, kind, text) in rows {
    match kind {
      "i" => m[k] = Int.parse(text) ?? fault("int ${text}")
      "f" => m[k] = Float.from_bits(Int.parse(text) ?? fault("float ${text}"))
      "b" => m[k] = text == "true"
      _ => m[k] = text
    }
  }
  m
}

fn write(level: Str, msg: Str, with: List[(Str, Str, Str)], fields: List[(Str, Str, Str)]) uses log {
  let fs = fields_of(fields)
  if with.is_empty() {
    match level {
      "DEBUG" => log.debug(msg, fs)
      "INFO" => log.info(msg, fs)
      "WARN" => log.warn(msg, fs)
      _ => log.error(msg, fs)
    }
    return
  }
  let l = log.with(fields_of(with))
  match level {
    "DEBUG" => l.debug(msg, fs)
    "INFO" => l.info(msg, fs)
    "WARN" => l.warn(msg, fs)
    _ => l.error(msg, fs)
  }
}

/// A JSON record with its time as T.
fn json_t(line: Str) -> Str {
  match line.cut("\"time\":\"") {
    Some((a, rest)) => match rest.cut("\"") {
      Some((_, b)) => a + "\"time\":\"T\"" + b
      None => line
    }
    None => line
  }
}

/// A text record without its time.
fn text_t(line: Str) -> Str {
  match line.cut(" ") {
    Some((_, rest)) => rest
    None => line
  }
}

test "std/log's JSON records are Go's encoding/json of the map, its text records the spec's" {
  let lines: Chan[Str] = Chan.new(8)
  let sink = log.func_sink(fn(l: Str) { lines.send(l) })
  log.configure_handler(log.multi([log.json_handler(sink, level: log.Level.Debug), log.text_handler(sink, level: log.Level.Debug)]))
  var bad: List[(Str, Str, Str)] = []
  for (level, msg, with, fields, want_json, want_text) in CASES {
    write(level, msg, with, fields)
    var got: List[Str] = []
    while got.len() < 2 {
      match lines.recv() {
        Some(l) => got.push(l)
        None => break
      }
    }
    if got.len() != 2 || json_t(got[0]) != want_json || text_t(got[1]) != want_text {
      bad.push((msg, got.join("\n"), want_json + "\n" + want_text))
    }
  }
  expect(bad[0..(if bad.len() < 10 { bad.len() } else { 10 })]) == []
}
`

func lgRows(fs []lgField) string {
	parts := make([]string, len(fs))
	for i, f := range fs {
		parts[i] = fmt.Sprintf("(%s, %q, %s)", xoStr(f.key), f.kind, xoStr(f.text))
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func lgXoFile(seed int64, cases []lgCase) string {
	var b strings.Builder
	fmt.Fprintf(&b, "// Generated by internal/stdtest/logfuzz_test.go (seed %d, %d cases) from Go's\n", seed, len(cases))
	b.WriteString("// encoding/json; do not edit: XO_LOGFUZZ_WRITE=1 go test ./internal/stdtest -run TestLogFuzzCorpusCurrent\n")
	b.WriteString("// rewrites it.\n")
	b.WriteString(lgCheckXo)
	b.WriteString("\n/// (level, message, the logger's fields, the record's fields as (key, kind s i f b, text; a float as its bits), Go's JSON record with time T, the text record without its time)\nconst CASES: List[(Str, Str, List[(Str, Str, Str)], List[(Str, Str, Str)], Str, Str)] = [\n")
	for _, c := range cases {
		fmt.Fprintf(&b, "  (%q, %s, %s, %s, %s, %s),\n", c.level, xoStr(c.msg), lgRows(c.with), lgRows(c.fields), xoStr(c.json), xoStr(c.text))
	}
	b.WriteString("]\n")
	return b.String()
}

// lgCoverage fails when the cases stop reaching loggers, replaced keys and
// each kind.
func lgCoverage(t *testing.T, cases []lgCase) {
	t.Helper()
	withs, kinds, special := 0, map[string]int{}, 0
	for _, c := range cases {
		if len(c.with) > 0 {
			withs++
		}
		for _, f := range append(append([]lgField{}, c.with...), c.fields...) {
			kinds[f.kind]++
			if f.key == "level" || f.key == "msg" || f.key == "module" {
				special++
			}
		}
	}
	n := len(cases)
	if withs < n/5 || special < n/10 || kinds["s"] < n/3 || kinds["i"] < n/3 || kinds["f"] < n/3 || kinds["b"] < n/3 {
		t.Fatalf("coverage fell: %d with loggers, %d level/msg/module keys, kinds %v in %d cases", withs, special, kinds, n)
	}
}

const lgCorpus = "testdata/corpus/logfuzz/logfuzz_test.xo"

// TestLogFuzzCorpusCurrent: the committed corpus is what seed 1 writes.
func TestLogFuzzCorpusCurrent(t *testing.T) {
	cases := lgCases(rand.New(rand.NewSource(1)), 300)
	lgCoverage(t, cases)
	bxCorpusCurrent(t, lgCorpus, "XO_LOGFUZZ_WRITE", lgXoFile(1, cases))
}

// TestLogFuzz runs a fresh seed's cases.
func TestLogFuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("the log fuzz is a heavy test; its seed 1 corpus runs in TestStdCorpora")
	}
	seed := int64(envInt(t, "XO_LOGFUZZ_SEED", 2))
	n := envInt(t, "XO_LOGFUZZ_N", 2000)
	cases := lgCases(rand.New(rand.NewSource(seed)), n)
	runFuzzDir(t, "logfuzz_test.xo", lgXoFile(seed, cases), envBackends("XO_LOGFUZZ_BACKENDS", []string{"arm64", "llvm"}), len(cases))
}
