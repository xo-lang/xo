package llvm_test

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/build"
)

// The native JSON parser (std/llrt/xo_json.c, decision 0074) against
// std/rt's jsonParse, which the Go backend's json.decode uses: the same
// values (keys at their first position with their last value) or the
// same error text, on a corpus and on random mutations of valid texts.

// goJSONObj is std/rt's jsonObj.
type goJSONObj struct {
	keys []string
	vals map[string]any
}

// goJSONParse is std/rt's jsonParse (std/rt/json.go), returning the
// error text after "invalid JSON: ".
func goJSONParse(data []byte) (any, string) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	x, err := goJSONValue(dec)
	if err != nil {
		return nil, strings.TrimPrefix(err.Error(), "json: ")
	}
	if _, err := dec.Token(); err == nil {
		return nil, "trailing data"
	}
	return x, ""
}

func goJSONValue(dec *json.Decoder) (any, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch t := tok.(type) {
	case json.Delim:
		switch t {
		case '{':
			obj := &goJSONObj{vals: map[string]any{}}
			for dec.More() {
				kt, err := dec.Token()
				if err != nil {
					return nil, err
				}
				k, _ := kt.(string)
				v, err := goJSONValue(dec)
				if err != nil {
					return nil, err
				}
				if _, dup := obj.vals[k]; !dup {
					obj.keys = append(obj.keys, k)
				}
				obj.vals[k] = v
			}
			if _, err := dec.Token(); err != nil {
				return nil, err
			}
			return obj, nil
		case '[':
			arr := []any{}
			for dec.More() {
				v, err := goJSONValue(dec)
				if err != nil {
					return nil, err
				}
				arr = append(arr, v)
			}
			if _, err := dec.Token(); err != nil {
				return nil, err
			}
			return arr, nil
		}
		return nil, fmt.Errorf("unexpected %v", t)
	}
	return tok, nil
}

// goJSONDump is the dump format of json_diff.c.
func goJSONDump(b *strings.Builder, x any) {
	switch v := x.(type) {
	case nil:
		b.WriteString("null")
	case bool:
		fmt.Fprint(b, v)
	case json.Number:
		b.WriteString(string(v))
	case string:
		b.WriteString("s" + hex.EncodeToString([]byte(v)))
	case []any:
		b.WriteByte('[')
		for i, e := range v {
			if i > 0 {
				b.WriteByte(',')
			}
			goJSONDump(b, e)
		}
		b.WriteByte(']')
	case *goJSONObj:
		b.WriteByte('{')
		for i, k := range v.keys {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(hex.EncodeToString([]byte(k)) + ":")
			goJSONDump(b, v.vals[k])
		}
		b.WriteByte('}')
	}
}

func goJSONResult(data []byte) string {
	x, e := goJSONParse(data)
	if e != "" {
		return "E " + e
	}
	var b strings.Builder
	goJSONDump(&b, x)
	return "O " + b.String()
}

// jsonCorpus are inputs worth checking by hand.
var jsonCorpus = []string{
	``, ` `, `null`, `true`, `false`, `0`, `-0`, `1.5e10`, `-1.5E-10`, `"a"`, `""`, `[]`, `{}`, `[1,2,3]`,
	`{"a":1,"b":[true,false,null],"c":{"d":"e"}}`, `{"a":1,"a":2}`, `{"a":1,"b":2,"a":3}`, `{"a":[1],"a":"x"}`,
	`{"a":1,}`, `[1,]`, `[,1]`, `{,}`, `{"a" 1}`, `{"a":1 "b":2}`, `[1 2]`, `{1:2}`, `{[`, `{]`, `[}`, `]`, `}`,
	`:`, `,`, `"abc`, `"a\qb"`, `"\u12"`, `"\u12g4"`, `"\ud83d\ude00"`, `"\ud83d"`, `"\ude00"`, `"\ud83dx"`,
	`"\ud83d\u0041"`, "\"\xff\"", "\"a\x01b\"", "\"\x7f\"", `-`, `-a`, `01`, `[01]`, `1.`, `1.x`, `1e`, `1e+`,
	`1ex`, `tru`, `trux`, `nul`, `nulx`, `fals`, `falsy`, `t`, `123abc`, `[123abc]`, `1 2`, `1 2x`, `{} {}`,
	`{}x`, `{} ]`, "1\n", "[\n1\n]\n", `[[[[[[]]]]]]`, `{"a":{"b":{"c":[{"d":1}]}}}`, "\xef\xbb\xbf{}", "\x00",
	`"\/\b\f\n\r\t\\\""`, `{"\u0061":1,"a":2}`, `{"a`, `{"a"`, `{"a":`, `{"a":1`, `[`, `[1`, `[1,`,
	"\xc3\xa9", "[\xa0]", "[\xad]", "[\x85]", "'a'", `"a" "b"`, `{"":1}`, `1e999`, `-1e-999`, `123456789012345678901234567890`,
}

