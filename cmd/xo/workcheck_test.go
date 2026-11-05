package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/mod/modtest"
)

// TestCheckWorkspaceImportClosure: `xo check DIR` in a workspace checks
// DIR and the workspace modules it imports, not the workspace's other
// modules (decision 0511, aws issue 11); a walk (DIR/...) or the
// workspace's own directory still checks them all (decision 0086).
func TestCheckWorkspaceImportClosure(t *testing.T) {
	ws := t.TempDir()
	modtest.WriteFiles(t, ws, map[string]string{
		"xo.work":          "xo preview\nuse ./app\nuse ./lib\nuse ./broken\n",
		"app/xo.mod":       "module example.com/app\nedition preview\n",
		"app/main/main.xo": "use example.com/lib/util\n\nfn main(os: Os) {\n  os.stdio.println(\"${util.seven()}\")\n}\n",
		"lib/xo.mod":       "module example.com/lib\nedition preview\n",
		"lib/util/util.xo": "pub fn seven() -> Int { 7 }\n",
		"broken/xo.mod":    "module example.com/broken\nedition preview\n",
		"broken/b/b.xo":    "pub fn f() -> Int { \"not an Int\" }\n",
	})
	// broken is not checked: nothing app imports is in it
	xo(t, "check", filepath.Join(ws, "app", "main")).want(0)
	// an error in an imported workspace module is still reported
	modtest.WriteFiles(t, ws, map[string]string{"lib/util/util.xo": "pub fn seven() -> Int { \"seven\" }\n"})
	xo(t, "check", filepath.Join(ws, "app", "main")).want(1)
	modtest.WriteFiles(t, ws, map[string]string{"lib/util/util.xo": "pub fn seven() -> Int { 7 }\n"})
	// a walk below the workspace's directory checks what it finds and its
	// imports, not the workspace's other modules
	xo(t, "check", filepath.Join(ws, "app")+"/...").want(0)
	// a walk checks every module (the workspace's directory too, when it
	// holds a module: here it has no .xo file, a usage error, decision 0345)
	r := xo(t, "check", "--json", ws+"/...").want(1)
	if !strings.Contains(r.out, "broken") {
		t.Errorf("check of the walk: the broken module's error is missing:\n%s", r.out)
	}
}

// TestCheckWorkspaceSizeMemory: a workspace of 300 modules nobody imports
// leaves a one-module check's peak memory where a workspace of only that
// module puts it (aws issue 11: 435 modules took it past 18 GB).
func TestCheckWorkspaceSizeMemory(t *testing.T) {
	if !hasProcPeakRSS {
		t.Skip("no peak resident set on this OS")
	}
	if testing.Short() {
		t.Skip("checks a 300 module workspace")
	}
	ws := t.TempDir()
	files := map[string]string{
		"app/xo.mod":    "module example.com/app\nedition preview\n",
		"app/a/a.xo":    "pub fn one() -> Int { 1 }\n",
		"small/xo.work": "xo preview\nuse ../app\n",
	}
	var work strings.Builder
	work.WriteString("xo preview\nuse ./app\n")
	var body strings.Builder
	for i := 0; i < 200; i++ {
		fmt.Fprintf(&body, "pub struct S%d { a: Int\n  b: Str\n  c: List[Int] }\n\npub fn f%d(s: S%d) -> Int { s.a + s.b.len() + s.c.len() }\n\n", i, i, i)
	}
	for i := 0; i < 300; i++ {
		files[fmt.Sprintf("m%d/xo.mod", i)] = fmt.Sprintf("module example.com/m%d\nedition preview\n", i)
		files[fmt.Sprintf("m%d/p/p.xo", i)] = body.String()
		fmt.Fprintf(&work, "use ./m%d\n", i)
	}
	files["xo.work"] = work.String()
	modtest.WriteFiles(t, ws, files)
	peakOf := func(workFile string, args ...string) int64 {
		cmd := command(t, xoBin(t), args...)
		cmd.Env = append(cmd.Env, "XO_WORK="+workFile, "XO_CHK_CACHE=off", "XO_NO_SERVE=1")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("check with %s: %v\n%s", workFile, err, out)
		}
		return procPeakRSS(cmd.ProcessState)
	}
	// xo check (decision 0511) and xo query type (decision 0515)
	for _, args := range [][]string{
		{"check", filepath.Join(ws, "app", "a")},
		{"query", "type", filepath.Join(ws, "app", "a", "a.xo") + ":1:23"},
		{"query", "api", filepath.Join(ws, "app", "a")},
		{"query", "def", "--dir=" + ws, "one"},
	} {
		small := peakOf(filepath.Join(ws, "small", "xo.work"), args...)
		large := peakOf(filepath.Join(ws, "xo.work"), args...)
		t.Logf("xo %s %s: peak RSS: 1 module workspace %d MB, 301 modules %d MB", args[0], args[1], small>>20, large>>20)
		if large > small+small/4+(32<<20) {
			t.Errorf("xo %s: a 301 module workspace raised a one-module peak RSS from %d MB to %d MB", args[0], small>>20, large>>20)
		}
	}
}

