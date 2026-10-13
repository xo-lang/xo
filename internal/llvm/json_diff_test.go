package llvm_test

import (
	"math/rand"
	"strings"
)

// The inputs of TestJSONTextDiff (jsontext_test.go): a corpus and random
// valid and mutated texts. TestJSONParseDiff, which ran them through the
// C parser (std/llrt/xo_json.c and testdata/json_diff.c), went with the
// C path (decision 0198).

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
