package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestSourceHashStopsAtRuntime: decision 0616 section 3 (amended
// 2026-11-14). uuid's hash is its own file and std/internal.xo, and no
// module's hash reaches the runtime core or the prelude.
func TestSourceHashStopsAtRuntime(t *testing.T) {
	if err := os.Chdir("../.."); err != nil {
		t.Fatal(err)
	}
	_, files, err := sourceHash("uuid")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"std/internal.xo", "std/uuid.xo"}; !reflect.DeepEqual(files, want) {
		t.Fatalf("uuid hashes %v, want %v", files, want)
	}
	rt, err := runtimeFiles()
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range []string{"time", "json", "url", "http"} {
		_, files, err := sourceHash(m)
		if err != nil {
			t.Fatal(err)
		}
		own := false
		for _, f := range files {
			if rt[f] || strings.HasPrefix(filepath.Base(f), "sys_") {
				t.Errorf("%s hashes runtime file %s", m, f)
			}
			own = own || f == filepath.Join("std", m+".xo")
		}
		if !own {
			t.Errorf("%s does not hash std/%s.xo: %v", m, m, files)
		}
	}
	// url hashes its native half too; the modules it imports (netip since
	// the URL type, decision 0642) may change, so present, not exact
	_, files, _ = sourceHash("url")
	has := map[string]bool{}
	for _, f := range files {
		has[f] = true
		if rt[f] {
			t.Errorf("url hashes runtime file %s", f)
		}
	}
	if !has["std/native/url_native.xo"] || !has["std/url.xo"] {
		t.Errorf("url hashes %v, want std/url.xo and its std/native/url_native.xo", files)
	}
}

// TestPerfOwner: a bench/perf directory is the longest std module's its
// name starts with.
func TestPerfOwner(t *testing.T) {
	for dir, want := range map[string]string{"html": "html", "htmlparse": "htmlparse", "htmltok": "htmltok", "jsontoken": "json", "timeformat-cache": "time", "url": "url"} {
		if got := perfOwner(dir); got != want {
			t.Errorf("perfOwner(%q) = %q, want %q", dir, got, want)
		}
	}
}
