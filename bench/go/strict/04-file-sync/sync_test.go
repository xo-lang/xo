package main

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeFile(t *testing.T, path, content string, mtime time.Time) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// snapshot records every entry under dir with its content and mtime.
func snapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	m := make(map[string]string)
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		if d.IsDir() {
			m[rel] = "dir"
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		m[rel] = string(b) + "|" + info.ModTime().String()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func testOptions(src, dst string, out io.Writer) Options {
	return Options{Src: src, Dst: dst, Out: out}
}

var base = time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)

func TestNewFileCopiedUnchangedSkipped(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src")
	dst := filepath.Join(root, "dst")
	writeFile(t, filepath.Join(src, "a.txt"), "alpha", base)
	writeFile(t, filepath.Join(src, "sub", "b.txt"), "beta", base)

	var out bytes.Buffer
	sum, err := Sync(testOptions(src, dst, &out))
	if err != nil {
		t.Fatal(err)
	}
	if sum != (Summary{Copied: 2}) {
		t.Fatalf("first sync summary = %+v, want 2 copied", sum)
	}
	if got := readFile(t, filepath.Join(dst, "a.txt")); got != "alpha" {
		t.Fatalf("a.txt = %q", got)
	}
	if got := readFile(t, filepath.Join(dst, "sub", "b.txt")); got != "beta" {
		t.Fatalf("sub/b.txt = %q", got)
	}
	info, err := os.Stat(filepath.Join(dst, "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !info.ModTime().Equal(base) {
		t.Fatalf("mtime not preserved: %v", info.ModTime())
	}

	// Second run: nothing changed.
	sum, err = Sync(testOptions(src, dst, &out))
	if err != nil {
		t.Fatal(err)
	}
	if sum != (Summary{Skipped: 2}) {
		t.Fatalf("second sync summary = %+v, want 2 skipped", sum)
	}

	// Same size but newer in src: copied.
	writeFile(t, filepath.Join(src, "a.txt"), "ALPHA", base.Add(time.Hour))
	// Different size, older in src: copied.
	writeFile(t, filepath.Join(src, "sub", "b.txt"), "be", base.Add(-time.Hour))
	sum, err = Sync(testOptions(src, dst, &out))
	if err != nil {
		t.Fatal(err)
	}
	if sum != (Summary{Copied: 2}) {
		t.Fatalf("third sync summary = %+v, want 2 copied", sum)
	}
	if got := readFile(t, filepath.Join(dst, "a.txt")); got != "ALPHA" {
		t.Fatalf("a.txt = %q", got)
	}
	if got := readFile(t, filepath.Join(dst, "sub", "b.txt")); got != "be" {
		t.Fatalf("sub/b.txt = %q", got)
	}

	// Same size, dst newer than src: skipped.
	writeFile(t, filepath.Join(dst, "a.txt"), "xxxxx", base.Add(2*time.Hour))
	sum, err = Sync(testOptions(src, dst, &out))
	if err != nil {
		t.Fatal(err)
	}
	if sum != (Summary{Skipped: 2}) {
		t.Fatalf("fourth sync summary = %+v, want 2 skipped", sum)
	}
}

func TestDryRunWritesNothing(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src")
	dst := filepath.Join(root, "dst")
	writeFile(t, filepath.Join(src, "new.txt"), "new", base)
	writeFile(t, filepath.Join(src, "dir", "changed.txt"), "changed", base.Add(time.Hour))
	writeFile(t, filepath.Join(dst, "dir", "changed.txt"), "old", base)
	writeFile(t, filepath.Join(dst, "extra.txt"), "extra", base)
	writeFile(t, filepath.Join(dst, "gone", "x.txt"), "x", base)

	before := snapshot(t, dst)
	var out bytes.Buffer
	opts := testOptions(src, dst, &out)
	opts.DryRun = true
	opts.Delete = true
	sum, err := Sync(opts)
	if err != nil {
		t.Fatal(err)
	}
	after := snapshot(t, dst)
	if len(before) != len(after) {
		t.Fatalf("dry run changed dst: before %v after %v", before, after)
	}
	for k, v := range before {
		if after[k] != v {
			t.Fatalf("dry run changed %s: %q -> %q", k, v, after[k])
		}
	}
	want := Summary{Copied: 2, Deleted: 3}
	if sum != want {
		t.Fatalf("summary = %+v, want %+v", sum, want)
	}
	for _, line := range []string{
		"would copy new.txt",
		"would copy dir/changed.txt",
		"would delete extra.txt",
		"would delete gone/x.txt",
		"would delete gone",
	} {
		if !strings.Contains(out.String(), line+"\n") {
			t.Errorf("output missing %q:\n%s", line, out.String())
		}
	}
}

func TestDryRunMissingDestination(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src")
	dst := filepath.Join(root, "dst")
	writeFile(t, filepath.Join(src, "sub", "a.txt"), "a", base)

	opts := testOptions(src, dst, io.Discard)
	opts.DryRun = true
	opts.Delete = true
	sum, err := Sync(opts)
	if err != nil {
		t.Fatal(err)
	}
	if sum != (Summary{Copied: 1}) {
		t.Fatalf("summary = %+v", sum)
	}
	if _, err := os.Stat(dst); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("dry run created destination: %v", err)
	}
}

