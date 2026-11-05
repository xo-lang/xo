package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/mod/modtest"
)

// TestGetChecksFetchedVersion (decision 0517, the owner's choice (b)): a
// build checks a dependency's bodies only as it reaches them, so `xo get`
// checks each newly required version in full once and warns about its
// errors; the requirement is still added (exit 0), and a version checked
// before is not checked again.
func TestGetChecksFetchedVersion(t *testing.T) {
	t.Parallel()
	src := t.TempDir()
	mod := "module github.com/acme/flaky\nedition preview\n"
	good := "pub fn seven() -> Int {\n  7\n}\n"
	modtest.WriteFiles(t, src, map[string]string{
		"github.com/acme/flaky@v1.0.0/xo.mod": mod,
		"github.com/acme/flaky@v1.0.0/f/f.xo": good + "\npub fn unused() -> Int {\n  \"not an Int\"\n}\n",
		"github.com/acme/flaky@v1.1.0/xo.mod": mod,
		"github.com/acme/flaky@v1.1.0/f/f.xo": good,
	})
	modSetup(t, src)
	ws := t.TempDir()
	app := filepath.Join(ws, "app")
	modtest.WriteFiles(t, app, map[string]string{
		"xo.mod":       "module example.com/app\nedition preview\n",
		"main/main.xo": "use github.com/acme/flaky/f\n\nfn main(os: Os) {\n  os.stdio.println(\"${f.seven()}\")\n}\n",
	})
	r := xo(t, "get", "--dir", app, "github.com/acme/flaky@v1.0.0").want(0)
	if !strings.Contains(r.err, "flaky@v1.0.0 has errors") || !strings.Contains(r.err, "f.xo") {
		t.Errorf("get of a version with an error: want the warning naming it:\n%s", r.err)
	}
	// the record: the same version required again is not checked again
	modtest.WriteFiles(t, app, map[string]string{"xo.mod": "module example.com/app\nedition preview\n"})
	os.Remove(filepath.Join(app, "xo.lock"))
	r = xo(t, "get", "--dir", app, "github.com/acme/flaky@v1.0.0").want(0)
	if strings.Contains(r.err, "has errors") {
		t.Errorf("get of a version checked before: warned again:\n%s", r.err)
	}
	r = xo(t, "get", "--dir", app, "github.com/acme/flaky@v1.1.0").want(0)
	if strings.Contains(r.err, "has errors") {
		t.Errorf("get of a clean version: warned:\n%s", r.err)
	}
}
