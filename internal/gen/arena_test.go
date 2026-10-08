package gen_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/build"
	"github.com/xo-lang/xo/internal/gen"
)

// generateArenas checks and generates src with the given arena setting.
func generateArenas(t *testing.T, src string, arenas bool) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	res, ok, err := build.Check([]string{dir}, false)
	if err != nil || !ok {
		t.Fatalf("check: %v %v", err, res.Diagnostics)
	}
	prog, err := gen.Generate(res, gen.Options{Mode: gen.Build, Arenas: arenas})
	if err != nil {
		t.Fatal(err)
	}
	if arenas && !strings.Contains(string(prog.Files["main.go"]), "EnableArenas()") {
		t.Errorf("main.go does not enable arenas")
	}
	return goSource(prog)
}

// TestArenaEscapeAnalysis checks the decisions of internal/gen/arena.go:
// which handlers confine their request (rt.Http_Router_arenaLast), which
// response calls are in return position (Http_json_arena), and which
// collection temporaries are arena memory (Map_values_arena and friends).
// Each case is one handler or function; want lists substrings that must
// appear and wantNot substrings that must not.
func TestArenaEscapeAnalysis(t *testing.T) {
	const head = `
struct In {
  name: Str
} derive Json

fn main(os: Os) {}
`
	cases := []struct {
		name    string
		src     string
		want    []string
		wantNot []string
	}{
		{"confined json param header", `
fn routes() -> http.Router {
  var r = http.router()
  r.handle("POST /x/{id:Int}", fn(req) {
    let n = req.json[In]().ok() ?? In{name: ""}
    http.json(200, {"n": n.name, "id": "${req.param_int("id")}", "h": req.header("x") ?? "", "m": req.method})
  })
  r
}`, []string{"Http_Router_arenaLast", "Http_json_arena"}, nil},
		{"body field escapes", `
fn routes(keep: Mutex[List[Bytes]]) -> http.Router {
  var r = http.router()
  r.handle("POST /x", fn(req) {
    keep.with(fn(var l) { l.push(req.body) })
    http.empty(204)
  })
  r
}`, nil, []string{"Http_Router_arenaLast"}},
		{"headers map escapes", `
fn routes() -> http.Router {
  var r = http.router()
  r.handle("GET /x", fn(req) {
    let h = req.headers
    http.text(200, "${h.len()}")
  })
  r
}`, []string{"Http_text_arena"}, []string{"Http_Router_arenaLast"}},
		{"request captured by a nested closure", `
fn routes(g: task.Group) -> http.Router {
  var r = http.router()
  r.handle("GET /x/{id}", fn(req) {
    g.spawn_background(fn() { let _ = req.param("id") })
    http.empty(204)
  })
  r
}`, nil, []string{"Http_Router_arenaLast"}},
		{"request passed to a function", `
fn name_of(req: http.Request) -> Str { req.param("id") }

fn routes() -> http.Router {
  var r = http.router()
  r.handle("GET /x/{id}", fn(req) { http.text(200, name_of(req)) })
  r
}`, []string{"Http_text_arena"}, []string{"Http_Router_arenaLast"}},
		{"response bound before return is not arena", `
fn routes(keep: Mutex[List[http.Response]]) -> http.Router {
  var r = http.router()
  r.handle("GET /x", fn(_req) {
    let resp = http.text(200, "hi")
    keep.with(fn(var l) { l.push(resp) })
    resp
  })
  r
}`, []string{"Http_Router_arenaLast"}, []string{"Http_text_arena"}},
		{"response with a header added is not arena", `
fn routes() -> http.Router {
  var r = http.router()
  r.handle("GET /x", fn(_req) { http.text(200, "hi").with_header("X-A", "b") })
  r
}`, []string{"Http_Router_arenaLast"}, []string{"Http_text_arena"}},
		{"response in if and match arms and return", `
fn routes() -> http.Router {
  var r = http.router()
  r.handle("GET /x/{id:Int}", fn(req) {
    let id = req.param_int("id")
    if id == 0 {
      return http.text(404, "none")
    }
    match id {
      1 => http.text(200, "one")
      _ => if id > 5 { http.json(200, [id]) } else { http.text(200, "small") }
    }
  })
  r
}`, []string{"Http_text_arena(tk, int64(404)", "Http_text_arena(tk, int64(200), \"one\")", "Http_json_arena[", "\"small\""}, []string{"Http_text(tk"}},
		{"temporaries of consumers and loops", `
fn stats(m: Map[Int, Str], s: Set[Int]) -> Int {
  var n = m.values().fold(0, fn(acc, v) { acc + v.len() })
  n += m.keys().len()
  for (k, _) in m.entries() {
    n += k
  }
  if s.to_list().contains(3) { n += 1 }
  n
}`, []string{"Map_values_arena[", "Map_keys_arena[", "Map_entries_arena[", "Set_to_list_arena["}, nil},
		{"stored or returned producer results are not temporaries", `
fn keep(m: Map[Str, Str]) -> List[Str] {
  let vs = m.values()
  let ks = m.keys().reverse()
  let both = m.values() + ks
  vs + both
}`, []string{"Map_keys_arena["}, []string{"Map_values_arena["}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			uses := ""
			if strings.Contains(c.src, "http.") {
				uses += "use std/http\n"
			}
			if strings.Contains(c.src, "task.") {
				uses += "use std/task\n"
			}
			src := uses + head + c.src + "\n"
			got := generateArenas(t, src, true)
			for _, w := range c.want {
				if !strings.Contains(got, w) {
					t.Errorf("generated code lacks %q", w)
				}
			}
			for _, w := range c.wantNot {
				if strings.Contains(got, w) {
					t.Errorf("generated code contains %q", w)
				}
			}
			off := generateArenas(t, src, false)
			if strings.Contains(off, "_arena") || strings.Contains(off, "arenaLast") {
				t.Errorf("arena code generated without Arenas")
			}
		})
	}
}