// TestCheckWorkspacePerModuleSameDiags: a whole-workspace check of 8
// members or more holds one member at a time (decision 0513, a shared
// base) and reports what one checker reports for fewer members: errors in
// a module two members import and in two members, each once; for the
// workspace's directory and for a walk of it.
func TestCheckWorkspacePerModuleSameDiags(t *testing.T) {
	files := map[string]string{
		"xo.mod":           "module example.com/root\nedition preview\n",
		"root.xo":          "pub fn root() -> Int { 0 }\n",
		"lib/xo.mod":       "module example.com/lib\nedition preview\n",
		"lib/util/util.xo": "pub fn seven() -> Int { 7 }\n\npub fn bad() -> Int { \"lib\" }\n",
		"a/xo.mod":         "module example.com/a\nedition preview\n",
		"a/a.xo":           "use example.com/lib/util\n\npub fn a() -> Int { util.seven() }\n\nfn bad_a() -> Str { 1 }\n",
		"b/xo.mod":         "module example.com/b\nedition preview\n",
		"b/b.xo":           "use example.com/lib/util\n\npub fn b() -> Int { util.seven() + \"b\" }\n",
	}
	// members c1..cN, error free, each importing lib
	workspace := func(n int) string {
		ws := t.TempDir()
		work := "xo preview\nuse .\nuse ./lib\nuse ./a\nuse ./b\n"
		fs := map[string]string{}
		for k, v := range files {
			fs[k] = v
		}
		for i := 1; i <= n; i++ {
			fs[fmt.Sprintf("c%d/xo.mod", i)] = fmt.Sprintf("module example.com/c%d\nedition preview\n", i)
			fs[fmt.Sprintf("c%d/c.xo", i)] = "use example.com/lib/util\n\npub fn c() -> Int { util.seven() }\n"
			work += fmt.Sprintf("use ./c%d\n", i)
		}
		fs["xo.work"] = work
		modtest.WriteFiles(t, ws, fs)
		return ws
	}
	small, large := workspace(1), workspace(7) // 4 and 10 members besides the root
	for _, walk := range []string{"", "/..."} {
		one := xo(t, "check", "--json", small+walk).want(1)
		each := xo(t, "check", "--json", large+walk).want(1)
		// the diagnostics, not the module lists (c1 against c1..c7)
		diags := func(out, ws string) string {
			var r struct{ Diagnostics json.RawMessage }
			if err := json.Unmarshal([]byte(strings.ReplaceAll(out, ws, "WS")), &r); err != nil {
				t.Fatalf("check WS%s: %v\n%s", walk, err, out)
			}
			return string(r.Diagnostics)
		}
		a, b := diags(one.out, small), diags(each.out, large)
		if a != b {
			t.Errorf("check WS%s: 10 members (per member) report other diagnostics than 4 (one checker):\n4 members:\n%s\n10 members:\n%s", walk, a, b)
		}
		if n := strings.Count(b, `"code"`); n != 3 {
			t.Errorf("check WS%s: want 3 diagnostics (lib, a, b), got %d:\n%s", walk, n, b)
		}
	}
}

// TestTestWorkspacePerModule: `xo test ./...` of a workspace of 8
// members or more runs one child per member (decision 0515 point 5) and
// passes and fails the same tests as one program of fewer members: the
// totals of its summaries and its status match.
func TestTestWorkspacePerModule(t *testing.T) {
	files := map[string]string{
		"xo.mod":             "module example.com/root\nedition preview\n",
		"core/core.xo":       "pub fn two() -> Int { 2 }\n",
		"core/core_test.xo":  "test \"two\" {\n  expect(two()) == 2\n}\n",
		"lib/xo.mod":         "module example.com/lib\nedition preview\n",
		"lib/util/util.xo":   "pub fn seven() -> Int { 7 }\n",
		"lib/util/u_test.xo": "test \"seven\" {\n  expect(seven()) == 7\n}\n",
		"a/xo.mod":           "module example.com/a\nedition preview\n",
		"a/a.xo":             "use example.com/lib/util\n\npub fn a() -> Int { util.seven() + 1 }\n",
		"a/a_test.xo":        "test \"eight\" {\n  expect(a()) == 8\n}\n\ntest \"nine\" {\n  expect(a() + 1) == 9\n}\n",
		"b/xo.mod":           "module example.com/b\nedition preview\n",
		"b/b.xo":             "use example.com/lib/util\n\npub fn b() -> Int { util.seven() }\n",
		"b/b_test.xo":        "test \"wrong\" {\n  expect(b()) == 8\n}\n",
	}
	// members c1..cN with no tests
	workspace := func(n int) string {
		ws := t.TempDir()
		work := "xo preview\nuse .\nuse ./lib\nuse ./a\nuse ./b\n"
		fs := map[string]string{}
		for k, v := range files {
			fs[k] = v
		}
		for i := 1; i <= n; i++ {
			fs[fmt.Sprintf("c%d/xo.mod", i)] = fmt.Sprintf("module example.com/c%d\nedition preview\n", i)
			fs[fmt.Sprintf("c%d/c.xo", i)] = "pub fn c() -> Int { 0 }\n"
			work += fmt.Sprintf("use ./c%d\n", i)
		}
		fs["xo.work"] = work
		modtest.WriteFiles(t, ws, fs)
		return ws
	}
	totals := func(out string) ([2]int, int) {
		var n [2]int
		lines := 0
		for _, l := range strings.Split(out, "\n") {
			var p, f int
			if i := strings.Index(l, ": "); i >= 0 {
				if _, err := fmt.Sscanf(l[i+2:], "%d passed, %d failed", &p, &f); err == nil {
					n[0] += p
					n[1] += f
					lines++
				}
			}
		}
		return n, lines
	}
	one := xo(t, "test", workspace(1)+"/...").want(1)
	each := xo(t, "test", workspace(6)+"/...").want(1)
	tw, _ := totals(one.out)
	te, children := totals(each.out)
	if tw != [2]int{4, 1} || te != tw {
		t.Errorf("passed and failed: one program %v, per member %v (want [4 1] both)\none program:\n%s\nper member:\n%s", tw, te, one.out, each.out)
	}
	if children < 2 {
		t.Errorf("per member: want a summary per child, got %d:\n%s", children, each.out)
	}
	if !strings.Contains(each.out+each.err, "wrong") {
		t.Errorf("per member: the failing test is not named:\n%s\n%s", each.out, each.err)
	}
}

