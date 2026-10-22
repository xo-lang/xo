package main

import (
	"bytes"
	"encoding/json"
	"testing"
)

func vetCodes(t *testing.T, src string) map[string][]int {
	t.Helper()
	dir := writeFiles(t, map[string]string{"main.xo": src})
	var out, errOut bytes.Buffer
	if code := run(t, []string{"vet", "--json", dir}, &out, &errOut); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	var res struct {
		Diagnostics []struct {
			Code string `json:"code"`
			Span struct {
				Line int `json:"line"`
			} `json:"span"`
		} `json:"diagnostics"`
		CheckErrors int `json:"check_errors"`
	}
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("bad JSON: %v\n%s", err, out.String())
	}
	if res.CheckErrors != 0 {
		t.Fatalf("check errors: %d\n%s", res.CheckErrors, out.String())
	}
	got := map[string][]int{}
	for _, d := range res.Diagnostics {
		got[d.Code] = append(got[d.Code], d.Span.Line)
	}
	return got
}

// TestVetStaleCapture covers XO0538: a closure that reads a var the
// enclosing function writes before the closure is used again.
func TestVetStaleCapture(t *testing.T) {
	t.Parallel()
	src := `fn bad(n: Int) -> Int {
  var up: List[Int] = [0, 1, 2]
  let root = fn(k: Int) -> Int { up[k] }
  up[0] = 2
  root(n) + 0
}

fn writes_before(n: Int) -> Int {
  var up: List[Int] = [0, 1, 2]
  up[0] = 2
  let root = fn(k: Int) -> Int { up[k] }
  root(n)
}

fn no_use_after(n: Int) -> Int {
  var up: List[Int] = [0, 1, 2]
  let root = fn(k: Int) -> Int { up[k] }
  let a = root(n)
  up[0] = 2
  a
}

fn param(n: Int) -> Int {
  var up: List[Int] = [0, 1, 2]
  let root = fn(k: Int, u: List[Int]) -> Int { u[k] }
  up[0] = 2
  root(n, up)
}

fn main(os: Os) {
  os.stdio.println("${bad(1)} ${writes_before(1)} ${no_use_after(1)} ${param(1)}")
}
`
	got := vetCodes(t, src)
	if len(got["XO0538"]) != 1 || got["XO0538"][0] != 4 {
		t.Errorf("XO0538 at %v, want [4]: %v", got["XO0538"], got)
	}
}
