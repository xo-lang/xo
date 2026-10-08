package main

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"
)

var (
	t0 = time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	t1 = t0.Add(time.Hour)
)

func writeFile(t *testing.T, root, rel, content string, mtime time.Time) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(p, mtime, mtime); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, root, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func exists(t *testing.T, root, rel string) bool {
	t.Helper()
	_, err := os.Lstat(filepath.Join(root, filepath.FromSlash(rel)))
	if err == nil {
		return true
	}
	if errors.Is(err, fs.ErrNotExist) {
		return false
	}
	t.Fatal(err)
	return false
}

// snapshot describes every entry under root: path, type, size, mtime, content.
func snapshot(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		info, err := d.Info()
		if err != nil {
			return err
		}
		line := rel + " " + info.Mode().String() + " " + info.ModTime().UTC().String()
		if d.Type().IsRegular() {
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			line += " " + string(b)
		}
		out = append(out, line)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(out)
	return out
}

func testOptions() Options {
	return Options{CaseInsensitive: false, SkipNames: nil}
}

func mustSync(t *testing.T, src, dst string, opt Options) Summary {
	t.Helper()
	sum, err := Sync(src, dst, opt)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	return sum
}

func TestNewFileCopiedUnchangedSkipped(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	writeFile(t, src, "new.txt", "hello", t0)
	writeFile(t, src, "sub/same.txt", "same", t0)
	writeFile(t, dst, "sub/same.txt", "same", t0)

	sum := mustSync(t, src, dst, testOptions())
	want := Summary{Copied: 1, Skipped: 1}
	if sum != want {
		t.Fatalf("summary = %+v, want %+v", sum, want)
	}
	if got := readFile(t, dst, "new.txt"); got != "hello" {
		t.Fatalf("new.txt = %q", got)
	}
	fi, err := os.Stat(filepath.Join(dst, "new.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !fi.ModTime().Equal(t0) {
		t.Fatalf("mtime = %v, want %v", fi.ModTime(), t0)
	}

	// A second run copies nothing.
	sum = mustSync(t, src, dst, testOptions())
	want = Summary{Skipped: 2}
	if sum != want {
		t.Fatalf("second summary = %+v, want %+v", sum, want)
	}
}

func TestChangedFilesCopied(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	writeFile(t, src, "size.txt", "longer content", t0)
	writeFile(t, dst, "size.txt", "short", t0)
	writeFile(t, src, "newer.txt", "AAAA", t1)
	writeFile(t, dst, "newer.txt", "BBBB", t0)
	// Same size, source older: not considered different.
	writeFile(t, src, "older.txt", "CCCC", t0)
	writeFile(t, dst, "older.txt", "DDDD", t1)

	sum := mustSync(t, src, dst, testOptions())
	want := Summary{Copied: 2, Skipped: 1}
	if sum != want {
		t.Fatalf("summary = %+v, want %+v", sum, want)
	}
	if got := readFile(t, dst, "size.txt"); got != "longer content" {
		t.Fatalf("size.txt = %q", got)
	}
	if got := readFile(t, dst, "newer.txt"); got != "AAAA" {
		t.Fatalf("newer.txt = %q", got)
	}
	if got := readFile(t, dst, "older.txt"); got != "DDDD" {
		t.Fatalf("older.txt = %q", got)
	}
}

func TestDryRunWritesNothing(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	writeFile(t, src, "a.txt", "a", t0)
	writeFile(t, src, "dir/b.txt", "b", t0)
	writeFile(t, src, "changed.txt", "new content", t1)
	writeFile(t, dst, "changed.txt", "old", t0)
	writeFile(t, dst, "extra.txt", "x", t0)
	writeFile(t, dst, "gone/c.txt", "c", t0)

	before := snapshot(t, dst)
	var out, errOut bytes.Buffer
	code := run([]string{"--dry-run", "--delete", src, dst}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, errOut.String())
	}
	after := snapshot(t, dst)
	if strings.Join(before, "\n") != strings.Join(after, "\n") {
		t.Fatalf("dry run modified dst:\nbefore %v\nafter  %v", before, after)
	}
	text := out.String()
	for _, w := range []string{"copy a.txt", "copy dir/b.txt", "copy changed.txt", "delete extra.txt", "delete gone/c.txt", "delete gone", "copied: 3, deleted: 3"} {
		if !strings.Contains(text, w) {
			t.Errorf("output missing %q:\n%s", w, text)
		}
	}

	// Dry run against a destination that does not exist must not create it.
	missing := filepath.Join(t.TempDir(), "nope")
	code = run([]string{"--dry-run", src, missing}, io.Discard, io.Discard)
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if _, err := os.Stat(missing); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("dry run created destination: %v", err)
	}
}

func TestDeleteRemovesExtra(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	writeFile(t, src, "keep.txt", "k", t0)
	writeFile(t, src, "sub/keep2.txt", "k2", t0)
	writeFile(t, dst, "keep.txt", "k", t0)
	writeFile(t, dst, "extra.txt", "x", t0)
	writeFile(t, dst, "sub/extra2.txt", "x", t0)
	writeFile(t, dst, "olddir/deep/file.txt", "x", t0)

	// Without --delete nothing is removed.
	sum := mustSync(t, src, dst, testOptions())
	if sum.Deleted != 0 || !exists(t, dst, "extra.txt") {
		t.Fatalf("removed without --delete: %+v", sum)
	}

	opt := testOptions()
	opt.Delete = true
	sum = mustSync(t, src, dst, opt)
	// extra.txt, sub/extra2.txt, olddir/deep/file.txt, olddir/deep, olddir
	if sum.Deleted != 5 || sum.Failed != 0 {
		t.Fatalf("summary = %+v", sum)
	}
	for _, p := range []string{"extra.txt", "sub/extra2.txt", "olddir"} {
		if exists(t, dst, p) {
			t.Errorf("%s still exists", p)
		}
	}
	for _, p := range []string{"keep.txt", "sub/keep2.txt"} {
		if !exists(t, dst, p) {
			t.Errorf("%s was removed", p)
		}
	}
	for _, p := range []string{"keep.txt", "sub/keep2.txt"} {
		if !exists(t, src, p) {
			t.Errorf("source %s was removed", p)
		}
	}
}

