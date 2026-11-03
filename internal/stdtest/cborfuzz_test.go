// A fuzz harness for std/cbor against github.com/fxamacker/cbor/v2 (MIT)
// (2026-11-03, decision 0461: a parser of untrusted input is Early on
// testing until it is fuzzed; decision 0468).
//
// Go has no CBOR in its standard library, so the oracle is that package,
// run by internal/stdtest/cborref (its own module, so the main module keeps
// no dependency; `go run .` there needs the module in the cache or the
// network). The cases are built at the byte level: a random item tree
// written with random head widths (not only the shortest), indefinite
// length strings, arrays and maps, floats of every width with the special
// values, tags (0 to 5, 24, 55799, large numbers, bignums on byte strings),
// simple values, repeated map keys and text that is not UTF-8; one case in
// three is then changed (bits flipped, cut, a byte inserted, removed or
// replaced, a chunk repeated).
//
// Each case is checked against what the oracle says: well formed or not,
// the diagnostic notation (compared when no item has an indefinite length),
// UTF-8 refusals, repeated map keys, and the deterministic re-encoding
// (compared unless a tag, undefined or NaN changes meaning in the oracle's
// interface decode).
//
// The cases from seed 1 are the committed corpus testdata/corpus/cborfuzz/
// cborfuzz_test.xo, which TestStdCorpora runs on every corpus backend;
// TestCborFuzzCorpusCurrent checks it is what this generator writes
// (XO_CBORFUZZ_WRITE=1 rewrites it; it skips when the oracle cannot run).
// TestCborFuzz runs a fresh seed (XO_CBORFUZZ_SEED, default 2;
// XO_CBORFUZZ_N cases, default 2000) on XO_CBORFUZZ_BACKENDS (default arm64
// and llvm).
package stdtest

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type cbCase struct {
	data []byte
	ref  cbRef
}

type cbRef struct {
	WF    bool   `json:"wf"`
	Diag  string `json:"diag"`
	Canon string `json:"canon"`
	Dup   string `json:"dup"`
}

// cbOracle runs the oracle; ok is false when it cannot run.
func cbOracle(t testing.TB, inputs [][]byte) (refs []cbRef, ok bool) {
	var in bytes.Buffer
	for _, b := range inputs {
		in.WriteString(hex.EncodeToString(b))
		in.WriteByte('\n')
	}
	cmd := exec.Command("go", "run", ".")
	cmd.Dir = "cborref"
	cmd.Stdin = &in
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Logf("the cbor oracle cannot run: %v\n%s", err, stderr.String())
		return nil, false
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		var r cbRef
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatal(err)
		}
		refs = append(refs, r)
	}
	if len(refs) != len(inputs) {
		t.Fatalf("the cbor oracle answered %d of %d inputs", len(refs), len(inputs))
	}
	return refs, true
}

type cbGen struct {
	r *rand.Rand
	b []byte
}

// head writes a head with argument n, in the shortest form or (one time in
// four) a longer one.
func (g *cbGen) head(major byte, n uint64) {
	m := major << 5
	min := 0
	switch {
	case n < 24:
		min = 0
	case n < 1<<8:
		min = 1
	case n < 1<<16:
		min = 2
	case n < 1<<32:
		min = 3
	default:
		min = 4
	}
	form := min
	if g.r.Intn(4) == 0 && min < 4 {
		form = min + 1 + g.r.Intn(4-min)
	}
	switch form {
	case 0:
		g.b = append(g.b, m|byte(n))
	case 1:
		g.b = append(g.b, m|24, byte(n))
	case 2:
		g.b = append(g.b, m|25, byte(n>>8), byte(n))
	case 3:
		g.b = append(g.b, m|26, byte(n>>24), byte(n>>16), byte(n>>8), byte(n))
	default:
		g.b = append(g.b, m|27, byte(n>>56), byte(n>>48), byte(n>>40), byte(n>>32), byte(n>>24), byte(n>>16), byte(n>>8), byte(n))
	}
}

func (g *cbGen) uint() uint64 {
	switch g.r.Intn(8) {
	case 0:
		return uint64(g.r.Intn(24))
	case 1:
		return uint64(g.r.Intn(256))
	case 2:
		return uint64(g.r.Intn(65536))
	case 3:
		return uint64(g.r.Uint32())
	case 4:
		return g.r.Uint64()
	case 5:
		return math.MaxUint64 - uint64(g.r.Intn(3))
	case 6:
		return uint64(1)<<uint(g.r.Intn(64)) - uint64(g.r.Intn(2))
	}
	return uint64(g.r.Intn(1000))
}

