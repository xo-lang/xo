package mod_test

import (
	"crypto/rand"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/mod"
	"github.com/xo-lang/xo/internal/mod/modtest"
	"github.com/xo-lang/xo/internal/sumdb"
)

// checksumLog starts a checksum log on loopback that records the module
// versions it is asked about from the test repositories, and returns the
// server, its URL, and the XO_SUMDB value naming it.
func checksumLog(t *testing.T) (*sumdb.Server, string, string) {
	t.Helper()
	signer, verifier, err := sumdb.GenerateKey("sum.test", rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	s, err := sumdb.ParseSigner(signer)
	if err != nil {
		t.Fatal(err)
	}
	cache := t.TempDir()
	t.Cleanup(func() { modtest.MakeWritable(cache) })
	srv := sumdb.NewServer(s, func(path, version string) ([]byte, error) {
		return mod.LogRecord(cache, path, version)
	})
	hs := httptest.NewServer(srv)
	t.Cleanup(hs.Close)
	return srv, hs.URL, verifier + " " + hs.URL
}

// TestChecksumLog: first fetches are verified against the log (decision
// 0124). A tag moved after the log recorded it is XO1313 for a client
// with no xo.lock, a private module (XO_NOSUMDB) is not checked, an
// unreachable log or a malformed XO_SUMDB is XO1314.
func TestChecksumLog(t *testing.T) {
	env := setup(t)
	srv, url, cfg := checksumLog(t)
	dir := app(t, "\nrequire github.com/acme/billing v1.2.0\nrequire github.com/acme/money v1.0.0\n", uses("billing", "money"))

	freshCache(t)
	t.Setenv("XO_SUMDB", cfg)
	if _, ps := mod.Tidy(dir); len(ps) > 0 {
		t.Fatalf("tidy with the log: %v", ps)
	}
	if srv.Size() == 0 {
		t.Fatal("the log was not consulted")
	}
	// the module cache remembers the tree head; a second fetch of other
	// versions must extend it
	if _, ps := mod.Get(dir, "github.com/acme/money@v1.3.0"); len(ps) > 0 {
		t.Fatalf("get with the log: %v", ps)
	}

	// money v1.3.0 is moved to other content after the log recorded it;
	// a new client without an xo.lock entry is stopped by the log
	other := t.TempDir()
	modtest.WriteFiles(t, other, map[string]string{"xo.mod": "module github.com/acme/money\nedition 2026\n", "money.xo": "pub fn cents(n: Int) -> Int { n * 7 }\n"})
	env.Commit(t, "github.com/acme/money", "v1.3.0", other, true)
	fresh := app(t, "\nrequire github.com/acme/money v1.3.0\n", uses("money"))
	freshCache(t)
	b := mod.Load(fresh, mod.Options{Update: true})
	if len(b.Problems) != 1 || b.Problems[0].Code != diag.ChecksumLog || !strings.Contains(b.Problems[0].Msg, "the checksum log sum.test records") {
		t.Fatalf("moved tag: %v", b.Problems)
	}
	if _, err := mod.Latest("github.com/acme/money"); err != nil {
		t.Fatal(err)
	}

	// a private module is not looked up
	t.Setenv("XO_NOSUMDB", "github.com/acme/money")
	freshCache(t)
	if b = mod.Load(fresh, mod.Options{Update: true}); len(b.Problems) > 0 {
		t.Fatalf("XO_NOSUMDB: %v", b.Problems)
	}
	t.Setenv("XO_NOSUMDB", "")

	// an unreachable log
	freshCache(t)
	t.Setenv("XO_SUMDB", strings.Replace(cfg, url, "http://127.0.0.1:1", 1))
	b = mod.Load(dir, mod.Options{})
	if len(b.Problems) == 0 || b.Problems[0].Code != diag.FetchSettings {
		t.Fatalf("unreachable log: %v", b.Problems)
	}
	// a key that is not the log's
	_, wrong, _ := sumdb.GenerateKey("sum.test", rand.Reader)
	freshCache(t)
	t.Setenv("XO_SUMDB", wrong+" "+url)
	b = mod.Load(dir, mod.Options{})
	if len(b.Problems) == 0 || b.Problems[0].Code != diag.ChecksumLog {
		t.Fatalf("wrong key: %v", b.Problems)
	}
	// malformed settings
	freshCache(t)
	t.Setenv("XO_SUMDB", "sum.test+nothex+AAAA")
	b = mod.Load(dir, mod.Options{})
	if len(b.Problems) == 0 || b.Problems[0].Code != diag.FetchSettings {
		t.Fatalf("malformed XO_SUMDB: %v", b.Problems)
	}
	// off: the moved tag is still caught by xo.lock where it is pinned
	freshCache(t)
	t.Setenv("XO_SUMDB", "off")
	if b = mod.Load(dir, mod.Options{}); strings.Join(codes(b.Problems), ",") != "XO1304" {
		t.Fatalf("XO_SUMDB=off: %v", b.Problems)
	}
}

// TestChecksumLogThroughProxy: the log checks trees that come from a
// proxy too; a proxy serving other content than the log recorded is
// XO1313 even for a version xo.lock does not pin yet.
func TestChecksumLogThroughProxy(t *testing.T) {
	setup(t)
	_, _, cfg := checksumLog(t)
	// the log records money v1.1.0 from git first
	freshCache(t)
	t.Setenv("XO_SUMDB", cfg)
	if _, p := mod.Download("github.com/acme/money", "v1.1.0"); p != nil {
		t.Fatal(p)
	}
	srv, _ := proxy(t, true)
	freshCache(t)
	t.Setenv("XO_PROXY", srv.URL)
	if _, p := mod.Download("github.com/acme/money", "v1.1.0"); p != nil {
		t.Fatalf("through the proxy: %v", p)
	}
	if _, p := mod.Download("github.com/acme/money", "v1.3.0"); p != nil {
		t.Fatalf("a version the log records on demand: %v", p)
	}
}
