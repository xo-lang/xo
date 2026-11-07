package main

import (
	"os"
	"path/filepath"
	"testing"
)

// fakeDeadlineTests are FakeNet connection deadlines in a test world
// (decision 0430): the deadline is on the test's virtual clock, so a
// waiting read or write is woken by it.
const fakeDeadlineTests = `fn show_read(c: Conn) -> Str {
  match c.read(64) {
    Ok(Some(b)) => "data ${b.to_str() ?? "?"}"
    Ok(None) => "end"
    Err(e) => "err ${e}"
  }
}

fn show_sread(c: ServerConn) -> Str {
  match c.read(64) {
    Ok(Some(b)) => "data ${b.to_str() ?? "?"}"
    Ok(None) => "end"
    Err(e) => "err ${e}"
  }
}

test "a read waiting past its deadline times out, and reads after a later one" {
  let l = os.net.listen("h.example.test:5672").or_fault("listen")
  let peer = Chan[ServerConn].new(cap: 1)
  scope s {
    s.spawn(fn() {
      if let Ok(sc) = l.accept() {
        peer.send(sc)
      }
    })
    let c = os.net.dial(l.addr()).or_fault("dial")
    let sc = peer.recv().or_fault("accept")
    c.set_read_deadline(os.clock, Some(os.clock.now() + 200ms))
    expect(show_read(c)) == "err i/o timeout"
    c.set_read_deadline(os.clock, Some(os.clock.now() + 5s))
    sc.write("hello".bytes()).or_fault("write")
    expect(show_read(c)) == "data hello"
  }
}

test "a deadline set while a read waits wakes it" {
  let l = os.net.listen("w.example.test:1").or_fault("listen")
  let c = os.net.dial("w.example.test:1").or_fault("dial")
  let sc = l.accept().or_fault("accept")
  scope s {
    s.spawn(fn() {
      os.clock.sleep(50ms)
      sc.set_read_deadline(os.clock, Some(os.clock.now()))
    })
    expect(show_sread(sc)) == "err i/o timeout"
  }
  c.close()
}

test "a write deadline stops a writer whose peer does not read" {
  let l = os.net.listen("f.example.test:1").or_fault("listen")
  let c = os.net.dial("f.example.test:1").or_fault("dial")
  // sc is used after the write: natively a connection is closed when it
  // is dropped, at its last use (std 3.2), and a closed peer is a reset
  let sc = l.accept().or_fault("accept")
  c.set_write_deadline(os.clock, Some(os.clock.now() + 100ms))
  expect(c.write("x".bytes().repeat(256 * 1024)) == Err(NetErr.Timeout)) == true
  sc.close()
}
`

// TestFakeNetDeadlineWakes: on every backend a FakeNet read or write that
// waits is woken by its deadline. The Go backend set the deadline in real
// time, so a test world reported a deadlock ("no timer is pending") and
// a waiting read was never woken (Xo Azure 1, 2026-11-06: three Azure
// SDK receive tests).
func TestFakeNetDeadlineWakes(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("builds test binaries on every backend")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "xo.mod"), []byte("module example.test/fakedl\nedition preview\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "dl_test.xo"), []byte(fakeDeadlineTests), 0o644); err != nil {
		t.Fatal(err)
	}
	setenv(t, "XO_BUILD_DIR", filepath.Join(t.TempDir(), "build"))
	for _, be := range append([]string{"go"}, nativeBackends(t)...) {
		if code, out := xoTest(t, "--backend="+be, "-v", dir); code != 0 {
			t.Errorf("%s: exit %d\n%s", be, code, out)
		}
	}
}