// TestQueryCallersReverseClosure: `xo query callers` of a workspace
// package loads that package and the packages that import it (decision
// 0515 point 2) and finds every caller: in the package itself, in a
// module that imports it, in a package of another module that imports
// it; c, which imports nothing of it, is not checked (its error is not
// reported).
func TestQueryCallersReverseClosure(t *testing.T) {
	ws := t.TempDir()
	modtest.WriteFiles(t, ws, map[string]string{
		"xo.work":          "xo preview\nuse ./lib\nuse ./a\nuse ./b\nuse ./c\n",
		"lib/xo.mod":       "module example.com/lib\nedition preview\n",
		"lib/util/util.xo": "pub fn seven() -> Int { 7 }\n\npub fn eight() -> Int { seven() + 1 }\n",
		"a/xo.mod":         "module example.com/a\nedition preview\n",
		"a/a.xo":           "use example.com/lib/util\n\npub fn a() -> Int { util.seven() }\n",
		"a/a_test.xo":      "test \"a\" {\n  expect(util.seven()) == 7\n}\n",
		"b/xo.mod":         "module example.com/b\nedition preview\n",
		"b/mid/mid.xo":     "use example.com/a\n\npub fn mid() -> Int { a.a() }\n",
		"b/top/top.xo":     "use example.com/lib/util\n\npub fn top() -> Int { util.seven() * 2 }\n",
		"c/xo.mod":         "module example.com/c\nedition preview\n",
		"c/c.xo":           "pub fn c() -> Int { 0 }\n\nfn bad() -> Int { \"c\" }\n",
	})
	dir := "--dir=" + filepath.Join(ws, "lib", "util")
	r := xo(t, "query", "callers", "--json", dir, "util.seven").want(0)
	if n := strings.Count(r.out, `"caller"`); n != 3 {
		t.Errorf("want 3 callers, got %d:\n%s", n, r.out)
	}
	for _, f := range []string{"util.xo", "a.xo", "top.xo"} {
		if !strings.Contains(r.out, f) {
			t.Errorf("want a caller in %s:\n%s", f, r.out)
		}
	}
}

// TestQueryCallersPerMember: callers of a package more than 32 workspace
// packages import (a core target) are found one importer at a time from
// a shared base (decision 0515 point 2): every caller, once, in file and
// position order.
func TestQueryCallersPerMember(t *testing.T) {
	ws := t.TempDir()
	work := "xo preview\nuse ./lib\n"
	files := map[string]string{
		"lib/xo.mod":       "module example.com/lib\nedition preview\n",
		"lib/util/util.xo": "pub fn seven() -> Int { 7 }\n",
	}
	const n = 40
	for i := 0; i < n; i++ {
		files[fmt.Sprintf("m%02d/xo.mod", i)] = fmt.Sprintf("module example.com/m%02d\nedition preview\n", i)
		files[fmt.Sprintf("m%02d/m.xo", i)] = "use example.com/lib/util\n\npub fn m() -> Int { util.seven() }\n"
		work += fmt.Sprintf("use ./m%02d\n", i)
	}
	files["xo.work"] = work
	modtest.WriteFiles(t, ws, files)
	r := xo(t, "query", "callers", "--json", "--dir="+filepath.Join(ws, "lib", "util"), "util.seven").want(0)
	if c := strings.Count(r.out, `"caller"`); c != n {
		t.Fatalf("want %d callers, got %d:\n%s", n, c, r.out)
	}
	last := -1
	for i := 0; i < n; i++ {
		k := strings.Index(r.out, fmt.Sprintf("/m%02d/", i))
		if k < 0 || k < last {
			t.Errorf("caller in m%02d missing or out of order:\n%s", i, r.out)
			break
		}
		last = k
	}
}
