package mod_test

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/mod"
	"github.com/xo-lang/xo/internal/mod/modtest"
)

var update = flag.Bool("update", false, "rewrite the golden files")

func setup(t *testing.T) *modtest.Env {
	t.Helper()
	return modtest.Setup(t, "testdata/repos", mod.Compare)
}

// app writes a main module requiring reqs ("path version" lines) with
// the given extra files and returns its directory.
func app(t *testing.T, reqs string, files map[string]string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "app")
	all := map[string]string{"xo.mod": "module example.com/app\nedition 2026\n" + reqs}
	for k, v := range files {
		all[k] = v
	}
	modtest.WriteFiles(t, dir, all)
	return dir
}

// uses is a main package importing billing/invoice and or money.
func uses(names ...string) map[string]string {
	src, body := "", ""
	for _, n := range names {
		switch n {
		case "billing":
			src += "use github.com/acme/billing/invoice\n"
			body += "  os.stdio.println(\"${invoice.total()}\")\n"
		case "money":
			src += "use github.com/acme/money\n"
			body += "  os.stdio.println(\"${money.cents(1)}\")\n"
		}
	}
	return map[string]string{"main/main.xo": src + "\nfn main(os: Os) {\n" + body + "}\n"}
}

func codes(ps []*mod.Problem) []string {
	var out []string
	for _, p := range ps {
		out = append(out, string(p.Code))
	}
	return out
}

func selected(b *mod.BuildList) string {
	var parts []string
	for _, m := range b.Modules {
		v := m.Version
		if m.Workspace {
			v = "(workspace)"
		}
		if m.Vendored {
			v += " (vendor)"
		}
		parts = append(parts, m.Path+" "+v)
	}
	return strings.Join(parts, ", ")
}

