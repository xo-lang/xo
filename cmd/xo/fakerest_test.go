package main

import (
	"os"
	"path/filepath"
	"testing"
)

const fakeRestTests = `use std/http

fn fetch(net: Net, url: Str) -> Str uses net.dial {
  match http.client(net).get(url) {
    Ok(r) => "${r.status} ${r.body.to_str() ?? ""}"
    Err(e) => e.display()
  }
}

test "a rest parameter takes the empty piece after a trailing slash, the root included" {
  os.net.handle("GET https://rest.example.test/{rest...}", fn(req: http.Request) -> http.Response { http.text(200, "rest=${req.param("rest")}") })
  os.net.handle("GET https://rest.example.test/files/{path...}", fn(req: http.Request) -> http.Response { http.text(200, "path=${req.param("path")}") })
  expect(fetch(os.net, "https://rest.example.test/")) == "200 rest="
  expect(fetch(os.net, "https://rest.example.test")) == "200 rest="
  expect(fetch(os.net, "https://rest.example.test/?comp=properties")) == "200 rest="
  expect(fetch(os.net, "https://rest.example.test/a/b")) == "200 rest=a/b"
  expect(fetch(os.net, "https://rest.example.test/files/")) == "200 path="
  expect(fetch(os.net, "https://rest.example.test/files")) == "200 rest=files"
  expect(fetch(os.net, "https://rest.example.test/files/x/y")) == "200 path=x/y"
}

test "a fake pattern takes Go 1.22's forms, as the Router does" {
  os.net.handle("GET https://sub.example.test/", fn(req: http.Request) -> http.Response { http.text(200, "sub") })
  os.net.handle("GET https://ex.example.test/users/{id}", fn(req: http.Request) -> http.Response { http.text(200, "user ${req.param("id")}") })
  os.net.handle("GET https://d.example.test/{$}", fn(req: http.Request) -> http.Response { http.text(200, "dollar") })
  os.net.handle("GET https://d.example.test/dir/", fn(req: http.Request) -> http.Response { http.text(200, "dir") })
  var got: List[Str] = []
  for u in ["https://sub.example.test/", "https://sub.example.test", "https://sub.example.test/x/y", "https://ex.example.test/users/7", "https://ex.example.test/users/7/", "https://d.example.test/", "https://d.example.test/x", "https://d.example.test/dir/", "https://d.example.test/dir/a", "https://d.example.test/dir"] {
    got.push("${u} -> ${fetch(os.net, u)}")
  }
  expect(got.join("\n")) == "https://sub.example.test/ -> 200 sub\nhttps://sub.example.test -> 200 sub\nhttps://sub.example.test/x/y -> 200 sub\nhttps://ex.example.test/users/7 -> 200 user 7\nhttps://ex.example.test/users/7/ -> no fake handler for GET https://ex.example.test/users/7/\nhttps://d.example.test/ -> 200 dollar\nhttps://d.example.test/x -> no fake handler for GET https://d.example.test/x\nhttps://d.example.test/dir/ -> 200 dir\nhttps://d.example.test/dir/a -> 200 dir\nhttps://d.example.test/dir -> no fake handler for GET https://d.example.test/dir"
}
`

// TestFakeNetRestRoot: on every backend a FakeNet pattern and path split
// as the Router splits them (a trailing slash a subtree, `{$}`, an exact
// path), and a `{rest...}` pattern
// matches a path that ends in a slash before it, "/" included, with the
// parameter "", as on the Go backend (natively it did not since e810443d4: Xo Azure, aztables'
// service get_properties on the account root, 2026-11-11).
func TestFakeNetRestRoot(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("builds test binaries on every backend")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "xo.mod"), []byte("module example.test/fakerest\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "rest_test.xo"), []byte(fakeRestTests), 0o644); err != nil {
		t.Fatal(err)
	}
	setenv(t, "XO_BUILD_DIR", filepath.Join(t.TempDir(), "build"))
	for _, be := range append([]string{"go"}, nativeBackends(t)...) {
		if code, out := xoTest(t, "--backend="+be, "-v", dir); code != 0 {
			t.Errorf("%s: exit %d\n%s", be, code, out)
		}
	}
}
