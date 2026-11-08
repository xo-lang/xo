package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/xo-lang/xo/internal/mod/modtest"
	"github.com/xo-lang/xo/internal/xotool"
)

// TestGitStalledRemote: a git remote that stops sending is abandoned
// (http.lowSpeedLimit 1000 for http.lowSpeedTime 60 on every git xo
// runs) instead of holding the fetch for good; a checksum log's lookup
// and its --refresh loop run the same git (decision 0568). About a
// minute, so not in -short.
func TestGitStalledRemote(t *testing.T) {
	if testing.Short() {
		t.Skip("waits out git's 60 s low speed time")
	}
	xotool.NeedPorts(t)
	t.Parallel()
	done := make(chan struct{})
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		// the start of a smart HTTP answer, then nothing
		w.Header().Set("Content-Type", "application/x-git-upload-pack-advertisement")
		w.WriteHeader(200)
		w.Write([]byte("001e# service=git-upload-pack\n"))
		w.(http.Flusher).Flush()
		select {
		case <-done:
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()
	defer close(done)
	modSetup(t, "../../internal/mod/testdata/repos")
	app := filepath.Join(t.TempDir(), "app")
	modtest.WriteFiles(t, app, map[string]string{"xo.mod": "module example.com/app\nedition preview\n"})
	setenv(t, "XO_GIT_REWRITE", "stall.example="+srv.URL+"/r")
	setenv(t, "XO_PROXY", "direct")
	setenv(t, "XO_SUMDB", "off")
	emptyCache(t)
	start := time.Now()
	r := xo(t, "get", "--dir", app, "stall.example/m@v1.0.0").want(1)
	// each git that reaches the remote gives up after its minute
	took := time.Since(start)
	if n := hits.Load(); n < 1 || took > time.Duration(n)*90*time.Second {
		t.Errorf("the stalled fetch took %v over %d git requests", took, n)
	}
	t.Logf("%v over %d git requests", took.Round(time.Second), hits.Load())
	if !strings.Contains(strings.ToLower(r.err), "too slow") && !strings.Contains(strings.ToLower(r.err), "low speed") {
		t.Errorf("want git's low speed abort, stderr:\n%s", r.err)
	}
}
