package rt

import (
	"net/url"
	"strings"
	"unicode/utf8"
)

// Url_escape is std/url's escape on the Go backend (decision 0597 part 2:
// the native backends run std/native/url_native.xo's): Go's escape with
// encodePathSegment or encodeQueryComponent. Every other std/url function
// is generated into std_gen_url.go from std/url.xo (std/rt_hosted.txt).
func Url_escape(_ *Task, s string, path bool) string {
	if path {
		return url.PathEscape(s)
	}
	return url.QueryEscape(s)
}

// urlText is s with each byte that is not UTF-8 kept as its %XX escape
// (decision 0259), as std/url's own decoding keeps it; std/rt's http uses
// it on text Go's net/url decoded.
func urlText(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	const hex = "0123456789ABCDEF"
	var b strings.Builder
	for i := 0; i < len(s); {
		r, n := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && n == 1 {
			b.WriteByte('%')
			b.WriteByte(hex[s[i]>>4])
			b.WriteByte(hex[s[i]&15])
		} else {
			b.WriteString(s[i : i+n])
		}
		i += n
	}
	return b.String()
}