func (g *cbGen) text() []byte {
	switch g.r.Intn(10) {
	case 0:
		return nil
	case 1:
		return []byte("\u00fc\u6c34\U00010151 \"\\\n\t\x01\x7f")
	case 2:
		if g.r.Intn(5) == 0 {
			b := make([]byte, 1+g.r.Intn(4))
			for i := range b {
				b[i] = byte(g.r.Intn(256))
			}
			return b // often not UTF-8
		}
	case 3:
		return []byte("a")
	}
	var b []byte
	for i, n := 0, g.r.Intn(12); i < n; i++ {
		switch g.r.Intn(8) {
		case 0:
			b = append(b, []byte(string(rune(0x80+g.r.Intn(0x700))))...)
		case 1:
			b = append(b, []byte(string(rune(0x1F600+g.r.Intn(64))))...)
		default:
			b = append(b, byte(32+g.r.Intn(95)))
		}
	}
	return b
}

func (g *cbGen) bytes() []byte {
	b := make([]byte, g.r.Intn(10))
	for i := range b {
		b[i] = byte(g.r.Intn(256))
	}
	return b
}

func (g *cbGen) float() {
	specials := []uint64{0, 1 << 63, 0x3ff0000000000000, 0x3ff199999999999a, 0x7ff0000000000000, 0xfff0000000000000, 0x7ff8000000000000,
		0x7e37e43c8800759c, 0x3e70000000000000, 0x3810000000000000, 0x47efffffe0000000, 0x7fefffffffffffff, 1}
	switch g.r.Intn(6) {
	case 0: // half
		h := uint16(g.r.Intn(65536))
		g.b = append(g.b, 0xf9, byte(h>>8), byte(h))
	case 1: // single
		s := g.r.Uint32()
		if g.r.Intn(2) == 0 {
			s = math.Float32bits(float32(g.r.NormFloat64() * 100))
		}
		g.b = append(g.b, 0xfa, byte(s>>24), byte(s>>16), byte(s>>8), byte(s))
	default:
		var d uint64
		switch g.r.Intn(3) {
		case 0:
			d = specials[g.r.Intn(len(specials))]
		case 1:
			d = math.Float64bits(g.r.NormFloat64() * math.Pow(10, float64(g.r.Intn(40)-20)))
		default:
			d = math.Float64bits(float64(g.r.Intn(2000)-1000) / float64(1+g.r.Intn(8)))
		}
		g.b = append(g.b, 0xfb, byte(d>>56), byte(d>>48), byte(d>>40), byte(d>>32), byte(d>>24), byte(d>>16), byte(d>>8), byte(d))
	}
}

func (g *cbGen) item(depth int) {
	k := g.r.Intn(16)
	if depth > 6 && k >= 6 && k <= 12 {
		k = g.r.Intn(6)
	}
	switch k {
	case 0, 1:
		g.head(0, g.uint())
	case 2:
		g.head(1, g.uint())
	case 3:
		b := g.bytes()
		g.head(2, uint64(len(b)))
		g.b = append(g.b, b...)
	case 4:
		t := g.text()
		g.head(3, uint64(len(t)))
		g.b = append(g.b, t...)
	case 5:
		g.float()
	case 6, 7:
		g.array(depth)
	case 8, 9:
		g.mapping(depth)
	case 10:
		g.tag(depth)
	case 11:
		g.chunks(2)
	case 12:
		g.chunks(3)
	case 13:
		g.b = append(g.b, []byte{0xf4, 0xf5, 0xf6, 0xf7}[g.r.Intn(4)])
	case 14:
		if g.r.Intn(2) == 0 {
			g.b = append(g.b, 0xe0|byte(g.r.Intn(20)))
		} else {
			g.b = append(g.b, 0xf8, byte(32+g.r.Intn(224)))
		}
	default:
		g.head(0, uint64(g.r.Intn(30)))
	}
}

func (g *cbGen) array(depth int) {
	n := g.r.Intn(5)
	if g.r.Intn(4) == 0 {
		g.b = append(g.b, 0x9f)
		for i := 0; i < n; i++ {
			g.item(depth + 1)
		}
		g.b = append(g.b, 0xff)
		return
	}
	g.head(4, uint64(n))
	for i := 0; i < n; i++ {
		g.item(depth + 1)
	}
}

