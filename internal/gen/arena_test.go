package gen_test

import (
	"fmt"
	"math/rand/v2"
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
  n += m.values().sort_by(fn(v) { v }).len()
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

// TestArenaConfineRandom generates one program with many routes whose
// handler bodies are random mixes of request uses, each either a confined
// form (a Str or decoded value comes out, never the request's memory) or
// an escaping one (the request, a table, the body, or a capture by a
// nested closure), optionally nested in if, match, scope, and loop
// bodies. A route must be marked arena (rt.Http_Router_arenaLast) exactly
// when its body has no escaping form (decision 0066).
func TestArenaConfineRandom(t *testing.T) {
	safe := []string{
		`let _v%d = req.param("id")`,
		`let _v%d = req.param_int("n")`,
		`let _v%d = req.header("x") ?? ""`,
		`let _v%d = req.query("q") ?? ""`,
		`let _v%d = req.json[In]().ok()`,
		`let _v%d = req.method`,
		`let _v%d = req.path`,
		`let _v%d = req.remote_ip`,
		`let _v%d = req.url.path`,
		`let _v%d = req.url.host`,
		`keep.with(fn(var l) { l.push("x%d") })`,
		`let _v%d = fn() { 1 }`,
	}
	escape := []string{
		`let _v%d = req`,
		`let _v%d = req.headers`,
		`let _v%d = req.body`,
		`let _v%d = req.url`,
		`let _v%d = req.url.query`,
		`let _v%d = name_of(req)`,
		`let _v%d = fn() { req.param("id") }`,
		`keep.with(fn(var l) { l.push(req.param("id%d")) })`,
		`let _v%d = [req]`,
		`let _v%d = Some(req)`,
	}
	wrap := []string{"%s", "if keep.with(fn(l) { l.len() }) > 0 {\n%s\n}", "for _i in 0..2 {\n%s\n}", "scope s {\n%s\n}", "match 1 {\n1 => {\n%s\n}\n_ => {}\n}"}
	rng := rand.New(rand.NewPCG(66, 2026))
	var src strings.Builder
	src.WriteString("use std/http\n\nstruct In {\n  name: Str\n} derive Json\n\nfn name_of(r: http.Request) -> Str { r.path }\n\nfn main(os: Os) {}\n\nfn routes(keep: Mutex[List[Str]]) -> http.Router {\n  var r = http.router()\n")
	const n = 160
	want := make([]bool, n)
	k := 0
	for i := 0; i < n; i++ {
		confined := true
		var body []string
		for j := 0; j < 1+rng.IntN(5); j++ {
			var form string
			if rng.IntN(4) == 0 {
				form, confined = escape[rng.IntN(len(escape))], false
			} else {
				form = safe[rng.IntN(len(safe))]
			}
			k++
			form = strings.ReplaceAll(form, "%d", fmt.Sprint(k))
			body = append(body, fmt.Sprintf(wrap[rng.IntN(len(wrap))], form))
		}
		want[i] = confined
		fmt.Fprintf(&src, "  r.handle(\"GET /r%d/{id}\", fn(req) {\n%s\n    http.text(200, \"ok\")\n  })\n", i, strings.Join(body, "\n"))
	}
	src.WriteString("  r\n}\n")
	nConfined := 0
	for _, c := range want {
		if c {
			nConfined++
		}
	}
	if nConfined < n/5 || nConfined > n*4/5 {
		t.Fatalf("%d of %d routes confined: the mix tests too little", nConfined, n)
	}
	got := generateArenas(t, src.String(), true)
	for i := 0; i < n; i++ {
		start := strings.Index(got, fmt.Sprintf(`"GET /r%d/{id}"`, i))
		if start < 0 {
			t.Fatalf("route %d not found", i)
		}
		end := len(got)
		if e := strings.Index(got[start:], fmt.Sprintf(`"GET /r%d/{id}"`, i+1)); e >= 0 {
			end = start + e
		}
		if marked := strings.Contains(got[start:end], "Http_Router_arenaLast"); marked != want[i] {
			t.Errorf("route %d: arena %v, want %v", i, marked, want[i])
		}
	}
	if t.Failed() {
		t.Logf("source:\n%s", src.String())
	}
}
