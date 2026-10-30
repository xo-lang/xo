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

// TestUseGoErrorValues (decision 0416): a go.Error goes to Go as the Go
// error it came from, so errors.is matches io's sentinel; go.error's
// error is not it; errors.join of two is one error with both texts.
func TestUseGoErrorValues(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("runs go list and go build")
	}
	setenv(t, "XO_GOBIND_DIR", "")
	dir := writeFiles(t, map[string]string{
		"xo.mod": "module errs\n",
		"main.xo": `use go "errors"
use go "io"
use go "strings"
use std/go

fn main(os: Os) {
  let r = strings.new_reader("")
  match io.read_all(r.as_reader()) {
    Ok(b) => os.stdio.println("read ${b.len()}")
    Err(e) => os.stdio.println("error ${e.message()}")
  }
  let eof = io.eof()
  os.stdio.println("${errors.is(Some(eof), Some(io.eof()))} ${errors.is(Some(go.error("EOF")), Some(io.eof()))}")
  match errors.join([Some(go.error("a")), None, Some(go.error("b"))]) {
    Ok(_) => os.stdio.println("none")
    Err(e) => os.stdio.println(e.message().replace("\n", "+"))
  }
}
`,
	})
	var out, errOut bytes.Buffer
	if code := run(t, []string{"run", "--backend=go", dir}, &out, &errOut); code != 0 {
		t.Fatalf("run: exit %d\n%s%s", code, out.String(), errOut.String())
	}
	want := "read 0\ntrue false\na+b"
	if got := strings.TrimSpace(out.String()); got != want {
		t.Errorf("output\n%s\nwant\n%s", got, want)
	}
}

// The h1 hashes of testdata/goproxy's synthetic example.com/hook v1.0.0.
const (
	hookSum    = "h1:Um3We+7CJxDwCR/pQi1uYYiBrZ46yr5mJuDsJcZkg9A="
	hookModSum = "h1:tBa4IcTtX8onrrDypPuSEcerWYEyzoMgygOPbx3zX5c="
)

// TestUseGoFuncSetters: a Go field or package variable of function type
// is set from an Xo function, which Go then calls (a field's error result
// is the callback's `! go.Error`).
func TestUseGoFuncSetters(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("runs go list and go build")
	}
	useGoProxy(t)
	dir := writeFiles(t, map[string]string{
		"xo.mod": "module hk\n\nrequire go example.com/hook v1.0.0\n",
		"xo.lock": "go example.com/hook v1.0.0 " + hookSum + "\n" +
			"go example.com/hook v1.0.0/go.mod " + hookModSum + "\n",
		"main.xo": `use go "example.com/hook"
use std/go

fn main(os: Os) {
  let s = hook.Server.new()
  s.set_name("db")
  os.stdio.println(s.run())
  s.set_check(fn(name: Str) -> () ! go.Error {
    if name == "db" {
      return Err(go.error("${name} down"))
    }
  })
  s.set_on_error(fn(msg: Str) { os.stdio.println("on_error ${msg}") })
  os.stdio.println(s.run())
  s.set_name("cache")
  os.stdio.println(s.run())
  os.stdio.println("${hook.log("x")}")
  hook.set_logf(fn(f: Str) -> Int { f.len() * 10 })
  os.stdio.println("${hook.log("abc")}")
}
`,
	})
	var out, errOut bytes.Buffer
	if code := run(t, []string{"run", "--backend=go", dir}, &out, &errOut); code != 0 {
		t.Fatalf("run: exit %d\n%s%s", code, out.String(), errOut.String())
	}
	want := "no check\non_error db down\nfailed: db down\nok\n-1\n30"
	if got := strings.TrimSpace(out.String()); got != want {
		t.Errorf("output\n%s\nwant\n%s", got, want)
	}
}

// TestUseGoNamedBasicMethods (decision 0416): a value receiver method of
// a named basic type is a package function taking the value first.
func TestUseGoNamedBasicMethods(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("runs go list and go build")
	}
	setenv(t, "XO_GOBIND_DIR", "")
	dir := writeFiles(t, map[string]string{
		"xo.mod": "module nb\n",
		"main.xo": `use go "time"

fn main(os: Os) {
  os.stdio.println(time.month_string(3))
  os.stdio.println(time.weekday_string(0))
}
`,
	})
	var out, errOut bytes.Buffer
	if code := run(t, []string{"run", "--backend=go", dir}, &out, &errOut); code != 0 {
		t.Fatalf("run: exit %d\n%s%s", code, out.String(), errOut.String())
	}
	if got, want := strings.TrimSpace(out.String()), "March\nSunday"; got != want {
		t.Errorf("output\n%s\nwant\n%s", got, want)
	}
}

