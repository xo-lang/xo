package main

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/xotool"
)

// TestCheckStdRecord covers the std declaration record under `xo check`
// (decision 0388 stage 1a-ii, used by xo check since 2026-11-08): the
// first check stores it, a second check over an unchanged std loads it
// and skips std, and an edit of a std file fails its header, so the next
// check reads std again and sees the edit.
func TestCheckStdRecord(t *testing.T) {
	t.Parallel()
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(root, "std")
	mod := probeModule(t, src)
	std := filepath.Join(t.TempDir(), "std")
	err = filepath.WalkDir(src, func(p string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		if e.IsDir() {
			if e.Name() == "testdata" || rel == "rt" || e.Name() == ".xo" {
				return filepath.SkipDir
			}
			return os.MkdirAll(filepath.Join(std, rel), 0o755)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(std, rel), b, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	dir := writeFiles(t, map[string]string{
		"xo.mod":  "module example.com/app\nedition preview\n",
		"main.xo": "use std/" + mod + "\n\nfn main(os: Os) {\n  os.stdio.println(\"${" + mod + ".zz_probe()}\")\n}\n",
	})
	setenv(t, "XO_STD", std)
	setenv(t, "XO_BACKEND_CACHE_DIR", t.TempDir())
	setenv(t, "XO_BACKEND_CACHE", "on")
	setenv(t, "XO_FE_CACHE", "")
	setenv(t, "XO_CHK_CACHE", "")
	setenv(t, "XO_STD_EXPORT", "")
	setenv(t, "XO_CACHE_STATS", "1")
	check := func() (int, string) {
		var out, errOut bytes.Buffer
		code := run(t, []string{"check", dir}, &out, &errOut)
		return code, out.String() + errOut.String()
	}
	code, got := check()
	if code == 0 || !strings.Contains(got, "zz_probe") || !strings.Contains(got, "cache stdx no entry") {
		t.Fatalf("first check (%s has no zz_probe, no record yet): exit %d\n%s", mod, code, got)
	}
	code, got = check()
	if code == 0 || !strings.Contains(got, "cache stdx record loaded") {
		t.Fatalf("second check over an unchanged std did not load the record: exit %d\n%s", code, got)
	}
	appendTo := func(rel, text string) {
		f, err := os.OpenFile(filepath.Join(std, rel), os.O_APPEND|os.O_WRONLY, 0)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if _, err := f.WriteString(text); err != nil {
			t.Fatal(err)
		}
	}
	appendTo(mod+".xo", "/// A test probe.\nfn zz_probe() -> Int\n")
	appendTo(filepath.Join("native", mod+".xo"), "\nfn zz_probe() -> Int { 1 }\n")
	code, got = check()
	if code != 0 || !strings.Contains(got, "cache stdx header failed") {
		t.Fatalf("check after a std edit (zz_probe added): exit %d\n%s", code, got)
	}
	code, got = check()
	if code != 0 || !strings.Contains(got, "cache stdx record loaded") {
		t.Fatalf("check after the edit's record was stored: exit %d\n%s", code, got)
	}
}

// probeModule picks, from the tree at run time, the first std module (by
// name) declared in std/M.xo and implemented in std/native/M.xo and not
// merged (std/merged.txt, decision 0588): a bodyless probe in its
// declaration file and a body in its native file. Picked rather than
// named, since modules move between those shapes (adler32 became a merged
// checksum with std/native/adler32_native.xo in 8659c2c2a and broke this
// test).
func probeModule(t *testing.T, src string) string {
	t.Helper()
	merged := map[string]bool{}
	if b, err := os.ReadFile(filepath.Join(src, "merged.txt")); err == nil {
		for _, l := range strings.Split(string(b), "\n") {
			if m, ok := strings.CutPrefix(strings.TrimSpace(l), "std/"); ok {
				merged[m] = true
			}
		}
	}
	es, err := os.ReadDir(src)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range es {
		m, ok := strings.CutSuffix(e.Name(), ".xo")
		if !ok || e.IsDir() || strings.HasSuffix(m, "_test") || m == "prelude" || merged[m] {
			continue
		}
		if _, err := os.Stat(filepath.Join(src, "native", m+".xo")); err == nil {
			return m
		}
	}
	t.Fatal("no std module with a declaration file and a native file")
	return ""
}
