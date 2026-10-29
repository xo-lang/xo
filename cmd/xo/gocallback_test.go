package main

import (
	"bytes"
	"strings"
	"testing"
)

// The h1 hashes of testdata/goproxy's synthetic example.com/cberr v1.0.0.
const (
	cberrSum    = "h1:yHcfcs9iCRy4n+v3AQ34cCgdlreboACNqR/IZp79xPw="
	cberrModSum = "h1:U/KSWeiepi293mlYuQcp5uHVQ6/e6iaZFiImjWoRBEI="
)

// TestUseGoCallbackErrors (decision 0416 step 1): a Go callback with an
// error result is an Xo function `-> T ! go.Error`; its Err is Go's
// error (go.error's text, wrapped by Go with %w), and a go.Error that came
// from Go goes back as that Go error, so errors.Is sees the sentinel.
func TestUseGoCallbackErrors(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("runs go list and go build")
	}
	useGoProxy(t)
	dir := writeFiles(t, map[string]string{
		"xo.mod": "module cb\n\nrequire go example.com/cberr v1.0.0\n",
		"xo.lock": "go example.com/cberr v1.0.0 " + cberrSum + "\n" +
			"go example.com/cberr v1.0.0/go.mod " + cberrModSum + "\n",
		"main.xo": `use go "example.com/cberr"
use std/go

fn main(os: Os) {
  let r = cberr.each(5, fn(i: Int) -> () ! go.Error {
    if i == 3 {
      return Err(go.error("boom"))
    }
  })
  match r {
    Ok(_) => os.stdio.println("each ok")
    Err(e) => os.stdio.println("each ${e.message()}")
  }
  match cberr.lengths(["ab", "", "c"], fn(s: Str) -> Int ! go.Error {
    if s.is_empty() {
      return Err(go.error("empty"))
    }
    s.len()
  }) {
    Ok(ns) => os.stdio.println("lengths ${ns.len()}")
    Err(e) => os.stdio.println("lengths ${e.message()}")
  }
  match cberr.lengths(["ab", "c"], fn(s: Str) -> Int ! go.Error { s.len() }) {
    Ok(ns) => os.stdio.println("lengths ${ns[0] + ns[1]}")
    Err(e) => os.stdio.println("lengths ${e.message()}")
  }
  let a = cberr.is_stop(fn() -> () ! go.Error { cberr.fail() })
  let b = cberr.is_stop(fn() -> () ! go.Error { Err(go.error("stop")) })
  os.stdio.println("${a} ${b}")
}
`,
	})
	var out, errOut bytes.Buffer
	if code := run(t, []string{"run", "--backend=go", dir}, &out, &errOut); code != 0 {
		t.Fatalf("run: exit %d\n%s%s", code, out.String(), errOut.String())
	}
	want := "each at 3: boom\nlengths \"\": empty\nlengths 3\ntrue false"
	if got := strings.TrimSpace(out.String()); got != want {
		t.Errorf("output\n%s\nwant\n%s", got, want)
	}
}
