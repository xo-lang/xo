package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/mod"
	"github.com/xo-lang/xo/internal/mod/modtest"
)

// TestEffectCeilings drives decision 0086 step 3: `require ... uses
// none` in xo.mod, an upgrade that adds an effect failing at the call
// site (XO1309), `xo get` printing the API and effect changes, ceilings
// on xo.work `use` lines, and /vN major version paths.
func TestEffectCeilings(t *testing.T) {
	src := t.TempDir()
	modtest.WriteFiles(t, src, map[string]string{
		"github.com/acme/tick@v1.0.0/xo.mod":  "module github.com/acme/tick\nedition preview\n",
		"github.com/acme/tick@v1.0.0/tick.xo": "pub fn now() -> Int { 1 }\n\npub fn apply[fx: Effects](f: fn() -> Int uses fx) -> Int uses fx { f() }\n",
		"github.com/acme/tick@v1.1.0/xo.mod":  "module github.com/acme/tick\nedition preview\n",
		"github.com/acme/tick@v1.1.0/tick.xo": "pub fn now() -> Int uses env { 1 }\n\npub fn apply[fx: Effects](f: fn() -> Int uses fx) -> Int uses fx { f() }\n",
	})
	env := modtest.Setup(t, src, mod.Compare)
	v2 := t.TempDir()
	modtest.WriteFiles(t, v2, map[string]string{
		"xo.mod":  "module github.com/acme/tick/v2\nedition preview\n",
		"tick.xo": "pub fn now() -> Int { 2 }\n",
	})
	env.Commit(t, "github.com/acme/tick", "v2.0.0", v2, false)

	ws := t.TempDir()
	app := filepath.Join(ws, "app")
	modtest.WriteFiles(t, app, map[string]string{
		"xo.mod": "module example.com/app\nedition preview\n\nrequire github.com/acme/tick v1.0.0 uses none\n",
		// the closure's stdio is the caller's effect, passed through
		// apply's effect variable: not the dependency's
		"main/main.xo": "use github.com/acme/tick\n\nfn main(os: Os) {\n  let n = tick.apply(fn() -> Int {\n    os.stdio.println(\"x\")\n    tick.now()\n  })\n  os.stdio.println(\"${n}\")\n}\n",
	})
	xo(t, "mod", "tidy", "--dir", app).want(0)
	xo(t, "check", app).want(0)

	// the upgrade adds `uses env` to now: the effect lock refuses it
	// (XO1317, decision 0156); accepted, xo get says so, the call fails
	r := xo(t, "get", "--json", "--dir", app, "github.com/acme/tick@v1.1.0").want(1)
	if got := diagCodes(t, r.out); got != "XO1317" {
		t.Errorf("get of a version with more effects: %s", got)
	}
	r = xo(t, "get", "--accept-effects", "--dir", app, "github.com/acme/tick@v1.1.0").want(0)
	if !strings.Contains(r.out, "github.com/acme/tick.now now uses env") || !strings.Contains(r.out, "above the effect ceiling") {
		t.Errorf("xo get does not print the effect change:\n%s", r.out)
	}
	data, _ := os.ReadFile(filepath.Join(app, "xo.mod"))
	if !strings.Contains(string(data), "require github.com/acme/tick v1.1.0 uses none") {
		t.Errorf("xo get dropped the ceiling:\n%s", data)
	}
	r = xo(t, "check", "--json", app).want(1)
	if got := diagCodes(t, r.out); got != "XO1309" {
		t.Fatalf("call above the ceiling: %s\n%s", got, r.out)
	}
	if !strings.Contains(r.out, "`now` of github.com/acme/tick v1.1.0 performs `env`, above the effect ceiling `uses none`") || !strings.Contains(r.out, "main.xo") {
		t.Errorf("XO1309 message:\n%s", r.out)
	}
	// raising the ceiling accepts it
	os.WriteFile(filepath.Join(app, "xo.mod"), []byte("module example.com/app\nedition preview\n\nrequire github.com/acme/tick v1.1.0 uses (env, fs.read)\n"), 0o644)
	xo(t, "check", app).want(0)
	// xo get -u upgrades every requirement and reports no changes for
	// one already latest (v2 is another module path)
	r = xo(t, "get", "-u", "--json", "--dir", app).want(0)
	if !strings.Contains(r.out, `"changes": []`) {
		t.Errorf("get -u at the latest v1: %s", r.out)
	}
	// a malformed ceiling is XO1301
	os.WriteFile(filepath.Join(app, "xo.mod"), []byte("module example.com/app\nedition preview\n\nrequire github.com/acme/tick v1.1.0 uses telepathy\n"), 0o644)
	r = xo(t, "check", "--json", app).want(1)
	if got := diagCodes(t, r.out); !strings.HasPrefix(got, "XO1301") {
		t.Errorf("malformed ceiling: %s", got)
	}

	// workspace: a `use` line's ceiling applies to the other modules
	os.WriteFile(filepath.Join(app, "xo.mod"), []byte("module example.com/app\nedition preview\n\nrequire github.com/acme/tick v1.1.0\n"), 0o644)
	modtest.WriteFiles(t, ws, map[string]string{
		"tick/xo.mod":  "module github.com/acme/tick\nedition preview\n",
		"tick/tick.xo": "pub fn now() -> Int uses net { 1 }\n\npub fn apply[fx: Effects](f: fn() -> Int uses fx) -> Int uses fx { f() }\n",
		"xo.work":      "xo preview\nuse ./app\nuse ./tick uses none\n",
	})
	r = xo(t, "check", "--json", app).want(1)
	if got := diagCodes(t, r.out); got != "XO1309" || !strings.Contains(r.out, "xo.work") || !strings.Contains(r.out, "`net`") {
		t.Errorf("workspace ceiling: %s\n%s", got, r.out)
	}
	os.Remove(filepath.Join(ws, "xo.work"))

	// /vN: major version 2 is another module path in the same repository
	os.WriteFile(filepath.Join(app, "xo.mod"), []byte("module example.com/app\nedition preview\n\nrequire github.com/acme/tick/v2 v2.0.0 uses none\n"), 0o644)
	os.WriteFile(filepath.Join(app, "main", "main.xo"), []byte("use github.com/acme/tick/v2\n\nfn main(os: Os) {\n  os.stdio.println(\"${tick.now()}\")\n}\n"), 0o644)
	xo(t, "mod", "tidy", "--dir", app).want(0)
	r = xo(t, "check", "--json", app).want(0)
	// v2 under the path without a suffix is XO1301
	os.WriteFile(filepath.Join(app, "xo.mod"), []byte("module example.com/app\nedition preview\n\nrequire github.com/acme/tick v2.0.0\n"), 0o644)
	r = xo(t, "check", "--json", app).want(1)
	if got := diagCodes(t, r.out); !strings.HasPrefix(got, "XO1301") || !strings.Contains(r.out, "github.com/acme/tick/v2") {
		t.Errorf("v2 without /v2: %s\n%s", got, r.out)
	}
}

