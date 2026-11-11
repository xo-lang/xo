package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/xo-lang/xo/internal/mod/modtest"
	"github.com/xo-lang/xo/internal/xotool"
)

// fakeS3 is a path style S3-compatible bucket store: a PUT must carry a
// valid AWS Signature Version 4 (checked here with Go's crypto, apart
// from xo's signer), a GET or HEAD reads an object as a public bucket
// served over a custom domain does, with its Cache-Control and
// Content-Type, and a signed GET must verify too.
type fakeS3 struct {
	keyID, secret, region string

	mu      sync.Mutex
	objects map[string]fakeObject
	puts    int
	down    bool // every PUT answers 503 (an R2 outage)
}

func (s *fakeS3) setDown(down bool) {
	s.mu.Lock()
	s.down = down
	s.mu.Unlock()
}

type fakeObject struct {
	body                 []byte
	contentType, caching string
}

func hmacSHA256(key []byte, msg string) []byte {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(msg))
	return m.Sum(nil)
}

// verify checks r's SigV4 Authorization over host, x-amz-content-sha256
// and x-amz-date, and that the payload hash is the body's.
func (s *fakeS3) verify(r *http.Request, body []byte) string {
	auth := r.Header.Get("Authorization")
	date := r.Header.Get("X-Amz-Date")
	payload := r.Header.Get("X-Amz-Content-Sha256")
	sum := sha256.Sum256(body)
	if payload != hex.EncodeToString(sum[:]) {
		return "x-amz-content-sha256 is not the body's hash"
	}
	t, err := time.Parse("20060102T150405Z", date)
	if err != nil || time.Since(t).Abs() > 15*time.Minute {
		return "x-amz-date " + date + " is malformed or not now"
	}
	day := date[:8]
	scope := day + "/" + s.region + "/s3/aws4_request"
	canonical := r.Method + "\n" + r.URL.EscapedPath() + "\n\nhost:" + r.Host + "\nx-amz-content-sha256:" + payload + "\nx-amz-date:" + date + "\n\nhost;x-amz-content-sha256;x-amz-date\n" + payload
	ch := sha256.Sum256([]byte(canonical))
	toSign := "AWS4-HMAC-SHA256\n" + date + "\n" + scope + "\n" + hex.EncodeToString(ch[:])
	k := hmacSHA256([]byte("AWS4"+s.secret), day)
	k = hmacSHA256(k, s.region)
	k = hmacSHA256(k, "s3")
	k = hmacSHA256(k, "aws4_request")
	want := "AWS4-HMAC-SHA256 Credential=" + s.keyID + "/" + scope + ", SignedHeaders=host;x-amz-content-sha256;x-amz-date, Signature=" + hex.EncodeToString(hmacSHA256(k, toSign))
	if auth != want {
		return "SignatureDoesNotMatch: got " + auth
	}
	return ""
}

