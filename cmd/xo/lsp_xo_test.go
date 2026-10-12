package main

// The requests of `xo lsp` that read the check, in Xo (selfhost/cli/servers
// resolve.xo and lsp_features.xo) against the Go server (decision 0178
// switch point B): with XO_CLI naming the self hosted binary.

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// lspFeatureProg exercises what internal/lsp resolve.go tells apart:
// declarations of every kind, receivers, parameters, closures, fields,
// variants, patterns, selectors, imports, and constants.
const lspFeatureProg = `use std/log
use std/strings as str

/// A point on the grid.
struct Point {
  x: Int
  y: Int = 0
}

/// Shapes.
enum Shape {
  Circle(r: Int)
  Square(side: Int)
  Empty
}

interface Area {
  fn area(self) -> Int
}

type Meters = Int

const ORIGIN_X = 0

/// Sum of the coordinates.
fn (p: Point) sum() -> Int {
  p.x + p.y
}

fn (s: Shape) area() -> Int {
  match s {
    Circle(r: r) => r * r * 3
    Square(side: n) => n * n
    Empty => 0
  }
}

fn make[T](x: Int, extra: T) -> Point {
  let _ = extra
  Point{x: x, y: ORIGIN_X}
}

fn main(os: Os) {
  let p = make(1, "a")
  var n: Meters = 0
  for i in 0..3 {
    n += i
  }
  let f = fn(k: Int) -> Int { k + n }
  let sh = Shape.Circle(r: f(2))
  if let Circle(r: rr) = sh {
    log.info("r ${rr}")
  }
  let q = Point{x: p.sum(), y: sh.area()}
  os.stdio.println(str.to_upper("total ${q.x} ${q.y}"))
}
`

// lspWordStarts are the offsets where an identifier starts (a letter or
// underscore not after another word character), the points the
// comparison asks about.
func lspWordStarts(src string) []int {
	var out []int
	word := func(c byte) bool {
		return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
	}
	for i := 0; i < len(src); i++ {
		c := src[i]
		if (c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z') && (i == 0 || !word(src[i-1])) {
			out = append(out, i)
		}
	}
	return out
}

func lspPosition(src string, off int) string {
	line := strings.Count(src[:off], "\n")
	col := off - (strings.LastIndex(src[:off], "\n") + 1)
	return `{"line":` + strconv.Itoa(line) + `,"character":` + strconv.Itoa(col) + `}`
}

