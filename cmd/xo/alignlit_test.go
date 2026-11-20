package main

import (
	"bytes"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const alignLitProg = `fn long() -> Bytes {
  "the quick brown fox jumps over z".bytes()
}

fn main(os: Os) {
  let z = "z".bytes()
  let s = "the quick brown fox jumps over z"
  let b = s.bytes()
  let c = "the quick brown fox jumps over z".bytes()
  let d = "short z".bytes()
  os.stdio.println("${b.rfind(z) ?? -1} ${c.rfind(z) ?? -1} ${d.rfind(z) ?? -1}")
  var n = 0
  for _ in 0..3 {
    let x = long()
    let y = x.replace(z, "Z".bytes())
    n = n + (x.rfind(z) ?? -1) * 10 + (y.rfind(z) ?? -1)
  }
  os.stdio.println("${n}")
}
`

// TestGoBackendAlignedLiteralBytes (task #467): on the Go backend,
// `.bytes()` on a plain string literal of 32 bytes or more, or on a `let`
// bound to one, reads a package variable made by rt.AlignedBytes (data on
// a 32-byte boundary, one copy per distinct text), so bytealg's aligned
// vector loads run no partial head block. A shorter literal keeps the
// zero-copy rt.Str_bytes view. The copy is made once, at package init,
// not per call (long() runs three times), and a value derived from it
// (Bytes is immutable, XO0502, so replace makes a new one) leaves the
// shared copy as it was: every call still sees the original text.
func TestGoBackendAlignedLiteralBytes(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go build")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(alignLitProg), 0o644); err != nil {
		t.Fatal(err)
	}
	bd := filepath.Join(t.TempDir(), "build")
	setenv(t, "XO_BUILD_DIR", bd)
	app := filepath.Join(dir, "app")
	var out, errOut bytes.Buffer
	if code := run(t, []string{"build", "--backend=go", "-o", app, dir}, &out, &errOut); code != 0 {
		t.Fatalf("build: exit %d\n%s%s", code, out.String(), errOut.String())
	}
	var src string
	filepath.WalkDir(bd, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && d.Name() == "xo.go" {
			if b, _ := os.ReadFile(p); strings.Contains(string(b), "func F_main(") {
				src = string(b)
			}
		}
		return nil
	})
	if src == "" {
		t.Fatalf("no generated xo.go with F_main under %s", bd)
	}
	if n := strings.Count(src, "AlignedBytes("); n != 1 {
		t.Errorf("AlignedBytes( appears %d times, want 1 (one copy shared by every long use)", n)
	}
	if !regexp.MustCompile(`(?m)^var xo_alit\d+ = rt\.AlignedBytes\(`).MatchString(src) {
		t.Errorf("the AlignedBytes call is not a package level var (it would run per call)")
	}
	if i := strings.Index(src, "func F_long("); i < 0 || strings.Contains(src[i:i+strings.Index(src[i:], "\n}")], "AlignedBytes(") {
		t.Errorf("F_long missing or calls AlignedBytes in its body")
	}
	if n := strings.Count(src, "Str_bytes("); n != 3 {
		t.Errorf("Str_bytes( appears %d times, want 3 (the short literals)", n)
	}
	got, err := exec.Command(app).CombinedOutput()
	if err != nil {
		t.Fatalf("run: %v\n%s", err, got)
	}
	if strings.TrimSpace(string(got)) != "31 31 6\n927" {
		t.Errorf("output %q, want \"31 31 6\\n927\"", got)
	}
}
