package main

import (
	"bytes"
	"net"
	"net/http"
	"path/filepath"
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

// startProxy starts `xo mod proxy serve --fetch root` on a free loopback
// port and returns its URL once it answers; the process stops with the
// test.
func startProxy(t *testing.T, root string) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	cmd := command(t, xoBin(t), "mod", "proxy", "serve", "--addr="+addr, "--fetch", root)
	var errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &errOut, &errOut
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cmd.Process.Kill() // the process this test started
		cmd.Wait()
	})
	url := "http://" + addr
	for i := 0; i < 200; i++ {
		if r, err := http.Get(url + "/"); err == nil {
			r.Body.Close()
			return url
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("xo mod proxy serve did not answer on %s: %s", addr, errOut.String())
	return ""
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
	xo(t, "check", app).want(0)
	xo(t, "get", "--dir", app, "github.com/acme/money@v1.3.0").want(0)
	// a second client with an empty cache and no repositories in reach:
	// everything comes through the proxy
	emptyCache(t)
	setenv(t, "XO_GIT_BASE", filepath.Join(ws, "no-repositories"))
	xo(t, "check", app).want(0)

	setenv(t, "XO_PROXY", "nonsense")
	emptyCache(t)
	r := xo(t, "check", "--json", app).want(1)
	if got := diagCodes(t, r.out); got == "" || got[:6] != "XO1314" {
		t.Errorf("malformed XO_PROXY: %s", got)
	}
	xo(t, "mod", "proxy").want(2)
	xo(t, "mod", "proxy", "serve", "a", "b").want(2)
}

// TestChecksumLogCommand (a checksum log on loopback, decision 0124) went
// with decision 0215: its log server was the Go internal/sumdb, and xo
// has no server of its own yet (the client is selfhost/cli/modules
// sumdb.xo). See decision 0215, "Tests not carried over".
