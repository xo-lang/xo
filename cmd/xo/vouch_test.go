package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/cbind"
	"github.com/xo-lang/xo/internal/mod"
	"github.com/xo-lang/xo/internal/mod/modtest"
)

// repoWorld writes module versions ("<module path>@<version>/<file>" ->
// text) into a temporary source tree and makes git repositories of them
// (modtest.Setup).
func repoWorld(t *testing.T, files map[string]string) *modtest.Env {
	t.Helper()
	src := t.TempDir()
	modtest.WriteFiles(t, src, files)
	return modtest.Setup(t, src, mod.Compare)
}

// TestVouches checks decision 0156 item 1: a dependency's `use go` of an
// impure package and its `use c` lines count only with a `vouch` of the
// consuming module for that exact version (XO1316); pure Go packages
// need none.
func TestVouches(t *testing.T) {
	repoWorld(t, map[string]string{
		"github.com/acme/osx@v1.0.0/xo.mod":   "module github.com/acme/osx\nedition preview\n",
		"github.com/acme/osx@v1.0.0/osx.xo":   "use go \"os\"\n\npub fn pid() -> Int uses go { os.getpid() }\n",
		"github.com/acme/osx@v1.1.0/xo.mod":   "module github.com/acme/osx\nedition preview\n",
		"github.com/acme/osx@v1.1.0/osx.xo":   "use go \"os\"\n\npub fn pid() -> Int uses go { os.getpid() + 0 }\n",
		"github.com/acme/strx@v1.0.0/xo.mod":  "module github.com/acme/strx\nedition preview\n",
		"github.com/acme/strx@v1.0.0/strx.xo": "use go \"strings\"\n\npub fn up(s: Str) -> Str uses go { strings.to_upper(s) }\n",
	})
	ws := t.TempDir()
	t.Setenv("XO_BUILD_DIR", filepath.Join(ws, ".build"))
	app := filepath.Join(ws, "app")
	xomod := "module example.com/app\nedition preview\n\nrequire github.com/acme/osx v1.0.0\nrequire github.com/acme/strx v1.0.0\n"
	modtest.WriteFiles(t, app, map[string]string{
		"xo.mod":       xomod,
		"main/main.xo": "use github.com/acme/osx\nuse github.com/acme/strx\n\nfn main(os: Os) {\n  os.stdio.println(\"${osx.pid() > 0} ${strx.up(\"a\")}\")\n}\n",
	})
	xo(t, "mod", "tidy", "--dir", app).want(0)
	r := xo(t, "check", "--json", app).want(1)
	if got := diagCodes(t, r.out); got != "XO1316" {
		t.Fatalf("unvouched use go: %s\n%s", got, r.out)
	}
	if !strings.Contains(r.out, `vouch github.com/acme/osx v1.0.0 go \"os\"`) {
		t.Errorf("XO1316 does not suggest the vouch line:\n%s", r.out)
	}
	// a dependency's own vouch does not count
	// (its xo.mod is fetched, so this is the consumer's line)
	os.WriteFile(filepath.Join(app, "xo.mod"), []byte(xomod+"\nvouch github.com/acme/osx v1.0.0 go \"os\"\n"), 0o644)
	xo(t, "check", app).want(0)
	// an upgrade makes the vouch stale: xo get refuses it before writing,
	// even with --accept-effects (it accepts effects, not escape hatches)
	modBefore, _ := os.ReadFile(filepath.Join(app, "xo.mod"))
	for _, args := range [][]string{
		{"get", "--json", "--dir", app, "github.com/acme/osx@v1.1.0"},
		{"get", "--json", "--accept-effects", "--dir", app, "github.com/acme/osx@v1.1.0"},
	} {
		r = xo(t, args...).want(1)
		if got := diagCodes(t, r.out); got != "XO1316" || !strings.Contains(r.out, `vouch github.com/acme/osx v1.1.0 go \"os\"`) {
			t.Errorf("%v of an unvouched version: %s\n%s", args, got, r.out)
		}
		if modAfter, _ := os.ReadFile(filepath.Join(app, "xo.mod")); string(modAfter) != string(modBefore) {
			t.Errorf("%v changed xo.mod:\n%s", args, modAfter)
		}
	}
	// a hand edit to the new version: the check reports the stale vouch
	os.WriteFile(filepath.Join(app, "xo.mod"), []byte(strings.Replace(string(modBefore), "osx v1.0.0\n", "osx v1.1.0\n", 1)), 0o644)
	xo(t, "mod", "tidy", "--dir", app).want(0)
	r = xo(t, "check", "--json", app).want(1)
	if got := diagCodes(t, r.out); got != "XO1316" || !strings.Contains(r.out, "reviewed github.com/acme/osx v1.0.0") {
		t.Errorf("stale vouch: %s\n%s", got, r.out)
	}
	// vouching for the new version lets xo get through
	os.WriteFile(filepath.Join(app, "xo.mod"), []byte(string(modBefore)+"vouch github.com/acme/osx v1.1.0 go \"os\"\n"), 0o644)
	xo(t, "get", "--dir", app, "github.com/acme/osx@v1.1.0").want(0)
	xo(t, "check", app).want(0)
	// malformed vouch lines are XO1301
	os.WriteFile(filepath.Join(app, "xo.mod"), []byte(xomod+"\nvouch github.com/acme/osx latest go os\n"), 0o644)
	r = xo(t, "check", "--json", app).want(1)
	if got := diagCodes(t, r.out); !strings.Contains(got, "XO1301") {
		t.Errorf("malformed vouch: %s", got)
	}
}

