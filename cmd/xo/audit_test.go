package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/mod/modtest"
)

// TestAudit checks `xo audit` (decision 0156): per dependency effects,
// lock state, ceiling, and escape hatches with their vouches; the
// consuming module's attenuations and the capabilities it passes into
// dependencies; unused vouches; the exit status.
func TestAudit(t *testing.T) {
	repoWorld(t, map[string]string{
		"github.com/acme/osx@v1.0.0/xo.mod":     "module github.com/acme/osx\nedition preview\n",
		"github.com/acme/osx@v1.0.0/osx.xo":     "use go \"os\"\n\npub fn pid() -> Int uses go { os.getpid() }\n",
		"github.com/acme/weather@v1.0.0/xo.mod": "module github.com/acme/weather\nedition preview\n",
		"github.com/acme/weather@v1.0.0/w.xo":   "pub fn ping(n: Net) -> Bool uses net.dial { n.dial(\"api.test:443\").is_ok() }\n\npub fn conf(e: Env) -> Str? uses env { e.get(\"W_KEY\") }\n",
	})
	ws := t.TempDir()
	app := filepath.Join(ws, "app")
	xomod := "module example.com/app\nedition preview\n\nrequire github.com/acme/osx v1.0.0\nrequire github.com/acme/weather v1.0.0 uses net.dial, env\n\nvouch github.com/acme/osx v1.0.0 go \"os\"\nvouch github.com/acme/gone v1.0.0 go \"os/exec\"\n"
	modtest.WriteFiles(t, app, map[string]string{
		"xo.mod": xomod,
		"main/main.xo": `use github.com/acme/osx
use github.com/acme/weather

fn main(os: Os) {
  let api = os.net.only(["api.test"])
  let up = weather.ping(api)
  let key = weather.conf(os.env)
  os.stdio.println("${osx.pid() > 0} ${up} ${key ?? ""}")
}
`,
	})
	xo(t, "mod", "tidy", "--dir", app).want(0)
	r := xo(t, "audit", "--json", "--dir", app).want(0)
	var rep auditReport
	if err := json.Unmarshal([]byte(r.out), &rep); err != nil {
		t.Fatalf("bad JSON: %v\n%s", err, r.out)
	}
	if rep.Module != "example.com/app" || len(rep.Dependencies) != 2 {
		t.Fatalf("report: %s", r.out)
	}
	osx, w := rep.Dependencies[0], rep.Dependencies[1]
	if osx.Path != "github.com/acme/osx" || strings.Join(osx.Effects, ",") != "go" || osx.Lock != "ok" || len(osx.Escapes) != 1 || !osx.Escapes[0].Vouched {
		t.Errorf("osx: %+v", osx)
	}
	if w.Lock != "ok" || w.Ceiling != "uses net.dial, env" || strings.Join(w.Effects, ",") != "env,net.dial" {
		t.Errorf("weather: %+v", w)
	}
	if len(rep.Attenuations) != 1 || rep.Attenuations[0].Method != "only" || !rep.Attenuations[0].Constant || strings.Join(rep.Attenuations[0].Args, ",") != "api.test" {
		t.Errorf("attenuations: %+v", rep.Attenuations)
	}
	if len(rep.Capabilities) != 2 || !rep.Capabilities[0].Attenuated || rep.Capabilities[0].Via != "Net.only(api.test)" || rep.Capabilities[1].Attenuated || rep.Capabilities[1].Capability != "Env" {
		t.Errorf("capabilities: %+v", rep.Capabilities)
	}
	if len(rep.UnusedVouches) != 1 || !strings.Contains(rep.UnusedVouches[0].Text, "acme/gone") {
		t.Errorf("unused vouches: %+v", rep.UnusedVouches)
	}
	if rep.Summary.UnattenuatedCaps != 1 || rep.Summary.Unvouched != 0 || rep.Summary.Errors != 0 {
		t.Errorf("summary: %+v", rep.Summary)
	}

	// without the vouch, and with the lock recording less: exit 1
	os.WriteFile(filepath.Join(app, "xo.mod"), []byte(strings.Replace(xomod, "vouch github.com/acme/osx v1.0.0 go \"os\"\n", "", 1)), 0o644)
	lock := filepath.Join(app, "xo.lock")
	data, _ := os.ReadFile(lock)
	os.WriteFile(lock, []byte(strings.Replace(string(data), "weather effects env,net.dial", "weather effects env", 1)), 0o644)
	r = xo(t, "audit", "--json", "--dir", app).want(1)
	rep = auditReport{}
	json.Unmarshal([]byte(r.out), &rep)
	if rep.Summary.Unvouched != 1 || rep.Summary.Widened != 1 || rep.Dependencies[1].Lock != "widened" || strings.Join(rep.Dependencies[1].Added, ",") != "net.dial" {
		t.Errorf("unvouched and widened: %+v\n%s", rep.Summary, r.out)
	}
	if got := diagCodes(t, r.out); got != "XO1317,XO1316" && got != "XO1316,XO1317" {
		t.Errorf("audit diagnostics: %s", got)
	}
	// the text form is for a terminal only (stdout is a pipe here, so
	// JSON); the Go printer this compared went with decision 0215
}
