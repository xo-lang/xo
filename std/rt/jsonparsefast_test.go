package rt

import (
	"math/rand"
	"reflect"
	"strings"
	"testing"
)

// TestJsonParseFastMatchesDecoder: wherever the fast reader answers, it
// answers what the token decoder answers; on mutated and random text it
// either gives up or agrees.
func TestJsonParseFastMatchesDecoder(t *testing.T) {
	r := rand.New(rand.NewSource(7))
	seeds := []string{
		`{"id":12345,"name":"hello world","tags":["a","bb","ccc"],"score":3.25,"nested":{"a":7,"b":true},"note":null}`,
		`[1,-0,0.5,1e3,-2E+4,"x\n\u00e9\"\\\/\b\f\r\t","\ud83d\ude00",true,false,null,{},[]]`,
		`{"a":1,"a":2,"b":{"c":[{},[],[[]]]}}`,
		` "plain" `, `12`, `-0.0`, `true`, `null`, `{"é":"日本語"}`, "{\"k\":\"a\u2028b\"}",
	}
	alpha := []byte(`{}[]",:\ 0123456789-+.eEtrufalsn` + "\n\tabcxyz\u00e9")
	n, agreed := 0, 0
	check := func(s string) {
		fast, ok := jsonParseFast([]byte(s))
		if !ok {
			return
		}
		slow, err := jsonParseSlow([]byte(s))
		if err != nil {
			t.Fatalf("fast accepted what the decoder rejects (%v): %q", err.F_msg, s)
		}
		if !reflect.DeepEqual(fast, slow) {
			t.Fatalf("fast and decoder differ on %q:\n fast %#v\n slow %#v", s, fast, slow)
		}
		agreed++
	}
	for _, s := range seeds {
		check(s)
		for k := 0; k < 4000; k++ {
			b := []byte(s)
			for m := r.Intn(3); m >= 0; m-- {
				switch r.Intn(4) {
				case 0:
					if len(b) > 0 {
						b = append(b[:r.Intn(len(b))], b[r.Intn(len(b)):]...)
					}
				case 1:
					i := r.Intn(len(b) + 1)
					b = append(b[:i], append([]byte{alpha[r.Intn(len(alpha))]}, b[i:]...)...)
				case 2:
					if len(b) > 0 {
						b[r.Intn(len(b))] = alpha[r.Intn(len(alpha))]
					}
				case 3:
					b = b[:r.Intn(len(b)+1)]
				}
			}
			check(string(b))
			n++
		}
	}
	for k := 0; k < 20000; k++ {
		var sb strings.Builder
		for l := r.Intn(12); l > 0; l-- {
			sb.WriteByte(alpha[r.Intn(len(alpha))])
		}
		check(sb.String())
	}
	if agreed < 5000 {
		t.Fatalf("only %d of the mutated texts were read by the fast reader", agreed)
	}
}

// TestJsonParseFastSurrogatePairs: a \u escaped surrogate pair is read by
// the fast reader (one rune, as the decoder reads it), so a document with
// one emoji escape does not go through the token decoder; a lone or
// reversed surrogate is still left to the decoder (U+FFFD there).
func TestJsonParseFastSurrogatePairs(t *testing.T) {
	for _, s := range []string{`"😀"`, `["x 😀 y","𝄞"]`, `{"k":"😀😀"}`} {
		fast, ok := jsonParseFast([]byte(s))
		if !ok {
			t.Fatalf("the fast reader gave up on %s", s)
		}
		slow, err := jsonParseSlow([]byte(s))
		if err != nil || !reflect.DeepEqual(fast, slow) {
			t.Fatalf("%s: fast %#v, decoder %#v (%v)", s, fast, slow, err)
		}
	}
	for _, s := range []string{`"\ud83d"`, `"\ude00\ud83d"`, `"\ud83d x"`, `"\ud83dA"`, `"\ud83d\ud83d"`, `"\ud83d\ude0"`} {
		if _, ok := jsonParseFast([]byte(s)); ok {
			t.Errorf("the fast reader accepted %s (the decoder's U+FFFD case)", s)
		}
	}
}

// TestJsonSyntaxErrMatchesDecoder: where the scanner's check answers, it
// answers what the token decoder answers, on mutated and random text.
func TestJsonSyntaxErrMatchesDecoder(t *testing.T) {
	r := rand.New(rand.NewSource(11))
	seeds := []string{
		`{"id":12345,"name":"hello world","tags":["a","bb","ccc"],"score":3.25,"nested":{"a":7,"b":true},"note":null}`,
		`[1,-0,0.5,1e3,-2E+4,"x\n\u00e9\"\\\/\b\f\r\t","\ud83d\ude00",true,false,null,{},[]]`,
		`{"a":1,"a":2,"b":{"c":[{},[],[[]]]}}`, `tru`, `[1, 2, 3e]`, `{"a" "b"}`, "\"a\x01\"",
	}
	alpha := []byte(`{}[]",:\ 0123456789-+.eEtrufalsn` + "\n\tabcxyz\u00e9\x01")
	answered := 0
	check := func(s string) {
		got := jsonSyntaxErr([]byte(s))
		if got == nil {
			return
		}
		_, want := jsonParseSlow([]byte(s))
		if want == nil || want.F_msg != got.F_msg {
			t.Fatalf("scanner and decoder differ on %q:\n scanner %v\n decoder %v", s, got, want)
		}
		answered++
	}
	for _, s := range seeds {
		check(s)
		for k := 0; k < 4000; k++ {
			b := []byte(s)
			for m := r.Intn(3); m >= 0; m-- {
				switch r.Intn(4) {
				case 0:
					if len(b) > 0 {
						b = append(b[:r.Intn(len(b))], b[r.Intn(len(b)):]...)
					}
				case 1:
					i := r.Intn(len(b) + 1)
					b = append(b[:i], append([]byte{alpha[r.Intn(len(alpha))]}, b[i:]...)...)
				case 2:
					if len(b) > 0 {
						b[r.Intn(len(b))] = alpha[r.Intn(len(alpha))]
					}
				case 3:
					b = b[:r.Intn(len(b)+1)]
				}
			}
			check(string(b))
		}
	}
	for k := 0; k < 50000; k++ {
		var sb strings.Builder
		for l := r.Intn(12); l > 0; l-- {
			sb.WriteByte(alpha[r.Intn(len(alpha))])
		}
		check(sb.String())
	}
	if answered < 5000 {
		t.Fatalf("the scanner answered only %d texts", answered)
	}
}

// TestJsonParseFastTrailingJunk: a byte after the value that starts no
// token leaves the value as the decoder reads it; one that starts a token
// is left to the decoder.
func TestJsonParseFastTrailingJunk(t *testing.T) {
	for _, tail := range []string{"x", " q", "\n)", "\x01", "é", "+", ".", "T", "}", "1", "[", "\"", "-", "t", ":", ","} {
		s := `{"a":[1,2],"b":"c"}` + tail
		slow, err := jsonParseSlow([]byte(s))
		fast, ok := jsonParseFast([]byte(s))
		if ok && (err != nil || !reflect.DeepEqual(fast, slow)) {
			t.Errorf("%q: fast %#v, decoder %#v (%v)", s, fast, slow, err)
		}
		if !ok && strings.ContainsAny(tail[len(tail)-1:], "xq)\x01+.T") {
			t.Errorf("%q: the fast reader gave up", s)
		}
	}
}
