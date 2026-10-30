package gobind

import (
	"go/types"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/gotool"
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
	if m, v := PinFor(pins, "github.com/google/uuid"); m != "github.com/google/uuid" || v != "v1.6.0" {
		t.Errorf("pin = %q %q", m, v)
	}
	if m, v := PinFor(pins, "example.com/a/sub"); m != "example.com/a" || v != "v0.1.0" {
		t.Errorf("sub package pin = %q %q", m, v)
	}
	if m, v := PinFor(pins, "example.com/ab"); m != "" || v != "" {
		t.Errorf("prefix without a slash matched: %q %q", m, v)
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
	if !strings.Contains(decl, "fn command_context(name: Str, arg: List[Str]) -> ExecCmd? uses go") {
		t.Errorf("command_context:\n%s", grep(decl, "command_context"))
	}
	if !strings.Contains(string(b.Adapter), "GoContext(tk)") {
		t.Error("adapter does not pass GoContext(tk)")
	}
	// fields are getters and setters; a nilable pointer field is an Option
	// os.Process is os's binding's type, not a copy (decision 0392)
	if !strings.Contains(decl, "fn (recv: ExecCmd) process() -> os.Process? uses go") {
		t.Errorf("process field:\n%s", grep(decl, "process"))
	}
	// a cached binding loads; another binder version or pin does not
	l, err := Load("os/exec", Options{Dir: dir})
	if err != nil || l == nil || l.Name != "exec" || !l.Standard {
		t.Fatalf("Load = %+v, %v", l, err)
	}
	// the skipped names read the same as Bind made them (XO1102's text)
	if !reflect.DeepEqual(l.Skipped, b.Skipped) {
		t.Errorf("Load Skipped = %v, Bind Skipped = %v", l.Skipped, b.Skipped)
	}
	dp := declPath(dir, "os/exec", nil)
	os.WriteFile(dp, []byte(strings.Replace(decl, "xo-bind: v"+strconv.Itoa(Version)+" ", "xo-bind: v1 ", 1)), 0o644)
	if l, _ := Load("os/exec", Options{Dir: dir}); l != nil {
		t.Error("stale binding loaded")
	}

	decl, sb, _ := bindDecl(t, "strings")
	// a callback never runs with the request's arena (decision 0066), and
	// a call on another goroutine is a task of its own (decision 0067)
	for _, want := range []string{"}(NewGoCallback(tk))", "ctk := cb.Begin(func() []any { return []any{c0} })", "defer cb.End(ctk)", "\tGoEnter(tk)\n\tdefer GoExit(tk, "} {
		if !strings.Contains(string(sb.Adapter), want) {
			t.Errorf("adapter lacks %q", want)
		}
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

// TestWorkspaceGoLine: the binding workspace's go.mod takes its `go`
// line from the installed go command (a fake one here), never above
// gotool.TestedGo, so the go command is not asked for a newer Go than
// itself (decision 0176).
func TestWorkspaceGoLine(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake go command is a shell script")
	}
	t.Setenv("XO_GO_LINE", "")
	for version, want := range map[string]string{"go1.25.3": "1.25", "go1.99.1": gotool.TestedGo, "go" + gotool.TestedGo + ".0": gotool.TestedGo} {
		bin := t.TempDir()
		fake := filepath.Join(bin, "go")
		os.WriteFile(fake, []byte("#!/bin/sh\nif [ \"$1\" = env ] && [ \"$2\" = GOVERSION ]; then echo "+version+"; exit 0; fi\nexit 2\n"), 0o755)
		t.Setenv("XO_GO", fake)
		dir := t.TempDir()
		if err := ensureWorkspace(dir); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(filepath.Join(workDir(dir), "go.mod"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), "\ngo "+want+"\n") {
			t.Errorf("%s: go.mod\n%s\nwant go %s", version, data, want)
		}
	}
}

// TestSharedHandles: a type of another package is that package's
// binding's (decision 0392; Intel tester 0.33.0, b14: net declared its
// own AddrPort, which netip.AddrPort could not be passed as).
func TestSharedHandles(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go list")
	}
	decl, b, dir := bindDecl(t, "net")
	if !strings.Contains(decl, "\nuse go \"net/netip\" as netip\n") {
		t.Errorf("no use go of net/netip:\n%s", grep(decl, "use "))
	}
	if !strings.Contains(decl, "fn tcp_addr_from_addr_port(addr: netip.AddrPort) -> ") {
		t.Errorf("tcp_addr_from_addr_port:\n%s", grep(decl, "addr_port"))
	}
	if strings.Contains(decl, "struct AddrPort {}") {
		t.Error("net declares its own AddrPort")
	}
	if !strings.Contains(string(b.Adapter), "Go_net_netip_AddrPort") {
		t.Error("the adapter does not use netip's rt type")
	}
	// netip is bound in the same run, with the handle net names
	home, err := os.ReadFile(declPath(dir, "net/netip", nil))
	if err != nil {
		t.Fatalf("net/netip not bound with net: %v", err)
	}
	if !strings.Contains(string(home), "struct AddrPort {}") {
		t.Errorf("netip's binding has no AddrPort:\n%s", grep(string(home), "struct "))
	}
	// a type of an internal package stays skipped, with the reason
	if !importable("net/netip") || importable("internal/poll") || importable("vendor/golang.org/x/net/dns/dnsmessage") || importable("example.com/m/internal/x") {
		t.Error("importable")
	}
}

// panicImporter panics as go1.26's importer does on Go 1.27's export data.
type panicImporter struct{}

func (panicImporter) Import(string) (*types.Package, error) {
	panic(`cannot decode "strings", export data version 4 is greater than maximum supported version 2`)
}

// TestImportNewerExportData: export data from a newer Go is an Error that
// names this helper's Go and the installed one, not a panic (the 0.33.0
// testers' bug 14: every bind panicked with Go 1.27 installed).
func TestImportNewerExportData(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake go command is a shell script")
	}
	bin := t.TempDir()
	fake := filepath.Join(bin, "go")
	os.WriteFile(fake, []byte("#!/bin/sh\nif [ \"$1\" = env ] && [ \"$2\" = GOVERSION ]; then echo go1.99.1; exit 0; fi\nexit 2\n"), 0o755)
	t.Setenv("XO_GO", fake)
	pkg, err := importPkg(panicImporter{}, "strings")
	if pkg != nil || err == nil {
		t.Fatalf("importPkg = %v, %v; want an error", pkg, err)
	}
	msg := err.Error()
	for _, want := range []string{runtime.Version(), "go1.99.1", "export data version 4", "XO_GOBIND"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q does not name %q", msg, want)
		}
	}
}