func (g *cbGen) mapping(depth int) {
	n := g.r.Intn(5)
	var keys [][]byte
	pair := func() {
		var k []byte
		if len(keys) > 0 && g.r.Intn(6) == 0 {
			k = keys[g.r.Intn(len(keys))] // a repeated key
			g.b = append(g.b, k...)
		} else {
			at := len(g.b)
			if g.r.Intn(3) == 0 {
				g.item(depth + 1)
			} else if g.r.Intn(2) == 0 {
				g.head(0, uint64(g.r.Intn(300)))
			} else {
				t := g.text()
				g.head(3, uint64(len(t)))
				g.b = append(g.b, t...)
			}
			k = append([]byte(nil), g.b[at:]...)
			keys = append(keys, k)
		}
		g.item(depth + 1)
	}
	if g.r.Intn(4) == 0 {
		g.b = append(g.b, 0xbf)
		for i := 0; i < n; i++ {
			pair()
		}
		g.b = append(g.b, 0xff)
		return
	}
	g.head(5, uint64(n))
	for i := 0; i < n; i++ {
		pair()
	}
}

func (g *cbGen) tag(depth int) {
	nums := []uint64{0, 1, 2, 3, 4, 5, 21, 22, 23, 24, 32, 55799, 1 << 32, math.MaxUint64}
	n := nums[g.r.Intn(len(nums))]
	if g.r.Intn(3) == 0 {
		n = uint64(g.r.Intn(40))
	}
	g.head(6, n)
	if (n == 2 || n == 3) && g.r.Intn(4) != 0 {
		b := g.bytes()
		g.head(2, uint64(len(b)))
		g.b = append(g.b, b...)
		return
	}
	g.item(depth + 1)
}

// chunks writes an indefinite length byte or text string.
func (g *cbGen) chunks(major byte) {
	g.b = append(g.b, major<<5|31)
	for i, n := 0, g.r.Intn(4); i < n; i++ {
		var c []byte
		if major == 2 {
			c = g.bytes()
		} else {
			c = g.text()
		}
		g.head(major, uint64(len(c)))
		g.b = append(g.b, c...)
	}
	g.b = append(g.b, 0xff)
}

func cbMutate(r *rand.Rand, b []byte) []byte {
	out := append([]byte(nil), b...)
	switch r.Intn(7) {
	case 0:
		if len(out) > 0 {
			out[r.Intn(len(out))] ^= byte(1 << r.Intn(8))
		}
	case 1:
		if len(out) > 0 {
			out = out[:r.Intn(len(out))]
		}
	case 2:
		i := r.Intn(len(out) + 1)
		out = append(out[:i], append([]byte{byte(r.Intn(256))}, out[i:]...)...)
	case 3:
		if len(out) > 0 {
			i := r.Intn(len(out))
			out = append(out[:i], out[i+1:]...)
		}
	case 4:
		out = append(out, byte(r.Intn(256)))
	case 5:
		if len(out) > 1 {
			i, j := r.Intn(len(out)), r.Intn(len(out))
			if i > j {
				i, j = j, i
			}
			out = append(out[:j], append(append([]byte(nil), out[i:j]...), out[j:]...)...)
		}
	default:
		if len(out) > 0 {
			out[r.Intn(len(out))] = byte(r.Intn(256))
		}
	}
	return out
}

func cbCases(t testing.TB, r *rand.Rand, n int) ([]cbCase, bool) {
	var inputs [][]byte
	for len(inputs) < n {
		g := &cbGen{r: r}
		g.item(0)
		d := g.b
		if r.Intn(3) == 0 {
			d = cbMutate(r, d)
			if r.Intn(4) == 0 {
				d = cbMutate(r, d)
			}
		}
		inputs = append(inputs, d)
	}
	refs, ok := cbOracle(t, inputs)
	if !ok {
		return nil, false
	}
	var cases []cbCase
	for i := range inputs {
		if strings.HasPrefix(refs[i].Diag, "PANIC") {
			continue
		}
		cases = append(cases, cbCase{inputs[i], refs[i]})
	}
	return cases, true
}

