package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/mod/modtest"
	"github.com/xo-lang/xo/internal/xotool"
)

// TestVouchEdges checks decision 0156's rules past TestVouches: a vouch in
// an intermediate dependency does not cover its own dependency, a vouch
// must name the use line's exact Go path and kind, a workspace module's
// vouch counts for the program, a workspace module's own use go needs
// none, and xo get refuses an escape hatch in a package the program does
// not import (core 4.6 Vouches).
func TestVouchEdges(t *testing.T) {
	t.Parallel()
	repoWorld(t, map[string]string{
		"github.com/acme/low@v1.0.0/xo.mod":       "module github.com/acme/low\nedition preview\n",
		"github.com/acme/low@v1.0.0/low.xo":       "use go \"os\"\n\npub fn pid() -> Int uses go { os.getpid() }\n",
		"github.com/acme/mid@v1.0.0/xo.mod":       "module github.com/acme/mid\nedition preview\n\nrequire github.com/acme/low v1.0.0\n\nvouch github.com/acme/low v1.0.0 go \"os\"\n",
		"github.com/acme/mid@v1.0.0/mid.xo":       "use github.com/acme/low\n\npub fn pid() -> Int uses go { low.pid() }\n",
		"github.com/acme/split@v1.0.0/xo.mod":     "module github.com/acme/split\nedition preview\n",
		"github.com/acme/split@v1.0.0/split.xo":   "pub fn one() -> Int { 1 }\n",
		"github.com/acme/split@v1.0.0/sub/sub.xo": "use go \"os\"\n\npub fn pid() -> Int uses go { os.getpid() }\n",
	})
	ws := t.TempDir()
	setenv(t, "XO_BUILD_DIR", filepath.Join(ws, ".build"))
	app := filepath.Join(ws, "app")
	main := "use github.com/acme/mid\n\nfn main(os: Os) {\n  os.stdio.println(\"${mid.pid() > 0}\")\n}\n"
	xomod := "module example.com/app\nedition preview\n\nrequire github.com/acme/mid v1.0.0\nrequire github.com/acme/low v1.0.0\n"
	modtest.WriteFiles(t, app, map[string]string{"xo.mod": xomod, "main/main.xo": main})
	xo(t, "mod", "tidy", "--dir", app).want(0)
	setMod := func(extra string) {
		b, _ := os.ReadFile(filepath.Join(app, "xo.mod"))
		s := string(b)
		if i := strings.Index(s, "\nvouch "); i >= 0 {
			s = s[:i+1]
		}
		os.WriteFile(filepath.Join(app, "xo.mod"), []byte(strings.TrimRight(s, "\n")+"\n"+extra), 0o644)
	}
	// mid's vouch for low does not count: only the consumer's
	r := xo(t, "check", "--json", app+"/...").want(1)
	if got := diagCodes(t, r.out); got != "XO1316" || !strings.Contains(r.out, `vouch github.com/acme/low v1.0.0 go \"os\"`) {
		t.Errorf("a dependency's vouch for its dependency: %s\n%s", got, r.out)
	}
	for _, tc := range []struct{ vouch, want string }{
		{"vouch github.com/acme/low v1.0.0 go \"os/exec\"\n", "XO1316"}, // another Go package
		{"vouch github.com/acme/low v1.0.0 c \"os\"\n", "XO1316"},       // another kind
		{"vouch github.com/acme/mid v1.0.0 go \"os\"\n", "XO1316"},      // another module
		{"vouch github.com/acme/low v1.0.0 go \"os\"\n", ""},
	} {
		setMod(tc.vouch)
		r := xo(t, "check", "--json", app+"/...")
		if got := diagCodes(t, r.out); got != tc.want {
			t.Errorf("%s: %s, want %q\n%s", strings.TrimSpace(tc.vouch), got, tc.want, r.out)
		}
	}

	// a workspace: helper's vouch covers low for the whole program, and
	// helper's own use go needs none
	setMod("")
	r = xo(t, "check", "--json", app+"/...")
	if got := diagCodes(t, r.out); got != "XO1316" {
		t.Errorf("the vouch removed again: %s\n%s", got, r.out)
	}
	helper := filepath.Join(ws, "helper")
	modtest.WriteFiles(t, helper, map[string]string{
		"xo.mod":    "module example.com/helper\nedition preview\n\nrequire github.com/acme/low v1.0.0\n\nvouch github.com/acme/low v1.0.0 go \"os\"\n",
		"helper.xo": "use go \"os\"\nuse github.com/acme/low\n\npub fn both() -> Int uses go { os.getpid() + low.pid() }\n",
	})
	xo(t, "mod", "tidy", "--dir", helper).want(0)
	os.WriteFile(filepath.Join(ws, "xo.work"), []byte("xo preview\nuse ./app\nuse ./helper\n"), 0o644)
	r = xo(t, "check", "--json", app+"/...", helper+"/...")
	if got := diagCodes(t, r.out); got != "" {
		t.Errorf("workspace module's vouch: %s\n%s", got, r.out)
	}
	r = xo(t, "check", "--workspace=off", "--json", app+"/...")
	if got := diagCodes(t, r.out); got != "XO1316" {
		t.Errorf("--workspace=off drops helper's vouch: %s\n%s", got, r.out)
	}
	r = xo(t, "check", "--json", app+"/...")
	if got := diagCodes(t, r.out); got != "" {
		t.Errorf("the workspace's own check of app alone: %s\n%s", got, r.out)
	}
	os.Remove(filepath.Join(ws, "xo.work"))
	r = xo(t, "check", "--json", app+"/...")
	if got := diagCodes(t, r.out); got != "XO1316" {
		t.Errorf("app with the workspace file removed: %s\n%s", got, r.out)
	}

	// split's escape hatch is in sub/, which the program does not import:
	// xo get refuses it from syntax; the check of the program reports
	// what the build loads
	app2 := filepath.Join(ws, "app2")
	xomod2 := "module example.com/app2\nedition preview\n"
	modtest.WriteFiles(t, app2, map[string]string{
		"xo.mod":       xomod2,
		"main/main.xo": "use github.com/acme/split\n\nfn main(os: Os) {\n  os.stdio.println(\"${split.one()}\")\n}\n",
	})
	r = xo(t, "get", "--json", "--dir", app2, "github.com/acme/split@v1.0.0").want(1)
	if got := diagCodes(t, r.out); got != "XO1316" || !strings.Contains(r.out, `vouch github.com/acme/split v1.0.0 go \"os\"`) {
		t.Errorf("xo get of a version with an unimported escape hatch: %s\n%s", got, r.out)
	}
	if b, _ := os.ReadFile(filepath.Join(app2, "xo.mod")); string(b) != xomod2 {
		t.Errorf("refused xo get changed xo.mod:\n%s", b)
	}
	os.WriteFile(filepath.Join(app2, "xo.mod"), []byte(xomod2+"\nvouch github.com/acme/split v1.0.0 go \"os\"\n"), 0o644)
	xo(t, "get", "--dir", app2, "github.com/acme/split@v1.0.0").want(0)
	xo(t, "check", app2+"/...").want(0)
	r = xo(t, "run", filepath.Join(app2, "main")).want(0)
	if r.out != "1\n" {
		t.Errorf("app2 printed %q", r.out)
	}
}