// The h1 hashes of testdata/goproxy's synthetic example.com/fnval v1.0.0.
const (
	fnvalSum    = "h1:pTdTGDMvFFf6JILcAIFspPDJxXuxfsi4zLjcYJdW6/A="
	fnvalModSum = "h1:JpeO3fNjkKTIgJTYs3UzUyXLdaoaxOXBXD1uoToZOp4="
)

// TestUseGoFuncValues: a Go function value given to Xo is an Xo function
// that calls it: a result (an error result is `! go.Error`), and a
// function field read back after Xo set it, which calls the Xo function
// through Go.
func TestUseGoFuncValues(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("runs go list and go build")
	}
	useGoProxy(t)
	dir := writeFiles(t, map[string]string{
		"xo.mod": "module fv\n\nrequire go example.com/fnval v1.0.0\n",
		"xo.lock": "go example.com/fnval v1.0.0 " + fnvalSum + "\n" +
			"go example.com/fnval v1.0.0/go.mod " + fnvalModSum + "\n",
		"main.xo": `use go "example.com/fnval"

fn main(os: Os) {
  let add = fnval.adder(2)
  os.stdio.println("${add(3)}")
  let parse = fnval.parser()
  for s in ["42", "x"] {
    match parse(s) {
      Ok(n) => os.stdio.println("parsed ${n}")
      Err(e) => os.stdio.println("error ${e.message()}")
    }
  }
  let h = fnval.Hooks.new()
  h.set_on_event(fn(name: Str) -> Str { "xo saw ${name}" })
  os.stdio.println(h.fire("a"))
  let f = h.on_event()
  os.stdio.println(f("b"))
}
`,
	})
	var out, errOut bytes.Buffer
	if code := run(t, []string{"run", "--backend=go", dir}, &out, &errOut); code != 0 {
		t.Fatalf("run: exit %d\n%s%s", code, out.String(), errOut.String())
	}
	want := "5\nparsed 42\nerror not a number: x\nxo saw a\nxo saw b"
	if got := strings.TrimSpace(out.String()); got != want {
		t.Errorf("output\n%s\nwant\n%s", got, want)
	}
}

// The h1 hashes of testdata/goproxy's synthetic example.com/fnopt v1.0.0.
const (
	fnoptSum    = "h1:tj/86s4Ib7OvcAqOdroFKxtBPzplJK8IDFblK0QA2jg="
	fnoptModSum = "h1:W/aLve4CnifMYE1pTDhsXEu9oQwx6flf0JgtvfbihNg="
)

// TestUseGoFuncOptions (decision 0422): a variadic of a function type,
// Go's functional options (optFns ...func(*Options), as AWS SDK v2's
// clients take), is a List of Xo functions that Go calls in order, also
// with an error result; a variadic of a named function type (Count's
// ...Option) stays a List of handles (Go's option helpers return them).
func TestUseGoFuncOptions(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("runs go list and go build")
	}
	useGoProxy(t)
	dir := writeFiles(t, map[string]string{
		"xo.mod": "module fo\n\nrequire go example.com/fnopt v1.0.0\n",
		"xo.lock": "go example.com/fnopt v1.0.0 " + fnoptSum + "\n" +
			"go example.com/fnopt v1.0.0/go.mod " + fnoptModSum + "\n",
		"main.xo": `use go "example.com/fnopt"
use std/go

fn main(os: Os) {
  os.stdio.println(fnopt.describe("db", []))
  os.stdio.println(fnopt.describe("db", [fn(o: fnopt.Options) { o.set_retries(3) }, fn(o: fnopt.Options) { o.set_name(o.name() + "-eu") }]))
  os.stdio.println("${fnopt.count([])}")
  match fnopt.check([fn(o: fnopt.Options) -> () ! go.Error { Ok(()) }, fn(o: fnopt.Options) -> () ! go.Error { Err(go.error("bad ${o.retries()}")) }]) {
    Ok(_) => os.stdio.println("ok")
    Err(e) => os.stdio.println(e.message())
  }
}
`,
	})
	var out, errOut bytes.Buffer
	if code := run(t, []string{"run", "--backend=go", dir}, &out, &errOut); code != 0 {
		t.Fatalf("run: exit %d\n%s%s", code, out.String(), errOut.String())
	}
	want := "db retries=1\ndb-eu retries=3\n0\noption: bad 0"
	if got := strings.TrimSpace(out.String()); got != want {
		t.Errorf("output\n%s\nwant\n%s", got, want)
	}
}