type publishOut struct {
	Ok        bool   `json:"ok"`
	Previous  string `json:"previous"`
	Version   string `json:"version"`
	Bump      string `json:"bump"`
	Suggested string `json:"suggested"`
	Tag       string `json:"tag"`
	Breaking  []struct {
		Kind string `json:"kind"`
		Name string `json:"name"`
	} `json:"breaking"`
	Diagnostics []struct {
		Code string `json:"code"`
	} `json:"diagnostics"`
	Definitions []struct {
		Kind string `json:"kind"`
		Name string `json:"name"`
	} `json:"definitions"`
}

func publish(t *testing.T, want int, args ...string) publishOut {
	t.Helper()
	r := xo(t, append([]string{"publish", "--json"}, args...)...).want(want)
	var p publishOut
	if err := json.Unmarshal([]byte(r.out), &p); err != nil {
		t.Fatalf("bad JSON: %v\n%s", err, r.out)
	}
	return p
}

func (p publishOut) codes() string {
	var cs []string
	for _, d := range p.Diagnostics {
		cs = append(cs, d.Code)
	}
	return strings.Join(cs, ",")
}

// TestPublish drives decision 0086 step 4: `xo publish --check` against
// the previous tag of a local git repository.
func TestPublish(t *testing.T) {
	t.Setenv("XO_NO_SERVE", "1")
	t.Setenv("XO_WORK", "")
	repo := t.TempDir()
	modtest.Git(t, repo, "init", "-q", "-b", "main")
	modtest.WriteFiles(t, repo, map[string]string{
		"xo.mod":         "module example.com/lib\nedition preview\n",
		"text/text.xo":   "pub fn shout(s: Str) -> Str { s.to_upper() }\n\npub fn whisper(s: Str) -> Str { s.to_lower() }\n",
		"text/x_test.xo": "test \"shout\" { expect(shout(\"a\") == \"A\") }\n",
	})
	modtest.Git(t, repo, "add", "-A")
	modtest.Git(t, repo, "commit", "-q", "-m", "v1.0.0")
	modtest.Git(t, repo, "tag", "v1.0.0")

	// unchanged: a patch
	p := publish(t, 0, "--check", "--dir", repo)
	if p.Previous != "v1.0.0" || p.Bump != "patch" || p.Suggested != "v1.0.1" {
		t.Errorf("unchanged: %+v", p)
	}
	// an added export: a minor release
	text := filepath.Join(repo, "text", "text.xo")
	os.WriteFile(text, []byte("pub fn shout(s: Str) -> Str { s.to_upper() }\n\npub fn whisper(s: Str) -> Str { s.to_lower() }\n\npub fn same(s: Str) -> Str { s }\n"), 0o644)
	p = publish(t, 0, "--check", "--dir", repo)
	if p.Bump != "minor" || p.Version != "v1.1.0" {
		t.Errorf("added export: %+v", p)
	}
	// an added effect and a removed export break a minor release
	os.WriteFile(text, []byte("pub fn shout(s: Str) -> Str uses clock { s.to_upper() }\n\npub fn same(s: Str) -> Str { s }\n"), 0o644)
	p = publish(t, 1, "--check", "--dir", repo)
	if p.codes() != "XO1310,XO1310" || len(p.Breaking) != 2 || p.Suggested != "v2.0.0" {
		t.Errorf("breaking changes: %s %+v", p.codes(), p)
	}
	kinds := p.Breaking[0].Kind + "," + p.Breaking[1].Kind
	if kinds != "effects_added,removed" {
		t.Errorf("breaking kinds: %s", kinds)
	}
	// a major version needs the /v2 path
	p = publish(t, 1, "--check", "--version", "v2.0.0", "--dir", repo)
	if p.codes() != "XO1311" {
		t.Errorf("v2 without /v2: %s", p.codes())
	}
	os.WriteFile(filepath.Join(repo, "xo.mod"), []byte("module example.com/lib/v2\nedition preview\n"), 0o644)
	p = publish(t, 0, "--check", "--version", "v2.0.0", "--dir", repo)
	if len(p.Breaking) != 2 {
		t.Errorf("v2: breaking changes are listed: %+v", p)
	}
	// without --check: uncommitted changes are XO1311
	p = publish(t, 1, "--version", "v2.0.0", "--dir", repo)
	if !strings.Contains(p.codes(), "XO1311") {
		t.Errorf("dirty tree: %s", p.codes())
	}
	modtest.Git(t, repo, "add", "-A")
	modtest.Git(t, repo, "commit", "-q", "-m", "v2")
	p = publish(t, 0, "--version", "v2.0.0", "--dir", repo)
	if p.Tag != "git tag v2.0.0 && git push origin v2.0.0" {
		t.Errorf("tag command: %q", p.Tag)
	}
	if tags := modtest.Git(t, repo, "tag", "--list"); strings.TrimSpace(tags) != "v1.0.0" {
		t.Errorf("xo publish created tags: %q", tags)
	}
	// an existing tag is XO1311
	p = publish(t, 1, "--check", "--version", "v1.0.0", "--dir", repo)
	if !strings.Contains(p.codes(), "XO1311") {
		t.Errorf("existing tag: %s", p.codes())
	}
}

// TestInit checks `xo init`: xo.mod, and xo.work in .gitignore for a
// single module repository but not under a workspace.
func TestInit(t *testing.T) {
	dir := t.TempDir()
	xo(t, "init", "--dir", dir, "github.com/acme/new").want(0)
	data, _ := os.ReadFile(filepath.Join(dir, "xo.mod"))
	if string(data) != "module github.com/acme/new\nedition preview\n" {
		t.Errorf("xo.mod:\n%s", data)
	}
	ig, _ := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if !strings.Contains(string(ig), "/xo.work\n") {
		t.Errorf(".gitignore:\n%s", ig)
	}
	xo(t, "init", "--dir", dir, "github.com/acme/new").want(1)
	xo(t, "init", "--dir", dir, "nohost").want(2)

	mono := t.TempDir()
	modtest.WriteFiles(t, mono, map[string]string{"xo.work": "xo preview\n"})
	sub := filepath.Join(mono, "svc")
	xo(t, "init", "--dir", sub, "github.com/acme/svc").want(0)
	if _, err := os.Stat(filepath.Join(sub, ".gitignore")); err == nil {
		t.Errorf("xo init under a workspace wrote .gitignore")
	}
}