// TestVouchesC checks `use c` in a dependency (native check): its own
// `uses none` is not trusted (XO1316, and the binding performs ffi, so
// its pure pub function is XO0701); the consumer's vouch decides the
// binding's effects.
func TestVouchesC(t *testing.T) {
	if _, err := cbind.Clang(); err != nil {
		t.Skip(err)
	}
	t.Setenv("XO_CBIND_DIR", "")
	repoWorld(t, map[string]string{
		"github.com/acme/cz@v1.0.0/xo.mod": "module github.com/acme/cz\nedition preview\n",
		"github.com/acme/cz@v1.0.0/tiny.h": "int tiny_add(int a, int b);\n",
		"github.com/acme/cz@v1.0.0/tiny.c": "int tiny_add(int a, int b) { return a + b; }\n",
		"github.com/acme/cz@v1.0.0/cz.xo":  "use c \"tiny.h\" link \"tiny.c\" uses none\n\npub fn add(a: Int32, b: Int32) -> Int32 { tiny.tiny_add(a, b) }\n",
	})
	ws := t.TempDir()
	t.Setenv("XO_BUILD_DIR", filepath.Join(ws, ".build"))
	app := filepath.Join(ws, "app")
	xomod := "module example.com/app\nedition preview\n\nrequire github.com/acme/cz v1.0.0\n"
	modtest.WriteFiles(t, app, map[string]string{
		"xo.mod":       xomod,
		"main/main.xo": "use github.com/acme/cz\n\nfn main(os: Os) {\n  os.stdio.println(\"${cz.add(2, 40)}\")\n}\n",
	})
	xo(t, "mod", "tidy", "--dir", app).want(0)
	r := xo(t, "check", "--no-gc", "--json", app).want(1)
	if got := diagCodes(t, r.out); got != "XO1316,XO0701" && got != "XO0701,XO1316" {
		t.Fatalf("unvouched use c: %s\n%s", got, r.out)
	}
	for _, tc := range []struct{ vouch, want string }{
		{"vouch github.com/acme/cz v1.0.0 c \"tiny.h\" uses none", ""},
		{"vouch github.com/acme/cz v1.0.0 c \"tiny.h\"", "XO0701"},          // the consumer says ffi
		{"vouch github.com/acme/cz v1.0.0 c \"tiny.h\" uses net", "XO0701"}, // the consumer disagrees
	} {
		os.WriteFile(filepath.Join(app, "xo.mod"), []byte(xomod+"\n"+tc.vouch+"\n"), 0o644)
		r := xo(t, "check", "--no-gc", "--json", app)
		if got := diagCodes(t, r.out); got != tc.want {
			t.Errorf("%s: %s, want %q\n%s", tc.vouch, got, tc.want, r.out)
		}
	}
	if !testing.Short() {
		os.WriteFile(filepath.Join(app, "xo.mod"), []byte(xomod+"\nvouch github.com/acme/cz v1.0.0 c \"tiny.h\" uses none\n"), 0o644)
		r := xo(t, "run", "--no-gc", filepath.Join(app, "main")).want(0)
		if r.out != "42\n" {
			t.Errorf("run with a vouched C dependency printed %q", r.out)
		}
	}
}
