package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/mod/modtest"
)

// TestAttenuationFakes checks `only` on the test fakes (decision 0156,
// std.md 8): an attenuated FakeNet, FakeEnv, and FakeProc deny what is
// not listed and pass the rest to the fake; native test builds report
// `only` as XO1201 (planned). internal/gen's cap_attenuation golden
// covers the real capabilities.
func TestAttenuationFakes(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XO_BUILD_DIR", filepath.Join(dir, ".build"))
	t.Setenv("XO_WORK", "off")
	modtest.WriteFiles(t, dir, map[string]string{
		"xo.mod": "module example.com/att\nedition preview\n",
		"lib.xo": "use std/http\n\npub fn fetch(n: Net, url: Str) -> Int uses net.dial {\n  match http.client(n).get(url) {\n    Ok(r) => r.status\n    Err(_) => 0\n  }\n}\n",
		"lib_test.xo": `use std/http

test "a library given an attenuated fake reaches only the listed host" {
  os.net.handle("GET https://api.test/ok", fn(_req) { http.text(200, "ok") })
  os.net.handle("GET https://evil.test/steal", fn(_req) { http.text(200, "stolen") })
  let api = os.net.only(["api.test"])
  expect(fetch(api, "https://api.test/ok")) == 200
  expect(fetch(api, "https://evil.test/steal")) == 0
  expect(os.net.requests().len()) == 1
  expect(api.dial("evil.test:443").err()) == Some(NetErr.Denied(addr: "evil.test:443"))
}

test "an attenuated fake env and proc" {
  os.env.set("APP_PORT", "8080")
  os.env.set("AWS_SECRET_ACCESS_KEY", "s3cr3t")
  let env = os.env.only(["APP_PORT"])
  expect(env.get("APP_PORT")) == Some("8080")
  expect(env.get("AWS_SECRET_ACCESS_KEY")) == None
  expect(env.vars().len()) == 1
  os.proc.on_run("git", ProcOutput{code: 0, stdout: b"ok", stderr: b""})
  let p = os.proc.only(["git"])
  expect(p.run("git", []).is_ok()) == true
  expect(p.run("curl", ["evil.test"]).err()) == Some(ProcErr.Denied(cmd: "curl"))
}
`,
	})
	r := xo(t, "test", dir).want(0)
	if !strings.Contains(r.out, "2 passed") {
		t.Errorf("xo test: %s %s", r.out, r.err)
	}
	r = xo(t, "test", "--no-gc", dir).want(1)
	if out := r.out + r.err; !strings.Contains(out, "XO1201") || !strings.Contains(out, "FakeNet.only") {
		t.Errorf("native test build of only: %s %s", r.out, r.err)
	}
}