func golden(t *testing.T, name, got string) {
	t.Helper()
	p := filepath.Join("testdata", "golden", name)
	if *update {
		if err := os.WriteFile(p, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != got {
		t.Errorf("%s differs:\n--- got\n%s\n--- want\n%s", name, got, want)
	}
}

func TestVersions(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want int
	}{
		{"v1.0.0", "v1.0.0", 0}, {"v1.0.0", "v1.0.1", -1}, {"v1.10.0", "v1.9.0", 1},
		{"v2.0.0", "v1.99.99", 1}, {"v1.0.0-rc.1", "v1.0.0", -1}, {"v1.0.0-rc.2", "v1.0.0-rc.10", -1},
		{"v1.0.0-alpha", "v1.0.0-beta", -1}, {"v1.0.0-1", "v1.0.0-alpha", -1},
	} {
		if got := mod.Compare(c.a, c.b); got != c.want {
			t.Errorf("Compare(%s, %s) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
	for v, ok := range map[string]bool{"v1.2.3": true, "v0.0.1-pre.1": true, "1.2.3": false, "v1.2": false, "v01.2.3": false, "latest": false} {
		if mod.ValidVersion(v) != ok {
			t.Errorf("ValidVersion(%q) != %v", v, ok)
		}
	}
	for p, ok := range map[string]bool{"github.com/acme/billing": true, "example.com/a-b/c_d": true, "acme/billing": false, "github.com/../x": false, "github.com/a/": false} {
		if mod.ValidPath(p) != ok {
			t.Errorf("ValidPath(%q) != %v", p, ok)
		}
	}
}

func TestParse(t *testing.T) {
	cases := []struct {
		name, src string
		reqs      int
		want      []string
	}{
		{"ok", "module example.com/a\nrequire github.com/x/y v1.0.0 // why\nrequire go github.com/google/uuid v1.6.0\n", 1, nil},
		{"bad version", "module example.com/a\nrequire github.com/x/y 1.0\n", 0, []string{"XO1301"}},
		{"bad path", "module example.com/a\nrequire acme v1.0.0\n", 0, []string{"XO1301"}},
		{"twice", "module example.com/a\nrequire github.com/x/y v1.0.0\nrequire github.com/x/y v1.1.0\n", 1, []string{"XO1301"}},
		{"replace", "module example.com/a\nreplace github.com/x/y => ../y\n", 0, []string{"XO1301"}},
		{"no module", "edition 2026\n", 0, []string{"XO1301"}},
	}
	for _, c := range cases {
		f, ps := mod.Parse("xo.mod", []byte(c.src))
		if len(f.Requires) != c.reqs || strings.Join(codes(ps), ",") != strings.Join(c.want, ",") {
			t.Errorf("%s: %d requires, problems %v; want %d, %v", c.name, len(f.Requires), codes(ps), c.reqs, c.want)
		}
	}
}

func TestSetRequire(t *testing.T) {
	src := "module example.com/a\nedition 2026\n"
	s := string(mod.SetRequire([]byte(src), "github.com/x/y", "v1.0.0"))
	s = string(mod.SetRequire([]byte(s), "github.com/x/z", "v2.0.0"))
	s = string(mod.SetRequire([]byte(s), "github.com/x/y", "v1.1.0"))
	want := "module example.com/a\nedition 2026\n\nrequire github.com/x/y v1.1.0\nrequire github.com/x/z v2.0.0\n"
	if s != want {
		t.Errorf("got %q, want %q", s, want)
	}
	s = string(mod.SetRequire([]byte(s), "github.com/x/y", ""))
	if strings.Contains(s, "x/y") {
		t.Errorf("not removed: %q", s)
	}
}

// TestMVSAndLock: the build list is the highest version any reached
// xo.mod requires; xo.lock pins every tree and xo.mod and is verified.
func TestMVSAndLock(t *testing.T) {
	setup(t)
	dir := app(t, "\nrequire github.com/acme/billing v1.2.0\nrequire github.com/acme/money v1.0.0\n", uses("billing", "money"))

	b := mod.Load(dir, mod.Options{})
	if got := strings.Join(codes(b.Problems), ","); !strings.Contains(got, "XO1305") || strings.Contains(got, "XO1304") {
		t.Fatalf("no lock: problems %v, want XO1305 only", got)
	}
	if got := selected(b); got != "github.com/acme/billing v1.2.0, github.com/acme/money v1.1.0" {
		t.Errorf("selected %s", got)
	}
	changes, ps := mod.Tidy(dir)
	if len(ps) > 0 {
		t.Fatalf("tidy: %v", ps)
	}
	if len(changes) != 0 {
		t.Errorf("tidy changed requires: %+v", changes)
	}
	lock, _ := os.ReadFile(filepath.Join(dir, "xo.lock"))
	golden(t, "mvs.lock", string(lock))

	b = mod.Load(dir, mod.Options{})
	if len(b.Problems) > 0 {
		t.Fatalf("after tidy: %v", b.Problems)
	}
	// the cache is read only
	info, err := os.Stat(filepath.Join(mod.VersionDir("github.com/acme/money", "v1.1.0"), "money.xo"))
	if err != nil || info.Mode().Perm()&0o222 != 0 {
		t.Errorf("cached file mode %v (%v), want read only", info.Mode(), err)
	}

	// a tampered lock line is XO1304
	tampered := strings.Replace(string(lock), "github.com/acme/money v1.1.0 h1:", "github.com/acme/money v1.1.0 h1:X", 1)
	os.WriteFile(filepath.Join(dir, "xo.lock"), []byte(tampered), 0o644)
	b = mod.Load(dir, mod.Options{})
	if got := codes(b.Problems); len(got) != 1 || got[0] != string(diag.ChecksumMismatch) {
		t.Fatalf("tampered lock: %v", b.Problems)
	}
	if b.Problems[0].Line == 0 || !strings.HasSuffix(b.Problems[0].File, "xo.lock") {
		t.Errorf("XO1304 not on its lock line: %+v", b.Problems[0])
	}
	for _, m := range b.Modules {
		if m.Path == "github.com/acme/money" {
			t.Errorf("a module that fails verification is still in the build list")
		}
	}
}

// TestMovedTag: a tag moved to other content after locking is XO1304
// once the version is fetched again.
func TestMovedTag(t *testing.T) {
	env := setup(t)
	dir := app(t, "\nrequire github.com/acme/money v1.1.0\n", uses("money"))
	if _, ps := mod.Tidy(dir); len(ps) > 0 {
		t.Fatal(ps)
	}
	other := t.TempDir()
	modtest.WriteFiles(t, other, map[string]string{"xo.mod": "module github.com/acme/money\nedition 2026\n", "money.xo": "pub fn cents(n: Int) -> Int { 0 }\n"})
	env.Commit(t, "github.com/acme/money", "v1.1.0", other, true)
	if err := mod.CleanCache(); err != nil {
		t.Fatal(err)
	}
	b := mod.Load(dir, mod.Options{})
	if got := strings.Join(codes(b.Problems), ","); !strings.Contains(got, "XO1304") {
		t.Fatalf("moved tag: problems %v, want XO1304", got)
	}
}

func TestFetchProblems(t *testing.T) {
	setup(t)
	cases := []struct {
		name, reqs, want string
	}{
		{"no tag", "require github.com/acme/money v1.9.9\n", "XO1302"},
		{"no repository", "require github.com/acme/nothing v1.0.0\n", "XO1302"},
		{"declares another path", "require github.com/acme/wrongpath v1.0.0\n", "XO1302"},
	}
	for _, c := range cases {
		dir := app(t, c.reqs, nil)
		b := mod.Load(dir, mod.Options{Update: true})
		if got := strings.Join(codes(b.Problems), ","); got != c.want {
			t.Errorf("%s: problems %v, want %s", c.name, b.Problems, c.want)
			continue
		}
		if p := b.Problems[0]; p.Line != 3 || !strings.HasSuffix(p.File, "xo.mod") {
			t.Errorf("%s: problem not on the require line: %+v", c.name, p)
		}
	}
}

// TestOffline: XO_OFFLINE never fetches (XO1303) and builds from the
// cache once it holds every version.
func TestOffline(t *testing.T) {
	setup(t)
	dir := app(t, "\nrequire github.com/acme/billing v1.2.0\n", uses("billing"))
	t.Setenv("XO_OFFLINE", "1")
	b := mod.Load(dir, mod.Options{Update: true})
	if got := strings.Join(codes(b.Problems), ","); got != "XO1303" {
		t.Fatalf("offline, empty cache: %v", b.Problems)
	}
	t.Setenv("XO_OFFLINE", "")
	if _, ps := mod.Tidy(dir); len(ps) > 0 {
		t.Fatal(ps)
	}
	t.Setenv("XO_OFFLINE", "1")
	b = mod.Load(dir, mod.Options{})
	if len(b.Problems) > 0 || selected(b) != "github.com/acme/billing v1.2.0, github.com/acme/money v1.1.0" {
		t.Fatalf("offline, cached: %v %s", b.Problems, selected(b))
	}
	// xo get resolves `latest` from the cache offline
	if _, ps := mod.Get(dir, "github.com/acme/money"); len(ps) > 0 {
		t.Fatalf("offline get: %v", ps)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "xo.mod"))
	if !strings.Contains(string(data), "require github.com/acme/money v1.1.0") {
		t.Errorf("offline latest is the highest cached version:\n%s", data)
	}
}

func TestGet(t *testing.T) {
	setup(t)
	dir := app(t, "", nil)
	steps := []struct {
		arg  string
		want string // the require lines of xo.mod after the step
	}{
		{"github.com/acme/billing@v1.0.0", "require github.com/acme/billing v1.0.0"},
		{"github.com/acme/billing", "require github.com/acme/billing v1.2.0"},
		{"github.com/acme/money/sub@latest", "require github.com/acme/billing v1.2.0\nrequire github.com/acme/money v1.3.0"},
		{"github.com/acme/money@v1.0.0", "require github.com/acme/billing v1.2.0\nrequire github.com/acme/money v1.0.0"},
		{"github.com/acme/money@none", "require github.com/acme/billing v1.2.0"},
	}
	for _, s := range steps {
		if _, ps := mod.Get(dir, s.arg); len(ps) > 0 {
			t.Fatalf("get %s: %v", s.arg, ps)
		}
		data, _ := os.ReadFile(filepath.Join(dir, "xo.mod"))
		var reqs []string
		for _, l := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(l, "require ") {
				reqs = append(reqs, l)
			}
		}
		if got := strings.Join(reqs, "\n"); got != s.want {
			t.Errorf("after get %s:\n%s\nwant\n%s", s.arg, got, s.want)
		}
		if b := mod.Load(dir, mod.Options{}); len(b.Problems) > 0 {
			t.Errorf("after get %s the lock does not verify: %v", s.arg, b.Problems)
		}
	}
	// a failed get leaves xo.mod alone
	before, _ := os.ReadFile(filepath.Join(dir, "xo.mod"))
	if _, ps := mod.Get(dir, "github.com/acme/money@v7.0.0"); len(ps) == 0 {
		t.Fatal("get of a missing tag succeeded")
	}
	after, _ := os.ReadFile(filepath.Join(dir, "xo.mod"))
	if string(before) != string(after) {
		t.Errorf("failed get changed xo.mod:\n%s", after)
	}
}

func TestTidy(t *testing.T) {
	setup(t)
	dir := app(t, "\nrequire github.com/acme/money v1.3.0\nrequire github.com/acme/wrongpath v1.0.0\n", map[string]string{
		"main/main.xo": "use github.com/acme/billing/invoice\n\nfn main(os: Os) {\n  os.stdio.println(\"${invoice.total()}\")\n}\n",
	})
	// wrongpath cannot be fetched: tidy reports it and writes nothing
	if _, ps := mod.Tidy(dir); len(ps) == 0 {
		t.Fatal("tidy with a broken require succeeded")
	}
	os.WriteFile(filepath.Join(dir, "xo.mod"), []byte("module example.com/app\nedition 2026\n\nrequire github.com/acme/money v1.3.0\n"), 0o644)
	changes, ps := mod.Tidy(dir)
	if len(ps) > 0 {
		t.Fatal(ps)
	}
	// billing is added; money v1.3.0 is not imported but raises the
	// version billing needs (v1.1.0), so it stays
	if len(changes) != 1 || changes[0] != (mod.Change{Path: "github.com/acme/billing", New: "v1.2.0"}) {
		t.Errorf("changes %+v", changes)
	}
	b := mod.Load(dir, mod.Options{})
	if len(b.Problems) > 0 || selected(b) != "github.com/acme/billing v1.2.0, github.com/acme/money v1.3.0" {
		t.Errorf("after tidy: %v %s", b.Problems, selected(b))
	}
	// money at a version the graph already selects is dropped
	os.WriteFile(filepath.Join(dir, "xo.mod"), []byte("module example.com/app\nedition 2026\n\nrequire github.com/acme/billing v1.2.0\nrequire github.com/acme/money v1.0.0\n"), 0o644)
	changes, ps = mod.Tidy(dir)
	if len(ps) > 0 || len(changes) != 1 || changes[0] != (mod.Change{Path: "github.com/acme/money", Old: "v1.0.0"}) {
		t.Errorf("changes %+v %v", changes, ps)
	}
	lock, _ := os.ReadFile(filepath.Join(dir, "xo.lock"))
	golden(t, "tidy.lock", string(lock))
}

// TestVendor: builds use vendor/ only, verified against xo.lock.
func TestVendor(t *testing.T) {
	setup(t)
	dir := app(t, "\nrequire github.com/acme/billing v1.2.0\n", uses("billing"))
	if _, ps := mod.Vendor(dir); len(ps) == 0 || ps[0].Code != diag.LockMissing {
		t.Fatalf("vendor without a lock: %v", ps)
	}
	if _, ps := mod.Tidy(dir); len(ps) > 0 {
		t.Fatal(ps)
	}
	mods, ps := mod.Vendor(dir)
	if len(ps) > 0 || len(mods) != 2 {
		t.Fatalf("vendor: %v %+v", ps, mods)
	}
	list, _ := os.ReadFile(filepath.Join(dir, "vendor", "modules.txt"))
	golden(t, "modules.txt", string(list))

	// no cache, no network
	if err := mod.CleanCache(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XO_OFFLINE", "1")
	b := mod.Load(dir, mod.Options{})
	if len(b.Problems) > 0 || !b.Vendor || selected(b) != "github.com/acme/billing v1.2.0 (vendor), github.com/acme/money v1.1.0 (vendor)" {
		t.Fatalf("vendored load: %v %s", b.Problems, selected(b))
	}
	// an edited vendored file is XO1304
	f := filepath.Join(dir, "vendor", "github.com", "acme", "money", "money.xo")
	os.WriteFile(f, []byte("pub fn cents(n: Int) -> Int { 0 }\n"), 0o644)
	if b := mod.Load(dir, mod.Options{}); strings.Join(codes(b.Problems), ",") != "XO1304" {
		t.Errorf("edited vendor: %v", b.Problems)
	}
	// a require vendor/ does not have is XO1307
	os.WriteFile(filepath.Join(dir, "xo.mod"), []byte("module example.com/app\nedition 2026\n\nrequire github.com/acme/billing v1.2.0\nrequire github.com/acme/wrongpath v1.0.0\n"), 0o644)
	if b := mod.Load(dir, mod.Options{}); !strings.Contains(strings.Join(codes(b.Problems), ","), "XO1307") {
		t.Errorf("stale vendor: %v", b.Problems)
	}
}

// TestWorkspace: xo.work overrides requirements with local modules,
// never changes xo.lock, and is ignored with XO_WORK=off.
func TestWorkspace(t *testing.T) {
	setup(t)
	dir := app(t, "\nrequire github.com/acme/billing v1.0.0\n", uses("billing"))
	if _, ps := mod.Tidy(dir); len(ps) > 0 {
		t.Fatal(ps)
	}
	lockBefore, _ := os.ReadFile(filepath.Join(dir, "xo.lock"))
	ws := filepath.Dir(dir)
	modtest.WriteFiles(t, ws, map[string]string{
		"billing/xo.mod":             "module github.com/acme/billing\nedition 2026\n\nrequire github.com/acme/money v1.3.0\n",
		"billing/invoice/invoice.xo": "use github.com/acme/money\n\npub fn total() -> Int { money.cents(7) }\n",
		"billing/xo.lock":            "",
		"xo.work":                    "xo 2026\nuse ./app\nuse ./billing\n",
	})
	// billing's own lock is empty: money v1.3.0 is missing from it
	b := mod.Load(dir, mod.Options{})
	if b.Work == nil || !strings.Contains(strings.Join(codes(b.Problems), ","), "XO1305") {
		t.Fatalf("workspace without billing's lock: %v", b.Problems)
	}
	if _, ps := mod.Tidy(filepath.Join(ws, "billing")); len(ps) > 0 {
		t.Fatal(ps)
	}
	b = mod.Load(dir, mod.Options{})
	if len(b.Problems) > 0 || selected(b) != "github.com/acme/billing (workspace), github.com/acme/money v1.3.0" {
		t.Fatalf("workspace: %v %s", b.Problems, selected(b))
	}
	if got := b.Overrides(); len(got) != 1 || got[0] != "github.com/acme/billing" {
		t.Errorf("overrides %v", got)
	}
	// tidy in a workspace module ignores the workspace
	if _, ps := mod.Tidy(dir); len(ps) > 0 {
		t.Fatal(ps)
	}
	lockAfter, _ := os.ReadFile(filepath.Join(dir, "xo.lock"))
	if string(lockBefore) != string(lockAfter) {
		t.Errorf("xo.lock rewritten with workspace overrides:\n%s", lockAfter)
	}
	if got := mod.WorkspaceDirs([]string{dir}); len(got) != 2 {
		t.Errorf("WorkspaceDirs = %v", got)
	}
	t.Setenv("XO_WORK", "off")
	b = mod.Load(dir, mod.Options{})
	if b.Work != nil || selected(b) != "github.com/acme/billing v1.0.0" {
		t.Errorf("XO_WORK=off: %s", selected(b))
	}
	// malformed xo.work
	t.Setenv("XO_WORK", "")
	os.WriteFile(filepath.Join(ws, "xo.work"), []byte("xo 2026\nuse ./app\nuse ./missing\nreplace x\n"), 0o644)
	b = mod.Load(dir, mod.Options{})
	if got := strings.Join(codes(b.Problems), ","); got != "XO1301,XO1301" {
		t.Errorf("bad xo.work: %v", b.Problems)
	}
}

func TestRepoURL(t *testing.T) {
	t.Setenv("XO_GIT_BASE", "")
	t.Setenv("XO_GIT_REWRITE", "")
	for p, want := range map[string]string{"github.com/acme/billing": "https://github.com/acme/billing.git", "example.com/x": "https://example.com/x"} {
		if got := mod.RepoURL(p); got != want {
			t.Errorf("RepoURL(%s) = %s, want %s", p, got, want)
		}
	}
	t.Setenv("XO_GIT_REWRITE", "github.com/acme=/srv/git/acme,github.com=file:///mirror")
	for p, want := range map[string]string{"github.com/acme/billing": "/srv/git/acme/billing", "github.com/x/y": "file:///mirror/x/y"} {
		if got := mod.RepoURL(p); got != want {
			t.Errorf("rewrite RepoURL(%s) = %s, want %s", p, got, want)
		}
	}
	if got := mod.EscapePath("github.com/Acme/X"); got != "github.com/!acme/!x" {
		t.Errorf("EscapePath = %s", got)
	}
}