// randJSON writes a random JSON value.
func randJSON(r *rand.Rand, b *strings.Builder, depth int) {
	k := r.Intn(9)
	if depth > 4 && k >= 6 {
		k = r.Intn(6)
	}
	switch k {
	case 0:
		b.WriteString("null")
	case 1:
		b.WriteString("true")
	case 2:
		b.WriteString("false")
	case 3:
		nums := []string{"0", "-1", "12", "3.25", "-0.5e3", "1E-2", "9223372036854775807", "9223372036854775808", "1e400"}
		b.WriteString(nums[r.Intn(len(nums))])
	case 4, 5:
		strs := []string{`"x"`, `""`, `"a b"`, `"\n\t"`, `"\u00e9"`, `"é"`, `"\ud83d\ude00"`, `"\\"`, `"q\"q"`}
		b.WriteString(strs[r.Intn(len(strs))])
	case 6, 7:
		b.WriteByte('[')
		n := r.Intn(4)
		for i := 0; i < n; i++ {
			if i > 0 {
				b.WriteString([]string{",", ", ", " ,"}[r.Intn(3)])
			}
			randJSON(r, b, depth+1)
		}
		b.WriteByte(']')
	case 8:
		b.WriteByte('{')
		n := r.Intn(4)
		for i := 0; i < n; i++ {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString([]string{`"a"`, `"b"`, `"c"`, `"\u0061"`}[r.Intn(4)])
			b.WriteString([]string{":", " : "}[r.Intn(2)])
			randJSON(r, b, depth+1)
		}
		b.WriteByte('}')
	}
}

// mutate changes a few bytes of s.
func mutate(r *rand.Rand, s []byte) []byte {
	alphabet := []byte("{}[]:,\"\\ -+.eE0123456789tfnulrsax\n\x00\x7f\xff\xc3")
	n := 1 + r.Intn(3)
	for i := 0; i < n; i++ {
		switch r.Intn(4) {
		case 0:
			if len(s) > 0 {
				j := r.Intn(len(s))
				s = append(s[:j:j], s[j+1:]...)
			}
		case 1:
			j := r.Intn(len(s) + 1)
			c := alphabet[r.Intn(len(alphabet))]
			s = append(s[:j:j], append([]byte{c}, s[j:]...)...)
		case 2:
			if len(s) > 0 {
				s[r.Intn(len(s))] = alphabet[r.Intn(len(alphabet))]
			}
		case 3:
			if len(s) > 0 {
				s = s[:r.Intn(len(s))]
			}
		}
	}
	return s
}

// TestJSONParseDiff runs json_diff.c (with AddressSanitizer when clang
// has it) on the corpus and on random valid and mutated texts.
func TestJSONParseDiff(t *testing.T) {
	t.Parallel() // builds nothing in the shared build directory
	if _, err := build.Clang(); err != nil {
		t.Skip("clang not found")
	}
	src := filepath.Join(rtDir, "testdata", "json_diff.c")
	bin := filepath.Join(t.TempDir(), "json_diff")
	if err := build.CompileC(asanFlags("-O1", "-g", "-fsanitize=address,undefined"), []string{src}, bin); err != nil {
		if err2 := build.CompileC([]string{"-O1", "-g"}, []string{src}, bin); err2 != nil {
			t.Fatal(err2)
		}
	}
	inputs := append([]string(nil), jsonCorpus...)
	r := rand.New(rand.NewSource(1))
	n := 20000
	if testing.Short() {
		n = 2000
	}
	for i := 0; i < n; i++ {
		var b strings.Builder
		randJSON(r, &b, 0)
		s := b.String()
		if i%3 != 0 {
			s = string(mutate(r, []byte(s)))
		}
		inputs = append(inputs, s)
	}
	var in bytes.Buffer
	for _, s := range inputs {
		var lb [8]byte
		binary.LittleEndian.PutUint64(lb[:], uint64(len(s)))
		in.Write(lb[:])
		in.WriteString(s)
	}
	cmd := exec.Command(bin)
	cmd.Stdin = &in
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("json_diff: %v\n%s", err, stderr.String())
	}
	sc := bufio.NewScanner(&out)
	sc.Buffer(make([]byte, 1<<20), 1<<26)
	i, bad := 0, 0
	for sc.Scan() {
		if i >= len(inputs) {
			t.Fatalf("more output lines than inputs")
		}
		want := goJSONResult([]byte(inputs[i]))
		if got := sc.Text(); got != want && bad < 20 {
			bad++
			t.Errorf("input %q:\n got %s\nwant %s", inputs[i], got, want)
		}
		i++
	}
	if i != len(inputs) {
		t.Fatalf("%d output lines for %d inputs\n%s", i, len(inputs), stderr.String())
	}
}