// TestOutputBuffers: a []byte parameter Go writes into is skipped (the
// Windows tester's bug 18, 2026-10-29: strings.Reader.read(p) returned n
// but left the program's Bytes as it was): io.Reader's Read(p), and a
// parameter named dst, unless the function returns the bytes (the append
// form). A []byte Go only reads (Write(p)) stays bound.
func TestOutputBuffers(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go list")
	}
	decl, b, _ := bindDecl(t, "strings")
	if strings.Contains(decl, ") read(p: Bytes)") {
		t.Errorf("strings.Reader.read is bound:\n%s", grep(decl, ") read("))
	}
	found := false
	for name, why := range b.Skipped {
		if strings.HasSuffix(name, "read") && strings.Contains(why, "Go writes into") {
			found = true
		}
	}
	if !found {
		t.Errorf("no skipped read with the output buffer reason: %v", b.Skipped)
	}
	if !strings.Contains(decl, ") write(p: Bytes)") {
		t.Errorf("strings.Builder.write is not bound:\n%s", grep(decl, ") write("))
	}
	hex, _, _ := bindDecl(t, "encoding/hex")
	if strings.Contains(hex, "fn decode(dst: Bytes") {
		t.Errorf("hex.decode(dst, src) is bound:\n%s", grep(hex, "fn decode("))
	}
	if !strings.Contains(hex, "fn append_encode(dst: Bytes") {
		t.Errorf("hex.append_encode (the append form) is not bound:\n%s", grep(hex, "append_encode"))
	}
}