// TestVouchCheckCache: removing a vouch line makes the next xo check
// report XO1316 again, with the module check cache on (decision 0351's
// addendum) as with it off.
func TestVouchCheckCache(t *testing.T) {
	t.Parallel()
	repoWorld(t, map[string]string{
		"github.com/acme/low@v1.0.0/xo.mod": "module github.com/acme/low\nedition preview\n",
		"github.com/acme/low@v1.0.0/low.xo": "use go \"os\"\n\npub fn pid() -> Int uses go { os.getpid() }\n",
	})
	ws := t.TempDir()
	setenv(t, "XO_BUILD_DIR", filepath.Join(ws, ".build"))
	app := filepath.Join(ws, "app")
	xomod := "module example.com/app\nedition preview\n\nrequire github.com/acme/low v1.0.0\n"
	modtest.WriteFiles(t, app, map[string]string{
		"xo.mod":       xomod,
		"main/main.xo": "use github.com/acme/low\n\nfn main(os: Os) {\n  os.stdio.println(\"${low.pid() > 0}\")\n}\n",
	})
	xo(t, "mod", "tidy", "--dir", app).want(0)
	b, _ := os.ReadFile(filepath.Join(app, "xo.mod"))
	tidied := string(b)
	steps := []struct{ vouch, want string }{
		{"", "XO1316"},
		{"vouch github.com/acme/low v1.0.0 go \"os\"\n", ""},
		{"", "XO1316"},
		{"vouch github.com/acme/low v1.0.0 go \"os/exec\"\n", "XO1316"},
	}
	for _, cache := range []string{"on", "off"} {
		if cache == "off" {
			setenv(t, "XO_CHK_CACHE", "off")
		}
		for i, st := range steps {
			os.WriteFile(filepath.Join(app, "xo.mod"), []byte(tidied+st.vouch), 0o644)
			r := xo(t, "check", "--json", app+"/...")
			if got := diagCodes(t, r.out); got != st.want {
				t.Errorf("check cache %s, step %d (vouch %q): %q, want %q", cache, i, strings.TrimSpace(st.vouch), got, st.want)
			}
		}
	}
}

