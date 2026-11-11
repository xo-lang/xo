package main

import (
	"bytes"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/xo-lang/xo/internal/mod/modtest"
	"github.com/xo-lang/xo/internal/xotool"
)

// emptyCache points XO_MODCACHE at a new, empty module cache.
func emptyCache(t *testing.T) {
	t.Helper()
	c := t.TempDir()
	t.Cleanup(func() { modtest.MakeWritable(c) })
	setenv(t, "XO_MODCACHE", c)
}

// startHTTPServe starts `xo <args>` (a `serve` taking --addr) on a free
// loopback port and returns its URL once it answers, and a stop function;
// the process also stops with the test.
func startHTTPServe(t *testing.T, args ...string) (string, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	// --addr right after "serve": flags end at the first operand
	var argv []string
	for _, a := range args {
		argv = append(argv, a)
		if a == "serve" {
			argv = append(argv, "--addr="+addr)
		}
	}
	cmd := command(t, xoBin(t), argv...)
	var errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &errOut, &errOut
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	var once sync.Once
	stop := func() {
		once.Do(func() {
			// the serve and the git fetches it started (its process
			// group), so none writes into the TempDir after this
			stopGroup(cmd.Process)
			cmd.Wait()
		})
	}
	t.Cleanup(stop)
	url := "http://" + addr
	for i := 0; i < 200; i++ {
		if r, err := http.Get(url + "/"); err == nil {
			r.Body.Close()
			return url, stop
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("xo %s did not answer on %s: %s", strings.Join(args, " "), addr, errOut.String())
	return "", nil
}

// startProxy starts `xo mod proxy serve --fetch root` (startHTTPServe).
func startProxy(t *testing.T, root string) string {
	t.Helper()
	url, _ := startHTTPServe(t, "mod", "proxy", "serve", "--fetch", root)
	return url
}

// TestModProxy: xo mod tidy and xo check fetch through a caching module
// proxy (decision 0126, `xo mod proxy serve --fetch`) on loopback, never
// from git themselves.
func TestModProxy(t *testing.T) {
	xotool.NeedPorts(t)
	t.Parallel()
	modSetup(t, "../../internal/mod/testdata/repos")
	ws := t.TempDir()
	app := filepath.Join(ws, "app")
	modtest.WriteFiles(t, app, map[string]string{
		"xo.mod":       "module example.com/app\nedition preview\n\nrequire github.com/acme/billing v1.2.0\n",
		"main/main.xo": appMain,
	})
	root := t.TempDir()
	t.Cleanup(func() { modtest.MakeWritable(root) })
	srv := startProxy(t, root)

	emptyCache(t)
	setenv(t, "XO_PROXY", srv)
	xo(t, "mod", "tidy", "--dir", app).want(0)
	xo(t, "check", app+"/...").want(0)
	xo(t, "get", "--dir", app, "github.com/acme/money@v1.3.0").want(0)
	// a second client with an empty cache and no repositories in reach:
	// everything comes through the proxy
	emptyCache(t)
	setenv(t, "XO_GIT_BASE", filepath.Join(ws, "no-repositories"))
	xo(t, "check", app+"/...").want(0)

	setenv(t, "XO_PROXY", "nonsense")
	emptyCache(t)
	r := xo(t, "check", "--json", app+"/...").want(1)
	if got := diagCodes(t, r.out); got == "" || got[:6] != "XO1314" {
		t.Errorf("malformed XO_PROXY: %s", got)
	}
	xo(t, "mod", "proxy").want(2)
	xo(t, "mod", "proxy", "serve", "a", "b").want(2)
}

// TestChecksumLog: the client of decision 0124 against `xo mod sumdb
// serve` on loopback, with modules coming through `xo mod proxy serve
// --fetch` (decision 0126): first fetches verify and lock; a proxy serving
// a tampered tree is refused by the log (XO1313) and, with the log skipped
// by XO_NOSUMDB, by xo.lock (XO1304); XO_NOPROXY fetches from git; a
// restarted log keeps its tree, and one that lost it is a rewrite
// (XO1313), as is a tag moved after the log recorded it.
func TestChecksumLog(t *testing.T) {
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
	if !strings.HasPrefix(verifier, "sum.test+") {
		t.Fatalf("keygen printed %q", verifier)
	}
	xo(t, "mod", "sumdb", "keygen", "sum.test", key).want(1) // a key is never replaced
	xo(t, "mod", "sumdb", "serve", "--key="+key).want(2)
	logData := t.TempDir()
	t.Cleanup(func() { modtest.MakeWritable(logData) })
	logURL, stopLog := startHTTPServe(t, "mod", "sumdb", "serve", "--key="+key, "--fetch", logData)
	proxyRoot := t.TempDir()
	t.Cleanup(func() { modtest.MakeWritable(proxyRoot) })
	proxyURL := startProxy(t, proxyRoot)
	setenv(t, "XO_PROXY", proxyURL)
	setenv(t, "XO_SUMDB", verifier+" "+logURL)

	// fetch through the proxy, verify with the log, lock
	emptyCache(t)
	xo(t, "mod", "tidy", "--dir", app).want(0)
	xo(t, "check", app+"/...").want(0)
	if _, err := os.Stat(filepath.Join(getenv(t, "XO_MODCACHE"), "cache", "sumdb", "sum.test", "latest")); err != nil {
		t.Errorf("the client kept no tree head: %v", err)
	}
	if b, err := os.ReadFile(filepath.Join(logData, "records")); err != nil || strings.Count(string(b), "\n") != 4 {
		t.Errorf("the log's records (billing and money, two lines each): %q %v", b, err)
	}

	// XO_NOPROXY: from git, though XO_PROXY is off
	setenv(t, "XO_PROXY", "off")
	setenv(t, "XO_NOPROXY", "github.com/acme/*")
	emptyCache(t)
	xo(t, "check", app+"/...").want(0)
	setenv(t, "XO_NOPROXY", "")
	setenv(t, "XO_PROXY", proxyURL)

	// a tampered tree from the proxy
	moneyFile := filepath.Join(proxyRoot, "github.com", "acme", "money@v1.1.0", "money.xo")
	orig, err := os.ReadFile(moneyFile)
	if err != nil {
		t.Fatal(err)
	}
	modtest.MakeWritable(proxyRoot)
	if err := os.WriteFile(moneyFile, append(append([]byte{}, orig...), []byte("\npub fn extra() -> Int { 1 }\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	emptyCache(t)
	r := xo(t, "check", "--json", app+"/...").want(1)
	if got := diagCodes(t, r.out); !strings.Contains(got, "XO1313") {
		t.Errorf("tampered tree with the log on: %s\n%s", got, r.out)
	}
	setenv(t, "XO_NOSUMDB", "github.com/acme/money")
	emptyCache(t)
	r = xo(t, "check", "--json", app+"/...").want(1)
	if got := diagCodes(t, r.out); !strings.Contains(got, "XO1304") || strings.Contains(got, "XO1313") {
		t.Errorf("tampered tree, the log skipped by XO_NOSUMDB: %s\n%s", got, r.out)
	}
	setenv(t, "XO_NOSUMDB", "")
	if err := os.WriteFile(moneyFile, orig, 0o644); err != nil {
		t.Fatal(err)
	}

	// a restarted log serves the same tree: a new version extends it
	emptyCache(t)
	xo(t, "check", app+"/...").want(0)
	stopLog()
	logURL, stopLog = startHTTPServe(t, "mod", "sumdb", "serve", "--key="+key, "--fetch", logData)
	setenv(t, "XO_SUMDB", verifier+" "+logURL)
	xo(t, "get", "--dir", app, "github.com/acme/money@v1.3.0").want(0)

	// a tag moved after the log recorded it, fetched from git
	other := t.TempDir()
	modtest.WriteFiles(t, other, map[string]string{"xo.mod": "module github.com/acme/money\nedition preview\n", "money.xo": "pub fn cents(n: Int) -> Int { n * 7 }\n"})
	env.Commit(t, "github.com/acme/money", "v1.1.0", other, true)
	setenv(t, "XO_NOPROXY", "github.com/acme/*")
	emptyCache(t)
	r = xo(t, "get", "--dir", app, "github.com/acme/money@v1.1.0").want(1)
	if !strings.Contains(r.out+r.err, "XO1313") {
		t.Errorf("moved tag under the log:\n%s%s", r.out, r.err)
	}
	setenv(t, "XO_NOPROXY", "")

	// a log that lost its records, under the same key: a rewrite of the
	// tree this cache verified (billing's lookup above stored it)
	stopLog()
	freshData := t.TempDir()
	t.Cleanup(func() { modtest.MakeWritable(freshData) })
	logURL, _ = startHTTPServe(t, "mod", "sumdb", "serve", "--key="+key, "--fetch", freshData)
	setenv(t, "XO_SUMDB", verifier+" "+logURL)
	r = xo(t, "get", "--dir", app, "github.com/acme/money@v1.3.0").want(1)
	if !strings.Contains(r.out+r.err, "XO1313") || !strings.Contains(r.out+r.err, "extend") {
		t.Errorf("a log that forgot its tree:\n%s%s", r.out, r.err)
	}
}
