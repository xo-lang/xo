package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/mod"
	"github.com/xo-lang/xo/internal/mod/modtest"
)

// TestSubdirModules drives modules in subdirectories of a repository
// (decision 0086, core 4.2): github.com/acme/tools holds the module
// github.com/acme/tools at its root and github.com/acme/tools/cli in cli/,
// versioned by tags `cli/vX.Y.Z` (and cli/v2 by `cli/v2.x.y`). Fetch,
// minimal version selection, xo.lock, vendoring, `xo get`, and `xo
// publish --check` all handle the prefix; the root module's tree leaves
// the nested module out.
func TestSubdirModules(t *testing.T) {
	env := modtest.Setup(t, t.TempDir(), mod.Compare)
	repo := filepath.Join(env.Repos, "github.com", "acme", "tools")
	os.MkdirAll(repo, 0o755)
	modtest.Git(t, repo, "init", "-q", "-b", "main")
	commit := func(files map[string]string, tags ...string) {
		t.Helper()
		modtest.WriteFiles(t, repo, files)
		modtest.Git(t, repo, "add", "-A")
		modtest.Git(t, repo, "commit", "-q", "-m", strings.Join(tags, " "))
		for _, tg := range tags {
			modtest.Git(t, repo, "tag", tg)
		}
	}
	commit(map[string]string{
		"xo.mod":            "module github.com/acme/tools\nedition preview\n",
		"util/util.xo":      "pub fn name() -> Str { \"tools\" }\n",
		"cli/xo.mod":        "module github.com/acme/tools/cli\nedition preview\n",
		"cli/flags/flag.xo": "pub fn count() -> Int { 12 }\n",
	}, "v1.0.0", "cli/v1.2.0")
	commit(map[string]string{"cli/flags/flag.xo": "pub fn count() -> Int { 13 }\n\npub fn extra() -> Int { 1 }\n"}, "cli/v1.3.0")
	commit(map[string]string{
		"cli/xo.mod":        "module github.com/acme/tools/cli/v2\nedition preview\n",
		"cli/flags/flag.xo": "pub fn count() -> Int { 20 }\n",
	}, "cli/v2.0.0")

	app := filepath.Join(t.TempDir(), "app")
	modtest.WriteFiles(t, app, map[string]string{
		"xo.mod":       "module example.com/app\nedition preview\n\nrequire github.com/acme/tools v1.0.0\nrequire github.com/acme/tools/cli v1.2.0\n",
		"main/main.xo": "use github.com/acme/tools/util\nuse github.com/acme/tools/cli/flags\n\nfn main(os: Os) {\n  os.stdio.println(\"${util.name()} ${flags.count()}\")\n}\n",
	})
	xo(t, "mod", "tidy", "--dir", app).want(0)
	lock, _ := os.ReadFile(filepath.Join(app, "xo.lock"))
	for _, want := range []string{"github.com/acme/tools v1.0.0 h1:", "github.com/acme/tools/cli v1.2.0 h1:", "github.com/acme/tools/cli v1.2.0/xo.mod h1:"} {
		if !strings.Contains(string(lock), want) {
			t.Errorf("xo.lock lacks %q:\n%s", want, lock)
		}
	}
	// the root module's tree leaves the nested module out
	if _, err := os.Stat(filepath.Join(mod.VersionDir("github.com/acme/tools", "v1.0.0"), "cli")); err == nil {
		t.Error("the root module's cache tree holds the nested module cli/")
	}
	if _, err := os.Stat(filepath.Join(mod.VersionDir("github.com/acme/tools/cli", "v1.2.0"), "flags", "flag.xo")); err != nil {
		t.Errorf("the cli module's tree: %v", err)
	}
	r := xo(t, "run", filepath.Join(app, "main")).want(0)
	if r.out != "tools 12\n" {
		t.Errorf("run: %q", r.out)
	}

	// xo get picks the latest cli/ tag
	r = xo(t, "get", "--dir", app, "github.com/acme/tools/cli").want(0)
	data, _ := os.ReadFile(filepath.Join(app, "xo.mod"))
	if !strings.Contains(string(data), "require github.com/acme/tools/cli v1.3.0") || !strings.Contains(r.out, "extra") {
		t.Errorf("xo get:\n%s\n%s", data, r.out)
	}

	// vendoring nests the cli module inside the tools directory; the
	// tools hash leaves it out
	xo(t, "vendor", "--dir", app).want(0)
	if _, err := os.Stat(filepath.Join(app, "vendor", "github.com", "acme", "tools", "cli", "flags", "flag.xo")); err != nil {
		t.Fatalf("vendored cli: %v", err)
	}
	r = xo(t, "run", "--offline", filepath.Join(app, "main")).want(0)
	if r.out != "tools 13\n" {
		t.Errorf("vendored run: %q", r.out)
	}
	os.RemoveAll(filepath.Join(app, "vendor"))

	// major version 2 of the subdirectory module: tags cli/v2.x.y
	os.WriteFile(filepath.Join(app, "xo.mod"), []byte("module example.com/app\nedition preview\n\nrequire github.com/acme/tools v1.0.0\nrequire github.com/acme/tools/cli/v2 v2.0.0\n"), 0o644)
	os.WriteFile(filepath.Join(app, "main", "main.xo"), []byte("use github.com/acme/tools/util\nuse github.com/acme/tools/cli/v2/flags\n\nfn main(os: Os) {\n  os.stdio.println(\"${util.name()} ${flags.count()}\")\n}\n"), 0o644)
	xo(t, "mod", "tidy", "--dir", app).want(0)
	r = xo(t, "run", filepath.Join(app, "main")).want(0)
	if r.out != "tools 20\n" {
		t.Errorf("cli/v2 run: %q", r.out)
	}

	// a missing subdirectory tag names the prefix
	os.WriteFile(filepath.Join(app, "xo.mod"), []byte("module example.com/app\nedition preview\n\nrequire github.com/acme/tools/cli v1.9.0\n"), 0o644)
	r = xo(t, "check", "--json", filepath.Join(app, "main")).want(1)
	if !strings.Contains(r.out, "XO1302") || !strings.Contains(r.out, "no tag cli/v1.9.0") || !strings.Contains(r.out, "versions are tags cli/vX.Y.Z") {
		t.Errorf("missing tag:\n%s", r.out)
	}

	// xo publish --check in the subdirectory compares with cli/v1.3.0
	// (cli/v2 tags belong to the /v2 path) and prints a prefixed tag
	work := t.TempDir()
	modtest.Git(t, work, "clone", "-q", repo, "tools")
	clone := filepath.Join(work, "tools")
	os.WriteFile(filepath.Join(clone, "cli", "xo.mod"), []byte("module github.com/acme/tools/cli\nedition preview\n"), 0o644)
	os.WriteFile(filepath.Join(clone, "cli", "flags", "flag.xo"), []byte("pub fn count() -> Int { 14 }\n\npub fn extra() -> Int { 1 }\n"), 0o644)
	modtest.Git(t, clone, "commit", "-q", "-am", "cli 1.3.1")
	p := publish(t, 0, "--dir", filepath.Join(clone, "cli"))
	if p.Previous != "v1.3.0" || p.Version != "v1.3.1" || p.Tag != "git tag cli/v1.3.1 && git push origin cli/v1.3.1" {
		t.Errorf("publish in cli/: %+v", p)
	}
	// the root module ignores cli/ tags and the nested module's API
	p = publish(t, 0, "--check", "--dir", clone)
	if p.Previous != "v1.0.0" || p.Bump != "patch" || p.Tag != "git tag v1.0.1 && git push origin v1.0.1" {
		t.Errorf("publish at the root: %+v", p)
	}
	// a module path that does not end with its directory is XO1311
	os.WriteFile(filepath.Join(clone, "cli", "xo.mod"), []byte("module github.com/acme/command\nedition preview\n"), 0o644)
	p = publish(t, 1, "--check", "--dir", filepath.Join(clone, "cli"))
	if p.codes() != "XO1311" {
		t.Errorf("path without the directory: %s", p.codes())
	}
}
