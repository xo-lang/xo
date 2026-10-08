package gobind

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSnake(t *testing.T) {
	cases := map[string]string{
		"FindAllStringSubmatchIndex": "find_all_string_submatch_index",
		"MustCompile":                "must_compile",
		"URL":                        "url",
		"ParseRequestURI":            "parse_request_uri",
		"Sum256":                     "sum256",
		"NewSHA1":                    "new_sha1",
		"EncodeToString":             "encode_to_string",
		"Atoi":                       "atoi",
		"HTTPServer":                 "http_server",
		"NewV7":                      "new_v7",
	}
	for in, want := range cases {
		if got := snake(in); got != want {
			t.Errorf("snake(%q) = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[string]string{"match": "match_", "type": "type_", "go": "go_", "map": "map", "self": "self_"} {
		if got := xoName(in); got != want {
			t.Errorf("xoName(%q) = %q, want %q", in, got, want)
		}
	}
	if got := upperSnake("BlockSize"); got != "BLOCK_SIZE" {
		t.Errorf("upperSnake = %q", got)
	}
}

func TestPins(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "xo.mod")
	os.WriteFile(p, []byte("module ex\n\nrequire go github.com/google/uuid v1.6.0\nrequire go example.com/a v0.1.0\n"), 0o644)
	pins := ReadPins(p)
	if v := PinFor(pins, "github.com/google/uuid"); v != "v1.6.0" {
		t.Errorf("pin = %q", v)
	}
	if v := PinFor(pins, "example.com/a/sub"); v != "v0.1.0" {
		t.Errorf("sub package pin = %q", v)
	}
	if v := PinFor(pins, "example.com/ab"); v != "" {
		t.Errorf("prefix without a slash matched: %q", v)
	}
}

// bindDecl binds a std package into a fresh cache and returns the
// declaration file.
func bindDecl(t *testing.T, path string) (string, *Binding, string) {
	t.Helper()
	dir := t.TempDir()
	b, err := Bind(path, Options{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	return string(b.Decl), b, dir
}

func TestMapping(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go list")
	}
	decl, b, dir := bindDecl(t, "os/exec")
	// a leading context.Context is supplied from the task (CommandContext is
	// not New* or Must*, so its pointer result is an Option)
	if !strings.Contains(decl, "fn command_context(name: Str, arg: List[Str]) -> Cmd? uses go") {
		t.Errorf("command_context:\n%s", grep(decl, "command_context"))
	}
	if !strings.Contains(string(b.Adapter), "GoContext(tk)") {
		t.Error("adapter does not pass GoContext(tk)")
	}
	// fields are getters and setters; a nilable pointer field is an Option
	if !strings.Contains(decl, "fn (recv: Cmd) process() -> Process? uses go") {
		t.Errorf("process field:\n%s", grep(decl, "process"))
	}
	// a cached binding loads; another binder version or pin does not
	l, err := Load("os/exec", Options{Dir: dir})
	if err != nil || l == nil || l.Name != "exec" || !l.Standard || len(l.Skipped) == 0 {
		t.Fatalf("Load = %+v, %v", l, err)
	}
	dp := declPath(dir, "os/exec")
	os.WriteFile(dp, []byte(strings.Replace(decl, "xo-bind: v2 ", "xo-bind: v1 ", 1)), 0o644)
	if l, _ := Load("os/exec", Options{Dir: dir}); l != nil {
		t.Error("stale binding loaded")
	}

	decl, sb, _ := bindDecl(t, "strings")
	// a callback never runs with the request's arena (decision 0066)
	if !strings.Contains(string(sb.Adapter), "}(GoCallbackTask(tk))") {
		t.Error("callback adapter does not use GoCallbackTask(tk)")
	}
	for _, want := range []string{
		"fn cut(s: Str, sep: Str) -> (Str, Str, Bool) uses go",
		"fn map[fx: Effects](mapping: fn(Rune) -> Rune uses fx, s: Str) -> Str uses go, fx",
		"fn lines(s: Str) -> List[Str] uses go",
		"fn new_replacer(oldnew: List[Str]) -> Replacer uses go",
		"fn (static: Builder) new() -> Builder uses go",
	} {
		if !strings.Contains(decl, want) {
			t.Errorf("strings binding lacks %q", want)
		}
	}
	decl, _, _ = bindDecl(t, "net/url")
	for _, want := range []string{
		"fn parse(raw_url: Str) -> URL ! go.Error uses go",
		"fn (recv: URL) resolve_reference(ref: URL) -> URL? uses go",
		"struct UrlError {}",
		"fn (recv: Values) get(key: Str) -> Str uses go",
	} {
		if !strings.Contains(decl, want) {
			t.Errorf("net/url binding lacks %q", want)
		}
	}
	decl, _, _ = bindDecl(t, "crypto/sha256")
	if !strings.Contains(decl, "fn sum256(data: Bytes) -> Bytes uses go") || !strings.Contains(decl, "const SIZE: Int = 32") {
		t.Errorf("sha256 binding:\n%s", decl)
	}
	decl, _, _ = bindDecl(t, "regexp")
	if !strings.Contains(decl, "fn match_(pattern: Str, b: Bytes) -> Bool ! go.Error uses go") {
		t.Errorf("keyword name not escaped:\n%s", grep(decl, "match"))
	}
	if _, err := Bind("no/such/pkg", Options{Dir: t.TempDir()}); err == nil {
		t.Error("binding a missing package succeeded")
	}
}

func grep(s, sub string) string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if strings.Contains(l, sub) {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}
