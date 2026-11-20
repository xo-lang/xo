package rt

import "testing"

// TestPatternError checks the Go backend's Router refuses the patterns Go's
// ServeMux refuses, in std/native/http.xo's words (task #238, b36; the
// cases of std/native/http_test.xo "the router refuses invalid patterns").
func TestPatternError(t *testing.T) {
	cases := []struct{ pattern, want string }{
		{"", "empty pattern"},
		{"/{r...}/admin", "{...} wildcard not at end"},
		{"/{a}/{a}", `duplicate wildcard name "a"`},
		{"/{a}/{a:Int}", `duplicate wildcard name "a"`},
		{"example.com/a", "host patterns are not supported (std.md 4.1)"},
		{"a", "host/path missing /"},
		{"G(T /a", `invalid method "G(T"`},
		{"GET /a/../b", "non-CONNECT pattern with unclean path can never match"},
		{"/x{a}", "bad wildcard segment (must start with '{')"},
		{"/{a", "bad wildcard segment (must end with '}')"},
		{"/{}", "empty wildcard"},
		{"/{...}", "empty wildcard"},
		{"/{1a}", `bad wildcard name "1a"`},
		{"/{$}/a", "{$} not at end"},
		// the forms Go and std.md 4.1 take
		{"GET\t/a", ""},
		{"GET   /a/{id:Int}/{rest...}", ""},
		{"/a/{$}", ""},
		{"/a/../b", ""},
		{"/{é_1}", ""},
	}
	for _, c := range cases {
		if got := patternError(c.pattern); got != c.want {
			t.Errorf("patternError(%q) = %q, want %q", c.pattern, got, c.want)
		}
	}
	if m, p := patternMethod("GET\t /t"); m != "GET" || p != "/t" {
		t.Errorf(`patternMethod("GET\t /t") = %q, %q`, m, p)
	}
}

// TestConflictError checks the Go backend's Router refuses conflicting
// routes with Go's ServeMux messages and takes the ones Go takes (task
// #238; std/native/http_test.xo has the same cases).
func TestConflictError(t *testing.T) {
	cases := []struct {
		old, new string
		want     string
	}{
		{"/a", "/a", "http: pattern \"/a\" conflicts with pattern \"/a\":\n/a matches the same requests as /a"},
		{"/{x}", "/{y}", "http: pattern \"/{y}\" conflicts with pattern \"/{x}\":\n/{y} matches the same requests as /{x}"},
		{"/a/{x}", "/{y}/b", "http: pattern \"/{y}/b\" conflicts with pattern \"/a/{x}\":\n/{y}/b and /a/{x} both match some paths, like \"/a/b\".\nBut neither is more specific than the other.\n/{y}/b matches \"/y/b\", but /a/{x} doesn't.\n/a/{x} matches \"/a/x\", but /{y}/b doesn't."},
		{"GET /a/{x}", "/a/b", "http: pattern \"/a/b\" conflicts with pattern \"GET /a/{x}\":\n/a/b matches more methods than GET /a/{x}, but has a more specific path pattern"},
		{"/a", "GET /a", ""},
		{"/a/{x}", "/a/b", ""},
		{"GET /a", "POST /a", ""},
		{"/{x...}", "/a/{y...}", ""},
		{"GET /", "/any/{v}", "http: pattern \"/any/{v}\" conflicts with pattern \"GET /\":\n/any/{v} matches more methods than GET /, but has a more specific path pattern"},
		{"/a/b", "GET /a/{x}", "http: pattern \"GET /a/{x}\" conflicts with pattern \"/a/b\":\nGET /a/{x} matches fewer methods than /a/b, but has a more general path pattern"},
		{"/files/{p...}", "/files/", "http: pattern \"/files/\" conflicts with pattern \"/files/{p...}\":\n/files/ matches the same requests as /files/{p...}"},
		{"GET /{x}", "POST /{y}", ""},
		{"GET /users/{id}", "GET /users/me", ""},
	}
	for _, c := range cases {
		m, segs := parsePattern(c.old)
		old := &route{method: m, segs: segs}
		m, segs = parsePattern(c.new)
		nr := &route{method: m, segs: segs}
		if got := conflictError([]*route{old}, nr); got != c.want {
			t.Errorf("%q then %q:\n got %q\nwant %q", c.old, c.new, got, c.want)
		}
	}
}
