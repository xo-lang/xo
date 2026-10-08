package check

import (
	"path/filepath"
	"testing"
)

// Go interop (decision 0041, prototype 2026-10-08): `use go` imports,
// binding failures (XO1101), names the binding skipped (XO1102), and the
// `go` effect of every call into Go (XO0701 when a pub function does not
// declare it).
func TestGoInterop(t *testing.T) {
	t.Setenv("XO_GOBIND_DIR", filepath.Join(t.TempDir(), "gobind"))
	runCases(t, []ruleCase{
		{name: "bound call", src: "use go \"strings\"\n\nfn f(s: Str) -> Str {\n  strings.to_upper(s)\n}\n"},
		{name: "alias", src: "use go \"strings\" as gs\n\nfn f(s: Str) -> Bool {\n  gs.has_prefix(s, \"a\")\n}\n"},
		{name: "error result with ?", src: "use go \"strconv\"\n\nfn f(s: Str) -> Int ! Error {\n  strconv.atoi(s)?\n}\n"},
		{name: "go.Error named with std/go", src: "use go \"strconv\"\nuse std/go\n\nfn f(s: Str) -> Int ! go.Error {\n  strconv.atoi(s)\n}\n"},
		{name: "unhandled go error", src: "use go \"strconv\"\n\nfn f(s: Str) {\n  strconv.atoi(s)\n}\n", want: []string{"XO0201"}},
		{name: "handle methods", src: "use go \"regexp\"\n\nfn f(s: Str) -> Bool {\n  regexp.must_compile(\"a+\").match_string(s)\n}\n"},
		{name: "pub function declares go", src: "use go \"strings\"\n\npub fn f(s: Str) -> Str uses go {\n  strings.to_upper(s)\n}\n"},
		{name: "pub function without uses go", src: "use go \"strings\"\n\npub fn f(s: Str) -> Str {\n  strings.to_upper(s)\n}\n", want: []string{"XO0701"}},
		{name: "contract must be pure", src: "use go \"strings\"\n\nfn f(s: Str) -> Str\n  requires strings.has_prefix(s, \"a\")\n{\n  s\n}\n", want: []string{"XO0701"}},
		{name: "package that does not exist", src: "use go \"example.invalid/nope\"\n\nfn f() {}\n", want: []string{"XO1101"}},
		{name: "std package that does not exist", src: "use go \"nosuchpkg/zzz\"\n\nfn f() {}\n", want: []string{"XO1101"}},
		{name: "skipped function", src: "use go \"strconv\"\n\nfn f() {\n  strconv.parse_complex(\"1\", 128)\n}\n", want: []string{"XO1102"}},
		{name: "skipped method", src: "use go \"net/url\"\n\nfn f(e: url.UrlError) {\n  e.err()\n}\n", want: []string{"XO1102"}},
		{name: "unknown name is still XO0601", src: "use go \"strings\"\n\nfn f() {\n  strings.no_such_fn()\n}\n", want: []string{"XO0601"}},
		{name: "unused go import", src: "use go \"strings\"\n\nfn f() {}\n", want: []string{"XO0110"}},
	})
}