// TestUnexportedParamTypes: an exported function or method whose parameter
// is an unexported named type of its package (a named basic type passed
// variadically, chi's MethodNotAllowedHandler(...methodTyp); a named
// callback type, bluemonday's urlRewriter) is skipped, not bound with an
// adapter that names the type, which failed the whole package's go build
// (the Windows tester's bug 19, 2026-10-29).
func TestUnexportedParamTypes(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go list")
	}
	dir := t.TempDir()
	src := filepath.Join(t.TempDir(), "unexp")
	files := map[string]string{
		"go.mod": "module example.com/unexp\n\ngo 1.21\n",
		"unexp.go": `package unexp

type methodTyp uint

type rewriter func(string) string

type Mux struct{ n int }

func NewMux() *Mux { return &Mux{} }

func (m *Mux) Allowed(ms ...methodTyp) int { return len(ms) }

func (m *Mux) Count() int { return m.n }

func Rewrite(r rewriter) string { return r("x") }

func Ok(n int) int { return n }
`,
	}
	for name, body := range files {
		if err := os.MkdirAll(src, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(src, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(workDir(dir), 0o755); err != nil {
		t.Fatal(err)
	}
	gm := workspaceMod(gotool.GoLine()) + "\nrequire example.com/unexp v0.0.0\n\nreplace example.com/unexp => " + src + "\n"
	if err := os.WriteFile(filepath.Join(workDir(dir), "go.mod"), []byte(gm), 0o644); err != nil {
		t.Fatal(err)
	}
	b, err := Bind("example.com/unexp", Options{Dir: dir, Version: "v0.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	decl := string(b.Decl)
	if !strings.Contains(decl, "fn ok(n: Int) -> Int") || !strings.Contains(decl, ") count() -> Int") {
		t.Errorf("ok or count not bound:\n%s", decl)
	}
	if strings.Contains(decl, ") allowed(") || strings.Contains(decl, "fn rewrite(") {
		t.Errorf("a function with an unexported parameter type is bound:\n%s", decl)
	}
	if a := string(b.Adapter); strings.Contains(a, "methodTyp") || strings.Contains(a, "rewriter") {
		t.Errorf("the adapter names an unexported type:\n%s", grep(a, "methodTyp"))
	}
	for _, name := range []string{"allowed", "rewrite"} {
		found := false
		for k, why := range b.Skipped {
			if strings.HasSuffix(k, name) && strings.Contains(why, "unexported type") {
				found = true
			}
		}
		if !found {
			t.Errorf("%s not skipped as an unexported type: %v", name, b.Skipped)
		}
	}
}

// TestNestedModulePin: a pin of an enclosing module (`require go
// example.com/root v0.0.0`) does not pin a package in a nested module of
// its own (example.com/root/sub at v0.1.0, as aws-sdk-go-v2's service/s3
// is): Bind fetched `example.com/root/sub@v0.0.0`, which does not exist
// (the Go workflows suite, 2026-10-29), and Load called its binding stale
// because the root's version is not the sub module's.
func TestNestedModulePin(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go list")
	}
	t.Setenv("GOPROXY", "off")
	dir := t.TempDir()
	srcs := t.TempDir()
	mods := map[string]map[string]string{
		"root": {
			"go.mod":  "module example.com/root\n\ngo 1.21\n",
			"root.go": "package root\n\nfunc One() int { return 1 }\n",
		},
		"sub": {
			"go.mod": "module example.com/root/sub\n\ngo 1.21\n\nrequire example.com/root v0.0.0\n",
			"sub.go": "package sub\n\nimport \"example.com/root\"\n\nfunc Two() int { return root.One() + 1 }\n",
		},
	}
	for m, files := range mods {
		for name, body := range files {
			if err := os.MkdirAll(filepath.Join(srcs, m), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(srcs, m, name), []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := os.MkdirAll(workDir(dir), 0o755); err != nil {
		t.Fatal(err)
	}
	gm := workspaceMod(gotool.GoLine()) + "\nrequire (\n\texample.com/root v0.0.0\n\texample.com/root/sub v0.1.0\n)\n\n" +
		"replace example.com/root v0.0.0 => " + filepath.Join(srcs, "root") + "\n" +
		"replace example.com/root/sub v0.1.0 => " + filepath.Join(srcs, "sub") + "\n"
	if err := os.WriteFile(filepath.Join(workDir(dir), "go.mod"), []byte(gm), 0o644); err != nil {
		t.Fatal(err)
	}
	opts := Options{Dir: dir, Version: "v0.0.0", Module: "example.com/root"}
	b, err := Bind("example.com/root/sub", opts)
	if err != nil {
		t.Fatal(err)
	}
	if b.Module != "example.com/root/sub" || b.ModVersion != "v0.1.0" {
		t.Errorf("binding module = %s %s, want example.com/root/sub v0.1.0", b.Module, b.ModVersion)
	}
	if !strings.Contains(string(b.Decl), "fn two() -> Int") {
		t.Errorf("two not bound:\n%s", b.Decl)
	}
	if l, err := Load("example.com/root/sub", opts); l == nil || err != nil {
		t.Errorf("the binding is stale under its enclosing module's pin (err %v)", err)
	}
}

// TestAnyValues (decision 0412): Go's any is go.Any, ...any a
// List[go.Any], and every handle has its Go value both ways for go.any
// and go.Any.as.
func TestAnyValues(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go list")
	}
	fmtDecl, _, _ := bindDecl(t, "fmt")
	for _, want := range []string{"fn sprintf(format: Str, a: List[go.Any]) -> Str", "fn sprint(a: List[go.Any]) -> Str"} {
		if !strings.Contains(fmtDecl, want) {
			t.Errorf("fmt: no %q:\n%s", want, grep(fmtDecl, "sprint"))
		}
	}
	jsonDecl, _, _ := bindDecl(t, "encoding/json")
	if !strings.Contains(jsonDecl, "fn unmarshal(data: Bytes, v: go.Any)") || !strings.Contains(jsonDecl, "fn marshal(v: go.Any) -> Bytes ! go.Error") {
		t.Errorf("encoding/json: marshal or unmarshal not bound with go.Any:\n%s", grep(jsonDecl, "marshal("))
	}
	sqlDecl, _, _ := bindDecl(t, "database/sql")
	if !strings.Contains(sqlDecl, ") scan(dest: List[go.Any])") {
		t.Errorf("database/sql: Rows.scan not bound:\n%s", grep(sqlDecl, "scan("))
	}
	_, b, _ := bindDecl(t, "net/url")
	a := string(b.Adapter)
	if !strings.Contains(a, ") XoGoValue() any { return h.P }") || !strings.Contains(a, ") XoFromGo(v any) (any, bool) {") {
		t.Errorf("net/url: a handle without XoGoValue or XoFromGo:\n%s", grep(a, "XoGo"))
	}
	for name, why := range b.Skipped {
		if strings.Contains(why, "interface{}") {
			t.Errorf("net/url: %s still skipped for any: %s", name, why)
		}
	}
}

// TestGenericFunctions (decision 0415): a generic Go function is an Xo
// generic with its constraints mapped (any: none, comparable: Eq,
// cmp.Ordered: Ord, ~[]E: List[E], ~map[K]V: Map[K, V]); its adapter is a
// Go generic that instantiates the Go function explicitly; any other
// constraint stays skipped, and so do Go's slices and maps (the prelude
// covers them).
func TestGenericFunctions(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go list")
	}
	dir := t.TempDir()
	src := filepath.Join(t.TempDir(), "gen")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"go.mod": "module example.com/gen\n\ngo 1.21\n",
		"gen.go": `package gen

import "cmp"

func Index[S ~[]E, E comparable](s S, v E) int {
	for i := range s {
		if s[i] == v {
			return i
		}
	}
	return -1
}

func Max[T cmp.Ordered](a, b T) T { return max(a, b) }

func Keep[T any](v T) T { return v }

func Zero[T any]() T {
	var z T
	return z
}

func Keys[M ~map[K]V, K comparable, V any](m M) []K {
	var ks []K
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}

func Show[T interface{ String() string }](v T) string { return v.String() }
`,
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(src, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(workDir(dir), 0o755); err != nil {
		t.Fatal(err)
	}
	gm := workspaceMod(gotool.GoLine()) + "\nrequire example.com/gen v0.0.0\n\nreplace example.com/gen => " + src + "\n"
	if err := os.WriteFile(filepath.Join(workDir(dir), "go.mod"), []byte(gm), 0o644); err != nil {
		t.Fatal(err)
	}
	b, err := Bind("example.com/gen", Options{Dir: dir, Version: "v0.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	decl, adapter := string(b.Decl), string(b.Adapter)
	for _, want := range []string{
		"fn index[E: Eq](s: List[E], v: E) -> Int uses go",
		"fn max[T: Ord](a: T, b: T) -> T uses go",
		"fn keep[T](v: T) -> T uses go",
		"fn zero[T]() -> T uses go",
		"fn keys[K: Eq, V](m: Map[K, V]) -> List[K] uses go",
	} {
		if !strings.Contains(decl, want) {
			t.Errorf("no %q in the declarations:\n%s", want, decl)
		}
	}
	for _, want := range []string{"[E comparable](tk *Task", ".Ordered](tk *Task", ".Index[[]E, E](", ".Zero[T]()", ".Keys[map[K]V, K, V]("} {
		if !strings.Contains(adapter, want) {
			t.Errorf("no %q in the adapter:\n%s", want, adapter)
		}
	}
	skipped := func(b *Binding, name string) string {
		for k, why := range b.Skipped {
			if k == name || strings.HasSuffix(k, "."+name) {
				return why
			}
		}
		return ""
	}
	if why := skipped(b, "show"); !strings.Contains(why, "has no Xo form") {
		t.Errorf("show: skipped %q, want a constraint with no Xo form (skipped: %v)", why, b.Skipped)
	}
	sl, sb, _ := bindDecl(t, "slices")
	if strings.Contains(sl, "fn index[") {
		t.Errorf("slices.index is bound:\n%s", grep(sl, "fn index["))
	}
	if why := skipped(sb, "index"); !strings.Contains(why, "prelude's List and Map") {
		t.Errorf("slices.index skipped %q, want the prelude pointer", why)
	}
}

// TestGenericTypes (decision 0415 stage 2): a generic Go type is a
// generic handle, its methods bound with its type parameters in scope,
// and an instance over type parameters (a constructor's *Box[T]) is the
// handle's instance; a type whose constraint has no Xo form is skipped
// with its methods.
func TestGenericTypes(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go list")
	}
	dir := t.TempDir()
	src := filepath.Join(t.TempDir(), "gen")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"go.mod": "module example.com/gen\n\ngo 1.21\n",
		"gen.go": `package gen

type Box[T any] struct {
	Val T
}

func NewBox[T any](v T) *Box[T] { return &Box[T]{Val: v} }

func (b *Box[T]) Get() T { return b.Val }

func (b *Box[X]) Set(v X) { b.Val = v }

type Set[K comparable] struct{ m map[K]bool }

func NewSet[K comparable]() *Set[K] { return &Set[K]{m: map[K]bool{}} }

func (s *Set[K]) Add(k K)          { s.m[k] = true }
func (s *Set[K]) Has(k K) bool     { return s.m[k] }
func (s *Set[K]) Len() int         { return len(s.m) }
func (s *Set[K]) Union(o *Set[K]) *Set[K] {
	for k := range o.m {
		s.m[k] = true
	}
	return s
}

type Strs[S ~[]string] struct{ v S }

func (s *Strs[S]) Len() int { return len(s.v) }

// zap's ObjectMarshalerPtr: a constraint interface, not a type
type LenPtr[T any] interface {
	*T
	Len() int
}

func Lens[T any, P LenPtr[T]](vs []T) int { return len(vs) }

type Number interface{ ~int | ~float64 }
`,
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(src, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(workDir(dir), 0o755); err != nil {
		t.Fatal(err)
	}
	gm := workspaceMod(gotool.GoLine()) + "\nrequire example.com/gen v0.0.0\n\nreplace example.com/gen => " + src + "\n"
	if err := os.WriteFile(filepath.Join(workDir(dir), "go.mod"), []byte(gm), 0o644); err != nil {
		t.Fatal(err)
	}
	b, err := Bind("example.com/gen", Options{Dir: dir, Version: "v0.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	decl, adapter := string(b.Decl), string(b.Adapter)
	for _, want := range []string{
		"struct Box[T] {}",
		"struct GenSet[K: Eq] {}",
		"fn new_box[T](v: T) -> Box[T] uses go",
		"fn (recv: Box[T]) get() -> T uses go",
		"fn (recv: Box[T]) set(v: T) uses go",
		"fn (recv: Box[T]) val() -> T uses go",
		"fn new_set[K: Eq]() -> GenSet[K] uses go",
		"fn (recv: GenSet[K]) union(o: GenSet[K]) -> GenSet[K]? uses go",
	} {
		if !strings.Contains(decl, want) {
			t.Errorf("no %q in the declarations:\n%s", want, decl)
		}
	}
	for _, want := range []string{
		"type Go_example_com_gen_Box[T any] struct{ P *",
		"type Go_example_com_gen_GenSet[K comparable] struct{ P *",
		"func (h Go_example_com_gen_Box[T]) M_get(tk *Task) T {",
		"func (h Go_example_com_gen_GenSet[K]) M_union(tk *Task, a0 Go_example_com_gen_GenSet[K]) Option[Go_example_com_gen_GenSet[K]] {",
		"func (h Go_example_com_gen_Box[T]) XoEq(o any) bool",
	} {
		if !strings.Contains(adapter, want) {
			t.Errorf("no %q in the adapter:\n%s", want, adapter)
		}
	}
	if strings.Contains(decl, "struct Strs") {
		t.Errorf("Strs is bound:\n%s", grep(decl, "struct Strs"))
	}
	if why := b.Skipped["Strs.len"]; !strings.Contains(why, "has no Xo form") {
		t.Errorf("Strs.len skipped %q, want a constraint with no Xo form (skipped: %v)", why, b.Skipped)
	}
	// a constraint interface is never named as a type (Go rejects it
	// outside a constraint, which failed zap's whole binding)
	for _, name := range []string{"LenPtr", "Number"} {
		if strings.Contains(adapter, "gen."+name) || strings.Contains(decl, "struct "+name) {
			t.Errorf("constraint interface %s is bound:\n%s", name, grep(adapter, "gen."+name))
		}
	}
	if _, ok := b.Skipped["lens"]; !ok {
		t.Errorf("lens is not skipped (skipped: %v)", b.Skipped)
	}
}

// TestCallbackErrors (decision 0416 step 1): a callback parameter with a
// final error result is an Xo function `-> T ! go.Error`, its Err given
// to Go as the error (rt GoErrorOf).
func TestCallbackErrors(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go list")
	}
	dir := t.TempDir()
	src := filepath.Join(t.TempDir(), "cb")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"go.mod": "module example.com/cb\n\ngo 1.21\n",
		"cb.go": `package cb

func Each(n int, f func(int) error) error { return nil }

func Lengths(xs []string, f func(string) (int, error)) ([]int, error) { return nil, nil }

func Two(f func() (int, string, error)) {}
`,
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(src, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(workDir(dir), 0o755); err != nil {
		t.Fatal(err)
	}
	gm := workspaceMod(gotool.GoLine()) + "\nrequire example.com/cb v0.0.0\n\nreplace example.com/cb => " + src + "\n"
	if err := os.WriteFile(filepath.Join(workDir(dir), "go.mod"), []byte(gm), 0o644); err != nil {
		t.Fatal(err)
	}
	b, err := Bind("example.com/cb", Options{Dir: dir, Version: "v0.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	decl, adapter := string(b.Decl), string(b.Adapter)
	for _, want := range []string{
		"fn each[fx: Effects](n: Int, f: fn(Int) -> () ! go.Error uses fx) ! go.Error uses go, fx",
		"fn lengths[fx: Effects](xs: List[Str], f: fn(Str) -> Int ! go.Error uses fx) -> List[Int] ! go.Error uses go, fx",
	} {
		if !strings.Contains(decl, want) {
			t.Errorf("no %q in the declarations:\n%s", want, decl)
		}
	}
	for _, want := range []string{"return GoErrorOf(r.E)", "return z0, GoErrorOf(r.E)"} {
		if !strings.Contains(adapter, want) {
			t.Errorf("no %q in the adapter:\n%s", want, adapter)
		}
	}
	// several results are a tuple
	if want := "fn two[fx: Effects](f: fn() -> (Int, Str) ! go.Error uses fx) uses go, fx"; !strings.Contains(decl, want) {
		t.Errorf("no %q in the declarations:\n%s", want, decl)
	}
	for _, want := range []string{"return z0, z1, GoErrorOf(r.E)", "return int(r.V.F0), r.V.F1, nil"} {
		if !strings.Contains(adapter, want) {
			t.Errorf("no %q in the adapter:\n%s", want, adapter)
		}
	}
}

// TestFuncSetters: a field or package variable of function type gets a
// setter taking an Xo callback, converted as a callback parameter is.
func TestFuncSetters(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go list")
	}
	dir := t.TempDir()
	src := filepath.Join(t.TempDir(), "hook")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"go.mod": "module example.com/hook\n\ngo 1.21\n",
		"hook.go": `package hook

type Check func(name string) error

type Server struct {
	Name    string
	OnError func(msg string)
	Check   Check
	Pick    func(n int) (int, string)
}

var Logf func(format string) int

var Hidden check

type check func() bool

func (s *Server) Run() error {
	if s.Check != nil {
		if err := s.Check(s.Name); err != nil {
			s.OnError(err.Error())
			return err
		}
	}
	return nil
}
`,
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(src, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(workDir(dir), 0o755); err != nil {
		t.Fatal(err)
	}
	gm := workspaceMod(gotool.GoLine()) + "\nrequire example.com/hook v0.0.0\n\nreplace example.com/hook => " + src + "\n"
	if err := os.WriteFile(filepath.Join(workDir(dir), "go.mod"), []byte(gm), 0o644); err != nil {
		t.Fatal(err)
	}
	b, err := Bind("example.com/hook", Options{Dir: dir, Version: "v0.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	decl, adapter := string(b.Decl), string(b.Adapter)
	for _, want := range []string{
		"fn (recv: Server) set_on_error[fx: Effects](v: fn(Str) uses fx) uses go, fx",
		"fn (recv: Server) set_check[fx: Effects](v: fn(Str) -> () ! go.Error uses fx) uses go, fx",
		"fn set_logf[fx: Effects](v: fn(Str) -> Int uses fx) uses go, fx",
		"fn (recv: Server) name() -> Str uses go",
		"fn (recv: Server) on_error() -> (fn(Str) uses go) uses go",
		"fn logf() -> (fn(Str) -> Int uses go) uses go",
		"fn (recv: Server) set_pick[fx: Effects](v: fn(Int) -> (Int, Str) uses fx) uses go, fx",
		"fn (recv: Server) pick() -> (fn(Int) -> (Int, Str) uses go) uses go",
	} {
		if !strings.Contains(decl, want) {
			t.Errorf("no %q in the declarations:\n%s", want, decl)
		}
	}
	for _, want := range []string{"h.P.OnError = v", "h.P.Check = v", ".Logf = v"} {
		if !strings.Contains(adapter, want) {
			t.Errorf("no %q in the adapter:\n%s", want, adapter)
		}
	}
	for name, why := range map[string]string{
		"set_hidden": "unexported type",
	} {
		if !strings.Contains(b.Skipped[name], why) {
			t.Errorf("%s skipped %q, want %q (skipped: %v)", name, b.Skipped[name], why, b.Skipped)
		}
	}
}

// TestFuncValues: a Go function value given to Xo (a result, a field)
// is an Xo function `uses go` that calls it.
func TestFuncValues(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go list")
	}
	dir := t.TempDir()
	src := filepath.Join(t.TempDir(), "fnval")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"go.mod": "module example.com/fnval\n\ngo 1.21\n",
		"fnval.go": `package fnval

type Op func(a, b int) int

func Adder(n int) func(int) int { return func(x int) int { return x + n } }

func Parser() func(string) (int, error) { return nil }

func Pick() Op { return nil }

func Closer() (func() error, error) { return nil, nil }

func Nested() func(func(int) int) int { return nil }

func Pair() func() (int, string) { return nil }
`,
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(src, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(workDir(dir), 0o755); err != nil {
		t.Fatal(err)
	}
	gm := workspaceMod(gotool.GoLine()) + "\nrequire example.com/fnval v0.0.0\n\nreplace example.com/fnval => " + src + "\n"
	if err := os.WriteFile(filepath.Join(workDir(dir), "go.mod"), []byte(gm), 0o644); err != nil {
		t.Fatal(err)
	}
	b, err := Bind("example.com/fnval", Options{Dir: dir, Version: "v0.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	decl, adapter := string(b.Decl), string(b.Adapter)
	for _, want := range []string{
		"fn adder(n: Int) -> (fn(Int) -> Int uses go) uses go",
		"fn parser() -> (fn(Str) -> Int ! go.Error uses go) uses go",
		"fn pick() -> Op uses go",
		"fn closer() -> (fn() -> () ! go.Error uses go) ! go.Error uses go",
		"fn pair() -> (fn() -> (Int, Str) uses go) uses go",
		"Replay: a Go handle crosses",
	} {
		if !strings.Contains(decl, want) {
			t.Errorf("no %q in the declarations:\n%s", want, decl)
		}
	}
	if !strings.Contains(adapter, "defer GoExit(tk, \"func(int) int\")") {
		t.Errorf("the function value's call does not enter Go:\n%s", adapter)
	}
	for name, why := range map[string]string{
		"nested": "function type",
	} {
		if !strings.Contains(b.Skipped[name], why) {
			t.Errorf("%s skipped %q, want %q (skipped: %v)", name, b.Skipped[name], why, b.Skipped)
		}
	}
}

// TestImplementations (decision 0416): each interface whose methods map
// as callbacks gets a constructor taking one function per method, named
// after it; a Read method is given len(p) and returns the bytes.
func TestImplementations(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go list")
	}
	io, ib, _ := bindDecl(t, "io")
	for _, want := range []string{
		"fn writer[fx: Effects](write: fn(Bytes) -> Int ! go.Error uses fx) -> Writer uses go, fx",
		"fn reader[fx: Effects](read: fn(Int) -> Bytes ! go.Error uses fx) -> Reader uses go, fx",
		"fn read_closer[fx: Effects](close: fn() -> () ! go.Error uses fx, read: fn(Int) -> Bytes ! go.Error uses fx) -> ReadCloser uses go, fx",
	} {
		if !strings.Contains(io, want) {
			t.Errorf("no %q in io's declarations:\n%s", want, grep(io, "fn "))
		}
	}
	ia := string(ib.Adapter)
	for _, want := range []string{
		"func (x *xoImpl_Go_io_Reader) Read(p0 []byte) (int, error) {",
		"n := copy(p0, b)",
		"if err == nil && n == 0 && len(p0) > 0 {",
		".EOF\n",
		"func (x *xoImpl_Go_io_Writer) Write(p0 []byte) (int, error) {",
	} {
		if !strings.Contains(ia, want) {
			t.Errorf("no %q in io's adapter", want)
		}
	}
	srt, _, _ := bindDecl(t, "sort")
	if want := "fn interface_[fx: Effects](len: fn() -> Int uses fx, less: fn(Int, Int) -> Bool uses fx, swap: fn(Int, Int) uses fx) -> Interface uses go, fx"; !strings.Contains(srt, want) {
		t.Errorf("no %q in sort's declarations:\n%s", want, grep(srt, "fn interface"))
	}
	h, _, _ := bindDecl(t, "net/http")
	if !strings.Contains(h, "fn handler[fx: Effects](serve_http: fn(ResponseWriter, Request) uses fx) -> Handler uses go, fx") {
		t.Errorf("no http.handler constructor:\n%s", grep(h, "fn handler"))
	}
}

// TestUpcasts: a handle whose Go type is assignable to another interface
// of its package gives it as that interface (`as_reader()`), since Go's
// assignability does not cross the handle boundary.
func TestUpcasts(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go list")
	}
	io, ib, _ := bindDecl(t, "io")
	for _, want := range []string{
		"fn (x: ReadCloser) as_reader() -> Reader uses go",
		"fn (x: ReadCloser) as_closer() -> Closer uses go",
		"fn (x: ReadWriteCloser) as_read_closer() -> ReadCloser uses go",
	} {
		if !strings.Contains(io, want) {
			t.Errorf("no %q in io's declarations:\n%s", want, grep(io, "as_"))
		}
	}
	if !strings.Contains(string(ib.Adapter), "func (h Go_io_ReadCloser) M_as_reader(tk *Task) Go_io_Reader {\n\treturn Go_io_Reader{V: h.V}\n}") {
		t.Errorf("no as_reader adapter in io's adapter")
	}
	if strings.Contains(io, "fn (x: Reader) as_read_closer()") {
		t.Errorf("a Reader converts to a ReadCloser:\n%s", grep(io, "as_read_closer"))
	}
	bs, _, _ := bindDecl(t, "bytes")
	for _, want := range []string{
		"fn (x: Buffer) as_writer() -> io.Writer uses go",
		"fn (x: Buffer) as_reader() -> io.Reader uses go",
		"fn (x: Buffer) as_writer_to() -> io.WriterTo uses go",
		"fn (x: Reader) as_byte_reader() -> io.ByteReader uses go",
	} {
		if !strings.Contains(bs, want) {
			t.Errorf("no %q in bytes' declarations:\n%s", want, grep(bs, "as_"))
		}
	}
	if strings.Contains(bs, "fmt.Stringer") {
		t.Errorf("bytes, which does not import fmt, offers fmt.Stringer:\n%s", grep(bs, "Stringer"))
	}
	us, _, _ := bindDecl(t, "net/url")
	if want := "fn (x: URL) as_stringer() -> fmt.Stringer uses go"; !strings.Contains(us, want) {
		t.Errorf("no %q in net/url's declarations:\n%s", want, grep(us, "as_"))
	}
}

// TestErrorValues (decision 0416): Go's error outside the final result is
// `go.Error?` (nil is None), and a sentinel error variable is a go.Error;
// an error that is the only result stays `! go.Error` (nil is Ok).
func TestErrorValues(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go list")
	}
	es, eb, _ := bindDecl(t, "errors")
	for _, want := range []string{
		"fn is(err: go.Error?, target: go.Error?) -> Bool uses go",
		"fn unwrap(err: go.Error?) ! go.Error uses go",
		"fn join(errs: List[go.Error?]) ! go.Error uses go",
	} {
		if !strings.Contains(es, want) {
			t.Errorf("no %q in errors' declarations:\n%s", want, grep(es, "fn "))
		}
	}
	if !strings.Contains(string(eb.Adapter), "GoErrorOfOpt(a0)") {
		t.Errorf("errors' adapter does not convert the error parameter")
	}
	io, _, _ := bindDecl(t, "io")
	if want := "fn eof() -> go.Error uses go"; !strings.Contains(io, want) {
		t.Errorf("no %q in io's declarations:\n%s", want, grep(io, "eof"))
	}
}

// TestNamedBasicMethods (decision 0416): a value receiver method of a
// named basic type is a package function `<type>_<method>` taking the
// value first; a pointer receiver method and a taken name are skipped.
func TestNamedBasicMethods(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go list")
	}
	dir := t.TempDir()
	src := filepath.Join(t.TempDir(), "lvl")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"go.mod": "module example.com/lvl\n\ngo 1.21\n",
		"lvl.go": `package lvl

type Level int8

func (l Level) String() string { return "" }

func (l Level) Enabled(x Level) bool { return x >= l }

func (l Level) Join(sep string, more ...Level) string { return "" }

func (l *Level) Set(s string) error { return nil }

type Mode uint32

func (Mode) IsDir() bool { return false }

func LevelString() string { return "" }
`,
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(src, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(workDir(dir), 0o755); err != nil {
		t.Fatal(err)
	}
	gm := workspaceMod(gotool.GoLine()) + "\nrequire example.com/lvl v0.0.0\n\nreplace example.com/lvl => " + src + "\n"
	if err := os.WriteFile(filepath.Join(workDir(dir), "go.mod"), []byte(gm), 0o644); err != nil {
		t.Fatal(err)
	}
	b, err := Bind("example.com/lvl", Options{Dir: dir, Version: "v0.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	decl, adapter := string(b.Decl), string(b.Adapter)
	for _, want := range []string{
		"fn level_enabled(l: Int8, x: Int8) -> Bool uses go",
		"fn level_join(l: Int8, sep: Str, more: List[Int8]) -> Str uses go",
		"fn mode_is_dir(v: UInt32) -> Bool uses go",
		"fn level_string() -> Str uses go",
	} {
		if !strings.Contains(decl, want) {
			t.Errorf("no %q in the declarations:\n%s", want, decl)
		}
	}
	if !strings.Contains(adapter, ".Level.Enabled(") {
		t.Errorf("level_enabled does not call the method expression:\n%s", adapter)
	}
	for name, why := range map[string]string{
		"level_set":    "pointer receiver",
		"level_string": "is taken",
	} {
		if !strings.Contains(b.Skipped[name], why) {
			t.Errorf("%s skipped %q, want %q (skipped: %v)", name, b.Skipped[name], why, b.Skipped)
		}
	}
}

// TestBasicPointers (decision 0419): a pointer to a basic type is `T?`,
// a copy at the boundary; a parameter Go writes through stays skipped.
func TestBasicPointers(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go list")
	}
	dir := t.TempDir()
	src := filepath.Join(t.TempDir(), "opt")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"go.mod": "module example.com/opt\n\ngo 1.21\n",
		"opt.go": `package opt

type Options struct {
	Name  *string
	Limit *int
	On    *bool
}

func Describe(o *Options) string { return "" }

func Or(x *int, d int) int {
	if x == nil {
		return d
	}
	return *x
}

func Ptr(n int) *int { return &n }

func IntVar(p *int, n int) { *p = n }
`,
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(src, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(workDir(dir), 0o755); err != nil {
		t.Fatal(err)
	}
	gm := workspaceMod(gotool.GoLine()) + "\nrequire example.com/opt v0.0.0\n\nreplace example.com/opt => " + src + "\n"
	if err := os.WriteFile(filepath.Join(workDir(dir), "go.mod"), []byte(gm), 0o644); err != nil {
		t.Fatal(err)
	}
	b, err := Bind("example.com/opt", Options{Dir: dir, Version: "v0.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	decl := string(b.Decl)
	for _, want := range []string{
		"fn (recv: Options) name() -> Str? uses go",
		"fn (recv: Options) set_limit(v: Int?) uses go",
		"fn or(x: Int?, d: Int) -> Int uses go",
		"fn ptr(n: Int) -> Int? uses go",
	} {
		if !strings.Contains(decl, want) {
			t.Errorf("no %q in the declarations:\n%s", want, decl)
		}
	}
	if why := b.Skipped["int_var"]; !strings.Contains(why, "writes through") {
		t.Errorf("int_var skipped %q, want writes through (skipped: %v)", why, b.Skipped)
	}
}

// TestChannels (decision 0421): a Go channel given to Xo is a go.Recv[T];
// a send only channel, a channel parameter and time's timer channels are
// skipped, and a channel field has no setter.
func TestChannels(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go list")
	}
	dir := t.TempDir()
	src := filepath.Join(t.TempDir(), "chans")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"go.mod": "module example.com/chans\n\ngo 1.21\n",
		"chans.go": `package chans

type Watcher struct {
	Names  chan string
	Errors chan error
}

func NewWatcher() *Watcher { return &Watcher{} }

func Ticks(n int) <-chan int { return nil }

func Sink(c chan<- int) {}

func Feed(c <-chan int) {}

func Lists() <-chan []string { return nil }
`,
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(src, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(workDir(dir), 0o755); err != nil {
		t.Fatal(err)
	}
	gm := workspaceMod(gotool.GoLine()) + "\nrequire example.com/chans v0.0.0\n\nreplace example.com/chans => " + src + "\n"
	if err := os.WriteFile(filepath.Join(workDir(dir), "go.mod"), []byte(gm), 0o644); err != nil {
		t.Fatal(err)
	}
	b, err := Bind("example.com/chans", Options{Dir: dir, Version: "v0.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	decl, adapter := string(b.Decl), string(b.Adapter)
	for _, want := range []string{
		"fn ticks(n: Int) -> go.Recv[Int] uses go",
		"fn lists() -> go.Recv[List[Str]] uses go",
		"fn (recv: Watcher) names() -> go.Recv[Str] uses go",
		"fn (recv: Watcher) errors() -> go.Recv[go.Error?] uses go",
	} {
		if !strings.Contains(decl, want) {
			t.Errorf("no %q in the declarations:\n%s", want, decl)
		}
	}
	if !strings.Contains(adapter, `GoRecvOf((<-chan int)(r0), "int", true,`) {
		t.Errorf("ticks does not wrap its channel:\n%s", adapter)
	}
	for name, why := range map[string]string{
		"sink":              "send only",
		"feed":              "does not pass them",
		"Watcher.set_names": "cannot go back to Go",
	} {
		if !strings.Contains(b.Skipped[name], why) {
			t.Errorf("%s skipped %q, want %q (skipped: %v)", name, b.Skipped[name], why, b.Skipped)
		}
	}
	tb, err := Bind("time", Options{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(tb.Skipped["after"], "use Xo's clock") {
		t.Errorf("time.after skipped %q, want Xo's clock", tb.Skipped["after"])
	}
}
