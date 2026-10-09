package gobind

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestReadRerun reads `replay rerun go` lines of an xo.mod (decision
// 0093).
func TestReadRerun(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "xo.mod")
	os.WriteFile(p, []byte("module ex\n\nreplay rerun go example.com/pure\nreplay rerun go os getpid Root.close // pure enough\nreplay go os\nrequire go example.com/pure v1.0.0\n"), 0o644)
	got := ReadRerun(p)
	want := map[string][]string{"example.com/pure": {"*"}, "os": {"getpid", "Root.close"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ReadRerun = %v, want %v", got, want)
	}
	if k := rerunKey([]string{"b", "a"}); k != "a b" {
		t.Errorf("rerunKey = %q", k)
	}
}

// TestReplayModes checks how adapters take part in record and replay
// (decision 0093): by value, re executed (handles, callbacks), or pure.
func TestReplayModes(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go list")
	}
	decl, b, dir := bindDecl(t, "os")
	ad := string(b.Adapter)
	for _, want := range []string{
		"return GoFxValue(tk, \"os.getpid\", nil, func() int64 { return goCall_Go_os_getpid(tk) })",
		"return GoFxValue(tk, \"os.read_file\", []any{a0}, func() Result[Bytes, Go_Error] { return goCall_Go_os_read_file(tk, a0) })",
		"GoFxValue(tk, \"os.clearenv\", nil, func() Unit { goCall_Go_os_clearenv(tk); return Unit{} })",
		"GoFxRerun(tk, \"os.open\", \"handle\")",
		"GoFxRerun(tk, \"os.File.close\", \"handle\")",
	} {
		if !strings.Contains(ad, want) {
			t.Errorf("os adapter lacks %q", want)
		}
	}
	if !strings.Contains(decl, "/// Replay: recorded; `xo replay` returns the recorded results\nfn getpid() -> Int uses go") {
		t.Errorf("getpid declaration:\n%s", grep(decl, "getpid"))
	}
	_, sb, _ := bindDecl(t, "sync")
	if !strings.Contains(string(sb.Adapter), "GoFxRerun(tk, \"sync.WaitGroup.go_\", \"callback\")") {
		t.Errorf("sync adapter: WaitGroup.go_ is not re executed as a callback call:\n%s", grep(string(sb.Adapter), "GoFx"))
	}
	// pure std packages are not wrapped, except where a handle of an
	// impure package crosses (regexp's io.RuneReader methods)
	_, pb, _ := bindDecl(t, "strconv")
	if strings.Contains(string(pb.Adapter), "GoFx") {
		t.Errorf("strconv adapter records calls:\n%s", grep(string(pb.Adapter), "GoFx"))
	}
	_, pb, _ = bindDecl(t, "regexp")
	ra := string(pb.Adapter)
	if strings.Contains(ra, "\"regexp.must_compile\"") || strings.Contains(ra, "\"regexp.Regexp.find_string\"") || !strings.Contains(ra, "GoFxRerun(tk, \"regexp.Regexp.match_reader\", \"handle\")") {
		t.Errorf("regexp adapter:\n%s", grep(ra, "GoFx"))
	}
	// an xo.mod rerun list: a binding of its own, not recorded
	opts := Options{Dir: dir, Rerun: []string{"getpid", "File.name"}}
	if l, _ := Load("os", opts); l != nil {
		t.Fatal("the plain os binding loaded for a rerun list")
	}
	rb, err := Bind("os", opts)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(rb.DeclPath, ".xo") || rb.DeclPath == b.DeclPath {
		t.Errorf("rerun binding path %s (plain %s)", rb.DeclPath, b.DeclPath)
	}
	rad := string(rb.Adapter)
	if strings.Contains(rad, "\"os.getpid\"") || strings.Contains(rad, "\"os.File.name\"") || !strings.Contains(rad, "\"os.getppid\"") {
		t.Errorf("rerun list not applied:\n%s", grep(rad, "GoFx"))
	}
	if !strings.Contains(string(rb.Decl), "// replay-rerun: File.name getpid\n") {
		t.Errorf("rerun header missing")
	}
	if l, _ := Load("os", opts); l == nil {
		t.Error("the rerun binding does not load")
	}
	if l, _ := Load("os", Options{Dir: dir}); l == nil || l.DeclPath != b.DeclPath {
		t.Error("the plain binding was overwritten")
	}
}