func (s *fakeS3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPut:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		if msg := s.verify(r, body); msg != "" {
			http.Error(w, msg, http.StatusForbidden)
			return
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.down {
			http.Error(w, "ServiceUnavailable", http.StatusServiceUnavailable)
			return
		}
		s.objects[r.URL.Path] = fakeObject{body, r.Header.Get("Content-Type"), r.Header.Get("Cache-Control")}
		s.puts++
	case http.MethodGet, http.MethodHead:
		// public reads carry no signature; a signed read must verify
		if r.Header.Get("Authorization") != "" {
			if msg := s.verify(r, nil); msg != "" {
				http.Error(w, msg, http.StatusForbidden)
				return
			}
		}
		s.mu.Lock()
		o, ok := s.objects[r.URL.Path]
		s.mu.Unlock()
		if !ok {
			http.Error(w, "NoSuchKey", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", o.contentType)
		w.Header().Set("Cache-Control", o.caching)
		w.Write(o.body)
	default:
		http.Error(w, "method", http.StatusMethodNotAllowed)
	}
}

func (s *fakeS3) object(path string) (fakeObject, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	o, ok := s.objects[path]
	return o, ok
}

// TestProxyFill: `xo mod proxy fill` (decision 0568, phase 1) fills an
// S3-compatible bucket with signed PUTs, checking each version against
// the checksum log; the bucket served as static files is a module proxy
// that a client with no git in reach builds from, verified by the log; a
// wrong secret and a tag moved under the log fill nothing.
func TestProxyFill(t *testing.T) {
	xotool.NeedPorts(t)
	t.Parallel()
	env := modSetup(t, "../../internal/mod/testdata/repos")
	ws := t.TempDir()
	app := filepath.Join(ws, "app")
	modtest.WriteFiles(t, app, map[string]string{
		"xo.mod":       "module example.com/app\nedition preview\n\nrequire github.com/acme/billing v1.2.0\n",
		"main/main.xo": appMain,
	})
	key := filepath.Join(ws, "log.key")
	verifier := strings.TrimSpace(xo(t, "mod", "sumdb", "keygen", "sum.test", key).want(0).out)
	logData := t.TempDir()
	t.Cleanup(func() { modtest.MakeWritable(logData) })
	logURL, _ := startHTTPServe(t, "mod", "sumdb", "serve", "--key="+key, "--fetch", logData)
	setenv(t, "XO_SUMDB", verifier+" "+logURL)

	s3 := &fakeS3{keyID: "test-key", secret: "test-secret", region: "auto", objects: map[string]fakeObject{}}
	srv := httptest.NewServer(s3)
	defer srv.Close()
	bucket := srv.URL + "/modules"
	setenv(t, "XO_FILL_ACCESS_KEY_ID", s3.keyID)
	setenv(t, "XO_FILL_SECRET_ACCESS_KEY", s3.secret)

	emptyCache(t) // the fill service's own cache
	xo(t, "mod", "proxy", "fill", "--to="+bucket, "github.com/acme/billing@v1.2.0", "github.com/acme/money@v1.1.0").want(0)
	for _, f := range []string{"github.com/acme/billing/@v/v1.2.0.zip", "github.com/acme/billing/@v/v1.2.0.lock", "github.com/acme/billing/@v/v1.2.0.mod", "github.com/acme/money/@v/v1.1.0.zip"} {
		o, ok := s3.object("/modules/" + f)
		if !ok || o.caching != "public, max-age=31536000, immutable" {
			t.Errorf("%s: present %v, Cache-Control %q", f, ok, o.caching)
		}
	}
	if o, ok := s3.object("/modules/github.com/acme/billing/@v/list"); !ok || o.caching != "public, max-age=60" || !strings.Contains(string(o.body), "v1.0.0\nv1.2.0\n") {
		t.Errorf("billing's list: present %v, Cache-Control %q, body %q", ok, o.caching, o.body)
	}

	// a wrong secret: refused, nothing written
	n := s3.puts
	setenv(t, "XO_FILL_SECRET_ACCESS_KEY", "wrong")
	r := xo(t, "mod", "proxy", "fill", "--to="+bucket, "github.com/acme/money@v1.3.0").want(1)
	if !strings.Contains(r.err, "SignatureDoesNotMatch") || s3.puts != n {
		t.Errorf("wrong secret: %d puts, stderr %s", s3.puts-n, r.err)
	}
	setenv(t, "XO_FILL_SECRET_ACCESS_KEY", s3.secret)
	xo(t, "mod", "proxy", "fill", "--to=ftp://x", "a.com/m").want(2)

	// a client with no git in reach builds from the bucket, checked by the log
	gitBase := getenv(t, "XO_GIT_BASE")
	setenv(t, "XO_PROXY", bucket)
	setenv(t, "XO_GIT_BASE", filepath.Join(ws, "no-repositories"))
	emptyCache(t)
	xo(t, "mod", "tidy", "--dir", app).want(0)
	xo(t, "check", app+"/...").want(0)
	setenv(t, "XO_PROXY", "direct")
	setenv(t, "XO_GIT_BASE", gitBase)

	// a tag moved after the log recorded it: the fill refuses it
	other := t.TempDir()
	modtest.WriteFiles(t, other, map[string]string{"xo.mod": "module github.com/acme/money\nedition preview\n", "money.xo": "pub fn cents(n: Int) -> Int { n * 7 }\n"})
	env.Commit(t, "github.com/acme/money", "v1.1.0", other, true)
	emptyCache(t)
	n = s3.puts
	r = xo(t, "mod", "proxy", "fill", "--to="+bucket, "github.com/acme/money@v1.1.0").want(1)
	if !strings.Contains(r.err, "XO1313") || s3.puts != n {
		t.Errorf("moved tag: %d puts, stderr %s", s3.puts-n, r.err)
	}
}

// get is an HTTP GET of url with extra headers: the status, the
// Cache-Control and the body.
func get(t *testing.T, url string, headers map[string]string) (int, string, string) {
	t.Helper()
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, resp.Header.Get("Cache-Control"), string(b)
}

// TestChecksumLogOps: the log's phase 1 operations (decision 0568): each
// version the log adds is written to the proxy bucket (--fill-to); each
// new record is copied to a bucket before it is added and `xo mod sumdb
// restore` rebuilds the same log from the copies; the index feed lists
// records by time; lookups that add a version are limited per client
// (Cloudflare's CF-Connecting-IP behind a local tunnel); the head is
// never cached and proofs are.
func TestChecksumLogOps(t *testing.T) {
	xotool.NeedPorts(t)
	t.Parallel()
	modSetup(t, "../../internal/mod/testdata/repos")
	ws := t.TempDir()
	app := filepath.Join(ws, "app")
	modtest.WriteFiles(t, app, map[string]string{
		"xo.mod":       "module example.com/app\nedition preview\n\nrequire github.com/acme/billing v1.2.0\n",
		"main/main.xo": appMain,
	})
	key := filepath.Join(ws, "log.key")
	verifier := strings.TrimSpace(xo(t, "mod", "sumdb", "keygen", "sum.test", key).want(0).out)
	s3 := &fakeS3{keyID: "test-key", secret: "test-secret", region: "auto", objects: map[string]fakeObject{}}
	srv := httptest.NewServer(s3)
	defer srv.Close()
	bucket := srv.URL + "/logcopy"
	setenv(t, "XO_FILL_ACCESS_KEY_ID", s3.keyID)
	setenv(t, "XO_FILL_SECRET_ACCESS_KEY", s3.secret)
	logData := t.TempDir()
	t.Cleanup(func() { modtest.MakeWritable(logData) })
	logURL, stopLog := startHTTPServe(t, "mod", "sumdb", "serve", "--key="+key, "--fetch", "--copy-to="+bucket, "--fill-to="+srv.URL+"/modules", "--rate=2", logData)
	setenv(t, "XO_SUMDB", verifier+" "+logURL)

	// the rate limit counts per clock minute (sumserve.xo allow, unix
	// time / 60): the two adds below and the third lookup must fall in one
	// minute, so start with at least 20 seconds of this one left (a
	// boundary between them let the third through, 2026-11-10's seed cut)
	if left := 60 - time.Now().Unix()%60; left < 20 {
		time.Sleep(time.Duration(left)*time.Second + 100*time.Millisecond)
	}
	minute := time.Now().Unix() / 60
	emptyCache(t)
	xo(t, "mod", "tidy", "--dir", app).want(0) // adds billing and money: this client's 2

	// a third new version from this client this minute waits; asked at
	// once, before the fill and the check below
	code3, _, body3 := get(t, logURL+"/lookup/github.com/acme/money@v1.3.0", nil)
	if now := time.Now().Unix() / 60; now != minute {
		t.Logf("a minute boundary passed during tidy: 429 not checked (got %d)", code3)
	} else if code3 != 429 {
		t.Errorf("third new version from one client: %d %s", code3, body3)
	}
	for _, id := range []string{"0000000000", "0000000001"} {
		o, ok := s3.object("/logcopy/records/" + id)
		if !ok || !strings.Contains(string(o.body), "\ngithub.com/acme/") {
			t.Errorf("record copy %s: present %v, %q", id, ok, o.body)
		}
	}

	// --fill-to: the versions the log added reach the proxy bucket (in the
	// background), and a client with no git in reach builds from it
	for _, f := range []string{"/modules/github.com/acme/billing/@v/v1.2.0.mod", "/modules/github.com/acme/money/@v/v1.1.0.mod", "/modules/github.com/acme/money/@v/list"} {
		deadline := time.Now().Add(20 * time.Second)
		for {
			if _, ok := s3.object(f); ok {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("the log did not fill %s", f)
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
	gitBase := getenv(t, "XO_GIT_BASE")
	setenv(t, "XO_PROXY", srv.URL+"/modules")
	setenv(t, "XO_GIT_BASE", filepath.Join(ws, "no-repositories"))
	emptyCache(t)
	xo(t, "check", app+"/...").want(0)
	setenv(t, "XO_PROXY", "direct")
	setenv(t, "XO_GIT_BASE", gitBase)

	// the rate limit: a new version through the tunnel from another
	// address is not limited
	if code, _, body := get(t, logURL+"/lookup/github.com/acme/money@v1.3.0", map[string]string{"CF-Connecting-IP": "192.0.2.7"}); code != 200 {
		t.Errorf("a new version from another client: %d %s", code, body)
	}
	if code, _, _ := get(t, logURL+"/lookup/github.com/acme/billing@v1.2.0", nil); code != 200 {
		t.Errorf("a version the log holds is not limited: %d", code)
	}

	// caching and the index
	code, cache, head := get(t, logURL+"/latest", nil)
	if code != 200 || cache != "no-store" {
		t.Errorf("/latest: %d, Cache-Control %q", code, cache)
	}
	if _, cache, _ := get(t, logURL+"/proof/consistency/1/3", nil); cache != "public, max-age=31536000, immutable" {
		t.Errorf("a proof's Cache-Control %q", cache)
	}
	code, _, index := get(t, logURL+"/index", nil)
	if code != 200 || strings.Count(index, "\n") != 3 || !strings.Contains(index, `{"Path":"github.com/acme/money","Version":"v1.3.0","Timestamp":"`) {
		t.Errorf("/index: %d\n%s", code, index)
	}
	if _, _, later := get(t, logURL+"/index?since=2200-01-01T00:00:00Z", nil); later != "" {
		t.Errorf("/index after every record: %q", later)
	}
	if code, _, _ := get(t, logURL+"/index?since=yesterday", nil); code != 400 {
		t.Errorf("/index with a bad since: %d", code)
	}

	// restore: the same records, times and tree from the copies
	stopLog()
	restored := filepath.Join(ws, "restored")
	xo(t, "mod", "sumdb", "restore", "--from="+bucket, restored).want(0)
	xo(t, "mod", "sumdb", "restore", "--from="+bucket, restored).want(1) // never over a log
	for _, f := range []string{"records", "times"} {
		a, _ := os.ReadFile(filepath.Join(logData, f))
		b, err := os.ReadFile(filepath.Join(restored, f))
		if err != nil || string(a) != string(b) {
			t.Errorf("restored %s differs: %v\n%q\n%q", f, err, a, b)
		}
	}
	logURL, _ = startHTTPServe(t, "mod", "sumdb", "serve", "--key="+key, restored)
	if _, _, again := get(t, logURL+"/latest", nil); again != head {
		t.Errorf("the restored log's head:\n%s\nwant\n%s", again, head)
	}
}

// TestChecksumLogRefresh: a new tag reaches the proxy bucket's @v/list
// without a lookup of it (decision 0568): `GET /refresh/<path>` rewrites
// the list of a module the log holds at once, and only when it changed;
// --refresh rewrites every module's list on its interval. A stale list
// would answer @latest with an older version, since a client falls back
// to git only on a 404.
func TestChecksumLogRefresh(t *testing.T) {
	xotool.NeedPorts(t)
	t.Parallel()
	env := modSetup(t, "../../internal/mod/testdata/repos")
	ws := t.TempDir()
	app := filepath.Join(ws, "app")
	modtest.WriteFiles(t, app, map[string]string{
		"xo.mod":       "module example.com/app\nedition preview\n\nrequire github.com/acme/billing v1.2.0\n",
		"main/main.xo": appMain,
	})
	key := filepath.Join(ws, "log.key")
	verifier := strings.TrimSpace(xo(t, "mod", "sumdb", "keygen", "sum.test", key).want(0).out)
	s3 := &fakeS3{keyID: "test-key", secret: "test-secret", region: "auto", objects: map[string]fakeObject{}}
	srv := httptest.NewServer(s3)
	defer srv.Close()
	fill := "--fill-to=" + srv.URL + "/modules"
	setenv(t, "XO_FILL_ACCESS_KEY_ID", s3.keyID)
	setenv(t, "XO_FILL_SECRET_ACCESS_KEY", s3.secret)
	logData := t.TempDir()
	t.Cleanup(func() { modtest.MakeWritable(logData) })
	xo(t, "mod", "sumdb", "serve", "--key="+key, "--refresh=1s", logData).want(2)
	xo(t, "mod", "sumdb", "serve", "--key="+key, fill, "--refresh=10ms", logData).want(2)
	logURL, stopLog := startHTTPServe(t, "mod", "sumdb", "serve", "--key="+key, "--fetch", fill, logData)
	setenv(t, "XO_SUMDB", verifier+" "+logURL)
	emptyCache(t)
	xo(t, "mod", "tidy", "--dir", app).want(0) // the log adds billing and money
	list := "/modules/github.com/acme/money/@v/list"
	waitList := func(want string) {
		t.Helper()
		deadline := time.Now().Add(20 * time.Second)
		for {
			if o, ok := s3.object(list); ok && strings.Contains(string(o.body), want) {
				return
			}
			if time.Now().After(deadline) {
				o, _ := s3.object(list)
				t.Fatalf("money's list never held %s: %q", want, o.body)
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
	waitList("v1.3.0\n")
	tag := func(version string) {
		dir := t.TempDir()
		modtest.WriteFiles(t, dir, map[string]string{"xo.mod": "module github.com/acme/money\nedition preview\n", "money.xo": "pub fn cents(n: Int) -> Int { n * 100 }\n"})
		env.Commit(t, "github.com/acme/money", version, dir, false)
	}

	// /refresh: at once, and no write when nothing changed
	tag("v1.4.0")
	if code, cache, body := get(t, logURL+"/refresh/github.com/acme/money", nil); code != 200 || body != "refreshed\n" || cache != "no-store" {
		t.Errorf("/refresh after a new tag: %d %q, Cache-Control %q", code, body, cache)
	}
	if o, _ := s3.object(list); !strings.HasSuffix(string(o.body), "v1.3.0\nv1.4.0\n") || o.caching != "public, max-age=60" {
		t.Errorf("money's list after /refresh: %q, Cache-Control %q", o.body, o.caching)
	}
	n := s3.puts
	if code, _, body := get(t, logURL+"/refresh/github.com/acme/money", nil); code != 200 || body != "unchanged\n" || s3.puts != n {
		t.Errorf("/refresh with no new tag: %d %q, %d puts", code, body, s3.puts-n)
	}
	if code, _, _ := get(t, logURL+"/refresh/github.com/acme/unknown", nil); code != 404 {
		t.Errorf("/refresh of a module the log does not hold: %d", code)
	}

	// --refresh: every module's list on the interval, with no request
	stopLog()
	logURL, _ = startHTTPServe(t, "mod", "sumdb", "serve", "--key="+key, "--fetch", fill, "--refresh=1s", logData)
	tag("v1.5.0")
	waitList("v1.4.0\nv1.5.0\n")
	if code, _, _ := get(t, logURL+"/latest", nil); code != 200 {
		t.Errorf("/latest beside the refresh: %d", code)
	}
}

// TestChecksumLogFillRetry: an upload to the proxy bucket that fails (an
// R2 outage) or that a restart cuts off stays in <data dir>/fill-pending
// and is retried, on the next --refresh tick and when the log starts
// (decision 0568), so a version the log added always reaches the bucket.
func TestChecksumLogFillRetry(t *testing.T) {
	xotool.NeedPorts(t)
	t.Parallel()
	modSetup(t, "../../internal/mod/testdata/repos")
	ws := t.TempDir()
	app := filepath.Join(ws, "app")
	modtest.WriteFiles(t, app, map[string]string{
		"xo.mod":       "module example.com/app\nedition preview\n\nrequire github.com/acme/billing v1.2.0\n",
		"main/main.xo": appMain,
	})
	key := filepath.Join(ws, "log.key")
	verifier := strings.TrimSpace(xo(t, "mod", "sumdb", "keygen", "sum.test", key).want(0).out)
	s3 := &fakeS3{keyID: "test-key", secret: "test-secret", region: "auto", objects: map[string]fakeObject{}}
	srv := httptest.NewServer(s3)
	defer srv.Close()
	fill := "--fill-to=" + srv.URL + "/modules"
	setenv(t, "XO_FILL_ACCESS_KEY_ID", s3.keyID)
	setenv(t, "XO_FILL_SECRET_ACCESS_KEY", s3.secret)
	logData := t.TempDir()
	t.Cleanup(func() { modtest.MakeWritable(logData) })
	pending := func() string {
		b, _ := os.ReadFile(filepath.Join(logData, "fill-pending"))
		return string(b)
	}
	waitFor := func(what string, ok func() bool) {
		t.Helper()
		deadline := time.Now().Add(20 * time.Second)
		for !ok() {
			if time.Now().After(deadline) {
				t.Fatalf("%s: never (fill-pending %q)", what, pending())
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
	has := func(f string) func() bool {
		return func() bool { _, ok := s3.object("/modules/" + f); return ok }
	}

	// an outage: the uploads wait in fill-pending, the next tick writes them
	s3.setDown(true)
	logURL, stopLog := startHTTPServe(t, "mod", "sumdb", "serve", "--key="+key, "--fetch", fill, "--refresh=1s", logData)
	setenv(t, "XO_SUMDB", verifier+" "+logURL)
	emptyCache(t)
	xo(t, "mod", "tidy", "--dir", app).want(0) // the log adds billing and money
	waitFor("both versions pending", func() bool {
		p := pending()
		return strings.Contains(p, "github.com/acme/billing@v1.2.0\n") && strings.Contains(p, "github.com/acme/money@v1.1.0\n")
	})
	if has("github.com/acme/billing/@v/v1.2.0.mod")() {
		t.Fatalf("a .mod was written during the outage")
	}
	// GET /healthz shows the failed upload (the version's files or its
	// @v/list, whichever failed last), and when the log last added
	if h := readHealth(t, logURL); h.LastRecord == 0 || h.LastError == nil || (h.LastError.Op != "fill" && h.LastError.Op != "list") || h.LastError.At == 0 || !strings.Contains(h.LastError.Error, "ServiceUnavailable") {
		t.Errorf("/healthz during the outage: %+v %+v", h, h.LastError)
	}
	s3.setDown(false)
	waitFor("billing filled after the outage", has("github.com/acme/billing/@v/v1.2.0.mod"))
	waitFor("money filled after the outage", has("github.com/acme/money/@v/v1.1.0.mod"))
	waitFor("fill-pending emptied", func() bool { return pending() == "" })

	// a restart: an upload left pending is retried when the log starts,
	// with no --refresh
	stopLog()
	s3.setDown(true)
	logURL, stopLog = startHTTPServe(t, "mod", "sumdb", "serve", "--key="+key, "--fetch", fill, logData)
	if code, _, body := get(t, logURL+"/lookup/github.com/acme/money@v1.3.0", nil); code != 200 {
		t.Fatalf("lookup of a new version: %d %s", code, body)
	}
	waitFor("money v1.3.0 pending", func() bool { return strings.Contains(pending(), "github.com/acme/money@v1.3.0\n") })
	stopLog()
	s3.setDown(false)
	logURL, stopLog = startHTTPServe(t, "mod", "sumdb", "serve", "--key="+key, "--fetch", fill, logData)
	waitFor("money v1.3.0 filled after the restart", has("github.com/acme/money/@v/v1.3.0.mod"))
	waitFor("fill-pending emptied after the restart", func() bool { return pending() == "" })
	if h := readHealth(t, logURL); h.LastError != nil || h.LastRecord != 0 {
		t.Errorf("/healthz of a fresh start with nothing failed or added: %+v %+v", h, h.LastError)
	}

	// --copy-to down: the record is refused (500), and /healthz says why
	stopLog()
	s3.setDown(true)
	logURL, _ = startHTTPServe(t, "mod", "sumdb", "serve", "--key="+key, "--fetch", "--copy-to="+srv.URL+"/logcopy", logData)
	if code, _, body := get(t, logURL+"/lookup/github.com/acme/money@v1.0.0", nil); code != 500 || !strings.Contains(body, "cannot record github.com/acme/money v1.0.0") {
		t.Fatalf("lookup with the copy bucket down: %d %s", code, body)
	}
	if h := readHealth(t, logURL); h.LastRecord != 0 || h.LastError == nil || h.LastError.Op != "record" || !strings.Contains(h.LastError.Error, "github.com/acme/money v1.0.0") || !strings.Contains(h.LastError.Error, "ServiceUnavailable") {
		t.Errorf("/healthz with the copy bucket down: %+v %+v", h, h.LastError)
	}
}

// healthz is GET /healthz's body (sumserve.xo).
type healthz struct {
	Records      int
	Pending      []string
	RefreshTicks int
	Failing      []struct {
		Path     string
		Fails    int
		NextTick int
		Error    string
	}
	LastRecord int64
	LastError  *struct {
		At    int64
		Op    string
		Error string
	}
}

func readHealth(t *testing.T, logURL string) healthz {
	t.Helper()
	code, cache, body := get(t, logURL+"/healthz", nil)
	var h healthz
	if err := json.Unmarshal([]byte(body), &h); code != 200 || cache != "no-store" || err != nil {
		t.Fatalf("/healthz: %d, Cache-Control %q, %v\n%s", code, cache, err, body)
	}
	return h
}

// TestChecksumLogRefreshBackoff: a module whose repository the --refresh
// loop cannot read waits 2^n ticks between tries, so dead repositories
// (a stalled one costs a minute of git) do not hold up the others, and
// GET /healthz shows the loop's ticks, the failures and the uploads still
// pending (decision 0568).
func TestChecksumLogRefreshBackoff(t *testing.T) {
	xotool.NeedPorts(t)
	t.Parallel()
	env := modSetup(t, "../../internal/mod/testdata/repos")
	ws := t.TempDir()
	app := filepath.Join(ws, "app")
	modtest.WriteFiles(t, app, map[string]string{
		"xo.mod":       "module example.com/app\nedition preview\n\nrequire github.com/acme/billing v1.2.0\n",
		"main/main.xo": appMain,
	})
	key := filepath.Join(ws, "log.key")
	verifier := strings.TrimSpace(xo(t, "mod", "sumdb", "keygen", "sum.test", key).want(0).out)
	s3 := &fakeS3{keyID: "test-key", secret: "test-secret", region: "auto", objects: map[string]fakeObject{}}
	srv := httptest.NewServer(s3)
	defer srv.Close()
	setenv(t, "XO_FILL_ACCESS_KEY_ID", s3.keyID)
	setenv(t, "XO_FILL_SECRET_ACCESS_KEY", s3.secret)
	logData := t.TempDir()
	t.Cleanup(func() { modtest.MakeWritable(logData) })
	logURL, _ := startHTTPServe(t, "mod", "sumdb", "serve", "--key="+key, "--fetch", "--fill-to="+srv.URL+"/modules", "--refresh=1s", logData)
	setenv(t, "XO_SUMDB", verifier+" "+logURL)
	emptyCache(t)
	xo(t, "mod", "tidy", "--dir", app).want(0) // the log adds billing and money

	read := func() healthz {
		t.Helper()
		return readHealth(t, logURL)
	}
	if h := read(); h.Records != 2 || len(h.Failing) != 0 {
		t.Fatalf("/healthz before: %+v", h)
	}

	// money's repository goes away: its tries back off, billing's go on
	money := filepath.Join(env.Repos, "github.com", "acme", "money")
	if err := os.Rename(money, money+".gone"); err != nil {
		t.Fatal(err)
	}
	start := read().RefreshTicks
	deadline := time.Now().Add(60 * time.Second)
	var h healthz
	for {
		h = read()
		if h.RefreshTicks >= start+7 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the refresh loop ran %d ticks in 60 s", h.RefreshTicks-start)
		}
		time.Sleep(100 * time.Millisecond)
	}
	// 7 ticks: tries at the first, 2 and then 4 ticks later, so at most 3
	if len(h.Failing) != 1 || h.Failing[0].Path != "github.com/acme/money" || h.Failing[0].Fails < 1 || h.Failing[0].Fails > 3 || h.Failing[0].Error == "" {
		t.Errorf("/healthz after 7 ticks with money's repository gone: %+v", h)
	}
	if code, _, body := get(t, logURL+"/refresh/github.com/acme/money", nil); code != 200 || !strings.Contains(body, "from the log's cache: git:") {
		t.Errorf("/refresh with the repository gone: %d %q", code, body)
	}

	// back: the next try clears it
	if err := os.Rename(money+".gone", money); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(60 * time.Second)
	for len(read().Failing) != 0 {
		if time.Now().After(deadline) {
			t.Fatalf("money still failing after its repository came back: %+v", read())
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// TestChecksumLogWarm: scripts/modservices/warm.sh looks up every tagged
// version of the repositories it is given, so the log records them and
// fills the proxy bucket and each module's @v/list with no client
// asking; a second run adds nothing, and a repository that
// cannot be read makes it exit 1.
func TestChecksumLogWarm(t *testing.T) {
	xotool.NeedPorts(t)
	if runtime.GOOS == "windows" {
		t.Skip("warm.sh is a bash script for the log's Linux host")
	}
	for _, tool := range []string{"bash", "curl", "git"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("no %s", tool)
		}
	}
	t.Parallel()
	env := modSetup(t, "../../internal/mod/testdata/repos")
	ws := t.TempDir()
	key := filepath.Join(ws, "log.key")
	xo(t, "mod", "sumdb", "keygen", "sum.test", key).want(0)
	s3 := &fakeS3{keyID: "test-key", secret: "test-secret", region: "auto", objects: map[string]fakeObject{}}
	srv := httptest.NewServer(s3)
	defer srv.Close()
	setenv(t, "XO_FILL_ACCESS_KEY_ID", s3.keyID)
	setenv(t, "XO_FILL_SECRET_ACCESS_KEY", s3.secret)
	logData := t.TempDir()
	t.Cleanup(func() { modtest.MakeWritable(logData) })
	logURL, _ := startHTTPServe(t, "mod", "sumdb", "serve", "--key="+key, "--fetch", "--fill-to="+srv.URL+"/modules", logData)

	warm := func(repos ...string) (string, error) {
		args := append([]string{"../../scripts/modservices/warm.sh", "--log=" + logURL, "--git-base=" + env.Repos}, repos...)
		out, err := command(t, "bash", args...).CombinedOutput()
		return string(out), err
	}
	out, err := warm("github.com/acme/money", "github.com/acme/billing")
	if err != nil {
		t.Fatalf("warm.sh: %v\n%s", err, out)
	}
	for _, v := range []string{"github.com/acme/money@v1.0.0: ok", "github.com/acme/money@v1.1.0: ok", "github.com/acme/money@v1.3.0: ok", "github.com/acme/billing@v1.2.0: ok"} {
		if !strings.Contains(out, v) {
			t.Errorf("warm.sh output lacks %q:\n%s", v, out)
		}
	}
	if h := readHealth(t, logURL); h.Records != 5 {
		t.Errorf("records after warm.sh: %d, want 5 (billing 2, money 3)", h.Records)
	}
	deadline := time.Now().Add(20 * time.Second)
	for {
		o, ok := s3.object("/modules/github.com/acme/money/@v/list")
		if ok && string(o.body) == "v1.0.0\nv1.1.0\nv1.3.0\n" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("money's @v/list in the bucket: %v %q", ok, o.body)
		}
		time.Sleep(50 * time.Millisecond)
	}

	// again: the log holds them all, so nothing is added
	if out, err := warm("github.com/acme/money"); err != nil || !strings.Contains(out, "github.com/acme/money@v1.3.0: ok") {
		t.Errorf("warm.sh again: %v\n%s", err, out)
	}
	if h := readHealth(t, logURL); h.Records != 5 {
		t.Errorf("records after a second warm.sh: %d", h.Records)
	}

	// a repository that cannot be read
	out, err = warm("github.com/acme/money", "github.com/acme/nosuch")
	if ee, ok := err.(*exec.ExitError); !ok || ee.ExitCode() != 1 || !strings.Contains(out, "github.com/acme/nosuch: git ls-remote:") {
		t.Errorf("warm.sh with a missing repository: %v\n%s", err, out)
	}
}

// TestChecksumLogLookupWait: a new version is fetched by a background task
// that outlives the lookup (Cloudflare cuts a request at 100 s): past
// --lookup-wait the answer is a 503 with Retry-After, and the client's
// next lookups (not counted in --rate) get the record once it is added,
// or the fetch's error.
func TestChecksumLogLookupWait(t *testing.T) {
	xotool.NeedPorts(t)
	t.Parallel()
	modSetup(t, "../../internal/mod/testdata/repos")
	ws := t.TempDir()
	key := filepath.Join(ws, "log.key")
	xo(t, "mod", "sumdb", "keygen", "sum.test", key).want(0)
	logData := t.TempDir()
	t.Cleanup(func() { modtest.MakeWritable(logData) })
	logURL, _ := startHTTPServe(t, "mod", "sumdb", "serve", "--key="+key, "--fetch", "--rate=2", "--lookup-wait=0s", logData)

	// until is the lookup's answer once it is no longer 503
	until := func(v string) (int, string) {
		t.Helper()
		saw503 := false
		deadline := time.Now().Add(60 * time.Second)
		for {
			resp, err := http.Get(logURL + "/lookup/github.com/acme/money@" + v)
			if err != nil {
				t.Fatal(err)
			}
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if resp.StatusCode != 503 {
				if !saw503 {
					t.Errorf("%s: %d before any 503 with --lookup-wait=0s", v, resp.StatusCode)
				}
				return resp.StatusCode, string(b)
			}
			saw503 = true
			if resp.Header.Get("Retry-After") == "" || !strings.Contains(string(b), "still fetching") {
				t.Errorf("%s: 503 with Retry-After %q: %s", v, resp.Header.Get("Retry-After"), b)
			}
			if time.Now().After(deadline) {
				t.Fatalf("%s: still 503 after 60 s", v)
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	if code, body := until("v1.1.0"); code != 200 || !strings.HasPrefix(body, "0\ngithub.com/acme/money v1.1.0 ") {
		t.Errorf("v1.1.0 after the fetch: %d %q", code, body)
	}
	if code, body := until("v9.9.9"); code != 404 || !strings.Contains(body, "github.com/acme/money v9.9.9") {
		t.Errorf("a version with no tag: %d %q", code, body)
	}
	// the many polls above counted 2 fetches: a third starts a 429
	if code, _, body := get(t, logURL+"/lookup/github.com/acme/money@v1.3.0", nil); code != 429 && time.Now().Unix()%60 > 2 {
		t.Errorf("a third fetch this minute: %d %s", code, body)
	}
	if h := readHealth(t, logURL); h.Records != 1 {
		t.Errorf("records: %d", h.Records)
	}
}
