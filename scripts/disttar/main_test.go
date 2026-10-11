package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestArchiveDeterministic archives the same files written at different
// times with different modes twice: the bytes are the same, entries are
// in path order, and only executables keep the executable bit.
func TestArchiveDeterministic(t *testing.T) {
	mk := func(mtime time.Time) string {
		dir := filepath.Join(t.TempDir(), "xo-v0.0.0-test")
		os.MkdirAll(filepath.Join(dir, "bin"), 0o755)
		os.MkdirAll(filepath.Join(dir, "lib", "xo"), 0o700)
		os.WriteFile(filepath.Join(dir, "bin", "xo"), []byte("binary"), 0o755)
		os.WriteFile(filepath.Join(dir, "lib", "xo", "b.o"), []byte("bb"), 0o600)
		os.WriteFile(filepath.Join(dir, "lib", "xo", "a.o"), []byte("a"), 0o664)
		for _, p := range []string{"bin/xo", "lib/xo/a.o", "lib/xo/b.o", "lib/xo", "bin", "."} {
			os.Chtimes(filepath.Join(dir, p), mtime, mtime)
		}
		return dir
	}
	a, err := archive(mk(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)))
	if err != nil {
		t.Fatal(err)
	}
	b, err := archive(mk(time.Date(2027, 6, 7, 8, 9, 10, 0, time.UTC)))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a, b) {
		t.Fatal("two archives of the same files differ")
	}
	zr, err := gzip.NewReader(bytes.NewReader(a))
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(zr)
	var got []string
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, h.Name+" "+os.FileMode(h.Mode).String())
	}
	want := []string{
		"xo-v0.0.0-test/ -rwxr-xr-x",
		"xo-v0.0.0-test/bin/ -rwxr-xr-x",
		"xo-v0.0.0-test/bin/xo -rwxr-xr-x",
		"xo-v0.0.0-test/lib/ -rwxr-xr-x",
		"xo-v0.0.0-test/lib/xo/ -rwxr-xr-x",
		"xo-v0.0.0-test/lib/xo/a.o -rw-r--r--",
		"xo-v0.0.0-test/lib/xo/b.o -rw-r--r--",
	}
	if len(got) != len(want) {
		t.Fatalf("entries %q", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("entry %d: %q, want %q", i, got[i], want[i])
		}
	}
}
