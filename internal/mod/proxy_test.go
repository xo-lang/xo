package mod_test

import (
	"archive/zip"
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/mod"
	"github.com/xo-lang/xo/internal/mod/modtest"
)

// proxy starts a caching proxy (it fetches from the test repositories
// with git) on loopback and counts its requests.
func proxy(t *testing.T, fetch bool) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	root := t.TempDir()
	t.Cleanup(func() { modtest.MakeWritable(root) })
	var n atomic.Int64
	h := mod.ProxyHandler(root, fetch)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		h.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv, &n
}

// freshCache points XO_MODCACHE at an empty cache.
func freshCache(t *testing.T) {
	t.Helper()
	c := t.TempDir()
	t.Cleanup(func() { modtest.MakeWritable(c) })
	t.Setenv("XO_MODCACHE", c)
}

// TestProxy: modules come through XO_PROXY with the same xo.lock hashes
// as from git; list, latest, fallback, off, and XO_NOPROXY.
func TestProxy(t *testing.T) {
	setup(t)
	dir := app(t, "\nrequire github.com/acme/billing v1.2.0\nrequire github.com/acme/money v1.0.0\n", uses("billing", "money"))
	if _, ps := mod.Tidy(dir); len(ps) > 0 {
		t.Fatal(ps)
	}
	direct, _ := os.ReadFile(filepath.Join(dir, "xo.lock"))

	srv, n := proxy(t, true)
	freshCache(t)
	t.Setenv("XO_PROXY", srv.URL)
	b := mod.Load(dir, mod.Options{})
	if len(b.Problems) > 0 {
		t.Fatalf("through the proxy: %v", b.Problems)
	}
	if n.Load() == 0 {
		t.Fatal("the proxy was not asked")
	}
	if got := selected(b); got != "github.com/acme/billing v1.2.0, github.com/acme/money v1.1.0" {
		t.Errorf("selected %s", got)
	}
	// the same hashes as the trees from git: tidy keeps the lock
	if _, ps := mod.Tidy(dir); len(ps) > 0 {
		t.Fatal(ps)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "xo.lock")); string(got) != string(direct) {
		t.Errorf("lock through the proxy differs:\n%s\nfrom git:\n%s", got, direct)
	}
	if v, err := mod.Latest("github.com/acme/money"); err != nil || v != "v1.3.0" {
		t.Errorf("Latest through the proxy = %s, %v", v, err)
	}

	// an empty proxy answers 404: a comma falls back to direct
	empty, _ := proxy(t, false)
	freshCache(t)
	t.Setenv("XO_PROXY", empty.URL)
	b = mod.Load(dir, mod.Options{})
	if got := strings.Join(codes(b.Problems), ","); !strings.Contains(got, "XO1302") || !strings.Contains(b.Problems[0].Msg, "not in this proxy") {
		t.Fatalf("empty proxy alone: %v", b.Problems)
	}
	t.Setenv("XO_PROXY", empty.URL+",direct")
	if b = mod.Load(dir, mod.Options{}); len(b.Problems) > 0 {
		t.Fatalf("empty proxy, direct: %v", b.Problems)
	}

	// a failing proxy: no fallback after a comma, fallback after a pipe
	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "down for maintenance", http.StatusInternalServerError)
	}))
	defer broken.Close()
	freshCache(t)
	t.Setenv("XO_PROXY", broken.URL+",direct")
	if b = mod.Load(dir, mod.Options{}); !strings.Contains(strings.Join(codes(b.Problems), ","), "XO1302") {
		t.Fatalf("broken proxy, comma: %v", b.Problems)
	}
	t.Setenv("XO_PROXY", broken.URL+"|direct")
	if b = mod.Load(dir, mod.Options{}); len(b.Problems) > 0 {
		t.Fatalf("broken proxy, pipe: %v", b.Problems)
	}

	// off, and XO_NOPROXY going direct around it
	freshCache(t)
	t.Setenv("XO_PROXY", "off")
	b = mod.Load(dir, mod.Options{})
	if len(b.Problems) == 0 || b.Problems[0].Code != diag.ModuleFetch || !strings.Contains(b.Problems[0].Msg, "XO_PROXY=off") {
		t.Fatalf("off: %v", b.Problems)
	}
	t.Setenv("XO_NOPROXY", "github.com/acme")
	if b = mod.Load(dir, mod.Options{}); len(b.Problems) > 0 {
		t.Fatalf("XO_NOPROXY: %v", b.Problems)
	}
	t.Setenv("XO_NOPROXY", "")

	// malformed settings
	for _, bad := range []string{"ftp://proxy.example", "direct,", "https://p.example,,direct"} {
		freshCache(t)
		t.Setenv("XO_PROXY", bad)
		if b = mod.Load(dir, mod.Options{}); len(b.Problems) == 0 || b.Problems[0].Code != diag.FetchSettings {
			t.Errorf("XO_PROXY=%s: %v", bad, b.Problems)
		}
	}
}