func TestDeleteRemovesExtras(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src")
	dst := filepath.Join(root, "dst")
	writeFile(t, filepath.Join(src, "keep.txt"), "keep", base)
	writeFile(t, filepath.Join(src, "dir", "keep2.txt"), "keep2", base)
	writeFile(t, filepath.Join(dst, "extra.txt"), "extra", base)
	writeFile(t, filepath.Join(dst, "dir", "extra2.txt"), "extra2", base)
	writeFile(t, filepath.Join(dst, "gone", "nested", "x.txt"), "x", base)
	if err := os.MkdirAll(filepath.Join(dst, "emptygone"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Without --delete extras stay.
	sum, err := Sync(testOptions(src, dst, io.Discard))
	if err != nil {
		t.Fatal(err)
	}
	if sum != (Summary{Copied: 2}) {
		t.Fatalf("summary = %+v", sum)
	}
	if _, err := os.Stat(filepath.Join(dst, "extra.txt")); err != nil {
		t.Fatalf("extra removed without --delete: %v", err)
	}

	opts := testOptions(src, dst, io.Discard)
	opts.Delete = true
	sum, err = Sync(opts)
	if err != nil {
		t.Fatal(err)
	}
	// extra.txt, dir/extra2.txt, gone/nested/x.txt, gone/nested, gone, emptygone.
	if want := (Summary{Skipped: 2, Deleted: 6}); sum != want {
		t.Fatalf("summary = %+v, want %+v", sum, want)
	}
	got := snapshot(t, dst)
	want := map[string]bool{".": true, "keep.txt": true, "dir": true, filepath.Join("dir", "keep2.txt"): true}
	if len(got) != len(want) {
		t.Fatalf("dst entries = %v, want %v", got, want)
	}
	for k := range got {
		if !want[k] {
			t.Fatalf("unexpected entry %s left in dst", k)
		}
	}
}

func TestCopyFailureLeavesNoTemp(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src")
	dst := filepath.Join(root, "dst")
	writeFile(t, filepath.Join(src, "sub", "big.txt"), strings.Repeat("z", 4096), base.Add(time.Hour))
	writeFile(t, filepath.Join(src, "sub", "new.txt"), "new", base)
	writeFile(t, filepath.Join(dst, "sub", "big.txt"), "old", base)

	boom := errors.New("disk on fire")
	opts := testOptions(src, dst, io.Discard)
	opts.copyFn = func(w io.Writer, r io.Reader) (int64, error) {
		// Write part of the data, then fail.
		n, err := io.CopyN(w, r, 10)
		if err != nil {
			return n, err
		}
		return n, boom
	}
	sum, err := Sync(opts)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Failed != 2 || sum.Copied != 0 {
		t.Fatalf("summary = %+v, want 2 failed", sum)
	}
	entries, err := os.ReadDir(filepath.Join(dst, "sub"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "big.txt" {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("dst/sub has %v, want only big.txt", names)
	}
	if got := readFile(t, filepath.Join(dst, "sub", "big.txt")); got != "old" {
		t.Fatalf("existing file was modified: %q", got)
	}

	// A dry run into a missing destination succeeds through the command path.
	if code := run([]string{"--dry-run", src, filepath.Join(root, "nope")}, io.Discard, io.Discard); code != 0 {
		t.Fatalf("dry run exit = %d, want 0", code)
	}
}

func TestSkipNamesAndCaseInsensitiveDelete(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src")
	dst := filepath.Join(root, "dst")
	writeFile(t, filepath.Join(src, ".DS_Store"), "junk", base)
	writeFile(t, filepath.Join(src, "Dir", "File.txt"), "f", base)
	writeFile(t, filepath.Join(dst, "Dir", ".DS_Store"), "junk", base)
	writeFile(t, filepath.Join(dst, "gone", ".DS_Store"), "junk", base)

	opts := testOptions(src, dst, io.Discard)
	opts.SkipNames = []string{".DS_Store"}
	opts.CaseInsensitive = true
	opts.Delete = true
	sum, err := Sync(opts)
	if err != nil {
		t.Fatal(err)
	}
	// Copied File.txt, skipped src .DS_Store, deleted gone/.DS_Store and gone.
	if want := (Summary{Copied: 1, Skipped: 1, Deleted: 2}); sum != want {
		t.Fatalf("summary = %+v, want %+v", sum, want)
	}
	if _, err := os.Stat(filepath.Join(dst, ".DS_Store")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("skipped name was copied: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, "Dir", ".DS_Store")); err != nil {
		t.Fatalf("ignored name in kept dir was deleted: %v", err)
	}

	s := &syncer{opts: Options{CaseInsensitive: true}, srcKeys: map[string]struct{}{}}
	s.srcKeys[s.key("Dir/File.txt")] = struct{}{}
	if !s.inSrc("dir/file.TXT") {
		t.Fatal("case insensitive lookup failed")
	}
	s.opts.CaseInsensitive = false
	if s.inSrc("dir/file.TXT") {
		t.Fatal("case sensitive lookup matched different case")
	}
}

func TestOverlapRejected(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src")
	writeFile(t, filepath.Join(src, "a.txt"), "a", base)
	if _, err := Sync(testOptions(src, filepath.Join(src, "inner"), io.Discard)); err == nil {
		t.Fatal("expected overlap error for dst inside src")
	}
	if _, err := Sync(testOptions(src, root, io.Discard)); err == nil {
		t.Fatal("expected overlap error for src inside dst")
	}
}

func TestRunUsageAndSummary(t *testing.T) {
	if code := run([]string{"only-one"}, io.Discard, io.Discard); code != 2 {
		t.Fatalf("usage exit = %d, want 2", code)
	}
	root := t.TempDir()
	src := filepath.Join(root, "src")
	dst := filepath.Join(root, "dst")
	writeFile(t, filepath.Join(src, "a.txt"), "a", base)
	var out bytes.Buffer
	if code := run([]string{"--delete", src, dst}, &out, io.Discard); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "summary: copied=1 deleted=0 skipped=0 failed=0") {
		t.Fatalf("missing summary:\n%s", out.String())
	}
	if code := run([]string{src, filepath.Join(root, "missing-src-check"), "extra"}, io.Discard, io.Discard); code != 2 {
		t.Fatalf("extra args exit = %d, want 2", code)
	}
	if code := run([]string{filepath.Join(root, "nosuch"), dst}, io.Discard, io.Discard); code != 1 {
		t.Fatalf("missing src exit = %d, want 1", code)
	}
}
