package main

import (
	"bytes"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/gotool"
	"github.com/xo-lang/xo/internal/mod"
	"github.com/xo-lang/xo/internal/mod/modtest"
)

// xoModulesBinary builds selfhost/cli/modules/xomod (the module commands
// in Xo, decision 0178 stage 4) natively, or returns $XO_MODULES_BIN.
func xoModulesBinary(t *testing.T) string {
	t.Helper()
	if b := os.Getenv("XO_MODULES_BIN"); b != "" {
		return b
	}
	// a release layout (bin/xomod, lib/xo/std): xomod finds std beside
	// itself through Env.program as xo1 does, since the steps run with no
	// $XO_STD and a check (audit) needs the prelude
	root := t.TempDir()
	bin := filepath.Join(root, "bin", "xomod")
	std, err := filepath.Abs(filepath.Join("..", "..", "std"))
	if err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(filepath.Join(root, "lib", "xo"), 0o755)
	os.MkdirAll(filepath.Join(root, "bin"), 0o755)
	if err := os.Symlink(std, filepath.Join(root, "lib", "xo", "std")); err != nil {
		t.Fatal(err)
	}
	// and the Go binder helper in bin/, as a release ships it (`use go`)
	gb := exec.Command("go", "build", "-o", filepath.Join(root, "bin", "xo-gobind"), "github.com/xo-lang/xo/cmd/xo-gobind")
	gb.Env = append(os.Environ(), gotool.CacheEnv())
	if out, err := gb.CombinedOutput(); err != nil {
		t.Fatalf("building xo-gobind: %v\n%s", err, out)
	}
	t.Setenv("XO_BUILD_DIR", filepath.Join(t.TempDir(), "build"))
	var out, errOut bytes.Buffer
	if code := runXo([]string{"build", "--backend=arm64", "-o", bin, filepath.Join("..", "..", "selfhost", "cli", "modules", "xomod")}, &out, &errOut); code != 0 {
		t.Fatalf("building xomod: exit %d\n%s%s", code, out.String(), errOut.String())
	}
	return bin
}

// sameStep is one command run by both implementations.
type sameStep struct {
	args []string
	// edit changes the app before the command, in both copies.
	edit func(app string)
}

