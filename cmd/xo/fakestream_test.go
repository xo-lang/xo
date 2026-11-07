package main

import (
	"os"
	"path/filepath"
	"testing"
)

// fakeStreamTests are http.stream responses from FakeNet handlers in a
// test world: Client.send and Client.open get the stream's body, and
// open gets each flush as it is written.
const fakeStreamTests = `use std/http

test "an http.stream body through FakeNet" {
  os.net.handle("GET https://s.example.test/plain", fn(req: http.Request) -> http.Response {
    http.text(200, "abc")
  })
  os.net.handle("GET https://s.example.test/stream", fn(req: http.Request) -> http.Response {
    http.stream(200, fn(w: http.BodyWriter) -> () ! IoErr {
      w.write(b"abc")?
      w.flush()?
    })
  })
  let c = http.client(os.net, 5s)
  for p in ["plain", "stream"] {
    let req = http.new_request("GET", "https://s.example.test/${p}", {}, b"")
    let sent = c.send(req)?
    let st = c.open(req)?
    let opened = st.read_all()?
    st.close()
    expect("${p} send=${sent.body.len()} open=${opened.len()}") == "${p} send=3 open=3"
  }
}

test "an opened http.stream body arrives flush by flush" {
  let next = Chan[Int].new(1)
  os.net.handle("GET https://e.example.test/events", fn(req: http.Request) -> http.Response {
    http.stream(200, fn(w: http.BodyWriter) -> () ! IoErr {
      w.write(b"data: 1\n\n")?
      w.flush()?
      // the second event only after the client has read the first
      let _ = next.recv()
      w.write(b"data: 2\n\n")?
      w.flush()?
    })
  })
  let c = http.client(os.net, 5s)
  let st = c.open(http.new_request("GET", "https://e.example.test/events", {}, b""))?
  expect(st.read(64)?) == Some(b"data: 1\n\n")
  next.send(1)
  expect(st.read(64)?) == Some(b"data: 2\n\n")
  expect(st.read(64)?) == None
  st.close()
}

test "a FakeNet handler reads the whole body of an opened request" {
  os.net.handle("POST https://b.example.test/echo", fn(req: http.Request) -> http.Response {
    http.text(200, "got ${req.body.len()}")
  })
  let c = http.client(os.net, 5s)
  let st = c.open(http.new_request("POST", "https://b.example.test/echo", {}, b"hello"))?
  expect(st.read_all()?) == b"got 5"
  st.close()
}
`

// TestFakeNetStreamBody: on every backend a FakeNet handler's
// http.stream body reaches Client.send and Client.open (it arrived empty
// natively: Xo Azure 1, 2026-11-07, azcore's SSE tests), and open reads
// it flush by flush.
func TestFakeNetStreamBody(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("builds test binaries on every backend")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "xo.mod"), []byte("module example.test/fakestream\nedition preview\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "stream_test.xo"), []byte(fakeStreamTests), 0o644); err != nil {
		t.Fatal(err)
	}
	setenv(t, "XO_BUILD_DIR", filepath.Join(t.TempDir(), "build"))
	for _, be := range append([]string{"go"}, nativeBackends(t)...) {
		if code, out := xoTest(t, "--backend="+be, "-v", dir); code != 0 {
			t.Errorf("%s: exit %d\n%s", be, code, out)
		}
	}
}
