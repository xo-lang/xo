package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestRecordReplayCLI drives `xo run --record` and `xo replay` (core
// 11.3) on a program that returns an error from main: the replay prints
// the same output and error offline, the crash bundle of the error replays
// too, and a changed source is reported.
func TestRecordReplayCLI(t *testing.T) {
	if testing.Short() {
		t.Skip("builds Go")
	}
	dir := t.TempDir()
	t.Setenv("XO_BUILD_DIR", filepath.Join(dir, "build"))
	crash := filepath.Join(dir, "crash")
	os.MkdirAll(crash, 0o755)
	t.Setenv("XO_CRASH_DIR", crash)
	src := filepath.Join(dir, "app")
	os.MkdirAll(src, 0o755)
	prog := `fn main(os: Os) ! {
  let n = os.rand.int(0, 1000000)
  os.stdio.println("args ${os.env.args().join(",")} rand ${n} now ${os.clock.now()}")
  let text = os.fs.read_text("missing-${n}.txt")?
  os.stdio.println(text)
}
`
	if err := os.WriteFile(filepath.Join(src, "main.xo"), []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	trace := filepath.Join(dir, "run.xotrace")
	var out, errOut bytes.Buffer
	if code := run([]string{"run", "--record=" + trace, src, "--", "a", "b"}, &out, &errOut); code != 1 {
		t.Fatalf("run --record: exit %d\n%s\n%s", code, out.String(), errOut.String())
	}
	recOut, recErr := out.String(), errOut.String()
	if !strings.HasPrefix(recOut, "args a,b rand ") || !strings.Contains(recErr, "error: NotFound(path: \"missing-") || !strings.Contains(recErr, "xo run: recorded "+trace) {
		t.Fatalf("recording:\n%s\n%s", recOut, recErr)
	}
	m := regexp.MustCompile(`(?m)^crash bundle: (.*)$`).FindStringSubmatch(recErr)
	if m == nil {
		t.Fatalf("no crash bundle for the error from main:\n%s", recErr)
	}
	var b struct {
		Kind   string   `json:"kind"`
		Error  string   `json:"error"`
		Frames []string `json:"frames"`
		Trace  []any    `json:"trace"`
	}
	data, _ := os.ReadFile(m[1])
	if err := json.Unmarshal(data, &b); err != nil || b.Kind != "error" || !strings.HasPrefix(b.Error, "NotFound") || len(b.Frames) == 0 || len(b.Trace) == 0 {
		t.Fatalf("bundle %v:\n%s", err, data)
	}

	out.Reset()
	errOut.Reset()
	if code := run([]string{"replay", trace}, &out, &errOut); code != 1 || out.String() != recOut || !strings.Contains(errOut.String(), "xo replay: identical run: error: NotFound(path: \"missing-") {
		t.Fatalf("replay: exit %d\n%s\nrecorded:\n%s\nstderr:\n%s", code, out.String(), recOut, errOut.String())
	}
	out.Reset()
	errOut.Reset()
	if code := run([]string{"replay", m[1]}, &out, &errOut); code != 1 || out.String() != recOut || !strings.Contains(errOut.String(), "xo replay: reproduced the crash: error: NotFound") {
		t.Fatalf("bundle replay: exit %d\n%s\nstderr:\n%s", code, out.String(), errOut.String())
	}

	// a changed source: reported, and the effects still match
	changed := strings.Replace(prog, "args ${", "argv ${", 1)
	os.WriteFile(filepath.Join(src, "main.xo"), []byte(changed), 0o644)
	out.Reset()
	errOut.Reset()
	code := run([]string{"replay", trace}, &out, &errOut)
	if code != 1 || !strings.Contains(errOut.String(), "xo replay: warning: the sources changed since the recording") || !strings.Contains(errOut.String(), "main.xo (changed)") || !strings.HasPrefix(out.String(), "argv a,b") {
		t.Fatalf("replay of a changed program: exit %d\n%s\n%s", code, out.String(), errOut.String())
	}

	out.Reset()
	errOut.Reset()
	if code := run([]string{"replay", filepath.Join(src, "main.xo")}, &out, &errOut); code != 2 || !strings.Contains(errOut.String(), "is not an Xo trace") {
		t.Fatalf("replay of a non trace: exit %d %s", code, errOut.String())
	}
}

// TestRecordReplayStamp: `--stamp` values are part of the build
// information (core 10.1, decision 0165), so `xo replay` of a run built
// with a stamp rebuilds the stamped program, not the declared default.
func TestRecordReplayStamp(t *testing.T) {
	if testing.Short() {
		t.Skip("builds Go")
	}
	dir := t.TempDir()
	t.Setenv("XO_BUILD_DIR", filepath.Join(dir, "build"))
	src := filepath.Join(dir, "app")
	os.MkdirAll(src, 0o755)
	prog := `const VERSION: Str = "dev"

fn main(os: Os) {
  os.stdio.println("version ${VERSION} rand ${os.rand.int(0, 1000000)}")
}
`
	if err := os.WriteFile(filepath.Join(src, "main.xo"), []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	trace := filepath.Join(dir, "run.xotrace")
	var out, errOut bytes.Buffer
	if code := run([]string{"run", "--stamp", "VERSION=1.4.0", "--record=" + trace, src}, &out, &errOut); code != 0 || !strings.HasPrefix(out.String(), "version 1.4.0 rand ") {
		t.Fatalf("run --stamp --record: exit %d\n%s\n%s", code, out.String(), errOut.String())
	}
	recOut := out.String()
	out.Reset()
	errOut.Reset()
	if code := run([]string{"replay", trace}, &out, &errOut); code != 0 || out.String() != recOut {
		t.Fatalf("replay: exit %d\n%s\nrecorded:\n%s\nstderr:\n%s", code, out.String(), recOut, errOut.String())
	}
}
