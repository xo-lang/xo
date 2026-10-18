package rt

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// Outside bytes becoming Str (decision 0259), as the native std does it
// (std/native/llrt/scan.xo text_at, url_text_at): valid UTF-8 stays as it
// is; each byte of an invalid sequence becomes U+FFFD (textOf) or, in a
// URL path, host, or query, a percent escape (urlTextOf). Every boundary
// where outside bytes become a Str goes through one of them.

// textOf is s (outside bytes) as valid UTF-8, U+FFFD for each invalid byte.
func textOf(s string) string { return fixText(s, false) }

// urlTextOf is s (a URL part from outside) as valid UTF-8, a percent
// escape for each invalid byte.
func urlTextOf(s string) string { return fixText(s, true) }

func fixText(s string, esc bool) string {
	if utf8.ValidString(s) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 8)
	for i := 0; i < len(s); {
		r, n := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && n == 1 {
			if esc {
				fmt.Fprintf(&b, "%%%02X", s[i])
			} else {
				b.WriteString("�")
			}
		} else {
			b.WriteString(s[i : i+n])
		}
		i += n
	}
	return b.String()
}

// textsOf is textOf of each of xs.
func textsOf(xs []string) []string {
	out := make([]string, len(xs))
	for i, x := range xs {
		out[i] = textOf(x)
	}
	return out
}
