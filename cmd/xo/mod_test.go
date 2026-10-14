package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/mod/modtest"
)

type modRun struct {
	t    *testing.T
	code int
	out  string
	err  string
}

func xo(t *testing.T, args ...string) modRun {
	t.Helper()
	var out, errOut bytes.Buffer
	code := run(args, &out, &errOut)
	return modRun{t, code, out.String(), errOut.String()}
}

func (r modRun) want(code int) modRun {
	r.t.Helper()
	if r.code != code {
		r.t.Fatalf("exit %d, want %d\nstdout:\n%s\nstderr:\n%s", r.code, code, r.out, r.err)
	}
	return r
}

func diagCodes(t *testing.T, out string) string {
	t.Helper()
	var res struct {
		Diagnostics []struct {
			Code string `json:"code"`
		} `json:"diagnostics"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("bad JSON: %v\n%s", err, out)
	}
	var cs []string
	for _, d := range res.Diagnostics {
		cs = append(cs, d.Code)
	}
	return strings.Join(cs, ",")
}

const appMain = "use github.com/acme/billing/invoice\n\nfn main(os: Os) {\n  os.stdio.println(\"${invoice.total()}\")\n}\n"

// TestModules drives decision 0086 step 1 and 2 through the xo command:
// require, xo.lock verification, xo mod tidy, xo get, offline builds,
// vendoring, and workspaces, with local git repositories.
func TestModules(t *testing.T) {
	modtest.Setup(t, "../../internal/mod/testdata/repos", modtest.Compare)
	ws := t.TempDir()
	t.Setenv("XO_BUILD_DIR", filepath.Join(ws, ".build"))
	app := filepath.Join(ws, "app")
	modtest.WriteFiles(t, app, map[string]string{
		"xo.mod":       "module example.com/app\nedition preview\n\nrequire github.com/acme/billing v1.2.0\n",
		"main/main.xo": appMain,
	})

	// no xo.lock yet: XO1305 on the require line
	r := xo(t, "check", "--json", app).want(1)
	if got := diagCodes(t, r.out); got != "XO1305,XO1305,XO1305,XO1305" {
		t.Errorf("check without a lock: %s", got)
	}
	xo(t, "mod", "tidy", "--dir", app).want(0)
	r = xo(t, "check", "--json", app).want(0)
	// the imported module resolves through the selected version
	r = xo(t, "query", "def", "--json", "--dir="+app, "cents").want(0)
	if !strings.Contains(r.out, "n * 100") && !strings.Contains(r.out, "money@v1.1.0") {
		t.Errorf("query def cents does not show money v1.1.0:\n%s", r.out)
	}
	r = xo(t, "doc", "--dir="+app, "github.com/acme/billing/invoice").want(0)
	if !strings.Contains(r.out, "total") {
		t.Errorf("doc of a required package:\n%s", r.out)
	}
	// xo edit never changes a required module's files
	r = xo(t, "edit", "--dir="+app, "rename", "total", "grand_total").want(1)
	if !strings.Contains(r.out, "required module github.com/acme/billing@v1.2.0") {
		t.Errorf("edit of a dependency: %s %s", r.out, r.err)
	}
	if !testing.Short() {
		r = xo(t, "run", filepath.Join(app, "main")).want(0)
		if r.out != "200\n" {
			t.Errorf("run with billing v1.2.0 and money v1.1.0 printed %q, want 200", r.out)
		}
	}

	// offline: from the cache; an empty cache is XO1303
	xo(t, "check", "--offline", app).want(0)
	if err := modtest.CleanCache(); err != nil {
		t.Fatal(err)
	}
	r = xo(t, "check", "--offline", "--json", app).want(1)
	if got := diagCodes(t, r.out); !strings.HasPrefix(got, "XO1303") {
		t.Errorf("offline with an empty cache: %s", got)
	}
	if os.Getenv("XO_OFFLINE") != "" {
		t.Errorf("--offline leaked into the environment")
	}

	// vendor: builds use vendor/ with no cache and no network
	xo(t, "vendor", "--dir", app).want(0)
	if err := modtest.CleanCache(); err != nil {
		t.Fatal(err)
	}
	xo(t, "check", "--offline", app).want(0)
	if err := os.RemoveAll(filepath.Join(app, "vendor")); err != nil {
		t.Fatal(err)
	}

	// xo get: a downgrade request; MVS keeps what the graph needs
	xo(t, "get", "--dir", app, "github.com/acme/billing@v1.0.0").want(0)
	r = xo(t, "check", "--json", app).want(0)
	r = xo(t, "get", "--json", "--dir", app, "github.com/acme/billing").want(0)
	if !strings.Contains(r.out, `"new": "v1.2.0"`) {
		t.Errorf("get latest: %s", r.out)
	}

	// an unrequired import is XO1306
	modtest.WriteFiles(t, app, map[string]string{"extra/extra.xo": "use github.com/acme/money\n\npub fn f() -> Int { money.cents(1) }\n"})
	r = xo(t, "check", "--json", app).want(1)
	if got := diagCodes(t, r.out); got != "XO1306" {
		t.Errorf("unrequired import: %s", got)
	}
	os.RemoveAll(filepath.Join(app, "extra"))

	// workspace: a local billing overrides the requirement
	modtest.WriteFiles(t, ws, map[string]string{
		"billing/xo.mod":             "module github.com/acme/billing\nedition preview\n",
		"billing/invoice/invoice.xo": "pub fn total() -> Int { 7 }\n",
		"xo.work":                    "xo preview\nuse ./app\nuse ./billing\n",
	})
	lock, _ := os.ReadFile(filepath.Join(app, "xo.lock"))
	xo(t, "check", app).want(0)
	if !testing.Short() {
		r = xo(t, "run", filepath.Join(app, "main")).want(0)
		if r.out != "7\n" {
			t.Errorf("workspace run printed %q, want 7", r.out)
		}
	}
	r = xo(t, "build", "--ci", "--json", filepath.Join(app, "main")).want(1)
	if got := diagCodes(t, r.out); got != "XO1308" {
		t.Errorf("release build in a workspace: %s", got)
	}
	if !strings.Contains(r.out, "github.com/acme/billing") {
		t.Errorf("XO1308 does not name the overridden module: %s", r.out)
	}
	// --release is a release build too (core 8)
	r = xo(t, "build", "--release", "--json", filepath.Join(app, "main")).want(1)
	if got := diagCodes(t, r.out); got != "XO1308" {
		t.Errorf("--release build in a workspace: %s", got)
	}
	// a workspace is one program: a rename reaches both modules
	r = xo(t, "edit", "--dir="+app, "rename", "total", "grand_total").want(0)
	for _, f := range []string{filepath.Join(ws, "billing", "invoice", "invoice.xo"), filepath.Join(app, "main", "main.xo")} {
		data, _ := os.ReadFile(f)
		if !strings.Contains(string(data), "grand_total") {
			t.Errorf("%s not renamed:\n%s", f, data)
		}
	}
	after, _ := os.ReadFile(filepath.Join(app, "xo.lock"))
	if string(lock) != string(after) {
		t.Errorf("xo.lock changed in a workspace")
	}
	// --workspace=off: the required v1.2.0 again, which has no grand_total
	r = xo(t, "check", "--workspace=off", "--json", app).want(1)
	if got := diagCodes(t, r.out); !strings.Contains(got, "XO0601") {
		t.Errorf("--workspace=off: %s", got)
	}
}
