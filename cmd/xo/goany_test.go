package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestUseGoAny covers decision 0412 on the Go backend with Go's std
// only: fmt.Sprintf's `...any` from a list literal (the implicit
// conversion), go.any and its as_* reads, a handle through go.Any and
// back, go.out for json.Unmarshal, and Go's == on go.Any.
func TestUseGoAny(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("runs go list and go build")
	}
	setenv(t, "XO_GOBIND_DIR", "")
	dir := writeFiles(t, map[string]string{
		"xo.mod": "module goany\n",
		"main.xo": `use go "fmt"
use go "encoding/json"
use go "net/url"
use std/go

fn main(os: Os) -> () ! Error {
  let n = 42
  os.stdio.println(fmt.sprintf("%d %s %v %.1f %v", [n, "x", true, 2.5, [1, 2]]))
  let a = go.any(3.5)
  os.stdio.println("${a.type_name()} ${a.as_float() ?? 0.0} ${a.as_int() == None} ${a.is_nil()}")
  let u = url.parse("https://example.com/p")?
  let h = go.any(u)
  let host = match h.as_handle[url.URL]() {
    Some(x) => x.host()
    None => "none"
  }
  os.stdio.println("${h.type_name()} ${host}")
  let v = go.out[go.Any]()
  json.unmarshal("{\"a\":[1,2],\"b\":\"s\"}".bytes(), v.any())?
  let m = v.get().as_map() ?? {}
  let b = (m.get("b") ?? go.any("?")).as_str() ?? "?"
  let xs = (m.get("a") ?? go.any(0)).as_list() ?? []
  os.stdio.println("${b} ${xs.len()} ${xs[0].as_float() ?? 0.0}")
  let s = go.out[Str]()
  json.unmarshal("\"hi\"".bytes(), s.any())?
  os.stdio.println(s.get())
  os.stdio.println("${go.any(1) == go.any(1)} ${go.any(1) == go.any("1")}")
}
`,
	})
	var out, errOut bytes.Buffer
	if code := run(t, []string{"run", "--backend=go", dir}, &out, &errOut); code != 0 {
		t.Fatalf("run: exit %d\n%s%s", code, out.String(), errOut.String())
	}
	want := "42 x true 2.5 [1 2]\nfloat64 3.5 true false\n*url.URL example.com\ns 2 1.0\nhi\ntrue false"
	if got := strings.TrimSpace(out.String()); got != want {
		t.Errorf("output\n%s\nwant\n%s", got, want)
	}
}

// TestUseGoAnyNoGoValue: an element with no Go value in a `...any` list,
// and go.any of one, are XO1106 (decision 0412).
func TestUseGoAnyNoGoValue(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("runs go list")
	}
	setenv(t, "XO_GOBIND_DIR", "")
	dir := writeFiles(t, map[string]string{
		"xo.mod": "module goany\n",
		"main.xo": `use go "fmt"
use std/go

enum Color { Red, Green }

fn main(os: Os) {
  os.stdio.println(fmt.sprint([Color.Red]))
  let _ = go.any(Color.Green)
}
`,
	})
	var out, errOut bytes.Buffer
	code := run(t, []string{"check", "--backend=go", dir}, &out, &errOut)
	if code != 1 || strings.Count(errOut.String(), "XO1106") != 2 || !strings.Contains(errOut.String(), "`Color` has no Go value") {
		t.Fatalf("check: exit %d\n%s%s", code, out.String(), errOut.String())
	}
}

// TestUseGoAnyReplay (decision 0412 7): a go.Any a recorded Go call
// returned replays from its recorded display text: displaying it replays
// the run identically, and reading it with as_int faults in the replay.
// testdata/goproxy's synthetic example.com/anyval has functions with no
// handle in their signatures (a call with one is re executed instead,
// decision 0093).
func TestUseGoAnyReplay(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("runs go list and go build")
	}
	proxy, err := filepath.Abs(filepath.Join("testdata", "goproxy"))
	if err != nil {
		t.Fatal(err)
	}
	modcache := t.TempDir()
	t.Cleanup(func() {
		filepath.WalkDir(modcache, func(p string, d os.DirEntry, err error) error {
			if err == nil {
				os.Chmod(p, 0o755)
			}
			return nil
		})
	})
	for k, v := range map[string]string{
		"GOPROXY": "file://" + filepath.ToSlash(proxy), "GOSUMDB": "off", "GOFLAGS": "",
		"GOMODCACHE": modcache, "GOTOOLCHAIN": "local", "GOPRIVATE": "", "GONOSUMDB": "", "GONOPROXY": "",
		"XO_GOBIND_DIR": "",
	} {
		setenv(t, k, v)
	}
	for _, c := range []struct{ name, line, fault string }{
		{"display", `os.stdio.println("answer ${anyval.answer()}")`, ""},
		{"as_int", `os.stdio.println("answer ${anyval.answer().as_int() ?? -1}")`, "on a replayed Go value"},
	} {
		dir := writeFiles(t, map[string]string{
			"xo.mod": "module goany\n\nrequire go example.com/anyval v1.0.0\n",
			"xo.lock": "go example.com/anyval v1.0.0 h1:VjIWiJiTiw9A3TO5MGMVx5xwHQWSO524rtRV4/0mVjE=\n" +
				"go example.com/anyval v1.0.0/go.mod h1:oTIpeosJMja4XXMjkP63QPPm41pHk5zzjYciue7EqBE=\n",
			"main.xo": "use go \"example.com/anyval\"\n\nfn main(os: Os) {\n  " + c.line + "\n}\n",
		})
		trace := filepath.Join(t.TempDir(), "run.xotrace")
		var out, errOut bytes.Buffer
		if code := run(t, []string{"run", "--backend=go", "--record=" + trace, dir}, &out, &errOut); code != 0 || strings.TrimSpace(out.String()) != "answer 42" {
			t.Fatalf("%s: run --record: exit %d\n%s%s", c.name, code, out.String(), errOut.String())
		}
		rec := out.String()
		out.Reset()
		errOut.Reset()
		code := run(t, []string{"replay", trace}, &out, &errOut)
		if c.fault == "" {
			if code != 0 || out.String() != rec {
				t.Errorf("%s: replay: exit %d\n%s%s", c.name, code, out.String(), errOut.String())
			}
		} else if code == 0 || !strings.Contains(errOut.String(), c.fault) {
			t.Errorf("%s: replay: exit %d, want a fault %q\n%s%s", c.name, code, c.fault, out.String(), errOut.String())
		}
	}
}