// TestXoLSPFeatures sends the requests that read the check (hover,
// definition, references, prepareRename, rename, formatting, semantic
// tokens) to the Go server in process and to the Xo one, at every
// identifier of a feature program, of the notes service example, of
// selfhost/lexer, and of selfhost/cli/core/format, and compares the
// responses byte for byte.
func TestXoLSPFeatures(t *testing.T) {
	bin := xoCLI(t)
	dir := writeFiles(t, map[string]string{
		"app/xo.mod":  "module example.com/app\nedition preview\n",
		"app/main.xo": lspFeatureProg,
	})
	corpus := [][]string{{filepath.Join(dir, "app", "main.xo")}}
	for _, d := range []string{filepath.Join("..", "..", "examples", "notes-service"), filepath.Join("..", "..", "selfhost", "lexer"), filepath.Join("..", "..", "selfhost", "cli", "core", "format")} {
		files, _ := filepath.Glob(filepath.Join(d, "*.xo"))
		var abs []string
		for _, f := range files {
			a, _ := filepath.Abs(f)
			abs = append(abs, a)
		}
		corpus = append(corpus, abs)
	}
	same, total := 0, 0
	byMethod := map[string][2]int{}
	var diffs []string
	for _, files := range corpus {
		gInR, gInW := io.Pipe()
		gOutR, gOutW := io.Pipe()
		gDone := make(chan int, 1)
		go func() { gDone <- runLSP(nil, gInR, gOutW, io.Discard); gOutW.Close() }()
		g := newLSPPeer(gInW, gOutR)
		cmd := exec.Command(bin, "lsp")
		cmd.Env = xoCLIEnv()
		xin, _ := cmd.StdinPipe()
		xout, _ := cmd.StdoutPipe()
		var xerr bytes.Buffer
		cmd.Stderr = &xerr
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		x := newLSPPeer(xin, xout)
		id := 0
		ask := func(method, params string) {
			id++
			body := `{"jsonrpc":"2.0","id":` + strconv.Itoa(id) + `,"method":"` + method + `","params":` + params + `}`
			// one server at a time: both copy std files into the same
			// lsp-std directory, and a copy being rewritten by one is
			// missing for the other
			want := `"id":` + strconv.Itoa(id) + `,`
			g.send(body)
			gm := g.until(t, want)
			x.send(body)
			xm := x.until(t, want)
			total++
			n := byMethod[method]
			n[1]++
			// the last message is the response; diagnostics may precede it
			var ga, xa string
			if len(gm) > 0 {
				ga = gm[len(gm)-1]
			}
			if len(xm) > 0 {
				xa = xm[len(xm)-1]
			}
			if ga == xa {
				same++
				n[0]++
			} else if len(diffs) < 12 {
				diffs = append(diffs, method+" "+params+"\n  go: "+ga+"\n  xo: "+xa)
			}
			byMethod[method] = n
		}
		ask("initialize", `{"capabilities":{}}`)
		g.send(`{"jsonrpc":"2.0","method":"initialized","params":{}}`)
		x.send(`{"jsonrpc":"2.0","method":"initialized","params":{}}`)
		srcs := map[string]string{}
		for _, p := range files {
			data, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			srcs[p] = string(data)
			open := `{"jsonrpc":"2.0","method":"textDocument/didOpen","params":{"textDocument":{"uri":` + strconv.Quote("file://"+p) + `,"languageId":"xo","version":1,"text":` + strconv.Quote(string(data)) + `}}}`
			g.send(open)
			x.send(open)
		}
		for _, p := range files {
			src := srcs[p]
			doc := `{"uri":` + strconv.Quote("file://"+p) + `}`
			ask("textDocument/semanticTokens/full", `{"textDocument":`+doc+`}`)
			ask("textDocument/formatting", `{"textDocument":`+doc+`,"options":{}}`)
			starts := lspWordStarts(src)
			for i, off := range starts {
				at := `{"textDocument":` + doc + `,"position":` + lspPosition(src, off) + `}`
				ask("textDocument/hover", at)
				ask("textDocument/definition", at)
				if i%5 == 0 {
					ask("textDocument/references", `{"textDocument":`+doc+`,"position":`+lspPosition(src, off)+`,"context":{"includeDeclaration":`+strconv.FormatBool(i%10 == 0)+`}}`)
					ask("textDocument/prepareRename", at)
				}
				if i%25 == 0 {
					ask("textDocument/rename", `{"textDocument":`+doc+`,"position":`+lspPosition(src, off)+`,"newName":"renamed_`+strconv.Itoa(i)+`"}`)
				}
			}
			if len(starts) > 0 {
				ask("textDocument/rename", `{"textDocument":`+doc+`,"position":`+lspPosition(src, starts[len(starts)-1])+`,"newName":"1bad"}`)
			}
		}
		g.send(`{"jsonrpc":"2.0","method":"exit"}`)
		x.send(`{"jsonrpc":"2.0","method":"exit"}`)
		<-gDone
		if err := cmd.Wait(); err != nil {
			t.Errorf("xo lsp: %v %s", err, xerr.String())
		}
	}
	for _, d := range diffs {
		t.Error(d)
	}
	for m, n := range byMethod {
		t.Logf("%s: %d of %d identical", m, n[0], n[1])
	}
	t.Logf("lsp features: %d of %d responses identical", same, total)
}
