package main

import (
	"crypto/rand"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/xo-lang/xo/internal/mod"
	"github.com/xo-lang/xo/internal/mod/modtest"
	"github.com/xo-lang/xo/internal/sumdb"
)

// emptyCache points XO_MODCACHE at a new, empty module cache.
func emptyCache(t *testing.T) {
	t.Helper()
	c := t.TempDir()
	t.Cleanup(func() { modtest.MakeWritable(c) })
	t.Setenv("XO_MODCACHE", c)
}

// TestModProxy: xo mod tidy and xo check fetch through a caching module
// proxy (decision 0126) on loopback, never from git themselves.
func TestModProxy(t *testing.T) {
	modtest.Setup(t, "../../internal/mod/testdata/repos", mod.Compare)
	ws := t.TempDir()
	app := filepath.Join(ws, "app")
	modtest.WriteFiles(t, app, map[string]string{
		"xo.mod":       "module example.com/app\nedition preview\n\nrequire github.com/acme/billing v1.2.0\n",
		"main/main.xo": appMain,
	})
	root := t.TempDir()
	t.Cleanup(func() { modtest.MakeWritable(root) })
	srv := httptest.NewServer(mod.ProxyHandler(root, true))
	defer srv.Close()

	emptyCache(t)
	t.Setenv("XO_PROXY", srv.URL)
	xo(t, "mod", "tidy", "--dir", app).want(0)
	xo(t, "check", app).want(0)
	xo(t, "get", "--dir", app, "github.com/acme/money@v1.3.0").want(0)
	// a second client with an empty cache, and the repositories out of
	// reach (the proxy in this process shares XO_GIT_BASE, so it now
	// serves only what it cached)
	emptyCache(t)
	t.Setenv("XO_GIT_BASE", filepath.Join(ws, "no-repositories"))
	xo(t, "check", app).want(0)

	t.Setenv("XO_PROXY", "nonsense")
	emptyCache(t)
	r := xo(t, "check", "--json", app).want(1)
	if got := diagCodes(t, r.out); got == "" || got[:6] != "XO1314" {
		t.Errorf("malformed XO_PROXY: %s", got)
	}
	xo(t, "mod", "proxy").want(2)
	xo(t, "mod", "proxy", "serve", "a", "b").want(2)
}

// TestChecksumLogCommand: xo mod tidy checks first fetches against a
// checksum log on loopback (decision 0124); a tag moved after the log
// recorded it is XO1313.
func TestChecksumLogCommand(t *testing.T) {
	env := modtest.Setup(t, "../../internal/mod/testdata/repos", mod.Compare)
	signer, verifier, err := sumdb.GenerateKey("sum.test", rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	s, _ := sumdb.ParseSigner(signer)
	logCache := t.TempDir()
	t.Cleanup(func() { modtest.MakeWritable(logCache) })
	srv := httptest.NewServer(sumdb.NewServer(s, func(path, version string) ([]byte, error) {
		return mod.LogRecord(logCache, path, version)
	}))
	defer srv.Close()
	t.Setenv("XO_SUMDB", verifier+" "+srv.URL)

	ws := t.TempDir()
	app := filepath.Join(ws, "app")
	modtest.WriteFiles(t, app, map[string]string{
		"xo.mod":       "module example.com/app\nedition preview\n\nrequire github.com/acme/billing v1.2.0\n",
		"main/main.xo": appMain,
	})
	emptyCache(t)
	xo(t, "mod", "tidy", "--dir", app).want(0)
	xo(t, "check", app).want(0)

	other := t.TempDir()
	modtest.WriteFiles(t, other, map[string]string{"xo.mod": "module github.com/acme/money\nedition preview\n", "money.xo": "pub fn cents(n: Int) -> Int { n * 7 }\n"})
	env.Commit(t, "github.com/acme/money", "v1.1.0", other, true)
	os.Remove(filepath.Join(app, "xo.lock"))
	emptyCache(t)
	r := xo(t, "mod", "tidy", "--json", "--dir", app).want(1)
	if got := diagCodes(t, r.out); got != "XO1313" {
		t.Errorf("moved tag under the log: %s\n%s", got, r.out)
	}
}