// TestVouchBuildCache: a build after a vouch line is removed fails with
// XO1316 again, through the caches a build keeps: the Go backend's for a
// dependency's use go, the native front end cache for its use c.
func TestVouchBuildCache(t *testing.T) {
	t.Parallel()
	files := map[string]string{
		"github.com/acme/low@v1.0.0/xo.mod": "module github.com/acme/low\nedition preview\n",
		"github.com/acme/low@v1.0.0/low.xo": "use go \"os\"\n\npub fn pid() -> Int uses go { os.getpid() }\n",
	}
	_, clangErr := xotool.Clang()
	if clangErr == nil {
		files["github.com/acme/cz@v1.0.0/xo.mod"] = "module github.com/acme/cz\nedition preview\n"
		files["github.com/acme/cz@v1.0.0/tiny.h"] = "int tiny_add(int a, int b);\n"
		files["github.com/acme/cz@v1.0.0/tiny.c"] = "int tiny_add(int a, int b) { return a + b; }\n"
		files["github.com/acme/cz@v1.0.0/cz.xo"] = "use c \"tiny.h\" link \"tiny.c\" uses none\n\npub fn add(a: Int32, b: Int32) -> Int32 { tiny.tiny_add(a, b) }\n"
	}
	repoWorld(t, files)
	setenv(t, "XO_CBIND_DIR", "")
	ws := t.TempDir()
	setenv(t, "XO_BUILD_DIR", filepath.Join(ws, ".build"))
	type app struct {
		name, dep, ver, vouch, main string
		args                        []string
	}
	apps := []app{{"gobe", "github.com/acme/low", "v1.0.0", "vouch github.com/acme/low v1.0.0 go \"os\"\n",
		"use github.com/acme/low\n\nfn main(os: Os) {\n  os.stdio.println(\"${low.pid() > 0}\")\n}\n", []string{"--backend=go"}}}
	if clangErr == nil {
		apps = append(apps, app{"native", "github.com/acme/cz", "v1.0.0", "vouch github.com/acme/cz v1.0.0 c \"tiny.h\" uses none\n",
			"use github.com/acme/cz\n\nfn main(os: Os) {\n  os.stdio.println(\"${cz.add(2, 40)}\")\n}\n", []string{"--no-gc"}})
	} else {
		t.Logf("no clang (%v): the native use c case is skipped", clangErr)
	}
	for _, a := range apps {
		dir := filepath.Join(ws, a.name)
		modtest.WriteFiles(t, dir, map[string]string{
			"xo.mod":       "module example.com/" + a.name + "\nedition preview\n\nrequire " + a.dep + " " + a.ver + "\n",
			"main/main.xo": a.main,
		})
		xo(t, "mod", "tidy", "--dir", dir).want(0)
		b, _ := os.ReadFile(filepath.Join(dir, "xo.mod"))
		tidied := string(b)
		for i, st := range []struct {
			vouch string
			code  int
		}{{"", 1}, {a.vouch, 0}, {"", 1}, {a.vouch, 0}, {"", 1}} {
			os.WriteFile(filepath.Join(dir, "xo.mod"), []byte(tidied+st.vouch), 0o644)
			for _, cmd := range []string{"build", "run"} {
				args := append([]string{cmd}, a.args...)
				if cmd == "build" {
					args = append(args, "-o", filepath.Join(ws, a.name+".bin"))
				}
				r := xo(t, append(args, filepath.Join(dir, "main"))...)
				if r.code != st.code {
					t.Errorf("%s: xo %s, step %d (vouch %q): exit %d, want %d\n%s%s", a.name, cmd, i, strings.TrimSpace(st.vouch), r.code, st.code, r.out, r.err)
				} else if st.code == 1 && !strings.Contains(r.out+r.err, "XO1316") {
					t.Errorf("%s: xo %s, step %d: failed without XO1316\n%s%s", a.name, cmd, i, r.out, r.err)
				}
			}
		}
	}
}
