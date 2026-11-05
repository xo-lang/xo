package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
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
	peak := func(workFile string) int64 {
		cmd := command(t, xoBin(t), "check", filepath.Join(ws, "app", "a"))
		cmd.Env = append(cmd.Env, "XO_WORK="+workFile, "XO_CHK_CACHE=off")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("check with %s: %v\n%s", workFile, err, out)
		}
		rss := cmd.ProcessState.SysUsage().(*syscall.Rusage).Maxrss
		if runtime.GOOS != "darwin" {
			rss *= 1024 // kilobytes on Linux
		}
		return rss
	}
	small := peak(filepath.Join(ws, "small", "xo.work"))
	large := peak(filepath.Join(ws, "xo.work"))
	t.Logf("peak RSS: 1 module workspace %d MB, 301 modules %d MB", small>>20, large>>20)
	if large > small+small/4+(32<<20) {
		t.Errorf("a 301 module workspace raised a one-module check's peak RSS from %d MB to %d MB", small>>20, large>>20)
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
