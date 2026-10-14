package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/mod/modtest"
)

// TestLogAttribution checks decision 0173 item 1: a log record written by
// a dependency's code carries the dependency ("[<path>@<version>]" after
// the level in text, the key "module" in JSON, Record.module in tests),
// the main module's records carry nothing, a dependency cannot attribute
// its record to another through a field named "module", and `xo audit`
// lists the dependency's log call sites with the attribution. Without
// -short, the native backends are checked the same way.
func TestLogAttribution(t *testing.T) {
	repoWorld(t, map[string]string{
		"evil.test/logleak@v1.0.0/xo.mod": "module evil.test/logleak\nedition preview\n",
		"evil.test/logleak@v1.0.0/l.xo": "use std/log\n\npub fn audit(token: Str) uses log {\n  log.warn(\"request\", {\"token\": token})\n}\n\n" +
			"pub fn forge() uses log {\n  log.info(\"forged\", {\"module\": \"example.com/app\"})\n}\n\n" +
			"pub fn each(xs: List[Str]) uses log {\n  for x in xs {\n    log.info(x)\n  }\n}\n",
	})
	ws := t.TempDir()
	t.Setenv("XO_BUILD_DIR", filepath.Join(ws, ".build"))
	app := filepath.Join(ws, "app")
	modtest.WriteFiles(t, app, map[string]string{
		"xo.mod": "module example.com/app\nedition preview\n\nrequire evil.test/logleak v1.0.0\n",
		"main/main.xo": "use std/log\nuse evil.test/logleak\n\nfn main(os: Os) {\n  log.info(\"start\")\n  logleak.audit(\"secret\")\n" +
			"  log.configure(os.stdio, format: log.Format.Json)\n  logleak.forge()\n  log.info(\"done\")\n}\n",
		"app_test.xo": "use std/log\nuse std/testing\nuse evil.test/logleak\n\ntest \"records carry the dependency\" {\n" +
			"  log.info(\"mine\")\n  logleak.each([\"a\"])\n  let rs = testing.logs()\n" +
			"  expect(rs.len()) == 2\n  expect(rs[0].module) == \"\"\n  expect(rs[1].module) == \"evil.test/logleak@v1.0.0\"\n}\n",
	})
	xo(t, "mod", "tidy", "--dir", app).want(0)
	r := xo(t, "run", filepath.Join(app, "main"))
	if r.code != 0 {
		t.Fatalf("run: %d\n%s\n%s", r.code, r.out, r.err)
	}
	checkAttributedLog(t, "go", r.err)
	// tests see Record.module
	if r := xo(t, "test", app); r.code != 0 {
		t.Errorf("xo test: %d\n%s\n%s", r.code, r.out, r.err)
	}
	// xo audit lists the log sites with the attribution
	r = xo(t, "audit", "--json", "--dir", app).want(0)
	var rep auditReport
	if err := json.Unmarshal([]byte(r.out), &rep); err != nil || len(rep.Dependencies) != 1 {
		t.Fatalf("audit: %v\n%s", err, r.out)
	}
	d := rep.Dependencies[0]
	if d.LogTag != "evil.test/logleak@v1.0.0" || len(d.LogSites) != 3 || d.LogSites[0].Call != "log.warn" || d.LogSites[0].Line != 4 {
		t.Errorf("audit log sites: %+v", d)
	}
	if testing.Short() {
		return
	}
	// the native backends attribute the same way
	for _, be := range nativeBackends(t) {
		r := xo(t, "run", "--backend="+be, filepath.Join(app, "main"))
		if r.code != 0 {
			t.Errorf("%s run: %d\n%s\n%s", be, r.code, r.out, r.err)
			continue
		}
		checkAttributedLog(t, be, r.err)
		if r := xo(t, "test", "--backend="+be, app); r.code != 0 {
			t.Errorf("%s xo test: %d\n%s\n%s", be, r.code, r.out, r.err)
		}
	}
}

// checkAttributedLog checks the four records of TestLogAttribution's
// program on backend be.
func checkAttributedLog(t *testing.T, be, stderr string) {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(stderr), "\n")
	if len(lines) != 4 {
		t.Fatalf("%s: want 4 records, got:\n%s", be, stderr)
	}
	// text: the main module's record is unchanged; the dependency's says
	// where it comes from
	if strings.Contains(lines[0], "[") || !strings.HasSuffix(lines[0], " INFO start") {
		t.Errorf("%s: main record: %q", be, lines[0])
	}
	if !strings.Contains(lines[1], " WARN [evil.test/logleak@v1.0.0] request token=secret") {
		t.Errorf("%s: dependency record: %q", be, lines[1])
	}
	// JSON: the key "module", which a dependency's own field cannot forge
	var forged, done map[string]any
	if err := json.Unmarshal([]byte(lines[2]), &forged); err != nil {
		t.Fatalf("%s: json: %v: %s", be, err, lines[2])
	}
	if forged["module"] != "evil.test/logleak@v1.0.0" || forged["msg"] != "forged" {
		t.Errorf("%s: forged record: %s", be, lines[2])
	}
	json.Unmarshal([]byte(lines[3]), &done)
	if _, ok := done["module"]; ok || done["msg"] != "done" {
		t.Errorf("%s: main JSON record: %s", be, lines[3])
	}
}
