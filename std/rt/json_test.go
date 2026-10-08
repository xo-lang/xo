package rt

import (
	"bytes"
	"encoding/json"
	"math/rand"
	"testing"
)

// encodingJSONString is what JsonW.str wrote before 2026-10-08: an
// encoding/json Encoder with SetEscapeHTML(false).
func encodingJSONString(s string) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.Encode(s)
	return string(bytes.TrimRight(buf.Bytes(), "\n"))
}

// TestAppendJSONString checks that the direct escaper writes exactly what
// encoding/json writes, on fixed cases and on random byte strings
// (control characters, quotes, HTML characters, invalid UTF-8, U+2028).
func TestAppendJSONString(t *testing.T) {
	cases := []string{
		"", "plain", `quote " and \ backslash`, "<a href='x'>&amp;</a>",
		"\b\f\n\r\t\x00\x01\x1f\x7f", "caf\u00e9 \u4e16\u754c \U0001F600",
		"\u2028\u2029", "bad \xff\xfe utf8 \xc3", "\xe2\x80", "x\xed\xa0\x80y",
	}
	r := rand.New(rand.NewSource(1))
	alphabet := []byte("ab\"\\<>&\x00\x1f\x7f\n\t\xc3\xa9\xe2\x80\xa8\xff\xf0\x9f\x98\x80")
	for i := 0; i < 20000; i++ {
		n := r.Intn(12)
		b := make([]byte, n)
		for j := range b {
			if r.Intn(4) == 0 {
				b[j] = byte(r.Intn(256))
			} else {
				b[j] = alphabet[r.Intn(len(alphabet))]
			}
		}
		cases = append(cases, string(b))
	}
	for _, s := range cases {
		got := string(appendJSONString(nil, s))
		if want := encodingJSONString(s); got != want {
			t.Fatalf("appendJSONString(%q) = %s, encoding/json writes %s", s, got, want)
		}
	}
}