const cbCheckXo = `use std/cbor


/// "" when the case agrees with the oracle, else what differed.
fn check(data: Bytes, wf: Bool, diag: Str, canon: Str, dup: Str) -> Str {
  if cbor.well_formed(data) != wf {
    return "well_formed " + data.to_hex()
  }
  match cbor.decode(data) {
    Ok(v) => {
      if !wf || diag == "UTF8" {
        return "decode accepted " + data.to_hex()
      }
      if diag != "SKIP" && diag != "ERR" && v.diagnostic() != diag {
        return "diagnostic " + data.to_hex() + ": " + v.diagnostic() + " want " + diag
      }
      match cbor.decode(data, reject_dup_keys: true) {
        Ok(_) => {
          if dup == "yes" {
            return "dup accepted " + data.to_hex()
          }
          if canon != "ERR" && canon != "SKIP" {
            match cbor.encode(v, deterministic: true) {
              Ok(e) => {
                if e.to_hex() != canon {
                  return "encode " + data.to_hex() + ": " + e.to_hex() + " want " + canon
                }
              }
              Err(e) => return "encode failed " + data.to_hex() + ": " + e.display()
            }
          }
        }
        Err(_) => {
          if dup == "no" {
            return "dup refused " + data.to_hex()
          }
        }
      }
      // what encode writes decodes back to the same item
      match cbor.encode(v) {
        Ok(e) => {
          match cbor.decode(e) {
            Ok(w) => {
              if w.diagnostic() != v.diagnostic() {
                return "round trip " + data.to_hex()
              }
            }
            Err(x) => return "round trip decode " + data.to_hex() + ": " + x.display()
          }
        }
        Err(x) => return "round trip encode " + data.to_hex() + ": " + x.display()
      }
      ""
    }
    Err(_) => {
      if wf && diag != "ERR" && diag != "UTF8" {
        return "decode refused " + data.to_hex()
      }
      ""
    }
  }
}

`

func cbXoFile(seed int64, cases []cbCase) string {
	var b strings.Builder
	fmt.Fprintf(&b, "// Generated by internal/stdtest/cborfuzz_test.go (seed %d, %d cases) from\n", seed, len(cases))
	b.WriteString("// github.com/fxamacker/cbor/v2 through internal/stdtest/cborref; do not edit:\n// XO_CBORFUZZ_WRITE=1 go test ./internal/stdtest -run TestCborFuzzCorpusCurrent rewrites it.\n")
	b.WriteString(cbCheckXo)
	for i, c := range cases {
		fmt.Fprintf(&b, "test \"cbor fuzz %d\" {\n  let m = check(%s, %v, %s, %s, %s)\n  expect(m) == \"\"\n}\n\n", i, ufBytes(c.data), c.ref.WF, xoStr(c.ref.Diag), xoStr(c.ref.Canon), xoStr(c.ref.Dup))
	}
	return b.String()
}

func cbCoverage(t *testing.T, cases []cbCase) {
	t.Helper()
	wf, utf8, dup := 0, 0, 0
	kinds := map[string]bool{}
	for _, c := range cases {
		if c.ref.WF {
			wf++
		}
		if c.ref.Diag == "UTF8" {
			utf8++
		}
		if c.ref.Dup == "yes" {
			dup++
		}
		if len(c.data) > 0 {
			kinds[fmt.Sprint(c.data[0]>>5)] = true
		}
	}
	t.Logf("answers: %d well formed of %d, %d refused as not UTF-8, %d repeated keys", wf, len(cases), utf8, dup)
	if wf < len(cases)/4 || utf8 == 0 || dup == 0 || len(kinds) < 8 {
		t.Fatalf("the cases stopped covering the decoder")
	}
}

const cbCorpus = "testdata/corpus/cborfuzz/cborfuzz_test.xo"

func TestCborFuzzCorpusCurrent(t *testing.T) {
	cases, ok := cbCases(t, rand.New(rand.NewSource(1)), 1500)
	if !ok {
		t.Skip("the oracle (internal/stdtest/cborref) cannot run here")
	}
	cbCoverage(t, cases)
	text := cbXoFile(1, cases)
	if os.Getenv("XO_CBORFUZZ_WRITE") == "1" {
		if err := os.MkdirAll(filepath.Dir(cbCorpus), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(cbCorpus, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	have, err := os.ReadFile(cbCorpus)
	if err != nil {
		t.Fatal(err)
	}
	if string(have) != text {
		t.Fatalf("%s is not what seed 1 writes (the oracle or the generator changed): XO_CBORFUZZ_WRITE=1 go test ./internal/stdtest -run TestCborFuzzCorpusCurrent", cbCorpus)
	}
}

func TestCborFuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("the cbor fuzz is a heavy test; its seed 1 corpus runs in TestStdCorpora")
	}
	seed := int64(envInt(t, "XO_CBORFUZZ_SEED", 2))
	n := envInt(t, "XO_CBORFUZZ_N", 2000)
	cases, ok := cbCases(t, rand.New(rand.NewSource(seed)), n)
	if !ok {
		t.Skip("the oracle (internal/stdtest/cborref) cannot run here")
	}
	runFuzzDir(t, "cborfuzz_test.xo", cbXoFile(seed, cases), envBackends("XO_CBORFUZZ_BACKENDS", []string{"arm64", "llvm"}), len(cases))
}
