package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/mod/modtest"
)

// TestEffectLock checks decision 0156 item 2: xo.lock records each
// dependency's effects; an update that widens them fails `xo get` and
// every loading command (XO1317) until `xo mod accept-effects` or `xo get
// --accept-effects`; a narrowing is recorded; a lock without effect lines
// is not checked and `xo mod tidy` adds them.
func TestEffectLock(t *testing.T) {
	t.Parallel()
	tick := func(extra string) string {
		return "pub fn pure() -> Int { 1 }\n" + extra
	}
	repoWorld(t, map[string]string{
		"github.com/acme/tick@v1.0.0/xo.mod":  "module github.com/acme/tick\nedition preview\n",
		"github.com/acme/tick@v1.0.0/tick.xo": tick("pub fn load(f: Fs) -> Bool uses fs.read { f.exists(\"x\").ok() ?? false }\n"),
		"github.com/acme/tick@v1.1.0/xo.mod":  "module github.com/acme/tick\nedition preview\n",
		"github.com/acme/tick@v1.1.0/tick.xo": tick("pub fn load(f: Fs) -> Bool uses fs.read { f.exists(\"x\").ok() ?? false }\npub fn ping(n: Net) -> Bool uses net.dial { n.dial(\"a:1\").is_ok() }\n"),
		"github.com/acme/tick@v1.2.0/xo.mod":  "module github.com/acme/tick\nedition preview\n",
		"github.com/acme/tick@v1.2.0/tick.xo": tick("pub fn ping(n: Net) -> Bool uses net.dial { n.dial(\"a:1\").is_ok() }\n"),
	})
	ws := t.TempDir()
	app := filepath.Join(ws, "app")
	modtest.WriteFiles(t, app, map[string]string{
		"xo.mod":       "module example.com/app\nedition preview\n\nrequire github.com/acme/tick v1.0.0\n",
		"main/main.xo": "use github.com/acme/tick\n\nfn main(os: Os) {\n  os.stdio.println(\"${tick.pure()}\")\n}\n",
	})
	lockFile := filepath.Join(app, "xo.lock")
	lockHas := func(want string) {
		t.Helper()
		data, _ := os.ReadFile(lockFile)
		if !strings.Contains(string(data), want+"\n") {
			t.Errorf("xo.lock lacks %q:\n%s", want, data)
		}
	}
	xo(t, "mod", "tidy", "--dir", app).want(0)
	lockHas("github.com/acme/tick effects fs.read")
	xo(t, "check", app).want(0)

	// an upgrade that adds net.dial fails and writes nothing
	modBefore, _ := os.ReadFile(filepath.Join(app, "xo.mod"))
	r := xo(t, "get", "--json", "--dir", app, "github.com/acme/tick@v1.1.0").want(1)
	if got := diagCodes(t, r.out); got != "XO1317" || !strings.Contains(r.out, "`net.dial`") {
		t.Errorf("get of a widening version: %s\n%s", got, r.out)
	}
	if modAfter, _ := os.ReadFile(filepath.Join(app, "xo.mod")); string(modAfter) != string(modBefore) {
		t.Errorf("xo.mod changed by a failed get:\n%s", modAfter)
	}
	lockHas("github.com/acme/tick effects fs.read")

	// a lock recorded narrower than the version: every load fails
	data, _ := os.ReadFile(lockFile)
	os.WriteFile(lockFile, []byte(strings.Replace(string(data), "tick effects fs.read", "tick effects none", 1)), 0o644)
	r = xo(t, "check", "--json", app).want(1)
	if got := diagCodes(t, r.out); got != "XO1317" {
		t.Errorf("check with a widened dependency: %s\n%s", got, r.out)
	}
	r = xo(t, "mod", "accept-effects", "--json", "--dir", app).want(0)
	if !strings.Contains(r.out, `"path": "github.com/acme/tick"`) {
		t.Errorf("accept-effects: %s", r.out)
	}
	lockHas("github.com/acme/tick effects fs.read")
	xo(t, "check", app).want(0)

	// --accept-effects on get accepts the upgrade's effects
	xo(t, "get", "--accept-effects", "--dir", app, "github.com/acme/tick@v1.1.0").want(0)
	lockHas("github.com/acme/tick effects fs.read,net.dial")
	// a narrowing is recorded without asking
	xo(t, "get", "--dir", app, "github.com/acme/tick@v1.2.0").want(0)
	lockHas("github.com/acme/tick effects net.dial")

	// a lock written before decision 0156 (no effect lines) is not
	// checked; tidy records the lines
	data, _ = os.ReadFile(lockFile)
	var keep []string
	for _, l := range strings.Split(string(data), "\n") {
		if !strings.Contains(l, " effects ") {
			keep = append(keep, l)
		}
	}
	os.WriteFile(lockFile, []byte(strings.Join(keep, "\n")), 0o644)
	xo(t, "check", app).want(0)
	xo(t, "mod", "tidy", "--dir", app).want(0)
	lockHas("github.com/acme/tick effects net.dial")
	// a malformed effects line is XO1301
	data, _ = os.ReadFile(lockFile)
	os.WriteFile(lockFile, []byte(strings.Replace(string(data), "tick effects net.dial", "tick effects telepathy", 1)), 0o644)
	r = xo(t, "check", "--json", app).want(1)
	if got := diagCodes(t, r.out); got != "XO1301" {
		t.Errorf("malformed effects line: %s", got)
	}
}

// TestTidyUnreadableTree: `xo mod tidy` whose scan of the module's
// imports cannot read a directory stops with XO1318 and leaves xo.mod as
// it was, rather than drop the requirements of the imports it did not
// see (a listing that failed was an empty directory before 2026-10-21).
func TestTidyUnreadableTree(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("root reads a mode 000 directory")
	}
	app := t.TempDir()
	mod := "module example.com/app\nedition preview\n\nrequire github.com/acme/tick v1.0.0\n"
	modtest.WriteFiles(t, app, map[string]string{
		"xo.mod":       mod,
		"main/main.xo": "fn main(os: Os) {}\n",
		"lib/lib.xo":   "use github.com/acme/tick\n\npub fn f() -> Int { tick.pure() }\n",
	})
	lib := filepath.Join(app, "lib")
	if err := os.Chmod(lib, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(lib, 0o755) })
	r := xo(t, "mod", "tidy", "--json", "--dir", app).want(1)
	if got := diagCodes(t, r.out); got != "XO1318" {
		t.Errorf("tidy over an unreadable directory: %s\n%s", got, r.out)
	}
	if data, _ := os.ReadFile(filepath.Join(app, "xo.mod")); string(data) != mod {
		t.Errorf("xo.mod changed:\n%s", data)
	}
}