// TestModulesXoSame runs the same module commands with internal/mod (in
// process) and with the module commands in Xo (xomod), each on its own
// copy of an app with its own module cache, and requires the same exit
// status, standard output and error (paths normalized), xo.mod, xo.lock,
// vendor/ tree, and module cache trees after every step (decision 0178
// stage 4: hashes and lock files byte identical to internal/mod's).
func TestModulesXoSame(t *testing.T) {
	if testing.Short() && os.Getenv("XO_MODULES_BIN") == "" {
		t.Skip("builds xomod natively; set XO_MODULES_BIN to run it with -short")
	}
	bin := xoModulesBinary(t)
	t.Run("billing", func(t *testing.T) {
		modtest.Setup(t, "../../internal/mod/testdata/repos", mod.Compare)
		files := map[string]string{
			"xo.mod":       "module example.com/app\nedition preview\n\nrequire github.com/acme/billing v1.2.0\n",
			"main/main.xo": appMain,
		}
		steps := []sameStep{
			{args: []string{"mod", "tidy", "--json"}},
			{args: []string{"mod", "tidy"}},
			{args: []string{"get", "--json", "github.com/acme/billing@v1.0.0"}},
			{args: []string{"get", "github.com/acme/billing"}},
			{args: []string{"get", "--json", "github.com/acme/money@v1.3.0"}},
			{args: []string{"get", "github.com/acme/money@v1.3.0"}},
			{args: []string{"get", "-u", "--json"}},
			{args: []string{"vendor", "--json"}},
			{args: []string{"vendor"}},
			{args: []string{"mod", "tidy", "--json"}, edit: func(app string) {
				os.RemoveAll(filepath.Join(app, "vendor"))
				os.WriteFile(filepath.Join(app, "extra", "extra.xo"), []byte("use github.com/acme/money\n\npub fn f() -> Int { money.cents(1) }\n"), 0o644)
			}},
			{args: []string{"get", "--json", "github.com/acme/money@none"}, edit: func(app string) {
				os.RemoveAll(filepath.Join(app, "extra"))
			}},
			{args: []string{"mod", "tidy", "--json"}},
			{args: []string{"get", "--json", "github.com/acme/wrongpath@v1.0.0"}},
			{args: []string{"get", "--json", "github.com/acme/nothere"}},
			{args: []string{"get", "--json", "Not A Path"}},
			{args: []string{"get", "--json", "github.com/acme/billing@1.0"}},
			{args: []string{"get"}},
			{args: []string{"get", "--bogus"}},
			{args: []string{"mod"}},
			{args: []string{"mod", "bogus"}},
			{args: []string{"mod", "proxy"}},
			{args: []string{"mod", "proxy", "serve", "a", "b"}},
			{args: []string{"vendor", "--json", "--offline"}},
			{args: []string{"mod", "tidy", "--json", "--offline"}, edit: func(app string) {
				os.Remove(filepath.Join(app, "xo.lock"))
			}},
		}
		sameRuns(t, bin, files, steps)
	})
	t.Run("effects", func(t *testing.T) {
		tick := func(extra string) string {
			return "pub fn pure() -> Int { 1 }\n" + extra
		}
		repoWorld(t, map[string]string{
			"github.com/acme/tick@v1.0.0/xo.mod":  "module github.com/acme/tick\nedition preview\n",
			"github.com/acme/tick@v1.0.0/tick.xo": tick("pub fn load(f: Fs) -> Bool uses fs.read { f.exists(\"x\").ok() ?? false }\n"),
			"github.com/acme/tick@v1.1.0/xo.mod":  "module github.com/acme/tick\nedition preview\n",
			"github.com/acme/tick@v1.1.0/tick.xo": tick("pub fn load(f: Fs) -> Bool uses fs.read { f.exists(\"x\").ok() ?? false }\npub fn ping(n: Net) -> Bool uses net.dial { n.dial(\"a:1\").is_ok() }\n"),
			"github.com/acme/tick@v1.2.0/xo.mod":  "module github.com/acme/tick\nedition preview\n",
			"github.com/acme/tick@v1.2.0/tick.xo": tick("pub fn ping(n: Net) -> Bool uses net.dial { n.dial(\"a:1\").is_ok() }\n"),
			"github.com/acme/osx@v1.0.0/xo.mod":   "module github.com/acme/osx\nedition preview\n",
			"github.com/acme/osx@v1.0.0/osx.xo":   "use go \"os\"\n\npub fn pid() -> Int uses go { os.getpid() }\n",
		})
		files := map[string]string{
			"xo.mod":       "module example.com/app\nedition preview\n\nrequire github.com/acme/tick v1.0.0\n",
			"main/main.xo": "use github.com/acme/tick\n\nfn main(os: Os) {\n  os.stdio.println(\"${tick.pure()}\")\n}\n",
		}
		lockSet := func(from, to string) func(app string) {
			return func(app string) {
				p := filepath.Join(app, "xo.lock")
				data, _ := os.ReadFile(p)
				os.WriteFile(p, []byte(strings.Replace(string(data), from, to, 1)), 0o644)
			}
		}
		steps := []sameStep{
			{args: []string{"mod", "tidy", "--json"}},
			{args: []string{"get", "--json", "github.com/acme/tick@v1.1.0"}},
			{args: []string{"get", "github.com/acme/tick@v1.1.0"}},
			{args: []string{"mod", "accept-effects", "--json"}, edit: lockSet("tick effects fs.read", "tick effects none")},
			{args: []string{"mod", "accept-effects"}, edit: lockSet("tick effects fs.read", "tick effects none")},
			{args: []string{"mod", "accept-effects", "--json", "github.com/acme/tick"}, edit: lockSet("tick effects fs.read", "tick effects none")},
			{args: []string{"get", "--accept-effects", "github.com/acme/tick@v1.1.0"}},
			{args: []string{"get", "--json", "github.com/acme/tick@v1.2.0"}},
			{args: []string{"mod", "tidy", "--json"}, edit: lockSet("tick effects net.dial", "tick effects telepathy")},
			{args: []string{"get", "--json", "github.com/acme/osx@v1.0.0"}, edit: lockSet("tick effects telepathy", "tick effects net.dial")},
			{args: []string{"get", "github.com/acme/osx@v1.0.0"}},
			{args: []string{"get", "--json", "github.com/acme/osx@v1.0.0"}, edit: func(app string) {
				p := filepath.Join(app, "xo.mod")
				data, _ := os.ReadFile(p)
				os.WriteFile(p, append(data, []byte("vouch github.com/acme/osx v1.0.0 go \"os\"\n")...), 0o644)
			}},
			{args: []string{"vendor", "--json"}},
		}
		sameRuns(t, bin, files, steps)
	})
	t.Run("audit", func(t *testing.T) {
		repoWorld(t, map[string]string{
			"github.com/acme/osx@v1.0.0/xo.mod":     "module github.com/acme/osx\nedition preview\n",
			"github.com/acme/osx@v1.0.0/osx.xo":     "use go \"os\"\n\npub fn pid() -> Int uses go { os.getpid() }\n",
			"github.com/acme/weather@v1.0.0/xo.mod": "module github.com/acme/weather\nedition preview\n",
			"github.com/acme/weather@v1.0.0/w.xo":   "use std/log\n\npub fn ping(n: Net) -> Bool uses (net.dial, log) {\n  log.info(\"ping\")\n  n.dial(\"api.test:443\").is_ok()\n}\n\npub fn conf(e: Env) -> Str? uses env { e.get(\"W_KEY\") }\n",
		})
		xomod := "module example.com/app\nedition preview\n\nrequire github.com/acme/osx v1.0.0\nrequire github.com/acme/weather v1.0.0 uses net.dial, env, log\n\nvouch github.com/acme/osx v1.0.0 go \"os\"\nvouch github.com/acme/gone v1.0.0 go \"os/exec\"\n"
		files := map[string]string{
			"xo.mod":       xomod,
			"main/main.xo": "use github.com/acme/osx\nuse github.com/acme/weather\n\nfn main(os: Os) {\n  let api = os.net.only([\"api.test\"])\n  let up = weather.ping(api)\n  let key = weather.conf(os.env)\n  let f = os.fs.sub(\"data\")\n  os.stdio.println(\"${osx.pid() > 0} ${up} ${key ?? \"\"} ${f.exists(\"x\").ok() ?? false}\")\n}\n",
		}
		steps := []sameStep{
			{args: []string{"mod", "tidy", "--json"}},
			// the Go checker writes the `use go` binding the Xo checker
			// reads (xo-gobind stays Go, decision 0178)
			{args: []string{"audit", "--json"}, edit: func(app string) {
				var out, errOut bytes.Buffer
				if code := runXo([]string{"check", app}, &out, &errOut); code != 0 {
					t.Fatalf("binding check: %s%s", out.String(), errOut.String())
				}
			}},
			{args: []string{"audit"}},
			{args: []string{"audit", "--json"}, edit: func(app string) {
				os.WriteFile(filepath.Join(app, "xo.mod"), []byte(strings.Replace(xomod, "vouch github.com/acme/osx v1.0.0 go \"os\"\n", "", 1)), 0o644)
				lock := filepath.Join(app, "xo.lock")
				data, _ := os.ReadFile(lock)
				os.WriteFile(lock, []byte(strings.Replace(string(data), "weather effects env,log,net.dial", "weather effects env", 1)), 0o644)
			}},
		}
		sameRuns(t, bin, files, steps)
	})
}