func listTemps(t *testing.T, root string) []string {
	t.Helper()
	var temps []string
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err == nil && strings.HasPrefix(d.Name(), tempPrefix) {
			temps = append(temps, p)
		}
		return nil
	})
	return temps
}

func TestCopyFailureLeavesNoTemp(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	writeFile(t, src, "a.txt", "new content that fails", t1)
	writeFile(t, src, "sub/b.txt", "brand new", t1)
	writeFile(t, dst, "a.txt", "old", t0)

	opt := testOptions()
	opt.copyFn = func(w io.Writer, r io.Reader) (int64, error) {
		n, _ := w.Write([]byte("partial"))
		return int64(n), errors.New("injected failure")
	}
	var errOut bytes.Buffer
	opt.Err = &errOut
	sum := mustSync(t, src, dst, opt)
	if sum.Failed != 2 || sum.Copied != 0 {
		t.Fatalf("summary = %+v", sum)
	}
	if temps := listTemps(t, dst); len(temps) != 0 {
		t.Fatalf("temp files left behind: %v", temps)
	}
	if got := readFile(t, dst, "a.txt"); got != "old" {
		t.Fatalf("a.txt = %q, want old content preserved", got)
	}
	if exists(t, dst, "sub/b.txt") {
		t.Fatal("partially copied sub/b.txt is visible")
	}
	if !strings.Contains(errOut.String(), "injected failure") {
		t.Fatalf("stderr = %q", errOut.String())
	}
}

func TestUnreadableSourceExitStatus(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("permission bits not enforced")
	}
	src, dst := t.TempDir(), t.TempDir()
	writeFile(t, src, "ok.txt", "ok", t0)
	writeFile(t, src, "locked.txt", "secret", t0)
	locked := filepath.Join(src, "locked.txt")
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(locked, 0o644) })

	var out bytes.Buffer
	code := run([]string{src, dst}, &out, io.Discard)
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(out.String(), "copied: 1, deleted: 0, skipped: 0, failed: 1") {
		t.Fatalf("output = %q", out.String())
	}
	if exists(t, dst, "locked.txt") {
		t.Fatal("locked.txt should not exist in dst")
	}
	if temps := listTemps(t, dst); len(temps) != 0 {
		t.Fatalf("temp files left behind: %v", temps)
	}
}

func TestCaseInsensitiveDeleteKeepsMatch(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	writeFile(t, src, "File.txt", "same", t0)
	writeFile(t, dst, "file.txt", "same", t0)

	opt := testOptions()
	opt.Delete = true
	opt.CaseInsensitive = true
	sum := mustSync(t, src, dst, opt)
	if sum.Deleted != 0 || sum.Failed != 0 {
		t.Fatalf("summary = %+v", sum)
	}
	if !exists(t, dst, "file.txt") {
		t.Fatal("file.txt removed despite case insensitive match")
	}
}

func TestSkipNames(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	writeFile(t, src, ".DS_Store", "junk", t0)
	writeFile(t, src, "real.txt", "r", t0)
	writeFile(t, dst, "Thumbs.db", "keep me", t0)

	opt := testOptions()
	opt.Delete = true
	opt.SkipNames = []string{".DS_Store", "Thumbs.db"}
	sum := mustSync(t, src, dst, opt)
	if sum.Copied != 1 || sum.Skipped != 1 || sum.Deleted != 0 {
		t.Fatalf("summary = %+v", sum)
	}
	if exists(t, dst, ".DS_Store") {
		t.Fatal(".DS_Store was copied")
	}
	if !exists(t, dst, "Thumbs.db") {
		t.Fatal("ignored Thumbs.db in dst was deleted")
	}
}

func TestOverlapRejected(t *testing.T) {
	src := t.TempDir()
	writeFile(t, src, "a.txt", "a", t0)
	if _, err := Sync(src, filepath.Join(src, "inner"), testOptions()); err == nil {
		t.Fatal("expected error for dst inside src")
	}
	if _, err := Sync(src, src, testOptions()); err == nil {
		t.Fatal("expected error for dst equal to src")
	}
	if exists(t, src, "inner") {
		t.Fatal("inner was created")
	}
}

func TestRunUsage(t *testing.T) {
	var errOut bytes.Buffer
	if code := run([]string{"onlyone"}, io.Discard, &errOut); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if code := run([]string{"--bogus", "a", "b"}, io.Discard, io.Discard); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
}

func TestRunFlagsAfterArgs(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	writeFile(t, src, "a.txt", "a", t0)
	writeFile(t, dst, "extra.txt", "x", t0)
	var out bytes.Buffer
	if code := run([]string{src, dst, "--delete"}, &out, io.Discard); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if exists(t, dst, "extra.txt") || !exists(t, dst, "a.txt") {
		t.Fatalf("unexpected dst state, output %q", out.String())
	}
}
