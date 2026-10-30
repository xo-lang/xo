package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestUseGoImplements (decision 0416): Xo functions implement Go's
// io.Reader and io.Writer (named callback parameters, one per method),
// io.Copy moves the bytes between them, an Xo `go.error` is the Go error
// a Go caller sees, and sort.Sort sorts an Xo list through sort.Interface.
func TestUseGoImplements(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("runs go list and go build")
	}
	setenv(t, "XO_GOBIND_DIR", "")
	dir := writeFiles(t, map[string]string{
		"xo.mod": "module impl\n",
		"main.xo": `use go "io"
use go "sort"
use std/go

fn main(os: Os) {
  let src = Chan[Bytes].new(cap: 4)
  src.send("hello, ".bytes())
  src.send("xo".bytes())
  src.close()
  let sink = Chan[Bytes].new(cap: 8)
  let r = io.reader(read: fn(max: Int) -> Bytes ! go.Error {
    if max < 1 {
      return Bytes.zeroed(0)
    }
    src.try_recv() ?? Bytes.zeroed(0)
  })
  let w = io.writer(write: fn(p: Bytes) -> Int ! go.Error {
    sink.send(p)
    p.len()
  })
  match io.copy(w, r) {
    Ok(n) => os.stdio.println("copied ${n}")
    Err(e) => os.stdio.println("copy ${e.message()}")
  }
  sink.close()
  var text = ""
  for b in sink {
    text = text + (b.to_str() ?? "?")
  }
  os.stdio.println(text)
  let bad = io.writer(write: fn(p: Bytes) -> Int ! go.Error { Err(go.error("disk full")) })
  match io.write_string(bad, "x") {
    Ok(_) => os.stdio.println("no error")
    Err(e) => os.stdio.println("write ${e.message()}")
  }
  let xs = Mutex[List[Int]].new([5, 2, 9, 1])
  sort.sort(sort.interface_(
    len: fn() -> Int { xs.with_read(fn(l) { l.len() }) },
    less: fn(i: Int, j: Int) -> Bool { xs.with_read(fn(l) { l[i] < l[j] }) },
    swap: fn(i: Int, j: Int) {
      xs.with(fn(var l) {
        let t = l[i]
        l[i] = l[j]
        l[j] = t
      })
    }))
  os.stdio.println(xs.with_read(fn(l) { "${l[0]} ${l[1]} ${l[2]} ${l[3]}" }))
}
`,
	})
	var out, errOut bytes.Buffer
	if code := run(t, []string{"run", "--backend=go", dir}, &out, &errOut); code != 0 {
		t.Fatalf("run: exit %d\n%s%s", code, out.String(), errOut.String())
	}
	want := "copied 9\nhello, xo\nwrite disk full\n1 2 5 9"
	if got := strings.TrimSpace(out.String()); got != want {
		t.Errorf("output\n%s\nwant\n%s", got, want)
	}
}

// TestUseGoImplementsHandler (decision 0416): an Xo http.Handler serves
// two concurrent requests through httptest; each call runs as a task of
// its own (Go calls ServeHTTP from its server's goroutines). The client
// reads each body with io.read_all(resp.body().as_reader()) (an
// io.ReadCloser given as an io.Reader).
func TestUseGoImplementsHandler(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("runs go list and go build")
	}
	setenv(t, "XO_GOBIND_DIR", "")
	dir := writeFiles(t, map[string]string{
		"xo.mod": "module handler\n",
		"main.xo": `use go "io"
use go "net/http"
use go "net/http/httptest"

fn get(url: Str) -> Str {
  match http.get(url) {
    Ok(resp) => {
      let body = io.read_all(resp.body().as_reader()).ok() ?? "".bytes()
      "${resp.status_code()} ${body.to_str() ?? "?"}"
    }
    Err(e) => "error ${e.message()}"
  }
}

fn main(os: Os) {
  let h = http.handler(serve_http: fn(w, r) {
    w.write_header(if r.request_uri() == "/a" { 201 } else { 202 })
    let _ = w.write("hello ${r.request_uri()}".bytes()).ignore_err("a client that went away")
  })
  let srv = httptest.new_server(h)
  let base = srv.url()
  let (a, b) = scope s {
    let x = s.spawn(fn() { get("${base}/a") })
    let y = s.spawn(fn() { get("${base}/b") })
    (x.await, y.await)
  }
  srv.close()
  os.stdio.println(a)
  os.stdio.println(b)
}
`,
	})
	var out, errOut bytes.Buffer
	if code := run(t, []string{"run", "--backend=go", dir}, &out, &errOut); code != 0 {
		t.Fatalf("run: exit %d\n%s%s", code, out.String(), errOut.String())
	}
	want := "201 hello /a\n202 hello /b"
	if got := strings.TrimSpace(out.String()); got != want {
		t.Errorf("output\n%s\nwant\n%s", got, want)
	}
}