// sameRuns runs steps in two copies of the app, with internal/mod and
// with xomod, comparing after each step.
func sameRuns(t *testing.T, bin string, files map[string]string, steps []sameStep) {
	t.Helper()
	ws := t.TempDir()
	t.Cleanup(func() { modtest.MakeWritable(ws) })
	sides := []struct{ name, app, cache string }{
		{"go", filepath.Join(ws, "go", "app"), filepath.Join(ws, "go", "cache")},
		{"xo", filepath.Join(ws, "xo", "app"), filepath.Join(ws, "xo", "cache")},
	}
	for _, s := range sides {
		modtest.WriteFiles(t, s.app, files)
		os.MkdirAll(filepath.Join(s.app, "extra"), 0o755)
	}
	for i, st := range steps {
		var got [2]string
		var trees [2]string
		for k, s := range sides {
			if st.edit != nil {
				st.edit(s.app)
			}
			t.Setenv("XO_MODCACHE", s.cache)
			args := append(append([]string{}, st.args...), "--dir", s.app)
			if st.args[0] == "mod" && (len(st.args) < 2 || st.args[1] == "proxy" || st.args[1] == "bogus") {
				args = st.args
			}
			var out, errOut bytes.Buffer
			var code int
			if s.name == "go" {
				code = runXo(args, &out, &errOut)
			} else {
				cmd := exec.Command(bin, args...)
				cmd.Stdout, cmd.Stderr = &out, &errOut
				err := cmd.Run()
				if ee, ok := err.(*exec.ExitError); ok {
					code = ee.ExitCode()
				} else if err != nil {
					t.Fatalf("xomod: %v", err)
				}
			}
			norm := func(x string) string {
				x = strings.ReplaceAll(x, s.app, "$APP")
				return strings.ReplaceAll(x, s.cache, "$CACHE")
			}
			modData, _ := os.ReadFile(filepath.Join(s.app, "xo.mod"))
			lockData, _ := os.ReadFile(filepath.Join(s.app, "xo.lock"))
			got[k] = "exit " + string(rune('0'+code)) + "\n--- stdout\n" + norm(out.String()) + "--- stderr\n" + norm(dropUsage(st.args[0], errOut.String())) + "--- xo.mod\n" + string(modData) + "--- xo.lock\n" + string(lockData)
			trees[k] = treeText(filepath.Join(s.app, "vendor")) + "--- cache\n" + cacheText(s.cache)
		}
		if os.Getenv("XO_SAME_LOG") != "" {
			t.Logf("step %d %v:\n%s", i, st.args, got[1])
		}
		if got[0] != got[1] {
			t.Errorf("step %d %v differs\n=== internal/mod\n%s\n=== xomod\n%s", i, st.args, got[0], got[1])
		}
		if trees[0] != trees[1] {
			t.Errorf("step %d %v: vendor or cache trees differ\n=== internal/mod\n%s\n=== xomod\n%s", i, st.args, trees[0], trees[1])
		}
	}
}

// treeText lists the files under dir with their contents' hashes and
// modes.
func treeText(dir string) string {
	var lines []string
	filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(dir, p)
		info, _ := d.Info()
		if d.IsDir() {
			lines = append(lines, rel+"/ "+info.Mode().String())
			return nil
		}
		data, _ := os.ReadFile(p)
		lines = append(lines, rel+" "+info.Mode().String()+" "+string(data))
		return nil
	})
	sort.Strings(lines)
	return strings.Join(lines, "\n") + "\n"
}

// cacheText is the module versions of a module cache with their tree
// hashes and modes (not the git clones under cache/).
func cacheText(dir string) string {
	var lines []string
	filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() && d.Name() == "cache" {
			return filepath.SkipDir
		}
		if d.IsDir() && strings.Contains(d.Name(), "@") {
			h, _ := mod.HashDir(p)
			rel, _ := filepath.Rel(dir, p)
			lines = append(lines, rel+" "+h+"\n"+treeText(p))
			return filepath.SkipDir
		}
		return nil
	})
	sort.Strings(lines)
	return strings.Join(lines, "")
}
