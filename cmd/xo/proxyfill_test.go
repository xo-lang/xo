package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
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
// Content-Type.
type fakeS3 struct {
	keyID, secret, region string

	mu      sync.Mutex
	objects map[string]fakeObject
	puts    int
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
	canonical := "PUT\n" + r.URL.EscapedPath() + "\n\nhost:" + r.Host + "\nx-amz-content-sha256:" + payload + "\nx-amz-date:" + date + "\n\nhost;x-amz-content-sha256;x-amz-date\n" + payload
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
		s.objects[r.URL.Path] = fakeObject{body, r.Header.Get("Content-Type"), r.Header.Get("Cache-Control")}
		s.puts++
		s.mu.Unlock()
	case http.MethodGet, http.MethodHead:
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
