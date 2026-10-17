package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/xotool"
)

// stdProbe writes, in a new directory, a module of the toolchain's own
// namespace (so std/internal is visible, decision 0214) whose main.xo
// imports every std module, and returns the directory.
func stdProbe(t *testing.T, std string) string {
	t.Helper()
	names, err := filepath.Glob(filepath.Join(std, "*.xo"))
	if err != nil {
		t.Fatal(err)
	}
	var src strings.Builder
	for _, n := range names {
		m := strings.TrimSuffix(filepath.Base(n), ".xo")
		if m == "prelude" {
			continue
		}
		fmt.Fprintf(&src, "use std/%s\n", m)
	}
	src.WriteString("\nfn main(os: Os) {}\n")
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "xo.mod"), []byte("module github.com/xo-lang/xo/stdprobe\nedition preview\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(src.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

type stdDiag struct {
	Code string `json:"code"`
	Span struct {
		File string `json:"file"`
		Line int    `json:"line"`
	} `json:"span"`
	Message string `json:"message"`
}

// nativeCheck runs the eager native check, `xo check --no-gc --json`, in
// dir and returns its diagnostics.
func nativeCheck(t *testing.T, dir string) []stdDiag {
	t.Helper()
	chdir(t, dir)
	var out, errOut bytes.Buffer
	run(t, []string{"check", "--no-gc", "--json", "."}, &out, &errOut)
	var r struct {
		Diagnostics []stdDiag `json:"diagnostics"`
	}
	if err := json.Unmarshal(out.Bytes(), &r); err != nil {
		t.Fatalf("xo check --no-gc --json: %v\n%s%s", err, out.String(), errOut.String())
	}
	return r.Diagnostics
}

// TestStdChecks checks every function body of the native runtime (the
// implementation modules std/native/*.xo and the runtime modules
// std/native/llrt/*.xo) with no diagnostic, in an eager native check of a
// program that imports every std module. A native build checks only the
// bodies it builds (decision 0247), so this is where an error in an
// unreached body is reported. The second part proves the program reaches
// every runtime file with a body: in a copy of std with a function that
// does not check appended to each, each one is reported.
func TestStdChecks(t *testing.T) {
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	std := filepath.Join(root, "std")
	dir := stdProbe(t, std)
	for _, d := range nativeCheck(t, dir) {
		if d.Code == "XO0110" && d.Span.File == "main.xo" {
			continue // the probe's imports, unused by design
		}
		t.Errorf("%s:%d: %s %s", d.Span.File, d.Span.Line, d.Code, d.Message)
	}
	if t.Failed() {
		return
	}

	copyStd := filepath.Join(t.TempDir(), "std")
	var want []string
	err = filepath.WalkDir(std, func(p string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(std, p)
		if e.IsDir() {
			if e.Name() == "testdata" || rel == "rt" {
				return filepath.SkipDir
			}
			return os.MkdirAll(filepath.Join(copyStd, rel), 0o755)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		native := strings.HasPrefix(filepath.ToSlash(rel), "native/") && strings.HasSuffix(rel, ".xo") && !strings.HasSuffix(rel, "_test.xo")
		if native && bytes.Contains(b, []byte("{\n")) && !declOnly(rel) {
			want = append(want, filepath.ToSlash(rel))
			b = append(b, []byte("\nfn zz_std_probe() -> Int { \"not an Int\" }\n")...)
		}
		return os.WriteFile(filepath.Join(copyStd, rel), b, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	setenv(t, "XO_STD", copyStd)
	got := map[string]bool{}
	for _, d := range nativeCheck(t, dir) {
		if d.Code != "XO0110" {
			f := filepath.ToSlash(d.Span.File)
			if i := strings.Index(f, "native/"); i >= 0 {
				got[f[i:]] = true
			}
		}
	}
	var missed []string
	for _, w := range want {
		if !got[w] {
			missed = append(missed, w)
		}
	}
	sort.Strings(missed)
	t.Logf("%d runtime files with bodies, %d reported", len(want), len(want)-len(missed))
	if len(missed) > 0 {
		t.Errorf("the probe program does not reach %d runtime file(s), so their bodies are never checked in full: %v", len(missed), missed)
	}
}

// declOnly reports the native files of declarations without bodies (the
// intrinsics std/native/rt*.xo and the platform symbols sys_*.xo), which
// the checker reads as declarations.
func declOnly(rel string) bool {
	base := filepath.Base(rel)
	return filepath.Dir(filepath.ToSlash(rel)) == "native" && (strings.HasPrefix(base, "rt") || strings.HasPrefix(base, "sys_"))
}
