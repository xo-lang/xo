package fix

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSelect(t *testing.T) {
	names := func(rs []Rewriter) string {
		var out []string
		for _, r := range rs {
			out = append(out, r.Name)
		}
		return strings.Join(out, ",")
	}
	cases := map[string]string{
		"draft-0.1": "variant-case,scope-timeout,interp-dollar,discard-reason,map-get-or-fault",
		"0.2":       "interp-dollar,discard-reason,map-get-or-fault",
		"v0.3":      "discard-reason,map-get-or-fault",
		"draft-0.6": "",
		"":          "",
	}
	for from, want := range cases {
		rs, err := Select(from)
		if err != nil {
			t.Fatalf("%q: %v", from, err)
		}
		if got := names(rs); got != want {
			t.Errorf("Select(%q) = %s, want %s", from, got, want)
		}
	}
	for _, bad := range []string{"draft-1.0", "latest", "0.x"} {
		if _, err := Select(bad); err == nil {
			t.Errorf("Select(%q) accepted", bad)
		}
	}
}

func TestRewriteVariantCase(t *testing.T) {
	src := `fn f(r: Int ! Str, o: Int?) -> Int ! Str {
  let err = 1
  let x = match r {
    ok(v) => v
    err(e) => return err(e)
  }
  let y = match o {
    some(v) => v
    none => 0
  }
  let z: Int? = none
  let q = r.ok()
  ok(x + y + err)
}
`
	want := `fn f(r: Int ! Str, o: Int?) -> Int ! Str {
  let err = 1
  let x = match r {
    Ok(v) => v
    Err(e) => return Err(e)
  }
  let y = match o {
    Some(v) => v
    None => 0
  }
  let z: Int? = None
  let q = r.ok()
  Ok(x + y + err)
}
`
	got, chs := rewriteVariantCase("a.xo", []byte(src))
	if string(got) != want {
		t.Errorf("got:\n%s", got)
	}
	if len(chs) != 7 {
		t.Errorf("%d changes, want 7", len(chs))
	}
}

func TestRewriteInterp(t *testing.T) {
	cases := []struct{ in, want string }{
		{`let s = "hi {name}, {a.b + 1} ok"` + "\n", `let s = "hi ${name}, ${a.b + 1} ok"` + "\n"},
		{`let s = "already ${x}"` + "\n", `let s = "already ${x}"` + "\n"},
		{`let s = "{\"json\": 1}"` + "\n", `let s = "{\"json\": 1}"` + "\n"},
		{`let s = "{ spaced }"` + "\n", `let s = "{ spaced }"` + "\n"},
		{`let s = r"{raw}"` + "\n", `let s = r"{raw}"` + "\n"},
		{`let s = "{f(x)} and {(a)}"` + "\n", `let s = "${f(x)} and ${(a)}"` + "\n"},
	}
	for _, tc := range cases {
		got, _ := rewriteInterp("a.xo", []byte(tc.in))
		if string(got) != tc.want {
			t.Errorf("%s: got %s", tc.in, got)
		}
	}
}

func TestRewriteScopeTimeout(t *testing.T) {
	src := `use std/http

fn f(clock: Clock) -> Int ! Error {
  let v = scope s timeout 5s {
    s.spawn(fn() { 1 })
    2
  }
  v
}
`
	got, chs := rewriteScopeTimeout("a.xo", []byte(src))
	for _, want := range []string{"use std/task\n", "task.with_timeout(clock, 5s, fn() { scope s {", "    2\n  } })\n"} {
		if !strings.Contains(string(got), want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if len(chs) != 1 {
		t.Errorf("changes = %v", chs)
	}
	// Without a clock the construct is left alone and reported.
	noClock := strings.ReplaceAll(src, "clock: Clock", "n: Int")
	got, chs = rewriteScopeTimeout("a.xo", []byte(noClock))
	if string(got) != noClock || len(chs) != 1 || !strings.HasPrefix(chs[0].Title, "skipped") {
		t.Errorf("no clock: changes %v, got:\n%s", chs, got)
	}
}

func writeModule(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for n, s := range files {
		p := filepath.Join(dir, n)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// TestFixLoop runs the whole loop on a draft 0.1 style module: the
// rewriters fix the syntax, then two rounds of check fixes (XO0206 discard,
// XO0410 Map read, XO0110 unused) leave it clean.
func TestFixLoop(t *testing.T) {
	src := `fn load(n: Int) -> Int ! Str {
  if n > 0 { return ok(n) }
  err("neg {n}")
}

fn main() {
  let _ = load(1)
  let m: Map[Str, Int] = {"a": 1}
  let unused = m["a"]
}
`
	dir := writeModule(t, map[string]string{"m/main.xo": src})
	mod := filepath.Join(dir, "m")
	rws, _ := Select("draft-0.1")

	dry, err := Run([]string{mod}, Options{Rewriters: rws, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(dry.Files) != 1 || len(dry.Remaining) != 0 {
		t.Fatalf("dry run: files %v, remaining %v, changes %v", dry.Files, dry.Remaining, dry.Changes)
	}
	if b, _ := os.ReadFile(filepath.Join(mod, "main.xo")); string(b) != src {
		t.Fatal("dry run wrote the file")
	}

	res, err := Run([]string{mod}, Options{Rewriters: rws})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Remaining) != 0 {
		t.Fatalf("remaining: %v", res.Remaining)
	}
	b, _ := os.ReadFile(filepath.Join(mod, "main.xo"))
	got := string(b)
	for _, want := range []string{"return Ok(n)", `Err("neg ${n}")`, `.ignore_err("TODO: reason")`, `.get_or_fault("a", "TODO: why the key must exist")`, "let _unused"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	rules := map[string]bool{}
	for _, c := range res.Changes {
		rules[c.Rule] = true
	}
	for _, r := range []string{"variant-case", "interp-dollar", "discard-reason (XO0206)", "map-get-or-fault (XO0410)", "XO0110"} {
		if !rules[r] {
			t.Errorf("no change credited to %s: %v", r, res.Changes)
		}
	}

	// A second run finds nothing to do.
	again, err := Run([]string{mod}, Options{Rewriters: rws})
	if err != nil {
		t.Fatal(err)
	}
	if len(again.Changes) != 0 || again.Rounds != 1 {
		t.Errorf("second run: %d changes, %d rounds", len(again.Changes), again.Rounds)
	}
}

func TestOverlaps(t *testing.T) {
	cases := []struct {
		a1, a2, b1, b2 int
		want           bool
	}{
		{0, 2, 2, 4, false},
		{0, 3, 2, 4, true},
		{2, 2, 2, 2, true},
		{2, 2, 0, 2, false},
		{1, 1, 0, 2, true},
	}
	for _, c := range cases {
		if got := overlaps(c.a1, c.a2, c.b1, c.b2); got != c.want {
			t.Errorf("overlaps(%d,%d,%d,%d) = %v", c.a1, c.a2, c.b1, c.b2, got)
		}
	}
}