// TestProxyUntrusted: a proxy that serves other content than the tag
// fails xo.lock (XO1304); one serving a zip with an escaping path, or a
// tree that contradicts its own .lock, fails to fetch (XO1302).
func TestProxyUntrusted(t *testing.T) {
	setup(t)
	dir := app(t, "\nrequire github.com/acme/money v1.1.0\n", uses("money"))
	if _, ps := mod.Tidy(dir); len(ps) > 0 {
		t.Fatal(ps)
	}
	evilZip := func(files map[string]string) []byte {
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		for name, body := range files {
			w, _ := zw.Create(name)
			w.Write([]byte(body))
		}
		zw.Close()
		return buf.Bytes()
	}
	var zipBody []byte
	var lockBody string
	evil := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, ".zip"):
			w.Write(zipBody)
		case strings.HasSuffix(r.URL.Path, ".lock") && lockBody != "":
			w.Write([]byte(lockBody))
		default:
			http.NotFound(w, r)
		}
	}))
	defer evil.Close()
	t.Setenv("XO_PROXY", evil.URL)
	const pre = "github.com/acme/money@v1.1.0/"
	good := map[string]string{pre + "xo.mod": "module github.com/acme/money\nedition 2026\n", pre + "money.xo": "pub fn cents(n: Int) -> Int { n * 1000 }\n"}

	freshCache(t)
	zipBody = evilZip(good)
	b := mod.Load(dir, mod.Options{})
	if got := strings.Join(codes(b.Problems), ","); got != "XO1304" {
		t.Fatalf("other content: %v, want XO1304", b.Problems)
	}

	freshCache(t)
	bad := map[string]string{pre + "xo.mod": good[pre+"xo.mod"], pre + "../../escape.xo": "x"}
	zipBody = evilZip(bad)
	b = mod.Load(dir, mod.Options{})
	if len(b.Problems) == 0 || b.Problems[0].Code != diag.ModuleFetch || !strings.Contains(b.Problems[0].Msg, "leaves the module") {
		t.Fatalf("escaping zip: %v", b.Problems)
	}

	freshCache(t)
	zipBody = evilZip(good)
	lockBody = "github.com/acme/money v1.1.0 h1:AAAA\n"
	b = mod.Load(dir, mod.Options{})
	if len(b.Problems) == 0 || b.Problems[0].Code != diag.ModuleFetch || !strings.Contains(b.Problems[0].Msg, "the proxy's .lock says") {
		t.Fatalf("inconsistent proxy: %v", b.Problems)
	}
}

// TestProxyServe: the protocol's files, escaping, and read only methods.
func TestProxyServe(t *testing.T) {
	setup(t)
	srv, _ := proxy(t, true)
	get := func(p string) (int, string) {
		resp, err := http.Get(srv.URL + p)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var buf bytes.Buffer
		buf.ReadFrom(resp.Body)
		return resp.StatusCode, buf.String()
	}
	if code, body := get("/github.com/acme/money/@v/list"); code != 200 || body != "v1.0.0\nv1.1.0\nv1.3.0\n" {
		t.Errorf("list: %d %q", code, body)
	}
	if code, body := get("/github.com/acme/money/@v/v1.1.0.mod"); code != 200 || !strings.HasPrefix(body, "module github.com/acme/money") {
		t.Errorf(".mod: %d %q", code, body)
	}
	if code, body := get("/github.com/acme/money/@v/v1.1.0.lock"); code != 200 || !strings.Contains(body, "github.com/acme/money v1.1.0/xo.mod h1:") {
		t.Errorf(".lock: %d %q", code, body)
	}
	code1, zip1 := get("/github.com/acme/money/@v/v1.1.0.zip")
	_, zip2 := get("/github.com/acme/money/@v/v1.1.0.zip")
	if code1 != 200 || zip1 != zip2 {
		t.Errorf(".zip: %d, deterministic %v", code1, zip1 == zip2)
	}
	for _, p := range []string{"/github.com/acme/money/@v/v9.9.9.zip", "/github.com/acme/nothing/@v/list", "/github.com/Acme/money/@v/list", "/github.com/acme/money/@v/v1.1.0.info"} {
		if code, _ := get(p); code != 404 {
			t.Errorf("%s: %d, want 404", p, code)
		}
	}
	resp, err := http.Post(srv.URL+"/github.com/acme/money/@v/list", "text/plain", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("POST: %d", resp.StatusCode)
	}
	for p, want := range map[string]bool{"github.com/acme/private/x": true, "github.com/acme": false, "corp.example.com/a": true, "example.com/a": false} {
		if got := mod.MatchPatterns("github.com/acme/private, *.example.com", p); got != want {
			t.Errorf("MatchPatterns(%s) = %v", p, got)
		}
	}
}
